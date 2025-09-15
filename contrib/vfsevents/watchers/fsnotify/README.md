# FSNotify Watcher

The FSNotify watcher provides real-time filesystem event monitoring for local directories using the [fsnotify](https://github.com/fsnotify/fsnotify) library. This watcher is ideal for monitoring local filesystem changes with minimal latency and resource usage.

## Features

- **Real-time Events**: Immediate notification of filesystem changes (no polling delay)
- **Cross-platform**: Works on Linux, macOS, Windows, and other platforms supported by fsnotify
- **Recursive Watching**: Optional recursive monitoring of subdirectories
- **Event Filtering**: Filter events by file type, path patterns, or custom logic
- **Event Debouncing**: Configurable time-based debouncing to consolidate multiple related events
- **Low Resource Usage**: Efficient kernel-level filesystem monitoring
- **VFS Integration**: Seamless integration with the VFS library for file operations

## Supported Event Types

- **Created**: New files or directories are created
- **Modified**: Existing files are modified (write operations, chmod)
- **Deleted**: Files or directories are removed or renamed

## Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/c2fo/vfs/contrib/vfsevents"
    "github.com/c2fo/vfs/contrib/vfsevents/watchers/fsnotify"
    "github.com/c2fo/vfs/v7/vfssimple"
)

func main() {
    // Create a VFS location for local filesystem
    location, err := vfssimple.NewLocation("file:///path/to/watch")
    if err != nil {
        log.Fatal(err)
    }

    // Create FSNotify watcher
    watcher, err := fsnotify.NewFSNotifyWatcher(location)
    if err != nil {
        log.Fatal(err)
    }

    // Define event handler
    eventHandler := func(event vfsevents.Event) {
        fmt.Printf("Event: %s on %s\n",
            event.Type.String(),
            event.URI)
    }

    // Define error handler
    errorHandler := func(err error) {
        log.Printf("FSNotify error: %v", err)
    }

    // Start watching
    ctx := context.Background()
    if err := watcher.Start(ctx, eventHandler, errorHandler); err != nil {
        log.Fatal(err)
    }

    // Stop watching when done
    defer watcher.Stop()

    // Keep the program running
    select {}
}
```

## Debouncing Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/c2fo/vfs/contrib/vfsevents"
    "github.com/c2fo/vfs/contrib/vfsevents/watchers/fsnotify"
    "github.com/c2fo/vfs/v7/vfssimple"
)

func main() {
    location, err := vfssimple.NewLocation("file:///path/to/watch")
    if err != nil {
        log.Fatal(err)
    }

    // Create FSNotify watcher with 500ms debouncing
    watcher, err := fsnotify.NewFSNotifyWatcher(location,
        fsnotify.WithDebounce(500*time.Millisecond),
        fsnotify.WithRecursive(true))
    if err != nil {
        log.Fatal(err)
    }

    eventHandler := func(event vfsevents.Event) {
        fmt.Printf("Debounced Event: %s on %s\n",
            event.Type.String(),
            event.URI)

        // Check if this was a consolidated event
        if op, exists := event.Metadata["fsnotify_op"]; exists && op == "multiple" {
            fmt.Printf("  -> Consolidated multiple operations\n")
        }
    }

    errorHandler := func(err error) {
        log.Printf("FSNotify error: %v", err)
    }

    ctx := context.Background()
    if err := watcher.Start(ctx, eventHandler, errorHandler); err != nil {
        log.Fatal(err)
    }

    defer watcher.Stop()
    select {}
}
```

## Configuration Options

### WithRecursive(bool)
Enables or disables recursive watching of subdirectories.
- **Default**: false (only watch the specified directory)
- **Recursive**: true (watch all subdirectories and newly created ones)

```go
// Watch directory and all subdirectories
watcher, err := fsnotify.NewFSNotifyWatcher(location,
    fsnotify.WithRecursive(true))
```

### WithDebounce(time.Duration)
Enables or disables event debouncing with the specified time interval.
- **Default**: 0 (no debouncing)
- **Debounce**: time.Duration (debounce events within the specified time interval)

