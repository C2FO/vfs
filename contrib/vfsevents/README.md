# vfsevents

`vfsevents` is a Go library that provides a unified interface for watching filesystem events across different backends. The primary goal of this project is to offer a consistent API for monitoring file changes, regardless of the underlying storage system.

## Why vfsevents?

Monitoring file changes is a common requirement in many applications, such as synchronizing files, triggering workflows, or auditing changes. However, different storage systems (e.g., local filesystems, cloud storage) have different mechanisms for detecting changes. `vfsevents` aims to abstract these differences and provide a single, easy-to-use interface for developers.

## Installation

```bash
go get github.com/c2fo/vfs/contrib/vfsevents
```

## Interface

The core of the `vfsevents` library is the `Watcher` interface, which defines methods for starting and stopping the monitoring of filesystem events with support for advanced configuration via functional options.

```go
package vfsevents

import "context"

// EventType represents the type of filesystem event.
type EventType int

const (
	EventCreated EventType = iota
	EventDeleted
	EventModified
	EventRenamed
	EventUnknown
)

// String returns the string representation of the EventType.
func (e EventType) String() string {
	switch e {
	case EventCreated:
		return "Created"
	case EventDeleted:
		return "Deleted"
	case EventModified:
		return "Modified"
	case EventRenamed:
		return "Renamed"
	case EventUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// Event represents a filesystem event.
type Event struct {
	URI       string            // The URI to the file
	Type      EventType         // The type of event (Created, Deleted, etc.)
	Metadata  map[string]string // Optional metadata (e.g., object version, size)
	Timestamp int64             // Unix timestamp of the event
}

// HandlerFunc is the function signature for handling events.
type HandlerFunc func(event Event)

// ErrorHandlerFunc is the function signature for handling errors.
// Errors can occur for many reasons once `Start` begins listening for events.
// For instance, network issues, permission errors, or invalid configurations can trigger errors.
type ErrorHandlerFunc func(error)

// Watcher is an interface for watching filesystem events.
type Watcher interface {
	// Start begins watching the specified path and triggers handler on events.
	// It also takes an error handler to handle any errors that occur during polling.
	// Additional options can be provided for advanced configuration.
	Start(ctx context.Context, handler HandlerFunc, errHandler ErrorHandlerFunc, opts ...StartOption) error

	// Stop stops the watcher with optional configuration.
	Stop(opts ...StopOption) error
}
```

## Advanced Configuration

The library supports advanced configuration through functional options:

### Start Options

```go
// Event filtering - only process events that match criteria
vfsevents.WithEventFilter(func(e Event) bool {
    return e.Type == EventCreated && strings.HasSuffix(e.URI, ".txt")
})

// Status callbacks - receive watcher status updates
vfsevents.WithStatusCallback(func(status WatcherStatus) {
    log.Printf("Events processed: %d, Running: %v", status.EventsProcessed, status.Running)
})

// Retry configuration - handle transient failures gracefully
vfsevents.WithRetryConfig(vfsevents.DefaultRetryConfig())
vfsevents.WithMaxRetries(5)
vfsevents.WithRetryBackoff(2*time.Second, 60*time.Second)

// Custom retryable error patterns
vfsevents.WithRetryableErrors([]string{"custom error pattern"})
```

### Stop Options

```go
// Graceful shutdown with timeout
vfsevents.WithTimeout(30 * time.Second)

// Force immediate shutdown
vfsevents.WithForceStop()
```

## Available Watchers

The `vfsevents` library provides four production-ready watcher implementations, each optimized for different use cases:

### FSNotify Watcher
**Real-time local filesystem monitoring using kernel-level events**
- **Best for**: Local development, desktop applications, configuration monitoring
- **Latency**: Immediate (< 1ms)
- **Platforms**: Linux (inotify), macOS (kqueue), Windows (ReadDirectoryChangesW)
- **Features**: Recursive watching, very low resource usage, no polling overhead

