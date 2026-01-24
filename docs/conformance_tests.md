# Conformance Tests

---

The `backend/testsuite` package provides conformance tests that verify VFS backend implementations correctly implement the `vfs.FileSystem`, `vfs.Location`, and `vfs.File` interfaces.

## Overview

Conformance tests ensure that all VFS backends behave consistently, regardless of the underlying storage system. By running these tests against your backend implementation, you can verify:

- FileSystem interface methods work correctly
- Location operations (list, exists, delete) behave as expected
- File operations (read, write, seek, copy, move, delete) function properly
- IO behavior matches the expected VFS contract

## Exported Test Functions

The package exports the following test functions:

| Function | Description |
|----------|-------------|
| `RunConformanceTests` | Runs all conformance tests (FileSystem, Location, File) |
| `RunFileSystemTests` | Tests `vfs.FileSystem` interface conformance |
| `RunLocationTests` | Tests `vfs.Location` interface conformance |
| `RunFileTests` | Tests `vfs.File` interface conformance |
| `RunIOTests` | Tests Read/Write/Seek behavior conformance |

## Using Conformance Tests

### For New Backend Developers

Create an integration test file in your backend package:

```go
//go:build vfsintegration

package mybackend

import (
    "os"
    "testing"

    "github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the backend.
// Run with: go test -v -tags=vfsintegration ./... -run TestConformance
//
// Required environment variables:
//   - MY_BACKEND_TOKEN: Valid access token for the backend
//   - MY_BACKEND_TEST_PATH: Base path for tests (e.g., "/vfs-test/")
func TestConformance(t *testing.T) {
    token := os.Getenv("MY_BACKEND_TOKEN")
    if token == "" {
        t.Skip("MY_BACKEND_TOKEN not set, skipping integration tests")
    }

    testPath := os.Getenv("MY_BACKEND_TEST_PATH")
    if testPath == "" {
        testPath = "/vfs-test/"
    }

    fs := NewFileSystem(WithAccessToken(token))
    location, err := fs.NewLocation("", testPath)
    if err != nil {
        t.Fatalf("failed to create test location: %v", err)
    }

    // Configure options for backend-specific limitations
    opts := testsuite.ConformanceOptions{
        SkipTouchTimestampTest: false,  // set true if backend has timestamp limitations
        SkipFTPSpecificTests:   false,  // set true for FTP-like backends
    }

    testsuite.RunConformanceTests(t, location, opts)
}

// TestIOConformance runs the IO conformance test suite.
func TestIOConformance(t *testing.T) {
    token := os.Getenv("MY_BACKEND_TOKEN")
    if token == "" {
        t.Skip("MY_BACKEND_TOKEN not set, skipping integration tests")
    }

    testPath := os.Getenv("MY_BACKEND_TEST_PATH")
    if testPath == "" {
        testPath = "/vfs-test/"
    }

    fs := NewFileSystem(WithAccessToken(token))
    location, err := fs.NewLocation("", testPath)
    if err != nil {
        t.Fatalf("failed to create test location: %v", err)
    }

    testsuite.RunIOTests(t, location)
}
```

### Running the Tests

```bash
# Set required environment variables
export MY_BACKEND_TOKEN="your-access-token"
export MY_BACKEND_TEST_PATH="/vfs-test/"

# Run conformance tests
go test -v -tags=vfsintegration ./path/to/backend -run TestConformance

# Run IO conformance tests
go test -v -tags=vfsintegration ./path/to/backend -run TestIOConformance

# Run all integration tests
go test -v -tags=vfsintegration ./path/to/backend
```

## ConformanceOptions

The `ConformanceOptions` struct allows backends to skip certain tests for known limitations:

```go
type ConformanceOptions struct {
    // SkipTouchTimestampTest skips the Touch timestamp update assertion.
    // Some backends may not update timestamps when content is unchanged.
    SkipTouchTimestampTest bool

    // SkipFTPSpecificTests skips tests that don't work well with FTP
    // protocol limitations (e.g., certain io.Copy patterns).
    SkipFTPSpecificTests bool
}
```

### When to Use Options

| Option | When to Use |
|--------|-------------|
| `SkipTouchTimestampTest` | Backend doesn't update `LastModified` when re-uploading identical content |
| `SkipFTPSpecificTests` | Backend has FTP-like limitations with certain IO patterns |

## Testing Approaches

The testsuite package supports two complementary testing approaches:

### Per-Backend Testing (Recommended for Development)

Each backend has its own `conformance_test.go` that directly instantiates the backend and runs tests:

