//go:build vfsintegration

package gs

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/utils"
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

// TestPersistentFolderHandling tests GCS-specific handling of persistent "folders".
// When a persistent "folder" is created through the UI, it creates a zero-length object
// with a trailing "/". The UI or gsutil interprets these as folders but they are still
// just objects. List() should ignore these objects.
func TestPersistentFolderHandling(t *testing.T) {
	bucket := os.Getenv("VFS_GS_BUCKET")
	if bucket == "" {
		t.Skip("VFS_GS_BUCKET not set, skipping GCS persistent folder test")
	}

	testPath := os.Getenv("VFS_GS_TEST_PATH")
	if testPath == "" {
		testPath = "/vfs-integration-test/"
	}

	fs := NewFileSystem()
	baseLoc, err := fs.NewLocation(bucket, testPath)
	require.NoError(t, err)

	// Get client since VFS doesn't allow a File ending with a slash
	client, err := fs.Client()
	require.NoError(t, err)

	objHandle := client.
		Bucket(bucket).
		Object(utils.RemoveLeadingSlash(baseLoc.Path() + "myfolder/"))

	ctx := t.Context()

	// Write zero-length object to simulate persistent folder
	writer := objHandle.NewWriter(ctx)
	_, err = writer.Write([]byte(""))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	// Create a file inside the "folder"
	f, err := baseLoc.NewFile("myfolder/file.txt")
	require.NoError(t, err)

	_, err = f.Write([]byte("some text"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// List "folder" should only return file.txt, not the zero-length folder object
	files, err := f.Location().List()
	require.NoError(t, err)
	assert.Len(t, files, 1, "check file count found")
	assert.Equal(t, "file.txt", files[0], "file.txt was found")

	// Clean up
	require.NoError(t, f.Delete(), "clean up file.txt")
	require.NoError(t, objHandle.Delete(ctx))
}
