package vfsevents

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// VfseventsTestSuite is the test suite for the vfsevents package
type VfseventsTestSuite struct {
	suite.Suite
}

// TestVfseventsTestSuite runs the test suite
func TestVfseventsTestSuite(t *testing.T) {
	suite.Run(t, new(VfseventsTestSuite))
}

func (s *VfseventsTestSuite) TestEventType_String() {
	tests := []struct {
		name      string
		eventType EventType
		expected  string
	}{
		{
			name:      "EventCreated",
			eventType: EventCreated,
			expected:  "Created",
		},
		{
			name:      "EventDeleted",
			eventType: EventDeleted,
			expected:  "Deleted",
		},
		{
			name:      "EventModified",
			eventType: EventModified,
			expected:  "Modified",
		},
		{
			name:      "Unknown event type",
			eventType: EventType(999),
			expected:  "Unknown",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expected, tt.eventType.String())
		})
	}
}

func (s *VfseventsTestSuite) TestErrorType_String() {
	tests := []struct {
		name     string
		errType  ErrorType
		expected string
	}{
		{
			name:     "ErrorTypeConnection",
			errType:  ErrorTypeConnection,
			expected: "Connection",
		},
		{
			name:     "ErrorTypeAuth",
			errType:  ErrorTypeAuth,
			expected: "Authentication",
		},
		{
			name:     "ErrorTypeConfig",
			errType:  ErrorTypeConfig,
			expected: "Configuration",
		},
		{
			name:     "ErrorTypeProcessing",
			errType:  ErrorTypeProcessing,
			expected: "Processing",
		},
		{
			name:     "ErrorTypeUnknown",
			errType:  ErrorTypeUnknown,
			expected: "Unknown",
		},
		{
			name:     "Invalid error type",
			errType:  ErrorType(999),
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expected, tt.errType.String())
		})
	}
}

func (s *VfseventsTestSuite) TestWatcherError() {
	s.Run("Error method", func() {
		tests := []struct {
			name     string
			err      *WatcherError
			expected string
		}{
			{
				name: "Error with cause",
				err: &WatcherError{
					Type:    ErrorTypeAuth,
					Message: "authentication failed",
					Cause:   errors.New("invalid token"),
				},
				expected: "Authentication error: authentication failed (caused by: invalid token)",
			},
			{
				name: "Error without cause",
				err: &WatcherError{
					Type:    ErrorTypeAuth,
					Message: "authentication failed",
					Cause:   nil,
				},
				expected: "Authentication error: authentication failed",
			},
		}

		for _, tt := range tests {
			s.Run(tt.name, func() {
				s.EqualError(tt.err, tt.expected)
			})
		}
	})

	s.Run("Unwrap method", func() {
		cause := errors.New("underlying error")
		err := &WatcherError{
			Type:    ErrorTypeConnection,
			Message: "connection failed",
			Cause:   cause,
		}

		s.Equal(cause, err.Unwrap())

		errNoCause := &WatcherError{
			Type:    ErrorTypeConnection,
			Message: "connection failed",
			Cause:   nil,
		}

		s.Nil(errNoCause.Unwrap())
	})
}

func (s *VfseventsTestSuite) TestNewWatcherError() {
	s.Run("With cause", func() {
		cause := errors.New("underlying error")
		err := NewWatcherError(ErrorTypeConnection, "connection failed", cause)

		s.Equal(ErrorTypeConnection, err.Type)
		s.Equal("connection failed", err.Message)
		s.Equal(cause, err.Cause)
	})

	s.Run("Without cause", func() {
		err := NewWatcherError(ErrorTypeAuth, "authentication failed", nil)

		s.Equal(ErrorTypeAuth, err.Type)
		s.Equal("authentication failed", err.Message)
		s.Nil(err.Cause)
	})
}

func (s *VfseventsTestSuite) TestDefaultRetryConfig() {
	config := DefaultRetryConfig()

	s.True(config.Enabled)
	s.Equal(3, config.MaxRetries)
	s.Equal(time.Second, config.InitialBackoff)
	s.Equal(30*time.Second, config.MaxBackoff)
	s.Equal(2.0, config.BackoffFactor)
	s.Empty(config.RetryableErrors)
}

func (s *VfseventsTestSuite) TestWithRetryConfig() {
	config := &StartConfig{}
	retryConfig := RetryConfig{
		Enabled:        true,
		MaxRetries:     5,
		InitialBackoff: 2 * time.Second,
	}

	WithRetryConfig(retryConfig)(config)

	s.Equal(retryConfig, config.RetryConfig)
}