```go
// Watch directory with 500ms debouncing
watcher, err := fsnotify.NewFSNotifyWatcher(location,
    fsnotify.WithDebounce(500*time.Millisecond))
```

## Advanced Usage

### Event Filtering

```go
// Start with event filtering - only process .txt and .log files
err := watcher.Start(ctx, eventHandler, errorHandler,
    vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
        return strings.HasSuffix(e.URI, ".txt") ||
               strings.HasSuffix(e.URI, ".log")
    }),
)
```

### Event Debouncing

Event debouncing consolidates multiple related filesystem events into single logical events, reducing noise and improving performance. This is particularly useful for:

- **Build tools**: Prevent excessive rebuilds during rapid file changes
- **Network filesystems**: Handle delayed writes on SFTP/NFS mounts
- **Hot reload systems**: Reduce handler spam during bulk operations
- **Text editors**: Consolidate multiple save operations

#### Basic Debouncing

```go
// Enable 200ms debouncing - events within 200ms are consolidated
watcher, err := fsnotify.NewFSNotifyWatcher(location,
    fsnotify.WithDebounce(200*time.Millisecond))
```

#### Network Filesystem Debouncing

```go
// For SFTP/NFS with delayed writes, use longer debounce periods
watcher, err := fsnotify.NewFSNotifyWatcher(location,
    fsnotify.WithDebounce(2*time.Second))
```

#### Event Consolidation Rules

When debouncing is enabled, events are consolidated using these rules:

1. **Delete events take priority** over Create/Modified events
2. **Create events take priority** over Modified events
3. **Multiple events** for the same file are merged into a single event
4. **Event metadata** includes `"fsnotify_op": "multiple"` for consolidated events
5. **Timestamp** reflects the first event time in the sequence

#### Performance Impact

- **Without debouncing**: File operations may generate 2-5 events each
- **With debouncing**: Multiple operations consolidated into 1 event
- **Typical reduction**: 50-80% fewer events processed
- **Latency**: Configurable based on debounce duration

### Debouncing Performance Impact

- **Event Reduction**: 50-80% fewer events for typical file operations
- **Memory Overhead**: ~200 bytes per pending file during debounce period
- **Timer Overhead**: One timer per file with pending events
- **Cleanup**: Automatic cleanup prevents memory leaks

### Resource Usage

- **Memory**: ~1-2MB base usage + ~100 bytes per watched directory + ~200 bytes per pending debounced file
- **CPU**: Minimal (event-driven, no polling)
- **File Descriptors**: 1 per watched directory (Linux/macOS)
- **Kernel Resources**: Uses inotify (Linux), kqueue (macOS), ReadDirectoryChangesW (Windows)

### Status Monitoring

```go
// Monitor watcher status and performance
err := watcher.Start(ctx, eventHandler, errorHandler,
    vfsevents.WithStatusCallback(func(status vfsevents.WatcherStatus) {
        fmt.Printf("Events processed: %d, Running: %t\n",
            status.EventsProcessed, status.Running)
    }),
)
```

### Graceful Shutdown

```go
// Stop with timeout
err := watcher.Stop(vfsevents.WithTimeout(30 * time.Second))
if err != nil {
    log.Printf("Timeout stopping watcher: %v", err)
}
```

## Event Metadata

FSNotify events include additional metadata:

```go
eventHandler := func(event vfsevents.Event) {
    // Access FSNotify-specific metadata
    if path, exists := event.Metadata["path"]; exists {
        fmt.Printf("Local path: %s\n", path)
    }
    if op, exists := event.Metadata["fsnotify_op"]; exists {
        fmt.Printf("FSNotify operation: %s\n", op)
    }
}
```

Available metadata fields:
- `path`: Local filesystem path
- `fsnotify_op`: Raw fsnotify operation (CREATE, WRITE, REMOVE, RENAME, CHMOD) or "multiple" for debounced events

## Performance Characteristics

### Advantages over VFSPoller

