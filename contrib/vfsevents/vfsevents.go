// Package vfsevents provides a unified interface for watching filesystem events across different backends.
//
// The primary goal of this package is to offer a consistent API for monitoring file changes,
// regardless of the underlying storage system (local filesystem, S3, GCS, etc.).
//
// Key Features:
//   - Unified Watcher interface with functional options for advanced configuration
//   - Support for event filtering, status callbacks, and metrics collection
//   - Built-in retry logic for cloud watchers with intelligent error classification
//   - Graceful shutdown with configurable timeouts
//   - Production-ready error handling and resource management
//
// Basic Usage:
//
//	watcher, err := vfspoller.NewPoller(location)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	handler := func(event vfsevents.Event) {
//		log.Printf("Event: %s %s", event.Type, event.URI)
//	}
//
//	errHandler := func(err error) {
//		log.Printf("Error: %v", err)
//	}
//
//	// Start watching
//	ctx := context.Background()
//	if err := watcher.Start(ctx, handler, errHandler); err != nil {
//		log.Fatal(err)
//	}
//
// Advanced Usage with Options:
//
//	// Start with event filtering and retry logic
//	err = watcher.Start(ctx, handler, errHandler,
//		vfsevents.WithEventFilter(func(e Event) bool {
//			return e.Type == EventCreated
//		}),
//		vfsevents.WithRetryConfig(vfsevents.DefaultRetryConfig()),
//		vfsevents.WithStatusCallback(statusHandler),
//	)
//
// Available Watchers:
//   - VFSPoller: Polls any VFS location for changes (works with any backend)
//   - S3Events: Real-time S3 event monitoring via SQS with retry logic
//   - GCSEvents: Real-time GCS event monitoring via Pub/Sub with retry logic
package vfsevents

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"strings"
	"time"
)

// EventType represents the type of filesystem event.
type EventType int

const (
	// EventCreated represents a file creation event.
	EventCreated EventType = iota
	// EventDeleted represents a file deletion event.
	EventDeleted
	// EventModified represents a file modification event.
	EventModified
	// EventRenamed represents a file rename event.
	EventRenamed
	// EventUnknown represents an unknown event type.
	EventUnknown
)

