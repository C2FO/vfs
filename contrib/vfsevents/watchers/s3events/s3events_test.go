package s3events

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/contrib/vfsevents/watchers/s3events/mocks"
	"github.com/c2fo/vfs/v7/vfssimple"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type S3WatcherTestSuite struct {
	suite.Suite
	sqsClient *mocks.SqsClient
	watcher   *S3Watcher
}

func (s *S3WatcherTestSuite) SetupTest() {
	s.sqsClient = &mocks.SqsClient{}
	s.watcher, _ = NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/my-queue", WithSqsClient(s.sqsClient))
}

func (s *S3WatcherTestSuite) TestNewS3Watcher() {
	tests := []struct {
		name     string
		queueURL string
		wantErr  bool
	}{
		{
			name:     "Valid queue URL",
			queueURL: "https://sqs.us-east-1.amazonaws.com/123456789012/my-queue",
			wantErr:  false,
		},
		{
			name:     "Invalid queue URL",
			queueURL: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			_, err := NewS3Watcher(tt.queueURL)
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
		})
	}
}

func (s *S3WatcherTestSuite) TestStart() {
	tests := []struct {
		name       string
		setupMocks func()
		wantErr    bool
	}{
		{
			name: "Valid start",
			setupMocks: func() {
				s.sqsClient.EXPECT().ReceiveMessage(mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(&sqs.ReceiveMessageOutput{}, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "ReceiveMessage error",
			setupMocks: func() {
				s.sqsClient.EXPECT().ReceiveMessage(mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(nil, fmt.Errorf("receive message error")).Once()
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

func (s *S3WatcherTestSuite) TestPoll() {
	tests := []struct {
		name       string
		setupMocks func()
		wantErr    bool
	}{
		{
			name: "Valid poll",
			setupMocks: func() {
				event := S3Event{
					Records: []S3Record{
						{
							EventTime: time.Now().Format(time.RFC3339Nano),
							EventName: "s3:ObjectCreated:Put",
							S3: S3Entity{
								Bucket: S3Bucket{
									Name: "bucket-name",
								},
								Object: S3Object{
									Key: "object-key",
								},
							},
						},
					},
				}
				body, _ := json.Marshal(event)
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(&sqs.ReceiveMessageOutput{
					Messages: []types.Message{
						{
							Body:          aws.String(string(body)),
							ReceiptHandle: aws.String("receipt-handle"),
							Attributes: map[string]string{
								"ApproximateReceiveCount": "1",
							},
						},
					},
				}, nil).Once()
				s.sqsClient.On("DeleteMessage", mock.Anything, &sqs.DeleteMessageInput{
					QueueUrl:      aws.String(s.watcher.queueURL),
					ReceiptHandle: aws.String("receipt-handle"),
				}).Return(&sqs.DeleteMessageOutput{}, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "File delete event",
			setupMocks: func() {
				event := S3Event{
					Records: []S3Record{
						{
							EventTime: time.Now().Format(time.RFC3339Nano),
							EventName: "s3:ObjectRemoved:Delete",
							S3: S3Entity{
								Bucket: S3Bucket{
									Name: "bucket-name",
								},
								Object: S3Object{
									Key: "object-key",
								},
							},
						},
					},
				}
				body, _ := json.Marshal(event)
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(&sqs.ReceiveMessageOutput{
					Messages: []types.Message{
						{
							Body:          aws.String(string(body)),
							ReceiptHandle: aws.String("receipt-handle"),
							Attributes: map[string]string{
								"ApproximateReceiveCount": "1",
							},
						},
					},
				}, nil).Once()
				s.sqsClient.On("DeleteMessage", mock.Anything, &sqs.DeleteMessageInput{
					QueueUrl:      aws.String(s.watcher.queueURL),
					ReceiptHandle: aws.String("receipt-handle"),
				}).Return(&sqs.DeleteMessageOutput{}, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "Neither put nor delete event",
			setupMocks: func() {
				event := S3Event{
					Records: []S3Record{
						{
							EventTime: time.Now().Format(time.RFC3339Nano),
							EventName: "s3:ObjectAccessed:Get",
							S3: S3Entity{
								Bucket: S3Bucket{
									Name: "bucket-name",
								},
								Object: S3Object{
									Key: "object-key",
								},
							},
						},
					},
				}
				body, _ := json.Marshal(event)
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(&sqs.ReceiveMessageOutput{
					Messages: []types.Message{
						{
							Body:          aws.String(string(body)),
							ReceiptHandle: aws.String("receipt-handle"),
							Attributes: map[string]string{
								"ApproximateReceiveCount": "1",
							},
						},
					},
				}, nil).Once()
				s.sqsClient.On("DeleteMessage", mock.Anything, &sqs.DeleteMessageInput{
					QueueUrl:      aws.String(s.watcher.queueURL),
					ReceiptHandle: aws.String("receipt-handle"),
				}).Return(&sqs.DeleteMessageOutput{}, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "ApproximateReceiveCount > 5",
			setupMocks: func() {
				event := S3Event{
					Records: []S3Record{
						{
							EventTime: time.Now().Format(time.RFC3339Nano),
							EventName: "s3:ObjectCreated:Put",
							S3: S3Entity{
								Bucket: S3Bucket{
									Name: "bucket-name",
								},
								Object: S3Object{
									Key: "object-key",
								},
							},
						},
					},
				}
				body, _ := json.Marshal(event)
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(&sqs.ReceiveMessageOutput{
					Messages: []types.Message{
						{
							Body:          aws.String(string(body)),
							ReceiptHandle: aws.String("receipt-handle"),
							Attributes: map[string]string{
								"ApproximateReceiveCount": "6",
							},
						},
					},
				}, nil).Once()
				s.sqsClient.EXPECT().DeleteMessage(mock.Anything, &sqs.DeleteMessageInput{
					QueueUrl:      aws.String(s.watcher.queueURL),
					ReceiptHandle: aws.String("receipt-handle"),
				}).Return(&sqs.DeleteMessageOutput{}, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "ReceiveMessage error",
			setupMocks: func() {
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(nil, fmt.Errorf("receive message error")).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.setupMocks()

			status := &vfsevents.WatcherStatus{
				Running:   true,
				StartTime: time.Now(),
			}
			config := &vfsevents.StartConfig{}

			err := s.watcher.pollOnce(context.TODO(), func(event vfsevents.Event) {}, status, config)
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
		})
	}
}

func (s *S3WatcherTestSuite) TestPollWithRetry() {
	tests := []struct {
		name           string
		retryConfig    vfsevents.RetryConfig
		setupMocks     func()
		wantErr        bool
		contextTimeout time.Duration
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
				s3Event := S3Event{
					Records: []S3Record{
						{
							EventName: "s3:ObjectCreated:Put",
							S3: S3Entity{
								Bucket: S3Bucket{Name: "test-bucket"},
								Object: S3Object{Key: "test-object"},
							},
						},
					},
				}
				body, _ := json.Marshal(s3Event)
				// First successful call
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(&sqs.ReceiveMessageOutput{
					Messages: []types.Message{
						{
							Body:          aws.String(string(body)),
							ReceiptHandle: aws.String("receipt-handle"),
							Attributes: map[string]string{
								"ApproximateReceiveCount": "1",
							},
						},
					},
				}, nil).Once()
				s.sqsClient.On("DeleteMessage", mock.Anything, &sqs.DeleteMessageInput{
					QueueUrl:      aws.String(s.watcher.queueURL),
					ReceiptHandle: aws.String("receipt-handle"),
				}).Return(&sqs.DeleteMessageOutput{}, nil).Once()
				// Subsequent calls return empty to avoid infinite loop
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(&sqs.ReceiveMessageOutput{Messages: []types.Message{}}, nil).Maybe()
			},
			wantErr:        false,
			contextTimeout: 100 * time.Millisecond,
		},
		{
			name: "Retry disabled - continues polling with errors",
			retryConfig: vfsevents.RetryConfig{
				Enabled: false,
			},
			setupMocks: func() {
				// S3 poll continues even with errors - they're handled by error handler
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(nil, fmt.Errorf("network error")).Maybe()
			},
			wantErr:        false, // poll method returns nil on context cancellation
			contextTimeout: 50 * time.Millisecond,
		},
		{
			name: "Context cancellation stops polling",
			retryConfig: vfsevents.RetryConfig{
				Enabled:        true,
				MaxRetries:     3,
				InitialBackoff: 10 * time.Millisecond,
				MaxBackoff:     100 * time.Millisecond,
				BackoffFactor:  2.0,
			},
			setupMocks: func() {
				// Any response is fine - context will cancel
				s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
					QueueUrl:                    aws.String(s.watcher.queueURL),
					MaxNumberOfMessages:         10,
					WaitTimeSeconds:             20,
					MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
				}).Return(&sqs.ReceiveMessageOutput{Messages: []types.Message{}}, nil).Maybe()
			},
			wantErr:        false, // Context cancellation returns nil
			contextTimeout: 50 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Reset mocks for each test
			s.sqsClient.ExpectedCalls = nil
			s.sqsClient.Calls = nil

			tt.setupMocks()

			// Create watcher with retry configuration
			watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/my-queue", WithSqsClient(s.sqsClient))
			s.NoError(err)

			// Create start config with retry settings
			config := &vfsevents.StartConfig{
				RetryConfig: tt.retryConfig,
			}

			// Create status to track retry attempts
			status := &vfsevents.WatcherStatus{}

			// Create context with timeout to control polling duration
			ctx, cancel := context.WithTimeout(context.Background(), tt.contextTimeout)
			defer cancel()

			// Track errors via error handler
			var errorCount int
			errorHandler := func(err error) {
				errorCount++
				// For tests expecting errors, this is normal behavior
			}

			// Test poll directly (pollWithRetry was removed as dead code)
			watcher.poll(
				ctx,
				func(event vfsevents.Event) {
					// Event handler - should be called on success
					s.Equal("s3://test-bucket/test-object", event.URI)
				},
				errorHandler,
				status,
				config,
			)

			// For error cases, verify that error handler was called
			if tt.name == "Retry disabled - continues polling with errors" {
				s.Greater(errorCount, 0, "Error handler should be called for polling errors")
			}
		})
	}
}

func (s *S3WatcherTestSuite) TestWithReceivedCount() {
	tests := []struct {
		name                   string
		receivedCount          uint
		messageReceiveCount    string
		expectMessageProcessed bool
		expectMessageDeleted   bool
		expectError            bool
	}{
		{
			name:                   "Message processed - receive count below threshold",
			receivedCount:          5,
			messageReceiveCount:    "3",
			expectMessageProcessed: true,
			expectMessageDeleted:   true,
			expectError:            false,
		},
		{
			name:                   "Message processed - receive count equals threshold",
			receivedCount:          5,
			messageReceiveCount:    "5",
			expectMessageProcessed: true,
			expectMessageDeleted:   true,
			expectError:            false,
		},
		{
			name:                   "Message skipped - receive count above threshold",
			receivedCount:          5,
			messageReceiveCount:    "6",
			expectMessageProcessed: false,
			expectMessageDeleted:   true, // Should still be deleted
			expectError:            false,
		},
		{
			name:                   "Message skipped - receive count much higher than threshold",
			receivedCount:          3,
			messageReceiveCount:    "10",
			expectMessageProcessed: false,
			expectMessageDeleted:   true,
			expectError:            false,
		},
		{
			name:                   "Error - invalid receive count format",
			receivedCount:          5,
			messageReceiveCount:    "invalid",
			expectMessageProcessed: false,
			expectMessageDeleted:   false,
			expectError:            true,
		},
		{
			name:                   "Message processed - receive count is 1",
			receivedCount:          5,
			messageReceiveCount:    "1",
			expectMessageProcessed: true,
			expectMessageDeleted:   true,
			expectError:            false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Create watcher with specific receivedCount
			watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				WithSqsClient(s.sqsClient),
				WithReceivedCount(tt.receivedCount))
			s.NoError(err)

			// Create test S3 event
			event := S3Event{
				Records: []S3Record{
					{
						EventTime: time.Now().Format(time.RFC3339Nano),
						EventName: "s3:ObjectCreated:Put",
						S3: S3Entity{
							Bucket: S3Bucket{Name: "test-bucket"},
							Object: S3Object{Key: "test-object"},
						},
					},
				},
			}
			body, _ := json.Marshal(event)

			// Setup mock expectations
			s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
				QueueUrl:                    aws.String(watcher.queueURL),
				MaxNumberOfMessages:         10,
				WaitTimeSeconds:             20,
				MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
			}).Return(&sqs.ReceiveMessageOutput{
				Messages: []types.Message{
					{
						Body:          aws.String(string(body)),
						ReceiptHandle: aws.String("receipt-handle"),
						Attributes: map[string]string{
							"ApproximateReceiveCount": tt.messageReceiveCount,
						},
					},
				},
			}, nil).Once()

			// Expect DeleteMessage call (should happen regardless of processing)
			if tt.expectMessageDeleted {
				s.sqsClient.EXPECT().DeleteMessage(mock.Anything, &sqs.DeleteMessageInput{
					QueueUrl:      aws.String(watcher.queueURL),
					ReceiptHandle: aws.String("receipt-handle"),
				}).Return(&sqs.DeleteMessageOutput{}, nil).Once()
			}

			// Track if handler was called
			handlerCalled := false
			handler := func(e vfsevents.Event) {
				handlerCalled = true
				s.Equal("s3://test-bucket/test-object", e.URI)
				s.Equal(vfsevents.EventCreated, e.Type)
			}

			config := &vfsevents.StartConfig{}
			status := &vfsevents.WatcherStatus{}

			err = watcher.pollOnce(context.Background(), handler, status, config)
			if tt.expectError {
				s.Error(err)
			} else {
				s.NoError(err)
			}

			s.Equal(tt.expectMessageProcessed, handlerCalled,
				"Handler call expectation mismatch for receive count %s with threshold %d",
				tt.messageReceiveCount, tt.receivedCount)
		})
	}
}

func (s *S3WatcherTestSuite) TestWithReceivedCountNoAttributes() {
	// Test case where message has no attributes
	watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		WithSqsClient(s.sqsClient),
		WithReceivedCount(5))
	s.NoError(err)

	event := S3Event{
		Records: []S3Record{
			{
				EventTime: time.Now().Format(time.RFC3339Nano),
				EventName: "s3:ObjectCreated:Put",
				S3: S3Entity{
					Bucket: S3Bucket{Name: "test-bucket"},
					Object: S3Object{Key: "test-object"},
				},
			},
		},
	}
	body, _ := json.Marshal(event)

	s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
		QueueUrl:                    aws.String(watcher.queueURL),
		MaxNumberOfMessages:         10,
		WaitTimeSeconds:             20,
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
	}).Return(&sqs.ReceiveMessageOutput{
		Messages: []types.Message{
			{
				Body:          aws.String(string(body)),
				ReceiptHandle: aws.String("receipt-handle"),
				Attributes:    nil, // No attributes
			},
		},
	}, nil).Once()

	s.sqsClient.EXPECT().DeleteMessage(mock.Anything, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(watcher.queueURL),
		ReceiptHandle: aws.String("receipt-handle"),
	}).Return(&sqs.DeleteMessageOutput{}, nil).Once()

	handlerCalled := false
	handler := func(e vfsevents.Event) {
		handlerCalled = true
	}

	config := &vfsevents.StartConfig{}
	status := &vfsevents.WatcherStatus{}

	err = watcher.pollOnce(context.Background(), handler, status, config)
	s.NoError(err)
	s.True(handlerCalled, "Handler should be called when no attributes are present")
}

func (s *S3WatcherTestSuite) TestWithReceivedCountMissingAttribute() {
	// Test case where message has attributes but no ApproximateReceiveCount
	watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		WithSqsClient(s.sqsClient),
		WithReceivedCount(5))
	s.NoError(err)

	event := S3Event{
		Records: []S3Record{
			{
				EventTime: time.Now().Format(time.RFC3339Nano),
				EventName: "s3:ObjectCreated:Put",
				S3: S3Entity{
					Bucket: S3Bucket{Name: "test-bucket"},
					Object: S3Object{Key: "test-object"},
				},
			},
		},
	}
	body, _ := json.Marshal(event)

	s.sqsClient.On("ReceiveMessage", mock.Anything, &sqs.ReceiveMessageInput{
		QueueUrl:                    aws.String(watcher.queueURL),
		MaxNumberOfMessages:         10,
		WaitTimeSeconds:             20,
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{"ApproximateReceiveCount"},
	}).Return(&sqs.ReceiveMessageOutput{
		Messages: []types.Message{
			{
				Body:          aws.String(string(body)),
				ReceiptHandle: aws.String("receipt-handle"),
				Attributes: map[string]string{
					"SomeOtherAttribute": "value",
					// ApproximateReceiveCount is missing
				},
			},
		},
	}, nil).Once()

	s.sqsClient.EXPECT().DeleteMessage(mock.Anything, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(watcher.queueURL),
		ReceiptHandle: aws.String("receipt-handle"),
	}).Return(&sqs.DeleteMessageOutput{}, nil).Once()

	handlerCalled := false
	handler := func(e vfsevents.Event) {
		handlerCalled = true
	}

	config := &vfsevents.StartConfig{}
	status := &vfsevents.WatcherStatus{}

	err = watcher.pollOnce(context.Background(), handler, status, config)
	s.NoError(err)
	s.True(handlerCalled, "Handler should be called when ApproximateReceiveCount attribute is missing")
}

func (s *S3WatcherTestSuite) TestNewS3WatcherWithReceivedCount() {
	tests := []struct {
		name          string
		receivedCount uint
		expected      int
	}{
		{
			name:          "Custom received count",
			receivedCount: 3,
			expected:      3,
		},
		{
			name:          "Zero received count",
			receivedCount: 0,
			expected:      0,
		},
		{
			name:          "Large received count",
			receivedCount: 100,
			expected:      100,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				WithSqsClient(s.sqsClient),
				WithReceivedCount(tt.receivedCount))

			s.NoError(err)
			s.Equal(tt.expected, watcher.receivedCount, "receivedCount should be set correctly")
		})
	}
}

