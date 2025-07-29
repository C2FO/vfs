// Package gcsevents provides an implementation of the vfsevents.Watcher interface using Pub/Sub events from a GCS bucket.
package gcsevents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"

	"github.com/c2fo/vfs/contrib/vfsevents"
)

const (
	EventObjectFinalize       = "OBJECT_FINALIZE"
	EventObjectMetadataUpdate = "OBJECT_METADATA_UPDATE"
	EventObjectDelete         = "OBJECT_DELETE"
	EventObjectArchive        = "OBJECT_ARCHIVE"
)

// GCSEvent represents the structure of a GCS event.
type GCSEvent struct {
	Size               uint64            `json:"size"`
	Generation         int64             `json:"generation"`
	TimeCreated        JSONTime          `json:"timeCreated"`
	TimeDeleted        JSONTime          `json:"timeDeleted,omitempty"`
	TimeFinalized      JSONTime          `json:"timeFinalized,omitempty"`
	Updated            JSONTime          `json:"updated,omitempty"`
	Name               string            `json:"name"`
	Bucket             string            `json:"bucket"`
	ContentType        string            `json:"contentType"`
	MD5Hash            string            `json:"md5Hash,omitempty"`
	ContentEncoding    string            `json:"contentEncoding,omitempty"`
	ContentDisposition string            `json:"contentDisposition,omitempty"`
	ContentLanguage    string            `json:"contentLanguage,omitempty"`
	Etag               string            `json:"etag,omitempty"`
	Owner              Owner             `json:"owner,omitempty"`
	Metadata           map[string]string `json:"metadata"`
	EventType          string            `json:"eventType"`
}

// Owner represents the owner of the GCS object.
type Owner struct {
	Entity   string `json:"entity"`
	EntityID string `json:"entityId,omitempty"`
}

type JSONTime time.Time

func (jt *JSONTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	if s == "" || s == "null" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	*jt = JSONTime(t)
	return nil
}

func (jt JSONTime) MarshalJSON() ([]byte, error) {
	t := time.Time(jt)
	return json.Marshal(t.Format(time.RFC3339))
}

// PubSubClient is an interface for the Pub/Sub client.
type PubSubClient interface {
	Receive(ctx context.Context, f func(context.Context, *pubsub.Message)) error
}

// GCSWatcher implements the vfsevents.Watcher interface using Pub/Sub events from a GCS bucket.
type GCSWatcher struct {
	pubsubClient PubSubClient
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
}

// Option is a functional option for configuring the GCSWatcher.
type Option func(*GCSWatcher)

// WithPubSubClient sets a custom Pub/Sub client for the GCSWatcher.
func WithPubSubClient(client PubSubClient) Option {
	return func(w *GCSWatcher) {
		w.pubsubClient = client
	}
}

// NewGCSWatcher initializes a GCSWatcher with the given Pub/Sub projectID and subscriptionID.
// It also accepts functional options to customize the watcher.
// Note: The projectID and subscriptionID will be used to construct the fully-qualified subscription name
// "projects/{project}/subscriptions/{subscription}".
func NewGCSWatcher(projectID, subscriptionID string, opts ...Option) (*GCSWatcher, error) {
	if projectID == "" {
		return nil, fmt.Errorf("projectID cannot be empty")
	}
	if subscriptionID == "" {
		return nil, fmt.Errorf("subscription cannot be empty")
	}

	w := &GCSWatcher{}

	for _, opt := range opts {
		opt(w)
	}

	if w.pubsubClient == nil {
		ctx := context.Background()
		client, err := pubsub.NewClient(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("error creating Pub/Sub client: %w", err)
		}

		subscriptionName := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionID)
		w.pubsubClient = client.Subscription(subscriptionName)
	}

	return w, nil
}

// Start begins watching for GCS events from the configured Pub/Sub subscription.
func (w *GCSWatcher) Start(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	errHandler vfsevents.ErrorHandlerFunc,
	opts ...vfsevents.StartOption) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return fmt.Errorf("GCS watcher is already running")
	}

	// Process start options
	config := &vfsevents.StartConfig{}
	for _, opt := range opts {
		opt(config)
	}

	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	// Create a wrapped handler that applies filtering if configured
	wrappedHandler := handler
	if config.EventFilter != nil {
		wrappedHandler = func(event vfsevents.Event) {
			if config.EventFilter(event) {
				handler(event)
			}
		}
	}

	// Status tracking
	var status vfsevents.WatcherStatus
	status.Running = true
	status.StartTime = time.Now()

	// Send initial status if callback is configured
	if config.StatusCallback != nil {
		config.StatusCallback(status)
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()

		if err := w.receiveWithRetry(ctx, wrappedHandler, errHandler, &status, config); err != nil {
			status.LastError = err
			if config.StatusCallback != nil {
				config.StatusCallback(status)
			}
			errHandler(err)
		}
	}()

	return nil
}

