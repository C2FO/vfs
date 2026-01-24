//go:build vfsintegration

package os

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the OS (local filesystem) backend.
//
// Optional environment variables:
//   - VFS_OS_TEST_PATH: Base path for tests (default: "/tmp/vfs-integration-test/")
func TestConformance(t *testing.T) {
	testPath := os.Getenv("VFS_OS_TEST_PATH")
	if testPath == "" {
		testPath = "/tmp/vfs-integration-test/"
	}

	// Ensure test directory exists
	if err := os.MkdirAll(testPath, 0750); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation("", testPath)
	if err != nil {
		t.Fatalf("failed to create OS test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs the IO conformance test suite against the OS (local filesystem) backend.
func TestIOConformance(t *testing.T) {
	testPath := os.Getenv("VFS_OS_TEST_PATH")
	if testPath == "" {
		testPath = "/tmp/vfs-integration-test/"
	}

	// Ensure test directory exists
	if err := os.MkdirAll(testPath, 0750); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation("", testPath)
	if err != nil {
		t.Fatalf("failed to create OS test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}
