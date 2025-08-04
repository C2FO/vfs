// Package s3events provides an implementation of the vfsevents.Watcher interface using SQS events from an S3 bucket.
package s3events

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/c2fo/vfs/contrib/vfsevents"
)

// S3Event represents the structure of an S3 event.
type S3Event struct {
	Records []S3Record `json:"Records"`
}

// S3Record represents a single record in an S3 event.
type S3Record struct {
	EventTime         string          `json:"eventTime"`
	EventName         string          `json:"eventName"`
	UserIdentity      S3UserIdentity  `json:"userIdentity"`
	RequestParameters S3RequestParams `json:"requestParameters"`
	S3                S3Entity        `json:"s3"`
	AwsRegion         string          `json:"awsRegion"`
}

// S3UserIdentity represents the user identity information in an S3 event.
type S3UserIdentity struct {
	PrincipalID string `json:"principalId"`
}

// S3RequestParams represents the request parameters in an S3 event.
type S3RequestParams struct {
	SourceIPAddress string `json:"sourceIPAddress"`
}

// S3Entity represents the S3 entity in an S3 event.
type S3Entity struct {
	Bucket S3Bucket `json:"bucket"`
	Object S3Object `json:"object"`
}

// S3Bucket represents the bucket information in an S3 event.
type S3Bucket struct {
	Name          string         `json:"name"`
	OwnerIdentity S3UserIdentity `json:"ownerIdentity"`
}

// S3Object represents the object information in an S3 event.
type S3Object struct {
	Key       string `json:"key"`
	Size      int64  `json:"size"`
	ETag      string `json:"eTag"`
	VersionID string `json:"versionId"`
	Sequencer string `json:"sequencer"`
}

// SqsClient is an interface for the SQS client.
type SqsClient interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

// S3Watcher implements the vfsevents.Watcher interface using SQS events from an S3 bucket.
type S3Watcher struct {
	sqsClient     SqsClient
	queueURL      string
	cancel        context.CancelFunc
	mu            sync.Mutex
	receivedCount int
	wg            sync.WaitGroup
}

type Option func(*S3Watcher)

// WithSqsClient sets a custom SQS client for the S3Watcher.
func WithSqsClient(client SqsClient) Option {
	return func(w *S3Watcher) {
		w.sqsClient = client
	}
}

// WithReceivedCount sets the number of times a message should be received before it is deleted.
// This is used as a protective measure to prevent infinite retry loops for unprocessable messages.
func WithReceivedCount(count uint) Option {
	return func(w *S3Watcher) {
		w.receivedCount = int(count)
	}
}

// NewS3Watcher initializes an S3Watcher with the given SQS queue URL.
func NewS3Watcher(queueURL string, opts ...Option) (*S3Watcher, error) {
	// validate queueURL
	if queueURL == "" {
		return nil, fmt.Errorf("queueURL cannot be empty")
	}

	w := &S3Watcher{
		queueURL:      queueURL,
		receivedCount: 5,
	}

	// apply options
	for _, opt := range opts {
		opt(w)
	}

	// if no client is provided, create a new one
	if w.sqsClient == nil {
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("error loading AWS config: %w", err)
		}

		w.sqsClient = sqs.NewFromConfig(cfg)
	}

	return w, nil
}

// Start begins watching for S3 events from the configured SQS queue.
func (w *S3Watcher) Start(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	errHandler vfsevents.ErrorHandlerFunc,
	opts ...vfsevents.StartOption) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return fmt.Errorf("S3 watcher is already running")
	}

	// Process start options
	c := &vfsevents.StartConfig{}
	for _, opt := range opts {
		opt(c)
	}

	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	// Create a wrapped handler that applies filtering if configured
	wrappedHandler := handler
	if c.EventFilter != nil {
		wrappedHandler = func(event vfsevents.Event) {
			if c.EventFilter(event) {
				handler(event)
			}
		}
	}

	// Status tracking
	var status vfsevents.WatcherStatus
	status.Running = true
	status.StartTime = time.Now()

	// Send initial status if callback is configured
	if c.StatusCallback != nil {
		c.StatusCallback(status)
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.poll(ctx, wrappedHandler, errHandler, &status, c)
	}()

	return nil
}

// poll retrieves messages from the SQS queue and processes them.
func (w *S3Watcher) poll(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	errHandler vfsevents.ErrorHandlerFunc,
	status *vfsevents.WatcherStatus,
	c *vfsevents.StartConfig) {
	for {
		select {
		case <-ctx.Done():
			status.Running = false
			if c.StatusCallback != nil {
				c.StatusCallback(*status)
			}
			return
		default:
			if err := w.pollOnce(ctx, handler, status, c); err != nil {
				status.LastError = err
				if c.StatusCallback != nil {
					c.StatusCallback(*status)
				}
				errHandler(fmt.Errorf("error polling: %w", err))
			}
		}
	}
}

// pollOnce performs a single poll operation
func (w *S3Watcher) pollOnce(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	status *vfsevents.WatcherStatus,
	c *vfsevents.StartConfig) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	result, err := w.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:                    aws.String(w.queueURL),
		MaxNumberOfMessages:         10,
		WaitTimeSeconds:             20, // Long polling
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
	})
	if err != nil {
		return fmt.Errorf("failed to receive messages: %w", err)
	}

	for _, message := range result.Messages {
		// Check if message has been received too many times
		if message.Attributes != nil {
			if receiveCountStr, exists := message.Attributes["ApproximateReceiveCount"]; exists {
				approximateReceiveCount, err := strconv.Atoi(receiveCountStr)
				if err != nil {
					return fmt.Errorf("failed to parse ApproximateReceiveCount: %w", err)
				}

				if approximateReceiveCount > w.receivedCount {
					// Delete the message from the queue without processing
					_, err := w.sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
						QueueUrl:      aws.String(w.queueURL),
						ReceiptHandle: message.ReceiptHandle,
					})
					if err != nil {
						return fmt.Errorf("failed to delete message: %w", err)
					}
					continue // Skip processing this message
				}
			}
		}

		if err := w.processMessage(message, handler, status, c); err != nil {
			return fmt.Errorf("failed to process message: %w", err)
		}

		// Delete the message from the queue
		_, err = w.sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(w.queueURL),
			ReceiptHandle: message.ReceiptHandle,
		})
		if err != nil {
			return fmt.Errorf("failed to delete message: %w", err)
		}
	}

	return nil
}

