# vfspoller

`vfspoller` provides a `Poller` that implements the `vfsevents.Watcher` interface by polling a `vfs.Location`.

## Features

- **Universal Compatibility**: Works with any VFS backend (local, S3, GCS, SFTP, FTP, etc.)
- **File Change Detection**: Detects created, modified, and deleted files
- **Performance Optimized**: Caches file metadata to minimize expensive `LastModified()` calls
- **Memory Management**: Automatic cleanup and configurable limits to prevent memory leaks
- **Retry Logic**: Automatic retry for transient List() failures with exponential backoff
- **Configurable**: Multiple options for polling interval, file age filtering, cache management, and retry behavior

## Performance Characteristics

- **Suitable for**: Small to medium directories (< 1000 files)
- **Memory usage**: Grows with number of files but includes automatic cleanup
- **CPU usage**: Moderate - performs periodic directory listings and metadata checks
- **Network usage**: Depends on VFS backend - may perform network calls for cloud storage

**For large directories or high-frequency changes, consider using cloud-native event watchers (S3Events, GCSEvents) instead.**

## Usage

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/c2fo/vfs/contrib/vfsevents"
    "github.com/c2fo/vfs/contrib/vfsevents/watchers/vfspoller"
    "github.com/c2fo/vfs/v7/vfssimple"
)