| Feature | FSNotify | FSNotify + Debouncing | VFSPoller |
|---------|----------|----------------------|-----------|
| **Latency** | Immediate (< 1ms) | Debounce duration | Polling interval (30s-5m) |
| **CPU Usage** | Very low | Very low | Moderate (periodic scans) |
| **Memory Usage** | Low | Low + pending events | Higher (file cache) |
| **Event Volume** | High (all events) | Reduced (50-80% fewer) | Low (polling) |
| **Scalability** | Excellent | Excellent | Limited by directory size |
| **Missed Events** | None | None | Possible during rapid changes |

## Platform-Specific Behavior

### Linux (inotify)
- Excellent performance and reliability
- Supports all event types
- Recursive watching requires manual directory traversal

### macOS (kqueue)
- Good performance
- Supports all event types
- Recursive watching requires manual directory traversal

### Windows (ReadDirectoryChangesW)
- Good performance
- Supports all event types
- Native recursive watching support

## Limitations

1. **Local Filesystem Only**: Only works with `file://` URIs (local paths)
2. **Platform Limits**: Subject to OS limits on watched directories
3. **Symlink Behavior**: Follows symlinks but may not detect changes to symlink targets
4. **Network Filesystems**: May not work reliably with NFS, SMB, or other network mounts
5. **Rapid Changes**: Very rapid file operations might be coalesced into fewer events

## Best Practices

1. **Use for Local Development**: Ideal for local development environments and tools
2. **Filter Events**: Use event filtering to reduce noise from temporary files
3. **Configure Debouncing**: Use appropriate debounce durations for your use case
4. **Handle Errors**: Implement robust error handling for filesystem issues
5. **Graceful Shutdown**: Always stop watchers cleanly to release resources
6. **Test Platform Behavior**: Test on your target platforms for specific behavior

## Troubleshooting

### High CPU Usage
- Check for recursive watching of large directory trees
- Implement event filtering to reduce processing
- Consider enabling debouncing to reduce event volume
- Verify no infinite loops in event handlers

### Too Many Events
- Enable debouncing with appropriate duration (100ms-2s)
- Use event filtering to exclude unwanted files
- Avoid watching directories with frequent temporary file creation

### Missing Events
- Ensure the directory exists and is accessible
- Check filesystem type (some network filesystems don't support fsnotify)
- Verify sufficient file descriptor limits
- Disable debouncing if immediate events are critical

### Delayed Events
- Check debounce duration - may be too long for your use case
- Network filesystems may have inherent delays
- Consider shorter debounce periods for local development

### Memory Usage
- Limit recursive watching depth for large directory structures
- Use event filtering to reduce memory overhead
- Monitor debounce map size during high-activity periods
- Ensure proper watcher shutdown to clean up pending events

## Comparison with Other Watchers

### When to Use FSNotify
- **Local development environments**
- **Desktop applications**
- **Build tools and file processors**
- **Local file synchronization**
- **Configuration file monitoring**

### When to Use Other Watchers
- **VFSPoller**: For cloud storage, network filesystems, or cross-platform compatibility
- **S3Events**: For Amazon S3 bucket monitoring
- **GCSEvents**: For Google Cloud Storage bucket monitoring

## API Reference

### NewFSNotifyWatcher

```go
func NewFSNotifyWatcher(location vfs.Location, opts ...Option) (*FSNotifyWatcher, error)
```

Creates a new FSNotify watcher for the given VFS location. The location must use the `file://` scheme.

**Parameters:**
- `location`: VFS location with `file://` scheme
- `opts`: Optional configuration options

**Returns:**
- `*FSNotifyWatcher`: The watcher instance
- `error`: Error if creation fails

### Start

```go
func (w *FSNotifyWatcher) Start(
    ctx context.Context,
    handler vfsevents.HandlerFunc,
    errHandler vfsevents.ErrorHandlerFunc,
    opts ...vfsevents.StartOption) error
```

Starts watching the filesystem location for changes.

### Stop

```go
func (w *FSNotifyWatcher) Stop(opts ...vfsevents.StopOption) error
```

Stops the filesystem watcher and releases resources.

### Options

#### WithRecursive

```go
func WithRecursive(recursive bool) Option
```

Enables or disables recursive watching of subdirectories.

#### WithDebounce

```go
func WithDebounce(debounce time.Duration) Option
```

Enables or disables event debouncing with the specified time interval.