func (s *VfseventsTestSuite) TestWithMaxRetries() {
	config := &StartConfig{}

	WithMaxRetries(10)(config)

	s.Equal(10, config.RetryConfig.MaxRetries)
}

func (s *VfseventsTestSuite) TestWithRetryBackoff() {
	config := &StartConfig{}
	initial := 5 * time.Second
	maxBackoff := 60 * time.Second

	WithRetryBackoff(initial, maxBackoff)(config)

	s.Equal(initial, config.RetryConfig.InitialBackoff)
	s.Equal(maxBackoff, config.RetryConfig.MaxBackoff)
}

func (s *VfseventsTestSuite) TestWithRetryableErrors() {
	config := &StartConfig{}
	patterns := []string{"timeout", "connection refused"}

	WithRetryableErrors(patterns)(config)

	s.Equal(patterns, config.RetryConfig.RetryableErrors)
}

func (s *VfseventsTestSuite) TestWithEventFilter() {
	config := &StartConfig{}
	filter := func(event Event) bool {
		return event.Type == EventCreated
	}

	WithEventFilter(filter)(config)

	s.NotNil(config.EventFilter)

	// Test the filter function
	createEvent := Event{Type: EventCreated}
	deleteEvent := Event{Type: EventDeleted}

	s.True(config.EventFilter(createEvent))
	s.False(config.EventFilter(deleteEvent))
}

func (s *VfseventsTestSuite) TestWithStatusCallback() {
	config := &StartConfig{}
	called := false
	callback := func(status WatcherStatus) {
		called = true
	}

	WithStatusCallback(callback)(config)

	s.NotNil(config.StatusCallback)

	// Test the callback function
	config.StatusCallback(WatcherStatus{})
	s.True(called)
}

func (s *VfseventsTestSuite) TestWithMetricsCollector() {
	config := &StartConfig{}
	called := false
	collector := func(metrics WatcherMetrics) {
		called = true
	}

	WithMetricsCollector(collector)(config)

	s.NotNil(config.MetricsCollector)

	// Test the collector function
	config.MetricsCollector(WatcherMetrics{})
	s.True(called)
}

func (s *VfseventsTestSuite) TestWithTimeout() {
	config := &StopConfig{}
	timeout := 30 * time.Second

	WithTimeout(timeout)(config)

	s.Equal(timeout, config.Timeout)
}

func (s *VfseventsTestSuite) TestWithForceStop() {
	config := &StopConfig{}

	WithForceStop()(config)

	s.True(config.Force)
}

func (s *VfseventsTestSuite) TestIsRetryableError() {
	config := DefaultRetryConfig()

	tests := []struct {
		name      string
		err       error
		config    RetryConfig
		retryable bool
	}{
		{
			name:      "Network timeout error",
			err:       errors.New("timeout"),
			config:    config,
			retryable: true,
		},
		{
			name:      "DNS error",
			err:       errors.New("dns lookup failed"),
			config:    config,
			retryable: true,
		},
		{
			name:      "Connection refused",
			err:       errors.New("connection refused"),
			config:    config,
			retryable: true,
		},
		{
			name:      "Connection reset",
			err:       errors.New("connection reset by peer"),
			config:    config,
			retryable: true,
		},
		{
			name:      "Timeout error",
			err:       errors.New("timeout"),
			config:    config,
			retryable: true,
		},
		{
			name:      "Context deadline exceeded",
			err:       context.DeadlineExceeded,
			config:    config,
			retryable: true,
		},
		{
			name:      "AWS throttling error",
			err:       errors.New("Throttling: Rate exceeded"),
			config:    config,
			retryable: true,
		},
		{
			name:      "GCS unavailable error",
			err:       errors.New("service unavailable"),
			config:    config,
			retryable: true,
		},
		{
			name:      "Custom retryable error",
			err:       errors.New("custom timeout occurred"),
			config:    config,
			retryable: true,
		},
		{
			name:      "Non-retryable error",
			err:       errors.New("permission denied"),
			config:    config,
			retryable: false,
		},
		{
			name: "Retry disabled",
			err:  errors.New("timeout"),
			config: RetryConfig{
				Enabled: false,
			},
			retryable: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := IsRetryableError(tt.err, tt.config)
			s.Equal(tt.retryable, result)
		})
	}
}

