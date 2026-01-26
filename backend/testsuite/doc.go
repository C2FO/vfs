/*
Package testsuite provides conformance tests for VFS backend implementations.

These tests can be imported by any backend (core or contrib) to verify correct
implementation of the vfs.FileSystem, vfs.Location, and vfs.File interfaces.

# Exported Test Functions

The package exports the following test functions for use by backend implementations:

  - RunConformanceTests: Runs all conformance tests (FileSystem, Location, File)
  - RunFileSystemTests: Tests vfs.FileSystem interface conformance
  - RunLocationTests: Tests vfs.Location interface conformance
  - RunFileTests: Tests vfs.File interface conformance
  - RunIOTests: Tests Read/Write/Seek behavior conformance

# Usage by Individual Backends

Each backend can create its own integration test file that imports and runs the conformance tests:

	//go:build vfsintegration

	package mybackend

	import (
	    "os"
	    "testing"
	    "github.com/c2fo/vfs/v7/backend/testsuite"
	)

	func TestConformance(t *testing.T) {
	    fs := NewFileSystem(WithAccessToken(os.Getenv("MY_BACKEND_TOKEN")))
	    location, err := fs.NewLocation("", "/test-path/")
	    if err != nil {
	        t.Fatalf("failed to create location: %v", err)
	    }

	    opts := testsuite.ConformanceOptions{
	        SkipTouchTimestampTest: true,  // if backend has timestamp limitations
	    }
	    testsuite.RunConformanceTests(t, location, opts)
	}

	func TestIOConformance(t *testing.T) {
	    fs := NewFileSystem(WithAccessToken(os.Getenv("MY_BACKEND_TOKEN")))
	    location, _ := fs.NewLocation("", "/test-path/")
	    testsuite.RunIOTests(t, location)
	}

Run with:

	MY_BACKEND_TOKEN=xxx go test -v -tags=vfsintegration ./... -run TestConformance

# Testing Approaches

This package supports two complementary approaches:

Per-Backend Testing: Each backend has its own conformance_test.go that directly
instantiates the backend and runs tests. This is ideal for isolated testing and
contrib backends:

	VFS_S3_BUCKET=my-bucket go test -tags=vfsintegration ./backend/s3

Centralized Testing: The backend_integration_test.go and io_integration_test.go files
in this package use vfssimple with VFS_INTEGRATION_LOCATIONS to test multiple backends
at once. Useful for CI pipelines:

	VFS_INTEGRATION_LOCATIONS="file:///tmp/vfs_test/;mem://A/path/to/;s3://mybucket/test/"
	go test -tags=vfsintegration ./backend/testsuite

# ConformanceOptions

The ConformanceOptions struct allows backends to skip certain tests for known limitations:

  - SkipTouchTimestampTest: Skip Touch timestamp update assertion (for backends that
    may not update timestamps when content is unchanged)
  - SkipFTPSpecificTests: Skip tests that don't work well with FTP protocol limitations

# Safety Note

For safety, os-based scheme will not clean up the top-level test location. All sub-locations
and files within the test path will be cleaned up after tests complete.
*/
package testsuite
