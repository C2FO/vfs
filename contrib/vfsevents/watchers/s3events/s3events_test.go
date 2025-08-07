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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/contrib/vfsevents/watchers/s3events/mocks"
	"github.com/c2fo/vfs/v7/vfssimple"
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

func (s *S3WatcherTestSuite) TestMapS3EventType() {
	tests := []struct {
		name      string
		eventName string
		expected  vfsevents.EventType
	}{
		{
			name:      "ObjectCreated:Put - direct upload",
			eventName: "s3:ObjectCreated:Put",
			expected:  vfsevents.EventCreated,
		},
		{
			name:      "ObjectCreated:Post - form upload",
			eventName: "s3:ObjectCreated:Post",
			expected:  vfsevents.EventCreated,
		},
		{
			name:      "ObjectCreated:Copy - copy operation",
			eventName: "s3:ObjectCreated:Copy",
			expected:  vfsevents.EventModified,
		},
		{
			name:      "ObjectCreated:CompleteMultipartUpload - large upload",
			eventName: "s3:ObjectCreated:CompleteMultipartUpload",
			expected:  vfsevents.EventModified,
		},
		{
			name:      "ObjectCreated:* - wildcard",
			eventName: "s3:ObjectCreated:*",
			expected:  vfsevents.EventCreated,
		},
		{
			name:      "ObjectRestore:Post - restore initiation",
			eventName: "s3:ObjectRestore:Post",
			expected:  vfsevents.EventModified,
		},
		{
			name:      "ObjectRestore:Completed - restore completion",
			eventName: "s3:ObjectRestore:Completed",
			expected:  vfsevents.EventModified,
		},
		{
			name:      "ObjectRestore:Delete - restored copy expires",
			eventName: "s3:ObjectRestore:Delete",
			expected:  vfsevents.EventDeleted,
		},
		{
			name:      "ObjectRemoved:Delete - object deletion",
			eventName: "s3:ObjectRemoved:Delete",
			expected:  vfsevents.EventDeleted,
		},
		{
			name:      "ObjectRemoved:DeleteMarkerCreated - versioned deletion",
			eventName: "s3:ObjectRemoved:DeleteMarkerCreated",
			expected:  vfsevents.EventDeleted,
		},
		{
			name:      "ObjectRemoved:* - wildcard deletion",
			eventName: "s3:ObjectRemoved:*",
			expected:  vfsevents.EventDeleted,
		},
		{
			name:      "Unknown event type",
			eventName: "s3:ObjectTagging:Put",
			expected:  vfsevents.EventUnknown,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := s.watcher.mapS3EventType(tt.eventName)
			s.Equal(tt.expected, result, "Event type mapping should be correct")
		})
	}
}

func (s *S3WatcherTestSuite) TestGetOperationType() {
	tests := []struct {
		name      string
		eventName string
		expected  string
	}{
		{
			name:      "Put operation",
			eventName: "s3:ObjectCreated:Put",
			expected:  "put",
		},
		{
			name:      "Post operation",
			eventName: "s3:ObjectCreated:Post",
			expected:  "post",
		},
		{
			name:      "Copy operation",
			eventName: "s3:ObjectCreated:Copy",
			expected:  "copy",
		},
		{
			name:      "Multipart operation",
			eventName: "s3:ObjectCreated:CompleteMultipartUpload",
			expected:  "multipart",
		},
		{
			name:      "Restore operation",
			eventName: "s3:ObjectRestore:Completed",
			expected:  "restore",
		},
		{
			name:      "Delete operation",
			eventName: "s3:ObjectRemoved:Delete",
			expected:  "delete",
		},
		{
			name:      "Unknown operation",
			eventName: "s3:ObjectTagging:Put",
			expected:  "unknown",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result := s.watcher.getOperationType(tt.eventName)
			s.Equal(tt.expected, result, "Operation type should be correct")
		})
	}
}

func (s *S3WatcherTestSuite) TestEnhancedMetadata() {
	// Test that enhanced metadata is properly captured and included in events
	var receivedEvent vfsevents.Event
	eventReceived := make(chan bool, 1)
	handler := func(event vfsevents.Event) {
		receivedEvent = event
		eventReceived <- true
	}
	// Create a comprehensive S3 event with all metadata fields
	s3Event := S3Event{
		Records: []S3Record{
			{
				EventTime: "2023-01-01T12:00:00.000Z",
				EventName: "s3:ObjectCreated:Copy",
				AwsRegion: "us-east-1",
				S3: S3Entity{
					Bucket: S3Bucket{
						Name: "test-bucket",
					},
					Object: S3Object{
						Key:       "test-file.txt",
						Size:      1024,
						ETag:      "d41d8cd98f00b204e9800998ecf8427e",
						VersionID: "3sL4kqtJlcpXroDTDmJ+rmSpXd3dIbrHY+MTRCxf3vjVBH40Nr8X8gdRQBpUMLUo",
						Sequencer: "0055AED6DCD90281E5",
					},
				},
			},
		},
	}

	body, _ := json.Marshal(s3Event)
	message := types.Message{
		Body: aws.String(string(body)),
	}

	s.sqsClient.On("ReceiveMessage", mock.Anything, mock.Anything).
		Return(&sqs.ReceiveMessageOutput{
			Messages: []types.Message{message},
		}, nil).
		Once()

	s.sqsClient.On("DeleteMessage", mock.Anything, mock.Anything).
		Return(&sqs.DeleteMessageOutput{}, nil).
		Once()

	// Use longer timeout for Windows compatibility
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := &vfsevents.StartConfig{}
	status := &vfsevents.WatcherStatus{}

	// Start pollOnce in goroutine
	go func() {
		_ = s.watcher.pollOnce(ctx, handler, status, config)
	}()

	// Wait for event to be received with timeout
	select {
	case <-eventReceived:
		// Event received successfully
	case <-time.After(2 * time.Second):
		s.Fail("Timeout waiting for S3 event to be processed")
		return
	}

	// Verify the event was processed and contains enhanced metadata
	s.Equal(vfsevents.EventModified, receivedEvent.Type, "Copy operation should be mapped to EventModified")
	s.Equal("s3://test-bucket/test-file.txt", receivedEvent.URI)

	// Verify enhanced metadata
	s.Equal("test-bucket", receivedEvent.Metadata["bucketName"])
	s.Equal("test-file.txt", receivedEvent.Metadata["key"])
	s.Equal("s3:ObjectCreated:Copy", receivedEvent.Metadata["eventName"])
	s.Equal("us-east-1", receivedEvent.Metadata["region"])
	s.Equal("2023-01-01T12:00:00.000Z", receivedEvent.Metadata["eventTime"])
	s.Equal("copy", receivedEvent.Metadata["operation"])
	s.Equal("3sL4kqtJlcpXroDTDmJ+rmSpXd3dIbrHY+MTRCxf3vjVBH40Nr8X8gdRQBpUMLUo", receivedEvent.Metadata["versionId"])
	s.Equal("true", receivedEvent.Metadata["isVersioned"])
	s.Equal("d41d8cd98f00b204e9800998ecf8427e", receivedEvent.Metadata["eTag"])
	s.Equal("0055AED6DCD90281E5", receivedEvent.Metadata["sequencer"])
	s.Equal("1024", receivedEvent.Metadata["size"])
}

