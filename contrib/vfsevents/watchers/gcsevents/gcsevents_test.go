package gcsevents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/contrib/vfsevents/watchers/gcsevents/mocks"
	"github.com/c2fo/vfs/v7/vfssimple"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type GCSWatcherTestSuite struct {
	suite.Suite
	pubsubClient *mocks.PubSubClient
	watcher      *GCSWatcher
}

func (s *GCSWatcherTestSuite) SetupTest() {
	s.pubsubClient = &mocks.PubSubClient{}
	s.watcher, _ = NewGCSWatcher("my-project-id", "my-subscription", WithPubSubClient(s.pubsubClient))
}

func (s *GCSWatcherTestSuite) TestNewGCSWatcher() {
	tests := []struct {
		name           string
		projectID      string
		subscriptionID string
		wantErr        bool
	}{
		{
			name:           "Valid subscription",
			projectID:      "my-project-id",
			subscriptionID: "my-subscription",
			wantErr:        false,
		},
		{
			name:           "Invalid project ID",
			projectID:      "",
			subscriptionID: "my-subscription",
			wantErr:        true,
		},
		{
			name:           "Invalid subscription ID",
			projectID:      "my-project-id",
			subscriptionID: "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			_, err := NewGCSWatcher(tt.projectID, tt.subscriptionID)
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
		})
	}
}

func (s *GCSWatcherTestSuite) TestStart() {
	tests := []struct {
		name       string
		setupMocks func()
		wantErr    bool
	}{
		{
			name: "Valid start",
			setupMocks: func() {
				s.pubsubClient.EXPECT().Receive(mock.Anything, mock.Anything).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "Receive error",
			setupMocks: func() {
				s.pubsubClient.EXPECT().Receive(mock.Anything, mock.Anything).Return(fmt.Errorf("receive error")).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.setupMocks()
			ctx := context.Background()
			handler := func(event vfsevents.Event) {}
			errHandler := func(err error) {
				if tt.wantErr {
					s.Error(err)
				} else {
					s.NoError(err)
				}
			}
			err := s.watcher.Start(ctx, handler, errHandler)
			s.NoError(err)
			s.NoError(s.watcher.Stop())
		})
	}
}

func (s *GCSWatcherTestSuite) TestPoll() {
	tests := []struct {
		name       string
		setupMocks func()
		wantErr    bool
	}{
		{
			name: "Valid poll",
			setupMocks: func() {
				event := GCSEvent{
					Name:        "object-name",
					Bucket:      "bucket-name",
					TimeCreated: JSONTime(time.Now()),
				}
				body, _ := json.Marshal(event)
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					handler := args.Get(1).(func(context.Context, *pubsub.Message))
					handler(context.TODO(), &pubsub.Message{
						Data: body,
						Attributes: map[string]string{
							"eventType": EventObjectFinalize,
							"eventTime": time.Now().Format(time.RFC3339),
						},
					})
				}).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "File delete event",
			setupMocks: func() {
				event := GCSEvent{
					Name:        "object-name",
					Bucket:      "bucket-name",
					TimeCreated: JSONTime(time.Now()),
				}
				body, _ := json.Marshal(event)
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					handler := args.Get(1).(func(context.Context, *pubsub.Message))
					handler(context.TODO(), &pubsub.Message{
						Data: body,
						Attributes: map[string]string{
							"eventType": EventObjectDelete,
							"eventTime": time.Now().Format(time.RFC3339),
						},
					})
				}).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "Neither finalize nor delete event",
			setupMocks: func() {
				event := GCSEvent{
					Name:        "object-name",
					Bucket:      "bucket-name",
					TimeCreated: JSONTime(time.Now()),
				}
				body, _ := json.Marshal(event)
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						handler := args.Get(1).(func(context.Context, *pubsub.Message))
						handler(context.TODO(), &pubsub.Message{
							Data: body,
							Attributes: map[string]string{
								"eventType": EventObjectMetadataUpdate,
								"eventTime": time.Now().Format(time.RFC3339),
							},
						})
					}).
					Return(nil).
					Once()
			},
			wantErr: false,
		},
		{
			name: "Receive error",
			setupMocks: func() {
				s.pubsubClient.EXPECT().
					Receive(mock.Anything, mock.Anything).
					Return(fmt.Errorf("receive error")).
					Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.setupMocks()

			_ = s.watcher.Start(context.TODO(), func(event vfsevents.Event) {}, func(err error) {
				if tt.wantErr {
					s.Error(err)
				} else {
					s.NoError(err)
				}
			})
			s.NoError(s.watcher.Stop())
		})
	}
}

