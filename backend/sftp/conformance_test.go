//go:build vfsintegration

package sftp

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the SFTP backend.
//
// Required environment variables:
//   - VFS_SFTP_HOST: SFTP server host (e.g., "localhost:22" or "sftp.example.com")
//   - VFS_SFTP_USERNAME: SFTP username
//   - VFS_SFTP_PASSWORD: SFTP password (or use VFS_SFTP_KEYFILE for key-based auth)
//   - VFS_SFTP_TEST_PATH: Base path for tests (default: "/vfs-integration-test/")
//
// Optional environment variables for key-based authentication:
//   - VFS_SFTP_KEYFILE: Path to SSH private key file
//   - VFS_SFTP_KEYFILE_PASSPHRASE: Passphrase for encrypted private key
//   - VFS_SFTP_KNOWN_HOSTS_FILE: Path to known_hosts file
//   - VFS_SFTP_INSECURE_KNOWN_HOSTS: Set to skip host key verification (not recommended)
func TestConformance(t *testing.T) {
	host := os.Getenv("VFS_SFTP_HOST")
	if host == "" {
		t.Skip("VFS_SFTP_HOST not set, skipping SFTP conformance tests")
	}

	testPath := os.Getenv("VFS_SFTP_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(host, testPath)
	if err != nil {
		t.Fatalf("failed to create SFTP test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs the IO conformance test suite against the SFTP backend.
func TestIOConformance(t *testing.T) {
	host := os.Getenv("VFS_SFTP_HOST")
	if host == "" {
		t.Skip("VFS_SFTP_HOST not set, skipping SFTP IO conformance tests")
	}

	testPath := os.Getenv("VFS_SFTP_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(host, testPath)
	if err != nil {
		t.Fatalf("failed to create SFTP test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}
