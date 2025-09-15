# s3events

`s3events` provides an `S3Watcher` that implements the `vfsevents.Watcher` interface by using SQS events from an S3 bucket.

## Installation

To install the package, use the following command:

```bash
go get github.com/c2fo/vfs/contrib/vfsevents/watchers/s3events
```

## Usage

### Example

The following example demonstrates how to use the `S3Watcher` with an S3 bucket:

```go
package main

import (
	"context"
	"log"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/contrib/vfsevents/watchers/s3events"
)

func main() {
	// Initialize the S3Watcher
	watcher, err := s3events.NewS3Watcher(
		"https://sqs.us-east-1.amazonaws.com/123456789012/my-queue",
		s3events.WithSqsClient(customSqsClient),
		s3events.WithReceivedCount(10),
	)
	if err != nil {
		log.Fatal(fmt.Errorf("error creating S3Watcher: %w", err))
	}

	// Define the event handler
	handler := func(event vfsevents.Event) {
		log.Printf("Event: %v\n", event)
	}

	// Define the error handler
	errHandler := func(err error) {
		log.Printf("Error: %v\n", err)
	}

	// Start the watcher
	ctx := context.Background()
	err = watcher.Start(ctx, handler, errHandler)
	if err != nil {
		log.Fatal(fmt.Errorf("error starting watcher: %v\n", err))
	}

	// Run the watcher for 2 minutes
	time.Sleep(2 * time.Minute)
	watcher.Stop()
}

## Event Mapping

The S3 watcher provides **semantic accuracy** in event mapping by using operation-based logic to distinguish between different types of S3 operations:

| S3 Event Type | VFS Event Type | Description |
|---|---|---|
| `s3:ObjectCreated:Put` | `EventCreated` | Direct file uploads (typically new files) |
| `s3:ObjectCreated:Post` | `EventCreated` | Form-based uploads (typically new files) |
| `s3:ObjectCreated:Copy` | `EventModified` | Copy operations (often overwrites/modifications) |
| `s3:ObjectCreated:CompleteMultipartUpload` | `EventModified` | Large uploads (often significant changes) |
| `s3:ObjectCreated:*` | `EventCreated` | Wildcard for broad compatibility |
| `s3:ObjectRestore:Post` | `EventModified` | Restore initiation from Glacier |
| `s3:ObjectRestore:Completed` | `EventModified` | Restore completion (object available) |
| `s3:ObjectRestore:Delete` | `EventDeleted` | Temporary restored copy expires |
| `s3:ObjectRemoved:Delete` | `EventDeleted` | Object deletion |
| `s3:ObjectRemoved:DeleteMarkerCreated` | `EventDeleted` | Versioned object deletion |
| `s3:ObjectRemoved:*` | `EventDeleted` | Wildcard deletion events |

### Semantic Accuracy Approach

The S3 watcher uses **operation-based mapping** to provide better semantic accuracy:

- **Direct uploads** (`Put`, `Post`) are mapped to `EventCreated` as they typically represent new file creation
- **Copy operations** are mapped to `EventModified` as they often represent overwrites or modifications of existing objects
- **Multipart uploads** are mapped to `EventModified` as large uploads often represent significant changes to existing content
- **Restore operations** are mapped to `EventModified` as they make previously archived objects available again

### Event Metadata

Each event includes rich metadata to provide additional context:

```go
event.Metadata = map[string]string{
    "bucketName":  "my-bucket",           // S3 bucket name
    "key":         "path/to/file.txt",    // Object key/path
    "eventName":   "s3:ObjectCreated:Copy", // Original S3 event name
    "region":      "us-east-1",           // AWS region
    "eventTime":   "2023-01-01T12:00:00.000Z", // Event timestamp
    "operation":   "copy",                // Operation type (put, post, copy, multipart, restore, delete)
    "versionId":   "abc123...",           // Object version ID (if versioning enabled)
    "isVersioned": "true",                // Whether bucket has versioning enabled
    "eTag":        "d41d8cd98f...",       // Object ETag
    "sequencer":   "0055AED6DCD90281E5",  // Event sequence number
    "size":        "1024",                // Object size in bytes
}
```

### Mapping Caveats and Limitations

#### **Version-Based Detection Limitations**
- **S3 version IDs are opaque strings**, not sequential numbers, making it impossible to determine if an event represents the "first version" of an object
- **Overwrite detection** relies on operation type rather than version analysis, as S3 doesn't provide explicit overwrite indicators like some other cloud storage services
- **Versioning must be enabled** on the bucket for version-related metadata to be available

#### **Operation-Based Mapping Trade-offs**
- **Copy operations** are mapped to `EventModified` because they often represent overwrites, but some copies may actually be to new object keys (true creations)
- **Multipart uploads** are mapped to `EventModified` as they often represent large file changes, but initial uploads of large files would technically be creations
- **Applications requiring precise create/modify distinction** should use the rich metadata to make their own semantic decisions

#### **Event Availability**
- **Only configured event types** will be received - ensure your S3 bucket notification configuration includes the events you need
- **SQS queue configuration** must match the S3 bucket's notification settings
- **Event ordering** is not guaranteed across different object keys, but the `sequencer` field can help order events for the same object

#### **Restore Event Behavior**
- **Restore operations** from Glacier storage classes generate multiple events (`Post`, `Completed`, `Delete`)
- **Temporary restored copies** have expiration times and generate `Delete` events when they expire
- **Applications should handle** the restore workflow appropriately based on their use case

### Best Practices

1. **Use rich metadata** for application-specific semantic decisions when operation-based mapping isn't sufficient
2. **Handle unknown event types** gracefully as AWS may introduce new S3 event types
3. **Consider event ordering** using the `sequencer` field when processing events for the same object
4. **Monitor SQS queue health** and configure appropriate dead letter queues for failed message processing
5. **Test with your specific S3 bucket configuration** as event behavior can vary based on versioning, lifecycle policies, and notification settings

## API

### S3Watcher

#### NewS3Watcher

```go
func NewS3Watcher(queueURL string) (*S3Watcher, error)
```

`NewS3Watcher` initializes an `S3Watcher` with the given SQS queue URL.

#### Start

```go
func (w *S3Watcher) Start(ctx context.Context, handler vfsevents.HandlerFunc, errHandler func(error)) error
```

`Start` begins watching the SQS queue for S3 events and triggers the handler on events. It also takes an error handler to handle any errors that occur during polling. The polling process can be stopped by calling `Stop`.

#### Stop

```go
func (w *S3Watcher) Stop() error
```

`Stop` stops the watcher.

### Options
#### WithSqsClient
```
func WithSqsClient(client *sqs.SQS) Option
```
`WithSqsClient` sets a custom SQS client for the watcher. This is useful for testing or if you want to use a specific SQS client configuration.

#### WithReceivedCount
```go
func WithReceivedCount(count uint) Option
```
`WithReceivedCount` sets the number of times a message should be received before it is deleted. This is used as a protective measure to prevent infinite retry loops for unprocessable messages.