func (s *GCSWatcherTestSuite) TestReceiveWithRetry() {
	tests := []struct {
		name           string
		retryConfig    vfsevents.RetryConfig
		setupMocks     func()
		expectCalls    int
		wantErr        bool
		wantRetryCount int
	}{
		{
			name: "Success on first attempt - no retry needed",
			retryConfig: vfsevents.RetryConfig{
				Enabled:        true,
				MaxRetries:     3,
				InitialBackoff: 10 * time.Millisecond,
				MaxBackoff:     100 * time.Millisecond,
				BackoffFactor:  2.0,
			},
			setupMocks: func() {
				event := GCSEvent{
					Name:        "test-object",
					Bucket:      "test-bucket",
					TimeCreated: JSONTime(time.Now()),
				}
				body, _ := json.Marshal(event)
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						handler := args.Get(1).(func(context.Context, *pubsub.Message))
						handler(context.TODO(), &pubsub.Message{
							Data: body,
							Attributes: map[string]string{
								"eventType": EventObjectFinalize,
								"eventTime": time.Now().Format(time.RFC3339),
							},
						})
					}).
					Return(nil).
					Once()
			},
			expectCalls:    1,
			wantErr:        false,
			wantRetryCount: 0,
		},
		{
			name: "Retry disabled - fail immediately",
			retryConfig: vfsevents.RetryConfig{
				Enabled: false,
			},
			setupMocks: func() {
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Return(fmt.Errorf("network error")).
					Once()
			},
			expectCalls:    1,
			wantErr:        true,
			wantRetryCount: 0,
		},
		{
			name: "Retryable error - success on second attempt",
			retryConfig: vfsevents.RetryConfig{
				Enabled:        true,
				MaxRetries:     3,
				InitialBackoff: 10 * time.Millisecond,
				MaxBackoff:     100 * time.Millisecond,
				BackoffFactor:  2.0,
			},
			setupMocks: func() {
				// First call fails with retryable error
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Return(fmt.Errorf("deadline exceeded")).
					Once()

				// Second call succeeds
				event := GCSEvent{
					Name:        "test-object",
					Bucket:      "test-bucket",
					TimeCreated: JSONTime(time.Now()),
				}
				body, _ := json.Marshal(event)
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						handler := args.Get(1).(func(context.Context, *pubsub.Message))
						handler(context.TODO(), &pubsub.Message{
							Data: body,
							Attributes: map[string]string{
								"eventType": EventObjectFinalize,
								"eventTime": time.Now().Format(time.RFC3339),
							},
						})
					}).
					Return(nil).
					Once()
			},
			expectCalls:    2,
			wantErr:        false,
			wantRetryCount: 1,
		},
		{
			name: "Non-retryable error - fail immediately",
			retryConfig: vfsevents.RetryConfig{
				Enabled:        true,
				MaxRetries:     3,
				InitialBackoff: 10 * time.Millisecond,
				MaxBackoff:     100 * time.Millisecond,
				BackoffFactor:  2.0,
			},
			setupMocks: func() {
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Return(fmt.Errorf("invalid credentials")).
					Once()
			},
			expectCalls:    1,
			wantErr:        true,
			wantRetryCount: 0,
		},
		{
			name: "Max retries exceeded - all attempts fail",
			retryConfig: vfsevents.RetryConfig{
				Enabled:        true,
				MaxRetries:     2,
				InitialBackoff: 10 * time.Millisecond,
				MaxBackoff:     100 * time.Millisecond,
				BackoffFactor:  2.0,
			},
			setupMocks: func() {
				// All attempts fail with retryable error
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Return(fmt.Errorf("service unavailable")).
					Times(3) // Initial attempt + 2 retries
			},
			expectCalls:    3,
			wantErr:        true,
			wantRetryCount: 3, // RetryAttempts incremented on each failure: 3 total attempts = 3 retry attempts
		},
		{
			name: "Context cancellation during retry",
			retryConfig: vfsevents.RetryConfig{
				Enabled:        true,
				MaxRetries:     3,
				InitialBackoff: 100 * time.Millisecond, // Longer backoff to allow cancellation
				MaxBackoff:     1000 * time.Millisecond,
				BackoffFactor:  2.0,
			},
			setupMocks: func() {
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Return(fmt.Errorf("deadline exceeded")).
					Maybe() // May be called multiple times before context cancellation
			},
			expectCalls:    1, // At least one call before cancellation
			wantErr:        true,
			wantRetryCount: 1, // Context canceled after first attempt fails and retry counter increments
		},
		{
			name: "Custom retryable error pattern",
			retryConfig: vfsevents.RetryConfig{
				Enabled:         true,
				MaxRetries:      2,
				InitialBackoff:  10 * time.Millisecond,
				MaxBackoff:      100 * time.Millisecond,
				BackoffFactor:   2.0,
				RetryableErrors: []string{"custom error pattern"},
			},
			setupMocks: func() {
				// First call fails with custom retryable error
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Return(fmt.Errorf("custom error pattern occurred")).
					Once()

				// Second call succeeds
				event := GCSEvent{
					Name:        "test-object",
					Bucket:      "test-bucket",
					TimeCreated: JSONTime(time.Now()),
				}
				body, _ := json.Marshal(event)
				s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						handler := args.Get(1).(func(context.Context, *pubsub.Message))
						handler(context.TODO(), &pubsub.Message{
							Data: body,
							Attributes: map[string]string{
								"eventType": EventObjectFinalize,
								"eventTime": time.Now().Format(time.RFC3339),
							},
						})
					}).
					Return(nil).
					Once()
			},
			expectCalls:    2,
			wantErr:        false,
			wantRetryCount: 1,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Reset mocks for each test
			s.pubsubClient.ExpectedCalls = nil
			s.pubsubClient.Calls = nil

			tt.setupMocks()

			// Create watcher with retry configuration
			watcher, err := NewGCSWatcher("test-project", "test-subscription", WithPubSubClient(s.pubsubClient))
			s.NoError(err)

			// Create start config with retry settings
			config := &vfsevents.StartConfig{
				RetryConfig: tt.retryConfig,
			}

			// Create status to track retry attempts
			status := &vfsevents.WatcherStatus{}

			// Create context with timeout for cancellation test
			ctx := context.Background()
			if tt.name == "Context cancellation during retry" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}

			// Test receiveWithRetry directly
			err = watcher.receiveWithRetry(
				ctx,
				func(event vfsevents.Event) {
					// Event handler - should be called on success
					s.Equal("gs://test-bucket/test-object", event.URI)
				},
				func(err error) {
					// Error handler - should be called on error
					if !tt.wantErr {
						s.Fail("Unexpected error: %v", err)
					}
				},
				status,
				config,
			)

			// Verify error expectation
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}

			// Verify retry count in status
			s.Equal(int64(tt.wantRetryCount), status.RetryAttempts)

			// Verify expected number of calls (allow some flexibility for context cancellation)
			if tt.name != "Context cancellation during retry" {
				s.pubsubClient.AssertNumberOfCalls(s.T(), "Receive", tt.expectCalls)
			}

			// Verify consecutive errors tracking
			if tt.wantErr && tt.wantRetryCount > 0 {
				s.Greater(status.ConsecutiveErrors, int64(0))
			} else if !tt.wantErr {
				s.Equal(int64(0), status.ConsecutiveErrors)
			}
		})
	}
}

