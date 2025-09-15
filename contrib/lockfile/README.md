# VFS Lockfile Utility

A portable advisory locking mechanism for VFS files that works across different backends (local filesystem, S3, GCS, SFTP, etc.).

## Overview

This package provides a simple yet powerful way to coordinate access to files across multiple processes or machines. It implements advisory locking using companion `.lock` files, which is a common pattern in distributed systems where mandatory locking is unavailable or unreliable.

### Key Features

- **Portable**: Works with any VFS backend that supports basic file operations
- **Advisory Locking**: Uses sidecar `.lock` files with atomic creation
- **Metadata Support**: Includes timestamp, PID, hostname, and optional TTL
- **Stale Lock Detection**: Automatic detection and handling of stale locks
- **TTL Support**: Optional time-to-live for locks
- **Owner Identification**: Optional owner ID for tracking lock ownership
- **Convenience Wrapper**: Simple `WithLock` function for scoped locking

## Installation

```bash
go get github.com/c2fo/vfs/contrib/lockfile
```

## Usage

### Basic Usage

```go
import (
    "github.com/c2fo/vfs/v7/vfssimple"
    "github.com/c2fo/vfs/v7/contrib/lockfile"
)

// Create a file using vfssimple
f, err := vfssimple.NewFile("mem:///inbox/data.csv")
if err != nil {
    log.Fatal(err)
}

// Create a lock for the file
lock, err := lockfile.NewLock(f)
if err != nil {
    log.Fatal(err)
}

// Try to acquire the lock
if err := lock.Acquire(); err != nil {
    if errors.Is(err, lockfile.ErrLockAlreadyHeld) {
        log.Println("File is already being processed")
        return
    }
    log.Fatal(err)
}
defer lock.Release() // Always release the lock when done

// Safely process the file
```

### Using the WithLock Convenience Wrapper

```go
// Basic usage
err := lockfile.WithLock(f, func(f vfs.File) error {
    // Safely process the file within this scope
    // The lock is automatically released when the function returns
    return nil
})
if err != nil {
    if errors.Is(err, lockfile.ErrLockAlreadyHeld) {
        log.Println("File is already being processed")
        return
    }
    log.Fatal(err)
}

// With TTL and owner ID
err := lockfile.WithLock(f, func(f vfs.File) error {
    // Process file with 5-minute timeout and owner identification
    return nil
}, lockfile.WithTTL(5*time.Minute), lockfile.WithOwnerID("ingestion-service-1"))

// With stale lock handler
err := lockfile.WithLock(f, func(f vfs.File) error {
    // Process file with stale lock handling
    return nil
}, lockfile.WithTTL(5*time.Minute), lockfile.OnStale(func(meta lockfile.Metadata) error {
    log.Printf("Found stale lock from %s (PID: %d)", meta.Hostname, meta.PID)
    return nil // Allow stealing the lock
}))
```

### Advanced WithLock Usage

```go
// Using WithLock with TTL and error handling
err := lockfile.WithLock(f, func(f vfs.File) error {
    // Read the file contents
    data, err := io.ReadAll(f)
    if err != nil {
        return fmt.Errorf("failed to read file: %w", err)
    }

    // Process the data
    processed, err := processData(data)
    if err != nil {
        return fmt.Errorf("failed to process data: %w", err)
    }
    // Create archive vfs.File
    archiveFile, err := f.Location().NewFile("archive/" + f.Name())
    if err != nil {
        return fmt.Errorf("failed to create archive file: %w", err)
    }

    // Move original file to archive
    if err := f.MoveToFile(archiveFile); err != nil {
        return fmt.Errorf("failed to move file to archive: %w", err)
    }

    return nil
})

// Handle different types of errors
switch {
case errors.Is(err, lockfile.ErrLockAlreadyHeld):
    log.Println("File is locked by another process")
    // Implement retry logic or queue the operation
case err != nil:
    log.Printf("Failed to process file: %v", err)
    // Handle other errors
default:
    log.Println("File processed successfully")
}
```

### Using TTLs

```go
// Create a lock with a 5-minute TTL
lock, err := lockfile.NewLock(f, lockfile.WithTTL(5 * time.Minute))
```

### Handling Stale Locks

```go
// Create a lock with stale handler
lock, err := lockfile.NewLock(f,
    lockfile.WithTTL(5 * time.Minute),
    lockfile.OnStale(func(meta lockfile.Metadata) error {
        log.Printf("Found stale lock from %s (PID: %d)", meta.Hostname, meta.PID)
        return nil // Allow stealing the lock
    }),
)
```

### Using Owner IDs

```go
// Create a lock with owner identification
lock, err := lockfile.NewLock(f, lockfile.WithOwnerID("ingestion-service-1"))
```

### Checking Lock Status

```go
// Get lock metadata
meta, err := lock.Metadata()
if err == nil {
    log.Printf("Lock held by %s (PID: %d) since %v",
        meta.Hostname, meta.PID, meta.CreatedAt)
}

// Check lock age
age, err := lock.Age()
if err == nil {
    log.Printf("Lock age: %v", age)
}
```

## Best Practices

1. **Always Use TTLs**: Set appropriate TTLs for long-running operations
2. **Implement Cleanup**: Use `OnStale` handlers to clean up stale locks
3. **Use defer**: Always use `defer lock.Release()` to ensure locks are released
4. **Document Strategy**: Document your locking strategy and ensure all services follow it
5. **Error Handling**: Implement proper error handling and retry logic for transient failures
6. **Consider WithLock**: Use the `WithLock` wrapper for simpler, scoped locking

## Implementation Details

The lockfile utility works by:

1. Creating a companion `.lock` file next to the target file
2. Writing metadata (timestamp, PID, hostname, TTL) to the lock file
3. Using atomic file operations to ensure consistency
4. Checking for stale locks based on TTL
5. Allowing lock stealing when appropriate

## Backend-Specific Considerations

### S3 and Eventual Consistency

When using S3, be aware of its eventual consistency model:

1. **Read-After-Write Consistency**:
   - New objects (PUTs) are immediately available
   - Updates to existing objects (PUTs) may take time to propagate
   - Deletes may take time to propagate

2. **Implications for Locking**:
   - A successful lock acquisition may not be immediately visible to all readers
   - Multiple processes might temporarily see different lock states
   - Deleted locks might still appear to exist for some time

3. **Mitigation Strategies**:
   ```go
   // Use longer TTLs to account for propagation delays
   lock, err := lockfile.NewLock(f, lockfile.WithTTL(10 * time.Minute))

   // Implement retry logic with exponential backoff
   var err error
   for i := 0; i < 3; i++ {
       if err = lock.Acquire(); err == nil {
           break
       }
       if errors.Is(err, lockfile.ErrLockAlreadyHeld) {
           // Wait longer between retries to allow for consistency
           time.Sleep(time.Second * time.Duration(math.Pow(2, float64(i))))
           continue
       }
       break
   }
   ```

4. **Best Practices for S3**:
   - Use longer TTLs than you would with local filesystems
   - Implement retry logic with exponential backoff
   - Consider using S3's strong consistency features where available
   - Monitor for lock contention and adjust TTLs accordingly

## Limitations

- This is an **advisory lock** - it only works if all processes respect the lock
- Network partitions or process crashes may leave stale locks
- Not suitable for high-contention scenarios (consider Redis or other distributed locks)
- S3's eventual consistency may affect lock visibility and reliability

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
