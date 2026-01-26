//go:build vfsintegration

package s3

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the S3 backend.
//
// Required environment variables:
//   - VFS_S3_BUCKET: S3 bucket name for testing
//   - VFS_S3_TEST_PATH: Base path for tests (default: "/vfs-integration-test/")
//
// AWS credentials are loaded via the default credential chain (env vars, shared credentials, IAM role, etc.)
func TestConformance(t *testing.T) {
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

// TestIOConformance runs the IO conformance test suite against the S3 backend.
func TestIOConformance(t *testing.T) {
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