// String returns the string representation of the EventType.
func (e EventType) String() string {
	switch e {
	case EventCreated:
		return "Created"
	case EventDeleted:
		return "Deleted"
	case EventModified:
		return "Modified"
	case EventRenamed:
		return "Renamed"
	case EventUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// Event represents a filesystem event.
type Event struct {
	// URI is the URI of the file that triggered the event.
	URI string
	// Type is the type of event (e.g., EventCreated, EventDeleted, etc.).
	Type EventType
	// Metadata is optional metadata associated with the event (e.g., object version, size, user identity, etc.).
	Metadata map[string]string
	// Timestamp is the Unix timestamp of the event.
	Timestamp int64
}

// HandlerFunc is the function signature for handling events.
type HandlerFunc func(event Event)

// ErrorHandlerFunc is the function signature for handling errors.
// Errors can occur for many reasons once `Start` begins listening for events.
// For instance, network issues, permission errors, or invalid configurations can trigger errors.
type ErrorHandlerFunc func(error)

// WatcherError represents different types of errors that can occur during watching.
type WatcherError struct {
	// Type is the category of watcher error (e.g., ErrorTypeConnection, ErrorTypeAuth, etc.).
	Type ErrorType
	// Message is a human-readable error message.
	Message string
	// Cause is the underlying cause of the error.
	Cause error
}

// ErrorType represents the category of watcher error.
type ErrorType int

const (
	// ErrorTypeConnection represents network or connection issues.
	ErrorTypeConnection ErrorType = iota
	// ErrorTypeAuth represents authentication or authorization issues.
	ErrorTypeAuth
	// ErrorTypeConfig represents configuration issues.
	ErrorTypeConfig
	// ErrorTypeProcessing represents event processing issues.
	ErrorTypeProcessing
	// ErrorTypeUnknown represents unknown or unclassified errors.
	ErrorTypeUnknown
)

// String returns the string representation of the ErrorType.
func (e ErrorType) String() string {
	switch e {
	case ErrorTypeConnection:
		return "Connection"
	case ErrorTypeAuth:
		return "Authentication"
	case ErrorTypeConfig:
		return "Configuration"
	case ErrorTypeProcessing:
		return "Processing"
	case ErrorTypeUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// Error implements the error interface.
func (e *WatcherError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s error: %s (caused by: %v)", e.Type.String(), e.Message, e.Cause)
	}
	return fmt.Sprintf("%s error: %s", e.Type.String(), e.Message)
}

// Unwrap returns the underlying cause of the error.
func (e *WatcherError) Unwrap() error {
	return e.Cause
}

// NewWatcherError creates a new WatcherError with the specified type and message.
func NewWatcherError(errorType ErrorType, message string, cause error) *WatcherError {
	return &WatcherError{
		Type:    errorType,
		Message: message,
		Cause:   cause,
	}
}

// StartOption configures the Start method behavior.
type StartOption func(*StartConfig)

// StopOption configures the Stop method behavior.
type StopOption func(*StopConfig)

// StartConfig holds configuration for the Start method.
type StartConfig struct {
	// EventFilter allows filtering events before they reach the handler
	EventFilter func(Event) bool

	// StatusCallback receives status updates during watching
	StatusCallback func(WatcherStatus)

	// MetricsCollector receives metrics about watcher performance
	MetricsCollector func(WatcherMetrics)

	// RetryConfig holds configuration for retry behavior
	RetryConfig RetryConfig
}

// StopConfig holds configuration for the Stop method.
type StopConfig struct {
	// Timeout specifies how long to wait for graceful shutdown
	Timeout time.Duration

	// Force indicates whether to force stop immediately
	Force bool
}

// RetryConfig holds configuration for retry behavior in cloud watchers.
//
// Retry logic is automatically applied to transient errors such as network timeouts,
// rate limiting, and temporary service unavailability. Permanent errors (e.g., authentication
// failures, file not found) are not retried.
//
// Example usage:
//
//	config := vfsevents.RetryConfig{
//		MaxRetries:      5,
//		InitialBackoff:  2 * time.Second,
//		MaxBackoff:      60 * time.Second,
//		BackoffFactor:   2.0,
//		RetryableErrors: []string{"custom error pattern"},
//		Enabled:         true,
//	}
//	watcher.Start(ctx, handler, errHandler, vfsevents.WithRetryConfig(config))
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (default: 3).
	MaxRetries int
	// InitialBackoff is the initial backoff delay (default: 1s).
	InitialBackoff time.Duration
	// MaxBackoff is the maximum backoff delay (default: 30s).
	MaxBackoff time.Duration
	// BackoffFactor is the backoff multiplier for exponential backoff (default: 2.0).
	BackoffFactor float64
	// RetryableErrors is a list of custom error patterns that should trigger retries.
	RetryableErrors []string
	// Enabled indicates whether retry logic is enabled (default: false).
	Enabled bool
}

// DefaultRetryConfig returns a sensible default retry configuration.
//
// The default configuration provides:
//   - 3 maximum retry attempts
//   - 1 second initial backoff
//   - 30 second maximum backoff
//   - 2.0 exponential backoff factor
//   - No custom retryable error patterns
//   - Retry enabled
//
// This configuration is suitable for most production use cases with cloud watchers.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:      3,
		InitialBackoff:  1 * time.Second,
		MaxBackoff:      30 * time.Second,
		BackoffFactor:   2.0,
		RetryableErrors: []string{},
		Enabled:         true,
	}
}

// WithRetryConfig sets the retry configuration
//
// This option allows you to customize the retry behavior of the watcher.
// You can specify the maximum number of retries, initial backoff, maximum backoff,
// backoff factor, and custom retryable error patterns.
func WithRetryConfig(config RetryConfig) StartOption {
	return func(c *StartConfig) {
		c.RetryConfig = config
	}
}

// WithMaxRetries sets the maximum number of retry attempts
//
// This option allows you to specify the maximum number of times the watcher
// should retry a failed operation. If not specified, the default value is 3.
func WithMaxRetries(maxRetries int) StartOption {
	return func(c *StartConfig) {
		c.RetryConfig.MaxRetries = maxRetries
		c.RetryConfig.Enabled = true
	}
}

// WithRetryBackoff sets the initial and maximum backoff durations
//
// This option allows you to specify the initial and maximum backoff durations
// for retrying failed operations. If not specified, the default values are 1s
// and 30s respectively.
func WithRetryBackoff(initial, maxBackoff time.Duration) StartOption {
	return func(c *StartConfig) {
		c.RetryConfig.InitialBackoff = initial
		c.RetryConfig.MaxBackoff = maxBackoff
		c.RetryConfig.Enabled = true
	}
}

// WithRetryableErrors sets custom error patterns that should trigger retries
//
// This option allows you to specify custom error patterns that should trigger
// retries. If not specified, only network-related and temporary cloud service
// errors are retried.
func WithRetryableErrors(patterns []string) StartOption {
	return func(c *StartConfig) {
		c.RetryConfig.RetryableErrors = patterns
		c.RetryConfig.Enabled = true
	}
}