// receiveWithRetry wraps the Pub/Sub receive method with retry logic
func (w *GCSWatcher) receiveWithRetry(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	errHandler vfsevents.ErrorHandlerFunc,
	status *vfsevents.WatcherStatus,
	c *vfsevents.StartConfig) error {
	if !c.RetryConfig.Enabled {
		// No retry - use original receive method
		return w.receive(ctx, handler, errHandler, status, c)
	}

	for attempt := 0; attempt <= c.RetryConfig.MaxRetries; attempt++ {
		err := w.receive(ctx, handler, errHandler, status, c)

		if err == nil {
			// Success - reset consecutive error count
			status.ConsecutiveErrors = 0
			return nil
		}

		// Check if error is retryable
		if !vfsevents.IsRetryableError(err, c.RetryConfig) {
			return err // Non-retryable error
		}

		// Update status with retry information
		status.RetryAttempts++
		status.ConsecutiveErrors++
		status.LastError = err
		status.LastRetryTime = time.Now()

		// Last attempt - don't retry
		if attempt == c.RetryConfig.MaxRetries {
			return fmt.Errorf("max retries (%d) exceeded: %w", c.RetryConfig.MaxRetries, err)
		}

		// Calculate backoff delay
		backoff := vfsevents.CalculateBackoff(attempt, c.RetryConfig)

		// Send status update if callback is configured
		if c.StatusCallback != nil {
			c.StatusCallback(*status)
		}

		// Wait before retry (with context cancellation support)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			// Continue to next retry
		}
	}

	return nil // Should never reach here
}

// receive handles the actual Pub/Sub message receiving
func (w *GCSWatcher) receive(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	errHandler vfsevents.ErrorHandlerFunc,
	status *vfsevents.WatcherStatus,
	c *vfsevents.StartConfig) error {
	err := w.pubsubClient.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		defer msg.Ack()

		// Parse the GCS event from the Pub/Sub message
		var gcsEvent GCSEvent
		if err := json.Unmarshal(msg.Data, &gcsEvent); err != nil {
			errHandler(fmt.Errorf("failed to unmarshal GCS event: %w", err))
			return
		}

		// Extract eventType from message attributes (fallback to message data)
		eventType := gcsEvent.EventType
		if eventType == "" {
			if attributeEventType, ok := msg.Attributes["eventType"]; ok {
				eventType = attributeEventType
			}
		}

		// Map GCS event type to vfsevents.EventType
		mappedEventType := w.mapGCSEventType(eventType)
		if mappedEventType != vfsevents.EventUnknown {
			event := vfsevents.Event{
				URI:       fmt.Sprintf("gs://%s/%s", gcsEvent.Bucket, gcsEvent.Name),
				Type:      mappedEventType,
				Timestamp: time.Time(gcsEvent.TimeCreated).Unix(),
				Metadata: map[string]string{
					"bucketName": gcsEvent.Bucket,
					"object":     gcsEvent.Name,
					"eventType":  eventType,
					"generation": fmt.Sprintf("%d", gcsEvent.Generation),
				},
			}

			status.EventsProcessed++
			status.LastEventTime = time.Now()
			if c.StatusCallback != nil {
				c.StatusCallback(*status)
			}

			handler(event)
		}
	})

	return err
}

// mapGCSEventType maps a GCS event type to a vfsevents.EventType
func (w *GCSWatcher) mapGCSEventType(eventType string) vfsevents.EventType {
	switch eventType {
	case EventObjectFinalize:
		return vfsevents.EventCreated
	case EventObjectDelete:
		return vfsevents.EventDeleted
	default:
		return vfsevents.EventUnknown
	}
}

// Stop stops the watcher.
func (w *GCSWatcher) Stop(opts ...vfsevents.StopOption) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel == nil {
		return fmt.Errorf("GCS watcher is not running")
	}

	// Process stop options
	config := &vfsevents.StopConfig{
		Timeout: 30 * time.Second, // Default timeout
		Force:   false,
	}
	for _, opt := range opts {
		opt(config)
	}

	w.cancel()
	w.cancel = nil

	// Handle graceful shutdown with timeout
	if config.Force {
		// Force immediate shutdown - don't wait for goroutines
		return nil
	}

	// Wait for the polling goroutine to exit with timeout
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(config.Timeout):
		return fmt.Errorf("timeout waiting for GCS watcher to stop after %v", config.Timeout)
	}
}
