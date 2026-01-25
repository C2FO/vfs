//go:build vfsintegration

package os

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the OS (local filesystem) backend.
//
// Optional environment variables:
//   - VFS_OS_TEST_PATH: Base path for tests (default: system temp directory)
func TestConformance(t *testing.T) {
	testPath := getTestPath(t)

	fs := NewFileSystem()
	location, err := fs.NewLocation("", testPath)
	if err != nil {
		t.Fatalf("failed to create OS test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs the IO conformance test suite against the OS (local filesystem) backend.
func TestIOConformance(t *testing.T) {
	testPath := getTestPath(t)

	fs := NewFileSystem()
	location, err := fs.NewLocation("", testPath)
	if err != nil {
		t.Fatalf("failed to create OS test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}

// getTestPath returns the test path from environment variable or creates a temp directory.
// When using t.TempDir(), the directory is automatically cleaned up after the test.
func getTestPath(t *testing.T) string {
	t.Helper()
	if testPath := os.Getenv("VFS_OS_TEST_PATH"); testPath != "" {
		// Ensure user-specified directory exists
		if err := os.MkdirAll(testPath, 0750); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		return testPath
	}
	// Use t.TempDir() for cross-platform temp directory with automatic cleanup
	return filepath.Clean(t.TempDir()) + string(filepath.Separator)
}