// IsRetryableError determines if an error is worth retrying
//
// This function checks if an error is retryable based on the retry configuration.
// It checks for network-related errors, temporary cloud service errors, and
// custom retryable error patterns.
func IsRetryableError(err error, config RetryConfig) bool {
	if !config.Enabled {
		return false
	}

	// Network-related errors
	if isNetworkError(err) {
		return true
	}

	// AWS/GCP temporary errors
	if isTemporaryCloudError(err) {
		return true
	}

	// Custom retryable error patterns
	errStr := err.Error()
	for _, pattern := range config.RetryableErrors {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// CalculateBackoff calculates exponential backoff with optional jitter
//
// This function calculates the backoff duration based on the retry configuration.
// It uses an exponential backoff strategy with optional jitter to prevent
// the "thundering herd" problem.
func CalculateBackoff(attempt int, config RetryConfig) time.Duration {
	backoff := config.InitialBackoff * time.Duration(math.Pow(config.BackoffFactor, float64(attempt)))

	if backoff > config.MaxBackoff {
		backoff = config.MaxBackoff
	}

	// Add up to 25% jitter to prevent thundering herd
	jitter := time.Duration(rand.Float64() * 0.25 * float64(backoff)) //nolint:gosec // G404: Use of weak random number generator
	return backoff + jitter
}

// isNetworkError checks for network-related errors
func isNetworkError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	errStr := strings.ToLower(err.Error())
	networkPatterns := []string{
		"connection refused", "timeout", "network", "dns",
		"connection reset", "broken pipe", "no route to host",
	}

	for _, pattern := range networkPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isTemporaryCloudError checks for temporary cloud service errors
func isTemporaryCloudError(err error) bool {
	errStr := strings.ToLower(err.Error())

	// AWS temporary errors
	temporaryAWSErrors := []string{
		"throttling", "rate exceeded", "service unavailable",
		"internal error", "temporary failure", "503", "502", "500",
		"slowdown", "request timeout", "provisioned throughput exceeded",
	}

	// GCP temporary errors
	temporaryGCPErrors := []string{
		"unavailable", "deadline exceeded", "resource exhausted",
		"internal", "temporary failure", "quota exceeded",
		"rate limited", "service temporarily unavailable",
	}

	//nolint:gocritic // appendAssign: append result not assigned to the same slice
	allTemporaryErrors := append(temporaryAWSErrors, temporaryGCPErrors...)

	for _, pattern := range allTemporaryErrors {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// Watcher is an interface for watching filesystem events.
type Watcher interface {
	// Start begins watching the specified path and triggers handler on events.
	// It also takes an error handler to handle any errors that occur during polling.
	// Optional StartOption parameters can be used to configure advanced behavior.
	Start(ctx context.Context, handler HandlerFunc, errHandler ErrorHandlerFunc, opts ...StartOption) error

	// Stop stops the watcher.
	// Optional StopOption parameters can be used to configure shutdown behavior.
	Stop(opts ...StopOption) error
}

// WatcherStatus represents the current status of a watcher.
type WatcherStatus struct {
	// Running indicates whether the watcher is currently running.
	Running bool
	// StartTime is the time when the watcher started.
	StartTime time.Time
	// EventsProcessed is the number of events processed by the watcher.
	EventsProcessed int64
	// LastEventTime is the time of the last event processed by the watcher.
	LastEventTime time.Time
	// LastError is the last error encountered by the watcher.
	LastError error
	// RetryAttempts is the total number of retry attempts made by the watcher.
	RetryAttempts int64
	// LastRetryTime is the time of the last retry attempt made by the watcher.
	LastRetryTime time.Time
	// ConsecutiveErrors is the number of consecutive errors encountered by the watcher.
	ConsecutiveErrors int64
}

// WatcherMetrics contains performance metrics for a watcher.
type WatcherMetrics struct {
	// EventsPerSecond is the average number of events processed per second.
	EventsPerSecond float64
	// AverageLatency is the average latency of event processing.
	AverageLatency time.Duration
	// ErrorRate is the rate of errors encountered by the watcher.
	ErrorRate float64
	// MemoryUsage is the memory usage of the watcher.
	MemoryUsage uint64
	// LastUpdated is the time when the metrics were last updated.
	LastUpdated time.Time
}

// Start option constructors

// WithEventFilter sets an event filter that determines which events are passed to the handler.
func WithEventFilter(filter func(Event) bool) StartOption {
	return func(config *StartConfig) {
		config.EventFilter = filter
	}
}

// WithStatusCallback sets a callback that receives status updates during watching.
func WithStatusCallback(callback func(WatcherStatus)) StartOption {
	return func(config *StartConfig) {
		config.StatusCallback = callback
	}
}

// WithMetricsCollector sets a callback that receives performance metrics.
func WithMetricsCollector(collector func(WatcherMetrics)) StartOption {
	return func(config *StartConfig) {
		config.MetricsCollector = collector
	}
}

// Stop option constructors

// WithTimeout sets a timeout for graceful shutdown.
func WithTimeout(timeout time.Duration) StopOption {
	return func(config *StopConfig) {
		config.Timeout = timeout
	}
}

// WithForceStop forces immediate shutdown without waiting for graceful cleanup.
func WithForceStop() StopOption {
	return func(config *StopConfig) {
		config.Force = true
	}
}