func (s *GCSWatcherTestSuite) TestReceiveWithRetryBackoffTiming() {
	// Test that backoff timing works correctly
	retryConfig := vfsevents.RetryConfig{
		Enabled:        true,
		MaxRetries:     2,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     200 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	// Setup mocks to fail twice, then succeed
	s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
		Return(fmt.Errorf("service unavailable")).
		Times(2)

	event := GCSEvent{
		Name:        "test-object",
		Bucket:      "test-bucket",
		TimeCreated: JSONTime(time.Now()),
	}
	body, _ := json.Marshal(event)
	s.pubsubClient.On("Receive", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			handler := args.Get(1).(func(context.Context, *pubsub.Message))
			handler(context.TODO(), &pubsub.Message{
				Data: body,
				Attributes: map[string]string{
					"eventType": EventObjectFinalize,
					"eventTime": time.Now().Format(time.RFC3339),
				},
			})
		}).
		Return(nil).
		Once()

	watcher, err := NewGCSWatcher("test-project", "test-subscription", WithPubSubClient(s.pubsubClient))
	s.NoError(err)

	config := &vfsevents.StartConfig{
		RetryConfig: retryConfig,
	}
	status := &vfsevents.WatcherStatus{}

	// Measure timing
	start := time.Now()
	err = watcher.receiveWithRetry(
		context.Background(),
		func(event vfsevents.Event) {},
		func(err error) {},
		status,
		config,
	)
	elapsed := time.Since(start)

	s.NoError(err)
	s.Equal(int64(2), status.RetryAttempts)

	// Should have taken at least the sum of backoff delays
	// First retry: 50ms, Second retry: 100ms = ~150ms minimum
	s.GreaterOrEqual(elapsed, 140*time.Millisecond, "Should respect backoff timing")
	s.LessOrEqual(elapsed, 300*time.Millisecond, "Should not take too long")
}