func (s *S3WatcherTestSuite) TestNonVersionedBucketMetadata() {
	// Test metadata for non-versioned bucket (versionId should be empty)
	var receivedEvent vfsevents.Event
	handler := func(event vfsevents.Event) {
		receivedEvent = event
	}

	// Create S3 event without version ID (non-versioned bucket)
	s3Event := S3Event{
		Records: []S3Record{
			{
				EventTime: "2023-01-01T12:00:00.000Z",
				EventName: "s3:ObjectCreated:Put",
				AwsRegion: "us-west-2",
				S3: S3Entity{
					Bucket: S3Bucket{
						Name: "non-versioned-bucket",
					},
					Object: S3Object{
						Key:       "simple-file.txt",
						Size:      512,
						ETag:      "098f6bcd4621d373cade4e832627b4f6",
						VersionID: "", // No version ID for non-versioned bucket
						Sequencer: "0055AED6DCD90281E6",
					},
				},
			},
		},
	}

	body, _ := json.Marshal(s3Event)
	message := types.Message{
		Body: aws.String(string(body)),
	}

	s.sqsClient.On("ReceiveMessage", mock.Anything, mock.Anything).
		Return(&sqs.ReceiveMessageOutput{
			Messages: []types.Message{message},
		}, nil).
		Once()

	s.sqsClient.On("DeleteMessage", mock.Anything, mock.Anything).
		Return(&sqs.DeleteMessageOutput{}, nil).
		Once()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	config := &vfsevents.StartConfig{}
	status := &vfsevents.WatcherStatus{}

	go func() {
		_ = s.watcher.pollOnce(ctx, handler, status, config)
	}()

	time.Sleep(50 * time.Millisecond)

	// Verify the event was processed
	s.Equal(vfsevents.EventCreated, receivedEvent.Type, "Put operation should be mapped to EventCreated")
	s.Equal("s3://non-versioned-bucket/simple-file.txt", receivedEvent.URI)

	// Verify non-versioned metadata
	s.Equal("false", receivedEvent.Metadata["isVersioned"])
	s.Equal("put", receivedEvent.Metadata["operation"])
	s.Equal("512", receivedEvent.Metadata["size"])

	// versionId should not be present in metadata for non-versioned buckets
	_, hasVersionID := receivedEvent.Metadata["versionId"]
	s.False(hasVersionID, "versionId should not be present for non-versioned buckets")
}

func TestS3WatcherTestSuite(t *testing.T) {
	suite.Run(t, new(S3WatcherTestSuite))
}

// Example demonstrates basic usage of S3Events watcher for monitoring S3 bucket events
func Example() {
	// Create S3 watcher with SQS queue URL
	watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/my-queue")
	if err != nil {
		log.Printf("Failed to create S3 watcher: %v", err)
		return
	}

	// Create VFS location for S3 bucket operations
	s3Location, err := vfssimple.NewLocation("s3://my-bucket/")
	if err != nil {
		log.Printf("Failed to create S3 VFS location: %v", err)
		return
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
				_ = file.Close() // Ignore error in example
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
		log.Printf("Failed to start S3 watcher: %v", err)
		return
	}

	// Stop watching
	_ = watcher.Stop() // Ignore error in example
}

// ExampleNewS3Watcher_withConfiguration demonstrates S3Events watcher with advanced configuration
func ExampleNewS3Watcher_withConfiguration() {
	// Create S3 watcher with custom configuration
	watcher, err := NewS3Watcher("https://sqs.us-east-1.amazonaws.com/123456789012/my-queue",
		WithReceivedCount(3), // Delete messages after 3 failed processing attempts
	)
	if err != nil {
		log.Printf("Failed to create S3 watcher: %v", err)
		return
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
		log.Printf("Failed to start S3 watcher: %v", err)
		return
	}

	// Graceful shutdown with timeout
	_ = watcher.Stop(vfsevents.WithTimeout(30 * time.Second)) // Ignore error in example
}