// processMessage processes a single SQS message
func (w *S3Watcher) processMessage(
	message types.Message,
	handler vfsevents.HandlerFunc,
	status *vfsevents.WatcherStatus,
	c *vfsevents.StartConfig) error {
	// Parse the S3 event from the SQS message
	var s3Event S3Event
	if err := json.Unmarshal([]byte(*message.Body), &s3Event); err != nil {
		return fmt.Errorf("failed to unmarshal S3 event: %w", err)
	}

	// Process each record in the S3 event
	for i := range s3Event.Records {
		eventType := w.mapS3EventType(s3Event.Records[i].EventName)
		if eventType != vfsevents.EventUnknown {
			// Create enhanced metadata with S3-specific attributes
			metadata := map[string]string{
				"bucketName": s3Event.Records[i].S3.Bucket.Name,
				"key":        s3Event.Records[i].S3.Object.Key,
				"eventName":  s3Event.Records[i].EventName,
				"region":     s3Event.Records[i].AwsRegion,
				"eventTime":  s3Event.Records[i].EventTime,
				"operation":  w.getOperationType(s3Event.Records[i].EventName),
			}

			// Add version information if available
			if s3Event.Records[i].S3.Object.VersionID != "" {
				metadata["versionId"] = s3Event.Records[i].S3.Object.VersionID
				metadata["isVersioned"] = "true"
			} else {
				metadata["isVersioned"] = "false"
			}

			// Add additional S3-specific attributes
			if s3Event.Records[i].S3.Object.ETag != "" {
				metadata["eTag"] = s3Event.Records[i].S3.Object.ETag
			}
			if s3Event.Records[i].S3.Object.Sequencer != "" {
				metadata["sequencer"] = s3Event.Records[i].S3.Object.Sequencer
			}
			if s3Event.Records[i].S3.Object.Size > 0 {
				metadata["size"] = fmt.Sprintf("%d", s3Event.Records[i].S3.Object.Size)
			}

			event := vfsevents.Event{
				URI:       fmt.Sprintf("s3://%s/%s", s3Event.Records[i].S3.Bucket.Name, s3Event.Records[i].S3.Object.Key),
				Type:      eventType,
				Timestamp: time.Now().Unix(),
				Metadata:  metadata,
			}

			status.EventsProcessed++
			status.LastEventTime = time.Now()
			if c.StatusCallback != nil {
				c.StatusCallback(*status)
			}

			handler(event)
		}
	}

	return nil
}

// mapS3EventType maps an S3 event name to a vfsevents.EventType with semantic accuracy
func (w *S3Watcher) mapS3EventType(eventName string) vfsevents.EventType {
	switch eventName {
	case "s3:ObjectCreated:Put", "s3:ObjectCreated:Post":
		// Direct uploads are typically new file creations
		return vfsevents.EventCreated
	case "s3:ObjectCreated:Copy":
		// Copy operations are more likely to be overwrites or modifications
		return vfsevents.EventModified
	case "s3:ObjectCreated:CompleteMultipartUpload":
		// Large uploads could be either, but often represent significant changes
		return vfsevents.EventModified
	case "s3:ObjectCreated:*":
		// Wildcard - default to created for broad compatibility
		return vfsevents.EventCreated
	case "s3:ObjectRestore:Post":
		// Restore initiation - modification-like operation
		return vfsevents.EventModified
	case "s3:ObjectRestore:Completed":
		// Restore completion - object is now available (modification)
		return vfsevents.EventModified
	case "s3:ObjectRestore:Delete":
		// Temporary restored copy expires - deletion
		return vfsevents.EventDeleted
	case "s3:ObjectRemoved:*", "s3:ObjectRemoved:Delete", "s3:ObjectRemoved:DeleteMarkerCreated":
		return vfsevents.EventDeleted
	default:
		return vfsevents.EventUnknown
	}
}

// getOperationType extracts the operation type from an S3 event name
func (w *S3Watcher) getOperationType(eventName string) string {
	switch {
	case strings.Contains(eventName, "ObjectCreated:Put"):
		return "put"
	case strings.Contains(eventName, "ObjectCreated:Post"):
		return "post"
	case strings.Contains(eventName, "ObjectCreated:Copy"):
		return "copy"
	case strings.Contains(eventName, "CompleteMultipartUpload"):
		return "multipart"
	case strings.Contains(eventName, "ObjectRestore"):
		return "restore"
	case strings.Contains(eventName, "ObjectRemoved") || strings.Contains(eventName, "ObjectCreated:Delete"):
		return "delete"
	default:
		return "unknown"
	}
}

// Stop stops the watcher.
func (w *S3Watcher) Stop(opts ...vfsevents.StopOption) error {
	// Process stop options
	c := &vfsevents.StopConfig{
		Timeout: 30 * time.Second, // Default timeout
		Force:   false,
	}
	for _, opt := range opts {
		opt(c)
	}

	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}

	// Handle graceful shutdown with timeout
	if c.Force {
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
	case <-time.After(c.Timeout):
		return fmt.Errorf("timeout waiting for S3 watcher to stop after %v", c.Timeout)
	}
}