```bash
# Test a specific backend
export VFS_S3_BUCKET="my-bucket"
export VFS_S3_TEST_PATH="/vfs-test/"
go test -v -tags=vfsintegration ./backend/s3

# Test the mem backend (no env vars needed)
go test -v -tags=vfsintegration ./backend/mem
```

**Benefits:**
- Self-contained: each backend can be tested independently
- No circular dependencies: backends import testsuite, not the reverse
- Ideal for contrib backends that aren't registered in `backend/all`
- Easier debugging: test failures are isolated to one backend

### Centralized Testing (Useful for CI)

The `backend/testsuite` package includes test runners that use `vfssimple` and `VFS_INTEGRATION_LOCATIONS` to test multiple backends at once:

```bash
# Test multiple backends in one command
export VFS_INTEGRATION_LOCATIONS="file:///tmp/vfs_test/;mem://A/path/to/;s3://mybucket/test/"
go test -v -tags=vfsintegration ./backend/testsuite
```

**Benefits:**
- Convenient for CI pipelines that test all configured backends
- Single command to run all backend tests
- Includes backend-specific tests (e.g., GCS folder handling)

**Files:**
- `backend/testsuite/backend_integration_test.go` - Runs `RunConformanceTests` for each configured backend
- `backend/testsuite/io_integration_test.go` - Runs `RunIOTests` with optional local baseline comparison

## What the Tests Cover

### FileSystem Tests (`RunFileSystemTests`)

- `NewFile()` with valid/invalid paths
- `NewLocation()` with valid/invalid paths
- Path validation (absolute vs relative)
- URI construction

### Location Tests (`RunLocationTests`)

- `NewLocation()` relative path resolution
- `NewFile()` relative file creation
- `List()` directory contents
- `ListByPrefix()` filtering
- `ListByRegex()` pattern matching
- `Exists()` location existence
- `DeleteFile()` file removal
- `Path()` and `URI()` methods

### File Tests (`RunFileTests`)

- `Read()` and `Write()` operations
- `Seek()` cursor positioning
- `Close()` and resource cleanup
- `Exists()` file existence
- `Size()` file size
- `LastModified()` timestamp
- `CopyToLocation()` and `CopyToFile()` copy operations
- `MoveToLocation()` and `MoveToFile()` move operations
- `Touch()` creating/updating files
- `Delete()` file removal
- Handling files with spaces in paths
- Operations on non-existent files (error handling)

### IO Tests (`RunIOTests`)

Tests 18 different Read/Write/Seek sequences including:

- Read, Close (file exists / doesn't exist)
- Read, Seek, Read, Close
- Write, Close (file exists / doesn't exist)
- Write, Seek, Write, Close
- Seek, Write, Close
- Seek, Read, Close
- Read, Write, Close
- Read, Seek, Write, Close
- Write, Seek, Read, Close

## Best Practices

### Environment Variables

Use a consistent naming convention for your backend's environment variables:

```
VFS_<BACKEND>_<CONFIG_NAME>
```

Examples:
- `VFS_S3_ACCESS_KEY_ID`
- `VFS_AZURE_STORAGE_ACCOUNT`
- `VFS_DROPBOX_ACCESS_TOKEN`

### Test Path Isolation

Always use a dedicated test path to avoid conflicts:

```go
testPath := os.Getenv("MY_BACKEND_TEST_PATH")
if testPath == "" {
    testPath = "/vfs-integration-test/"
}
```

### Cleanup

The conformance tests handle cleanup of files and subdirectories created during testing. However, for safety with `file://` scheme, the top-level test directory is not automatically removed.

### CI/CD Integration

```yaml
# GitHub Actions example
- name: Run Backend Integration Tests
  env:
    MY_BACKEND_TOKEN: ${{ secrets.MY_BACKEND_TOKEN }}
    MY_BACKEND_TEST_PATH: "/ci-test/"
  run: |
    go test -v -tags=vfsintegration ./path/to/backend
```

## Troubleshooting

### Tests Skip Immediately

Ensure your environment variables are set:

```bash
echo $MY_BACKEND_TOKEN  # Should not be empty
```

### Authentication Errors

Verify your credentials are valid and have the required permissions:
- Read/write access to the test path
- Delete permissions
- List permissions

### Timeout Errors

Network-based backends may need longer timeouts. Consider:
- Using a test path geographically close to your test runner
- Increasing Go test timeout: `go test -timeout 10m ...`

### Path Not Found

Ensure:
- The test path format is correct for your backend
- The parent directory exists (if required by the backend)
- The path ends with `/` for locations