func (s *S3WatcherTestSuite) TestDefaultReceivedCount() {
	// Test that default receivedCount is 5
	watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		WithSqsClient(s.sqsClient))

	s.NoError(err)
	s.Equal(5, watcher.receivedCount, "Default receivedCount should be 5")
}

func TestS3WatcherTestSuite(t *testing.T) {
	suite.Run(t, new(S3WatcherTestSuite))
}

// Example demonstrates basic usage of S3Events watcher for monitoring S3 bucket events
func Example() {
	// Create S3 watcher with SQS queue URL
	watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/my-queue")
	if err != nil {
		log.Fatalf("Failed to create S3 watcher: %v", err)
	}

	// Create VFS location for S3 bucket operations
	s3Location, err := vfssimple.NewLocation("s3://my-bucket/")
	if err != nil {
		log.Fatalf("Failed to create S3 VFS location: %v", err)
	}

	// Define event handler with VFS integration
	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("S3 Event: %s | %s\n", event.Type.String(), event.URI)

		// Access the file through VFS for additional operations
		if event.Type == vfsevents.EventCreated {
			file, err := s3Location.NewFile(event.URI)
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
		fmt.Printf("S3 watcher error: %v\n", err)
	}

	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler)
	if err != nil {
		log.Fatalf("Failed to start S3 watcher: %v", err)
	}

	// Stop watching
	watcher.Stop()
}