### VFS Poller
**Universal polling-based monitoring for any VFS backend**
- **Best for**: Cloud storage, network filesystems, cross-platform compatibility
- **Latency**: Configurable polling interval (30s - 5m)
- **Platforms**: Any VFS-supported backend (S3, GCS, SFTP, FTP, etc.)
- **Features**: Retry logic, memory management, modification detection

### S3Events Watcher
**Real-time Amazon S3 bucket monitoring via SQS**
- **Best for**: Production S3 workflows, serverless architectures
- **Latency**: Near real-time (seconds)
- **Platforms**: Amazon S3 with SQS integration
- **Features**: Native S3 event notifications, automatic retry, message filtering

### GCSEvents Watcher
**Real-time Google Cloud Storage monitoring via Pub/Sub**
- **Best for**: Production GCS workflows, cloud-native applications
- **Latency**: Near real-time (seconds)
- **Platforms**: Google Cloud Storage with Pub/Sub integration
- **Features**: Native GCS event notifications, automatic retry, subscription management

## Watcher Comparison

| Feature | FSNotify | VFS Poller | S3Events | GCSEvents |
|---------|----------|------------|----------|-----------|
| **Latency** | < 1ms | 30s - 5m | ~1-5s | ~1-5s |
| **Resource Usage** | Very Low | Moderate | Low | Low |
| **Scalability** | Excellent | Limited | Excellent | Excellent |
| **Local Files** | ✅ | ✅ | ❌ | ❌ |
| **Cloud Storage** | ❌ | ✅ | ✅ (S3) | ✅ (GCS) |
| **Network FS** | ❌ | ✅ | ❌ | ❌ |
| **Recursive** | ✅ | ❌ | ✅ | ✅ |
| **Event Filtering** | ✅ | ✅ | ✅ | ✅ |
| **Retry Logic** | N/A | ✅ | ✅ | ✅ |
| **Cross-Platform** | ✅ | ✅ | ✅ | ✅ |

## Usage Examples

### FSNotify Watcher (Real-time Local Filesystem)

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/c2fo/vfs/contrib/vfsevents"
    "github.com/c2fo/vfs/contrib/vfsevents/watchers/fsnotify"
    "github.com/c2fo/vfs/v7/vfssimple"
)

func main() {
    // Create VFS location for local directory
    location, err := vfssimple.NewLocation("file:///path/to/watch/")
    if err != nil {
        log.Fatal(err)
    }

    // Create FSNotify watcher with recursive watching
    watcher, err := fsnotify.NewFSNotifyWatcher(location, 
        fsnotify.WithRecursive(true))
    if err != nil {
        log.Fatal(err)
    }

    // Define event handler
    eventHandler := func(event vfsevents.Event) {
        fmt.Printf("FSNotify Event: %s on %s\n", 
            event.Type.String(), event.URI)
        
        // Access FSNotify-specific metadata
        if path, exists := event.Metadata["path"]; exists {
            fmt.Printf("Local path: %s\n", path)
        }
    }

    // Define error handler
    errorHandler := func(err error) {
        log.Printf("FSNotify error: %v", err)
    }

    // Start with event filtering
    ctx := context.Background()
    err = watcher.Start(ctx, eventHandler, errorHandler,
        vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
            // Only process .txt and .log files
            return strings.HasSuffix(e.URI, ".txt") || 
                   strings.HasSuffix(e.URI, ".log")
        }),
        vfsevents.WithStatusCallback(func(status vfsevents.WatcherStatus) {
            fmt.Printf("Events processed: %d\n", status.EventsProcessed)
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Graceful shutdown
    defer watcher.Stop(vfsevents.WithTimeout(10 * time.Second))

    // Keep running
    select {}
}
```

### VFS Poller (Universal Backend Support)

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/c2fo/vfs/contrib/vfsevents"
    "github.com/c2fo/vfs/contrib/vfsevents/watchers/vfspoller"
    "github.com/c2fo/vfs/v7/vfssimple"
)

func main() {
    // Create VFS location (works with any backend)
    location, err := vfssimple.NewLocation("s3://my-bucket/path/")
    if err != nil {
        log.Fatal(err)
    }

    // Create poller with configuration
    poller, err := vfspoller.NewPoller(location,
        vfspoller.WithInterval(30*time.Second),    // Poll every 30 seconds
        vfspoller.WithMinAge(5*time.Second),       // Ignore files newer than 5 seconds
        vfspoller.WithMaxFiles(5000),              // Limit cache to 5000 files
        vfspoller.WithCleanupAge(24*time.Hour),    // Clean up old entries after 24 hours
    )
    if err != nil {
        log.Fatal(err)
    }

    // Define handlers
    eventHandler := func(event vfsevents.Event) {
        fmt.Printf("VFS Event: %s on %s at %s\n", 
            event.Type.String(), 
            event.URI, 
            time.Unix(event.Timestamp, 0).Format("2006-01-02 15:04:05"))
    }

    errorHandler := func(err error) {
        log.Printf("Poller error: %v", err)
    }

    // Start with retry configuration
    ctx := context.Background()
    err = poller.Start(ctx, eventHandler, errorHandler,
        vfsevents.WithRetryConfig(vfsevents.RetryConfig{
            Enabled:         true,
            MaxRetries:      3,
            InitialBackoff:  1 * time.Second,
            MaxBackoff:      30 * time.Second,
            BackoffFactor:   2.0,
            RetryableErrors: []string{"timeout", "connection"},
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    defer poller.Stop()
    select {}
}
```

### S3Events Watcher (Real-time S3 Monitoring)

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/c2fo/vfs/contrib/vfsevents"
    "github.com/c2fo/vfs/contrib/vfsevents/watchers/s3events"
)

