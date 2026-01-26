//go:build vfsintegration

package ftp

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the FTP backend.
//
// Required environment variables:
//   - VFS_FTP_HOST: FTP server host (e.g., "localhost:21" or "ftp.example.com")
//   - VFS_FTP_TEST_PATH: Base path for tests (default: "/vfs-integration-test/")
//
// Optional environment variables:
//   - VFS_FTP_USERNAME: FTP username (default: "anonymous")
//   - VFS_FTP_PASSWORD: FTP password (default: "anonymous")
//   - VFS_FTP_PROTOCOL: Protocol type - FTP, FTPS, or FTPES (default: FTP)
//   - VFS_FTP_DISABLE_EPSV: Set to disable EPSV mode
func TestConformance(t *testing.T) {
	host := os.Getenv("VFS_FTP_HOST")
	if host == "" {
		t.Skip("VFS_FTP_HOST not set, skipping FTP conformance tests")
	}

	testPath := os.Getenv("VFS_FTP_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(host, testPath)
	if err != nil {
		t.Fatalf("failed to create FTP test location: %v", err)
	}

	opts := testsuite.ConformanceOptions{
		SkipFTPSpecificTests: true,
	}

	testsuite.RunConformanceTests(t, location, opts)
}

// TestIOConformance runs the IO conformance test suite against the FTP backend.
func TestIOConformance(t *testing.T) {
	host := os.Getenv("VFS_FTP_HOST")
	if host == "" {
		t.Skip("VFS_FTP_HOST not set, skipping FTP IO conformance tests")
	}

	testPath := os.Getenv("VFS_FTP_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(host, testPath)
	if err != nil {
		t.Fatalf("failed to create FTP test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}
