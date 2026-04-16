//go:build vfsintegration

package s3

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v8/backend/testsuite"
)

// TestConformance runs the v8 conformance suite against the S3 backend.
//
// Required environment variables:
//   - VFS_S3_BUCKET: S3 bucket name for testing
//   - VFS_S3_TEST_PATH: Base path prefix for tests (default: "/vfs-integration-test/")
//
// AWS credentials use the default SDK chain (environment, shared credentials, IAM role, etc.).
func TestConformance(t *testing.T) {
	t.Helper()

	bucket := os.Getenv("VFS_S3_BUCKET")
	if bucket == "" {
		t.Skip("VFS_S3_BUCKET not set, skipping S3 conformance tests")
	}

	testPath := os.Getenv("VFS_S3_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(bucket, testPath)
	if err != nil {
		t.Fatalf("failed to create S3 test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs scripted I/O conformance tests (requires seek-capable files).
func TestIOConformance(t *testing.T) {
	t.Helper()

	bucket := os.Getenv("VFS_S3_BUCKET")
	if bucket == "" {
		t.Skip("VFS_S3_BUCKET not set, skipping S3 IO conformance tests")
	}

	testPath := os.Getenv("VFS_S3_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(bucket, testPath)
	if err != nil {
		t.Fatalf("failed to create S3 test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}
