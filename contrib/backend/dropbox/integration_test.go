//go:build vfsintegration

package dropbox

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the Dropbox backend.
// Run with: go test -v -tags=vfsintegration ./... -run TestConformance
//
// Required environment variables:
//   - VFS_DROPBOX_ACCESS_TOKEN: Valid Dropbox access token
//   - VFS_DROPBOX_TEST_PATH: Base path for tests (e.g., "/vfs-test/")
func TestConformance(t *testing.T) {
	token := os.Getenv("VFS_DROPBOX_ACCESS_TOKEN")
	if token == "" {
		t.Skip("VFS_DROPBOX_ACCESS_TOKEN not set, skipping integration tests")
	}

	testPath := os.Getenv("VFS_DROPBOX_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-test/"
	}

	fs := NewFileSystem(WithAccessToken(token))
	location, err := fs.NewLocation("", testPath)
	if err != nil {
		t.Fatalf("failed to create test location: %v", err)
	}

	// Dropbox has timestamp limitations - skip the Touch timestamp test
	opts := testsuite.ConformanceOptions{
		SkipTouchTimestampTest: true,
	}

	testsuite.RunConformanceTests(t, location, opts)
}

// TestIOConformance runs the IO conformance test suite against the Dropbox backend.
func TestIOConformance(t *testing.T) {
	token := os.Getenv("VFS_DROPBOX_ACCESS_TOKEN")
	if token == "" {
		t.Skip("VFS_DROPBOX_ACCESS_TOKEN not set, skipping integration tests")
	}

	testPath := os.Getenv("VFS_DROPBOX_TEST_PATH")
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
