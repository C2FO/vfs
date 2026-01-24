//go:build vfsintegration

package azure

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the Azure Blob Storage backend.
//
// Required environment variables:
//   - VFS_AZURE_STORAGE_ACCOUNT: Azure storage account name
//   - VFS_AZURE_STORAGE_ACCESS_KEY: Azure storage account access key (for shared key auth)
//   - VFS_AZURE_CONTAINER: Azure container name for testing
//   - VFS_AZURE_TEST_PATH: Base path for tests (default: "/vfs-integration-test/")
//
// Alternative authentication via service principal:
//   - VFS_AZURE_TENANT_ID: Azure tenant ID
//   - VFS_AZURE_CLIENT_ID: Azure client ID
//   - VFS_AZURE_CLIENT_SECRET: Azure client secret
func TestConformance(t *testing.T) {
	container := os.Getenv("VFS_AZURE_CONTAINER")
	if container == "" {
		t.Skip("VFS_AZURE_CONTAINER not set, skipping Azure conformance tests")
	}

	// Check for credentials
	if os.Getenv("VFS_AZURE_STORAGE_ACCOUNT") == "" {
		t.Skip("VFS_AZURE_STORAGE_ACCOUNT not set, skipping Azure conformance tests")
	}

	testPath := os.Getenv("VFS_AZURE_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(container, testPath)
	if err != nil {
		t.Fatalf("failed to create Azure test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs the IO conformance test suite against the Azure Blob Storage backend.
func TestIOConformance(t *testing.T) {
	container := os.Getenv("VFS_AZURE_CONTAINER")
	if container == "" {
		t.Skip("VFS_AZURE_CONTAINER not set, skipping Azure IO conformance tests")
	}

	if os.Getenv("VFS_AZURE_STORAGE_ACCOUNT") == "" {
		t.Skip("VFS_AZURE_STORAGE_ACCOUNT not set, skipping Azure IO conformance tests")
	}

	testPath := os.Getenv("VFS_AZURE_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	location, err := fs.NewLocation(container, testPath)
	if err != nil {
		t.Fatalf("failed to create Azure test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}