func main() {
    // Create a VFS location
    location, err := vfssimple.NewLocation("file:///tmp/watch")
    if err != nil {
        log.Fatal(err)
    }

    // Create poller with options
    poller, err := vfspoller.NewPoller(location,
        vfspoller.WithInterval(30*time.Second),    // Poll every 30 seconds
        vfspoller.WithMinAge(5*time.Second),       // Ignore files newer than 5 seconds
        vfspoller.WithMaxFiles(5000),              // Limit cache to 5000 files
        vfspoller.WithCleanupAge(24*time.Hour),    // Clean up old entries after 24 hours
    )
    if err != nil {
        log.Fatal(err)
    }

    // Define event handler
    eventHandler := func(event vfsevents.Event) {
        log.Printf("File %s: %s at %s",
            event.Type.String(),
            event.URI,
            time.Unix(event.Timestamp, 0).Format("2006-01-02 15:04:05"))
    }

    // Define error handler
    errorHandler := func(err error) {
        log.Printf("Poller error: %v", err)
    }

    // Start watching
    ctx := context.Background()
    if err := poller.Start(ctx, eventHandler, errorHandler); err != nil {
        log.Fatal(err)
    }

    // Stop watching when done
    defer poller.Stop()

    // Keep the program running
    select {}
}
```

## Configuration Options

### WithInterval(duration)
Sets the polling interval (how often to check for changes).
- **Default**: 1 minute
- **Recommended**: 30 seconds to 5 minutes depending on requirements
- **Trade-off**: Lower intervals = faster detection but higher resource usage

### WithMinAge(duration)
Minimum age of files to consider for events. Useful for ignoring files that are still being written.
- **Default**: 0 (no minimum age)
- **Recommended**: 5-30 seconds for active directories
- **Use case**: Prevents processing of incomplete file uploads

### WithMaxFiles(count)
Maximum number of files to track in memory cache.
- **Default**: 10,000 files
- **Recommended**: Adjust based on directory size and available memory
- **Behavior**: When exceeded, oldest files are removed from cache

### WithCleanupAge(duration)
Frequency for cleaning up deleted files from cache to prevent memory leaks.
- **Default**: 24 hours
- **Recommended**: 12-48 hours depending on file turnover rate
- **Trade-off**: Shorter intervals = more frequent cleanup but slightly higher CPU usage

## Retry Configuration

VFSPoller includes automatic retry logic for handling transient failures during directory listing operations. This is particularly valuable when using cloud storage backends (S3, GCS) or network filesystems (SFTP, NFS) that may experience temporary connectivity issues.

### Enabling Retry Logic

```go
// Start with retry configuration
err := poller.Start(ctx, eventHandler, errorHandler,
    vfsevents.WithRetryConfig(vfsevents.RetryConfig{
        Enabled:         true,
        MaxRetries:      3,                    // Maximum retry attempts
        InitialBackoff:  1 * time.Second,     // Initial backoff delay
        MaxBackoff:      30 * time.Second,    // Maximum backoff delay
        BackoffFactor:   2.0,                 // Exponential backoff multiplier
        RetryableErrors: []string{            // Custom retryable error patterns
            "timeout",
            "connection",
            "SlowDown",
            "ServiceUnavailable",
        },
    }),
)
```

### Retry Configuration Options

- **Enabled**: Enable/disable retry logic (default: false for backward compatibility)
- **MaxRetries**: Maximum number of retry attempts (default: 3)
- **InitialBackoff**: Starting delay between retries (default: 1 second)
- **MaxBackoff**: Maximum delay between retries (default: 30 seconds)
- **BackoffFactor**: Exponential backoff multiplier (default: 2.0)
- **RetryableErrors**: Custom error patterns to consider retryable

### Default Retryable Errors

The following error patterns are automatically considered retryable:
- Network timeouts (`timeout`, `deadline exceeded`)
- Connection issues (`connection refused`, `connection reset`)
- AWS S3 throttling (`SlowDown`, `RequestTimeout`)
- GCS temporary errors (`Unavailable`, `ResourceExhausted`)
- HTTP 5xx server errors (`Internal Server Error`, `Service Unavailable`)

### Retry Benefits

**For Cloud Storage (S3, GCS):**
- Handles rate limiting and throttling gracefully
- Recovers from temporary service outages
- Manages network connectivity issues

**For Network Filesystems (SFTP, NFS):**
- Recovers from network partitions
- Handles temporary authentication issues
- Manages server overload conditions

**Operational Benefits:**
- Reduces false alerts from transient issues
- Improves monitoring reliability
- Decreases manual intervention requirements

### Monitoring Retry Status

```go
// Monitor retry status via callback
err := poller.Start(ctx, eventHandler, errorHandler,
    vfsevents.WithRetryConfig(retryConfig),
    vfsevents.WithStatusCallback(func(status vfsevents.WatcherStatus) {
        if status.RetryAttempts > 0 {
            log.Printf("Retry attempt %d after error: %v",
                status.RetryAttempts, status.LastError)
        }
    }),
)
```

## Event Types

The VFS Poller detects three types of file events:

1. **Created**: New files appear in the directory
2. **Modified**: Existing files change (detected via LastModified time or file size)
3. **Deleted**: Files are removed from the directory

## Performance Tuning

### For Small Directories (< 100 files)
```go
poller, err := vfspoller.NewPoller(location,
    vfspoller.WithInterval(10*time.Second),  // Fast polling
    vfspoller.WithMaxFiles(1000),            // Small cache
)
```

### For Medium Directories (100-1000 files)
```go
poller, err := vfspoller.NewPoller(location,
    vfspoller.WithInterval(30*time.Second),  // Moderate polling
    vfspoller.WithMaxFiles(5000),            // Medium cache
    vfspoller.WithCleanupAge(12*time.Hour),  // Regular cleanup
)
```

### For Large Directories (> 1000 files)
**Consider using cloud-native event watchers instead:**
- S3Events for Amazon S3
- GCSEvents for Google Cloud Storage
- Or implement custom event-driven solutions

If you must use VFSPoller for large directories:
```go
poller, err := vfspoller.NewPoller(location,
    vfspoller.WithInterval(2*time.Minute),   // Slower polling
    vfspoller.WithMaxFiles(20000),           // Large cache
    vfspoller.WithCleanupAge(6*time.Hour),   // Frequent cleanup
)
```

## Limitations

1. **Polling Delay**: Changes are detected with a delay equal to the polling interval
2. **Resource Usage**: Performs directory listings and metadata checks on each poll
3. **Scalability**: Not suitable for directories with thousands of files
4. **Rapid Changes**: May miss very rapid file creation/deletion cycles
5. **Network Overhead**: For cloud storage, each poll involves network requests

## Best Practices

1. **Choose appropriate polling interval**: Balance between detection speed and resource usage
2. **Use MinAge for active directories**: Prevents processing incomplete files
3. **Monitor memory usage**: Adjust MaxFiles based on your directory size
4. **Consider alternatives**: Use cloud-native event watchers for better performance
5. **Handle errors gracefully**: Implement robust error handling for network issues
6. **Test with realistic data**: Validate performance with actual file counts and sizes

## Troubleshooting

### High Memory Usage
- Reduce `MaxFiles` limit
- Decrease `CleanupAge` duration
- Consider switching to event-driven watchers

### Slow Performance
- Increase polling `Interval`
- Reduce directory size
- Use cloud-native event watchers for better performance

### Missing Events
- Decrease polling `Interval`
- Check `MinAge` setting
- Verify file system supports modification time detection

## API

### Poller

#### NewPoller

```go
func NewPoller(location vfs.Location, opts ...Option) (*Poller, error)
```

`NewPoller` initializes a `Poller` with a `vfs.Location`.

#### Start

```go
func (s *Poller) Start(ctx context.Context, handler vfsevents.HandlerFunc, errHandler vfsevents.ErrorHandlerFunc) error
```

`Start` begins polling the location for file changes and triggers the handler on events.. It also takes an error handler to handle any errors that occur during polling.  The polling process can be stopped by calling `Stop`.

#### Stop

```go
func (s *Poller) Stop() error
```

`Stop` stops the polling process.

### Options

#### WithInterval

```go
func WithInterval(interval time.Duration) Option
```

`WithInterval` sets the polling interval. Default is 1 minute.

#### WithMinAge

```go
func WithMinAge(age time.Duration) Option
```

`WithMinAge` sets the minimum age of files to consider. This is useful for ignoring files that are still being written on some filesystems like OS, SFTP, and FTP.