func (s *VfseventsTestSuite) TestCalculateBackoff() {
	config := DefaultRetryConfig()

	tests := []struct {
		name    string
		attempt int
		config  RetryConfig
		minTime time.Duration
		maxTime time.Duration
	}{
		{
			name:    "First attempt",
			attempt: 0,
			config:  config,
			minTime: config.InitialBackoff,                           // 1s base
			maxTime: config.InitialBackoff + config.InitialBackoff/4, // 1s + 25% jitter = 1.25s
		},
		{
			name:    "Second attempt",
			attempt: 1,
			config:  config,
			minTime: config.InitialBackoff * 2,                           // 2s base
			maxTime: config.InitialBackoff*2 + config.InitialBackoff*2/4, // 2s + 25% jitter = 2.5s
		},
		{
			name:    "Third attempt",
			attempt: 2,
			config:  config,
			minTime: config.InitialBackoff * 4,                           // 4s base
			maxTime: config.InitialBackoff*4 + config.InitialBackoff*4/4, // 4s + 25% jitter = 5s
		},
		{
			name:    "Max backoff reached",
			attempt: 10,
			config:  config,
			minTime: config.MaxBackoff,                       // 30s base
			maxTime: config.MaxBackoff + config.MaxBackoff/4, // 30s + 25% jitter = 37.5s
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			backoff := CalculateBackoff(tt.attempt, tt.config)
			s.True(backoff >= tt.minTime && backoff <= tt.maxTime,
				"Expected backoff between %v and %v, got %v", tt.minTime, tt.maxTime, backoff)
		})
	}
}

func (s *VfseventsTestSuite) TestIsNetworkError() {
	tests := []struct {
		name      string
		err       error
		isNetwork bool
	}{
		{
			name:      "Net.OpError",
			err:       &net.OpError{Op: "dial", Err: errors.New("timeout")},
			isNetwork: true,
		},
		{
			name:      "DNS error",
			err:       errors.New("dns lookup failed"),
			isNetwork: true,
		},
		{
			name:      "Connection refused",
			err:       errors.New("connection refused"),
			isNetwork: true,
		},
		{
			name:      "Connection reset",
			err:       errors.New("connection reset by peer"),
			isNetwork: true,
		},
		{
			name:      "Timeout error",
			err:       errors.New("timeout"),
			isNetwork: true,
		},
		{
			name:      "Context deadline exceeded",
			err:       context.DeadlineExceeded,
			isNetwork: true,
		},
		{
			name:      "Non-network error",
			err:       errors.New("permission denied"),
			isNetwork: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := isNetworkError(tt.err)
			s.Equal(tt.isNetwork, result)
		})
	}
}

func (s *VfseventsTestSuite) TestIsTemporaryCloudError() {
	tests := []struct {
		name        string
		err         error
		isTemporary bool
	}{
		{
			name:        "AWS Throttling",
			err:         errors.New("Throttling: Rate exceeded"),
			isTemporary: true,
		},
		{
			name:        "AWS SlowDown",
			err:         errors.New("SlowDown"),
			isTemporary: true,
		},
		{
			name:        "AWS ServiceUnavailable",
			err:         errors.New("Service Unavailable"),
			isTemporary: true,
		},
		{
			name:        "AWS InternalError",
			err:         errors.New("Internal Error"),
			isTemporary: true,
		},
		{
			name:        "GCS Unavailable",
			err:         errors.New("Unavailable"),
			isTemporary: true,
		},
		{
			name:        "GCS DeadlineExceeded",
			err:         errors.New("Deadline Exceeded"),
			isTemporary: true,
		},
		{
			name:        "GCS ResourceExhausted",
			err:         errors.New("Resource Exhausted"),
			isTemporary: true,
		},
		{
			name:        "HTTP 500 error",
			err:         errors.New("500 Internal Server Error"),
			isTemporary: true,
		},
		{
			name:        "HTTP 502 error",
			err:         errors.New("502 Bad Gateway"),
			isTemporary: true,
		},
		{
			name:        "HTTP 503 error",
			err:         errors.New("503 Service Unavailable"),
			isTemporary: true,
		},
		{
			name:        "Non-temporary error",
			err:         errors.New("permission denied"),
			isTemporary: false,
		},
		{
			name:        "HTTP 404 error",
			err:         errors.New("404 Not Found"),
			isTemporary: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := isTemporaryCloudError(tt.err)
			s.Equal(tt.isTemporary, result)
		})
	}
}

