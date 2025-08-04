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
- **overwrittenByGeneration**: Present for suppressed delete/archive events
- **eventTime**: When the event occurred

## Usage

```go
import "github.com/c2fo/vfs/contrib/vfsevents/watchers/gcsevents"

// Create watcher
watcher, err := gcsevents.NewGCSWatcher("my-project", "my-subscription")
if err != nil {
    log.Fatal(err)
}

// Start watching
err = watcher.Start(ctx, func(event vfsevents.Event) error {
    fmt.Printf("Event: %s on %s\n", event.Type, event.Path)
    return nil
})
```

## Configuration

The watcher supports various configuration options through functional options:

- Retry configuration for handling transient errors
- Custom Pub/Sub client settings
- Event filtering and processing options

See the package documentation for detailed configuration options.