func TestGCSWatcherTestSuite(t *testing.T) {
	suite.Run(t, new(GCSWatcherTestSuite))
}

// Example demonstrates basic usage of GCSEvents watcher for monitoring GCS bucket events
func Example() {
	// Create GCS watcher with Pub/Sub subscription
	watcher, err := NewGCSWatcher("my-project", "my-subscription")
	if err != nil {
		log.Fatalf("Failed to create GCS watcher: %v", err)
	}

	// Create VFS location for GCS bucket operations
	gcsLocation, err := vfssimple.NewLocation("gs://my-bucket/")
	if err != nil {
		log.Fatalf("Failed to create GCS VFS location: %v", err)
	}

	// Define event handler with VFS integration
	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("GCS Event: %s | %s\n", event.Type.String(), event.URI)

		// Access the file through VFS for additional operations
		if event.Type == vfsevents.EventCreated {
			file, err := gcsLocation.NewFile(event.URI)
			if err == nil {
				if size, err := file.Size(); err == nil {
					fmt.Printf("New file size: %d bytes\n", size)
				}

				// Read file content if needed
				content, err := io.ReadAll(file)
				if err == nil {
					fmt.Printf("File content preview: %.100s...\n", string(content))
				}
				file.Close()
			}
		}
	}

	// Define error handler
	errorHandler := func(err error) {
		fmt.Printf("GCS watcher error: %v\n", err)
	}

	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler)
	if err != nil {
		log.Fatalf("Failed to start GCS watcher: %v", err)
	}

	// Stop watching
	watcher.Stop()
}

// ExampleNewGCSWatcher_withRetryLogic demonstrates GCS watcher with retry configuration
func ExampleNewGCSWatcher_withRetryLogic() {
	// Create GCS watcher
	watcher, err := NewGCSWatcher("my-project", "my-subscription")
	if err != nil {
		log.Fatalf("Failed to create GCS watcher: %v", err)
	}

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("Processing GCS event: %s on %s\n", event.Type.String(), event.URI)
		
		// Extract metadata from the event
		if bucketName, exists := event.Metadata["bucketName"]; exists {
			fmt.Printf("Bucket: %s\n", bucketName)
		}
		if objectName, exists := event.Metadata["objectName"]; exists {
			fmt.Printf("Object: %s\n", objectName)
		}
		if eventType, exists := event.Metadata["eventType"]; exists {
			fmt.Printf("GCS Event Type: %s\n", eventType)
		}
	}

	errorHandler := func(err error) {
		fmt.Printf("Error processing GCS events: %v\n", err)
	}

	ctx := context.Background()

	// Start with retry logic and advanced configuration
	err = watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithRetryConfig(vfsevents.RetryConfig{
			Enabled:        true,
			MaxRetries:     5,
			InitialBackoff: 2 * time.Second,
			MaxBackoff:     60 * time.Second,
			BackoffFactor:  2.0,
			RetryableErrors: []string{"unavailable", "deadline exceeded"},
		}),
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			// Only process image files
			return strings.HasSuffix(e.URI, ".jpg") || 
				   strings.HasSuffix(e.URI, ".png") ||
				   strings.HasSuffix(e.URI, ".gif")
		}),
		vfsevents.WithStatusCallback(func(status vfsevents.WatcherStatus) {
			if status.RetryAttempts > 0 {
				fmt.Printf("GCS Watcher retrying: attempt %d, consecutive errors: %d\n", 
					status.RetryAttempts, status.ConsecutiveErrors)
			}
		}),
		vfsevents.WithMetricsCollector(func(metrics vfsevents.WatcherMetrics) {
			fmt.Printf("GCS Watcher metrics: %.2f events/sec, error rate: %.2f%%\n",
				metrics.EventsPerSecond, metrics.ErrorRate*100)
		}),
	)
	if err != nil {
		log.Fatalf("Failed to start GCS watcher: %v", err)
	}

	// Graceful shutdown with timeout
	watcher.Stop(vfsevents.WithTimeout(30 * time.Second))
}