func (s *VfseventsTestSuite) TestEvent() {
	s.Run("Event struct creation", func() {
		event := Event{
			URI:       "s3://bucket/file.txt",
			Type:      EventCreated,
			Metadata:  map[string]string{"size": "1024"},
			Timestamp: 1234567890,
		}

		s.Equal("s3://bucket/file.txt", event.URI)
		s.Equal(EventCreated, event.Type)
		s.Equal("1024", event.Metadata["size"])
		s.Equal(int64(1234567890), event.Timestamp)
	})
}

func (s *VfseventsTestSuite) TestWatcherStatus() {
	s.Run("WatcherStatus struct creation", func() {
		status := WatcherStatus{
			Running:           true,
			StartTime:         time.Now(),
			EventsProcessed:   100,
			LastError:         errors.New("test error"),
			RetryAttempts:     5,
			ConsecutiveErrors: 2,
			LastRetryTime:     time.Now(),
		}

		s.True(status.Running)
		s.Equal(int64(100), status.EventsProcessed)
		s.EqualError(status.LastError, "test error")
		s.Equal(int64(5), status.RetryAttempts)
		s.Equal(int64(2), status.ConsecutiveErrors)
	})
}

func (s *VfseventsTestSuite) TestWatcherMetrics() {
	s.Run("WatcherMetrics struct creation", func() {
		metrics := WatcherMetrics{
			EventsPerSecond: 10.5,
			AverageLatency:  100 * time.Millisecond,
			ErrorRate:       0.05,
			MemoryUsage:     1024 * 1024,
		}

		s.Equal(10.5, metrics.EventsPerSecond)
		s.Equal(100*time.Millisecond, metrics.AverageLatency)
		s.Equal(0.05, metrics.ErrorRate)
		s.Equal(uint64(1024*1024), metrics.MemoryUsage)
	})
}

// Example demonstrates basic usage of the vfsevents package with functional options
func Example() {
	// Example demonstrates the functional options pattern used by watchers
	// In practice, you would use a specific watcher like VFSPoller, S3Events, or GCSEvents

	// Define handlers
	eventHandler := func(event Event) {
		fmt.Printf("Event: %s on %s\n", event.Type.String(), event.URI)
	}

	errorHandler := func(err error) {
		fmt.Printf("Error: %v\n", err)
	}

	// Example of functional options that can be used with any watcher
	_ = WithEventFilter(func(e Event) bool {
		// Only process .txt files
		return strings.HasSuffix(e.URI, ".txt")
	})

	_ = WithStatusCallback(func(status WatcherStatus) {
		fmt.Printf("Status: Running=%t, Events=%d\n", status.Running, status.EventsProcessed)
	})

	_ = WithTimeout(10 * time.Second)

	// Example event processing
	event := Event{
		Type: EventCreated,
		URI:  "/path/to/file.txt",
		Metadata: map[string]string{
			"size": "1024",
		},
	}
	eventHandler(event)

	// Example error handling
	err := errors.New("connection timeout")
	errorHandler(err)

	// Output:
	// Event: Created on /path/to/file.txt
	// Error: connection timeout
}

// ExampleWithRetryConfig demonstrates retry configuration for watchers
func ExampleWithRetryConfig() {
	// Create retry configuration
	retryConfig := RetryConfig{
		Enabled:         true,
		MaxRetries:      3,
		InitialBackoff:  time.Second,
		MaxBackoff:      30 * time.Second,
		BackoffFactor:   2.0,
		RetryableErrors: []string{"timeout", "connection refused"},
	}

	// Example of using retry configuration with functional options
	_ = WithRetryConfig(retryConfig)
	_ = WithMaxRetries(5)
	_ = WithRetryBackoff(2*time.Second, 60*time.Second)

	// Example status callback for retry monitoring
	statusCallback := WithStatusCallback(func(status WatcherStatus) {
		if status.RetryAttempts > 0 {
			fmt.Printf("Retry attempt: %d\n", status.RetryAttempts)
		}
	})

	fmt.Printf("Retry config enabled: %t\n", retryConfig.Enabled)
	fmt.Printf("Max retries: %d\n", retryConfig.MaxRetries)
	fmt.Printf("Status callback created: %t\n", statusCallback != nil)

	// Output:
	// Retry config enabled: true
	// Max retries: 3
	// Status callback created: true
}
