# GCS Events Watcher

This package provides a watcher for Google Cloud Storage (GCS) events using Pub/Sub notifications.

## Features

- **Semantic Event Mapping**: Distinguishes between true file creations and modifications using GCS event attributes
- **Overwrite Suppression**: Eliminates redundant events for file overwrites to provide clean, atomic operation semantics
- **Enhanced Metadata**: Rich context including generation numbers, event times, and GCS-specific attributes
- **Retry Logic**: Configurable retry behavior for handling transient errors

## Event Mapping

The watcher maps GCS Pub/Sub events to VFS events with semantic accuracy:

| GCS Event Type | VFS Event Type | Condition | Description |
|---|---|---|---|
| `OBJECT_FINALIZE` | `EventCreated` | No `overwroteGeneration` | New file creation |
| `OBJECT_FINALIZE` | `EventModified` | Has `overwroteGeneration` | File overwrite, copy, or restore |
| `OBJECT_METADATA_UPDATE` | `EventModified` | Always | Metadata changes |
| `OBJECT_DELETE` | `EventDeleted` | No `overwrittenByGeneration` | True file deletion |
| `OBJECT_DELETE` | *Suppressed* | Has `overwrittenByGeneration` | Part of overwrite (not emitted) |
| `OBJECT_ARCHIVE` | `EventDeleted` | No `overwrittenByGeneration` | File archival |
| `OBJECT_ARCHIVE` | *Suppressed* | Has `overwrittenByGeneration` | Part of overwrite (not emitted) |

### Overwrite Event Suppression

When you overwrite a file in GCS (e.g., `gsutil cp new.txt gs://bucket/existing.txt`), the watcher presents this as a single logical `EventModified` event, even though GCS internally publishes multiple events.

**What GCS publishes:**
1. `OBJECT_FINALIZE` (with `overwroteGeneration`) → Maps to `EventModified` ✅
2. `OBJECT_DELETE` (with `overwrittenByGeneration`) → **Suppressed** (not emitted) ❌

**Why suppress the second event?**
- **User-centric semantics**: You performed one action (overwrite), so you get one event
- **Atomic operations**: One logical file operation = one event
- **Intuitive behavior**: Overwrites appear as modifications (as expected)
- **Reduced complexity**: Applications don't need to correlate multiple events for single operations
- **Clean abstraction**: Hide cloud storage implementation details from file system users

**Technical Implementation:**
Events with `overwrittenByGeneration` are detected and suppressed before processing - they are never mapped or emitted. This ensures that overwrite operations are represented as single `EventModified` events rather than confusing `EventModified` + `EventDeleted` pairs.

### Event Metadata

Each event includes comprehensive metadata:

- **bucketName**: GCS bucket name
- **object**: Object name/path
- **eventType**: Original GCS event type
- **generation**: Object generation number
- **overwroteGeneration**: Present for overwrites (indicates which generation was replaced)
- **eventTime**: When the event occurred (from the Pub/Sub message attributes, if present)

Delete/archive events that carry `overwrittenByGeneration` are suppressed, so emitted events never include that key.

## Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/c2fo/vfs/contrib/vfsevents"
    "github.com/c2fo/vfs/contrib/vfsevents/watchers/gcsevents"
)

func main() {
    // Create watcher
    watcher, err := gcsevents.NewGCSWatcher("my-project", "my-subscription")
    if err != nil {
        log.Fatal(err)
    }

    handler := func(event vfsevents.Event) {
        fmt.Printf("Event: %s on %s\n", event.Type, event.URI)
    }

    errHandler := func(err error) {
        log.Printf("GCS watcher error: %v", err)
    }

    // Start watching with retry logic
    ctx := context.Background()
    err = watcher.Start(ctx, handler, errHandler,
        vfsevents.WithRetryConfig(vfsevents.DefaultRetryConfig()),
    )
    if err != nil {
        log.Fatal(err)
    }

    defer watcher.Stop()
    select {}
}
```

## Configuration

### Watcher Options

```go
func WithPubSubClient(client PubSubClient) Option
```

`WithPubSubClient` sets a custom Pub/Sub client. Any type implementing the `PubSubClient` interface (`Receive(ctx, func(context.Context, *pubsub.Message)) error`) is accepted, such as a `*pubsub.Subscriber` from `cloud.google.com/go/pubsub/v2`. This is useful for testing or custom client configuration. When omitted, `NewGCSWatcher` creates a client for `projects/{project}/subscriptions/{subscription}`.

### Start and Stop Options

`Start` accepts `vfsevents.StartOption` values for retry configuration (`WithRetryConfig`, `WithMaxRetries`, `WithRetryBackoff`, `WithRetryableErrors`), event filtering (`WithEventFilter`), and status callbacks (`WithStatusCallback`). `Stop` accepts `vfsevents.StopOption` values (`WithTimeout`, default 30 seconds, and `WithForceStop`).

See the package documentation for detailed configuration options.
