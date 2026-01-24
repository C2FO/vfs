//go:build vfsintegration

package gs

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the GCS backend.
//
// Required environment variables:
//   - VFS_GS_BUCKET: GCS bucket name for testing
//   - VFS_GS_TEST_PATH: Base path for tests (default: "/vfs-integration-test/")
//
// GCS credentials are loaded via the default credential chain (GOOGLE_APPLICATION_CREDENTIALS, default credentials, etc.)
func TestConformance(t *testing.T) {
	bucket := os.Getenv("VFS_GS_BUCKET")
	if bucket == "" {
		t.Skip("VFS_GS_BUCKET not set, skipping GCS conformance tests")
	}

	testPath := os.Getenv("VFS_GS_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(bucket, testPath)
	if err != nil {
		t.Fatalf("failed to create GCS test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs the IO conformance test suite against the GCS backend.
func TestIOConformance(t *testing.T) {
	bucket := os.Getenv("VFS_GS_BUCKET")
	if bucket == "" {
		t.Skip("VFS_GS_BUCKET not set, skipping GCS IO conformance tests")
	}

	testPath := os.Getenv("VFS_GS_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(bucket, testPath)
	if err != nil {
		t.Fatalf("failed to create GCS test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}
