# gcsevents

`gcsevents` provides a `GCSWatcher` that implements the `vfsevents.Watcher` interface by using Pub/Sub events from a GCS bucket.

## Installation

To install the package, use the following command:

```bash
go get github.com/c2fo/vfs/contrib/vfsevents/watchers/gcsevents
```

## Event Mapping

The GCS watcher provides **semantic accuracy** in event mapping by using GCS-specific attributes to distinguish between different types of operations:

| GCS Event Type | VFS Event Type | Condition | Description |
|---|---|---|---|
| `OBJECT_FINALIZE` | `EventCreated` | No `overwroteGeneration` | New file creation |
| `OBJECT_FINALIZE` | `EventModified` | Has `overwroteGeneration` | File overwrite, copy, or restore |
| `OBJECT_METADATA_UPDATE` | `EventModified` | Always | Metadata changes |
| `OBJECT_DELETE` | `EventDeleted` | No `overwrittenByGeneration` | True file deletion |
| `OBJECT_DELETE` | *Suppressed* | Has `overwrittenByGeneration` | Part of overwrite (redundant) |
| `OBJECT_ARCHIVE` | `EventDeleted` | No `overwrittenByGeneration` | File archival |
| `OBJECT_ARCHIVE` | *Suppressed* | Has `overwrittenByGeneration` | Part of overwrite (redundant) |

### Overwrite Event Suppression

When a file is overwritten in GCS (e.g., `gsutil cp new.txt gs://bucket/existing.txt`), GCS publishes **two events**:

1. `OBJECT_FINALIZE` (with `overwroteGeneration`) → Maps to `EventModified` 
2. `OBJECT_DELETE` (with `overwrittenByGeneration`) → **Suppressed** 

**Why suppress the second event?**
- **Cleaner semantics**: One logical operation = one event
- **Reduced noise**: No confusing "deleted" events for overwrites  
- **Better UX**: Applications get the expected `EventModified` for overwrites
- **Maintains accuracy**: True deletions still generate `EventDeleted`

This ensures that overwrite operations are represented as single `EventModified` events rather than confusing `EventModified` + `EventDeleted` pairs.

### Event Metadata

Each event includes comprehensive metadata:

```go
event.Metadata = map[string]string{
    "bucketName":              "my-bucket",
    "object":                  "path/to/file.txt",
    "eventType":               "OBJECT_FINALIZE",
    "generation":              "1234567890",
    "overwroteGeneration":     "9876543210",  // Present for overwrites
    "overwrittenByGeneration": "1111111111",  // Present for deletes/archives
    "eventTime":               "2023-01-01T12:00:00Z",
}
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
