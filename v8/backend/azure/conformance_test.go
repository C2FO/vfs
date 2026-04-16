//go:build vfsintegration

package azure

import (
	"os"
	"testing"

	"github.com/c2fo/vfs/v8/backend/testsuite"
)

// TestConformance runs the v8 conformance suite against the Azure Blob Storage backend.
//
// Required environment variables:
//   - VFS_AZURE_CONTAINER: Azure container name for testing
//   - VFS_AZURE_STORAGE_ACCOUNT: Azure storage account name
//   - VFS_AZURE_STORAGE_ACCESS_KEY: (shared key) or use service principal env vars
//   - VFS_AZURE_TEST_PATH: Base path prefix (default: "/vfs-integration-test/")
//
// Service principal: VFS_AZURE_TENANT_ID, VFS_AZURE_CLIENT_ID, VFS_AZURE_CLIENT_SECRET.
func TestConformance(t *testing.T) {
	t.Helper()

	container := os.Getenv("VFS_AZURE_CONTAINER")
	if container == "" {
		t.Skip("VFS_AZURE_CONTAINER not set, skipping Azure conformance tests")
	}

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

// TestIOConformance runs scripted I/O conformance tests.
func TestIOConformance(t *testing.T) {
	t.Helper()

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