func main() {
    // Create S3 watcher
    watcher, err := s3events.NewS3Watcher("https://sqs.region.amazonaws.com/account/queue")
    if err != nil {
        log.Fatal(err)
    }

    // Define handlers
    eventHandler := func(event vfsevents.Event) {
        fmt.Printf("S3 Event: %s on %s\n", event.Type.String(), event.URI)
        
        // Access S3-specific metadata
        if bucketName, exists := event.Metadata["bucketName"]; exists {
            fmt.Printf("Bucket: %s\n", bucketName)
        }
        if eventType, exists := event.Metadata["eventType"]; exists {
            fmt.Printf("S3 Event Type: %s\n", eventType)
        }
    }

    errorHandler := func(err error) {
        log.Printf("S3 watcher error: %v", err)
    }

    // Start watching
    ctx := context.Background()
    err = watcher.Start(ctx, eventHandler, errorHandler)
    if err != nil {
        log.Fatal(err)
    }

    defer watcher.Stop()
    select {}
}
```

### GCSEvents Watcher (Real-time GCS Monitoring)

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
    // Create GCS watcher
    watcher, err := gcsevents.NewGCSWatcher("my-project", "my-subscription")
    if err != nil {
        log.Fatal(err)
    }

    // Define handlers
    eventHandler := func(event vfsevents.Event) {
        fmt.Printf("GCS Event: %s on %s\n", event.Type.String(), event.URI)
        
        // Access GCS-specific metadata
        if objectName, exists := event.Metadata["objectName"]; exists {
            fmt.Printf("Object: %s\n", objectName)
        }
    }

    errorHandler := func(err error) {
        log.Printf("GCS watcher error: %v", err)
    }

    // Start watching with retry logic
    ctx := context.Background()
    err = watcher.Start(ctx, eventHandler, errorHandler,
        vfsevents.WithRetryConfig(vfsevents.DefaultRetryConfig()),
    )
    if err != nil {
        log.Fatal(err)
    }

    defer watcher.Stop()
    select {}
}
```

## Examples

See the [examples](./examples/) directory for comprehensive usage examples including:
- Basic file monitoring with VFS Poller
- Real-time S3 event processing
- GCS event handling with file operations
- Production-ready file processing pipelines
- FSNotify watcher usage examples

## Contributing

Contributions are welcome! Please see the main VFS repository for contribution guidelines.