// ExampleNewS3Watcher_withConfiguration demonstrates S3Events watcher with advanced configuration
func ExampleNewS3Watcher_withConfiguration() {
	// Create S3 watcher with custom configuration
	watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/my-queue",
		WithReceivedCount(3), // Delete messages after 3 failed processing attempts
	)
	if err != nil {
		log.Fatalf("Failed to create S3 watcher: %v", err)
	}

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("Processing S3 event: %s on %s\n", event.Type.String(), event.URI)
		
		// Extract metadata from the event
		if bucketName, exists := event.Metadata["bucketName"]; exists {
			fmt.Printf("Bucket: %s\n", bucketName)
		}
		if objectKey, exists := event.Metadata["objectKey"]; exists {
			fmt.Printf("Object Key: %s\n", objectKey)
		}
		if eventName, exists := event.Metadata["eventName"]; exists {
			fmt.Printf("S3 Event Name: %s\n", eventName)
		}
	}

	errorHandler := func(err error) {
		fmt.Printf("Error processing S3 events: %v\n", err)
	}

	ctx := context.Background()

	// Start with event filtering and status monitoring
	err = watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			// Only process .json files
			return strings.HasSuffix(e.URI, ".json")
		}),
		vfsevents.WithStatusCallback(func(status vfsevents.WatcherStatus) {
			fmt.Printf("S3 Watcher Status: Running=%t, Events=%d\n", 
				status.Running, status.EventsProcessed)
		}),
	)
	if err != nil {
		log.Fatalf("Failed to start S3 watcher: %v", err)
	}

	// Graceful shutdown with timeout
	watcher.Stop(vfsevents.WithTimeout(30 * time.Second))
}
