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
```

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