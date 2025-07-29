# gcsevents

`gcsevents` provides a `GCSWatcher` that implements the `vfsevents.Watcher` interface by using Pub/Sub events from a GCS bucket.

## Installation

To install the package, use the following command:

```bash
go get github.com/c2fo/vfs/contrib/vfsevents/watchers/gcsevents
```

## Usage
### Example
The following example demonstrates how to create a `GCSWatcher` that listens for changes in a GCS bucket and handles the events:

```go
package main

import (
    "context"
    "log"

    "github.com/c2fo/vfs/contrib/vfsevents"
    "github.com/c2fo/vfs/contrib/vfsevents/watchers/gcsevents"
)

func main() {
    // Initialize the GCSWatcher
    watcher, err := gcsevents.NewGCSWatcher("c2fo-application", "buyer-incoming-events-subscription")
    if err != nil {
        log.Fatal(err)
    }

    // Define the event handler
    handler := func(event vfsevents.Event) {
        log.Println("Event:", event)
    }

    // Define the error handler
    errHandler := func(err error) {
        log.Println("Error:", err)
    }

    // Start the watcher
    ctx := context.Background()
    err = watcher.Start(ctx, handler, errHandler)
    if err != nil {
        log.Fatal(err)
    }

    // Run the watcher for 2 minutes
    select {
    case <-time.After(2 * time.Minute):
        watcher.Stop()
    }
}
```

## API

### GCSWatcher

#### NewGCSWatcher
```go
func NewGCSWatcher(projectID, subscriptionID string, opts ...Option) (*GCSWatcher, error) {
```

`NewGCSWatcher` initializes a GCSWatcher with the given Pub/Sub projectID and subscriptionID. It accepts functional options to customize the watcher.

#### Start
```go
func (w *GCSWatcher) Start(ctx context.Context, handler vfsevents.EventHandler, errHandler vfsevents.ErrorHandler) error
```

`Start` begins watching the Pub/Sub subscription for GCS events and triggers the handler on events. It also takes an error handler to handle any errors that occur during polling. The polling process can be stopped by calling `Stop`.

#### Stop
```go
func (w *GCSWatcher) Stop() error
```
`Stop` stops the GCSWatcher from polling for events. It gracefully shuts down the watcher and cleans up any resources.

### Options

#### WithPubSubClient
```go
func WithPubSubClient(client *pubsub.Client) func(*GCSWatcher)
```

`WithPubSubClient` allows you to provide a custom Pub/Sub client to the `GCSWatcher`. This is useful if you want to use a pre-configured client or mock it for testing purposes.
