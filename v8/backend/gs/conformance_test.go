//go:build vfsintegration

package gs

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v8/backend/testsuite"
)

// TestConformance runs the v8 conformance suite against the Google Cloud Storage backend.
//
// Required environment variables:
//   - VFS_GS_BUCKET: GCS bucket name for testing
//   - VFS_GS_TEST_PATH: Base path prefix for tests (default: "/vfs-integration-test/")
//
// Credentials use Application Default Credentials (e.g. GOOGLE_APPLICATION_CREDENTIALS, gcloud, GCE metadata).
func TestConformance(t *testing.T) {
	t.Helper()

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

// TestIOConformance runs scripted I/O conformance tests (requires seek-capable files).
func TestIOConformance(t *testing.T) {
	t.Helper()

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
// When a zero-length object name ends with "/", some UIs treat it as a folder marker.
// Listing should still return only real object base names (e.g. file.txt), not the marker object.
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

	client, err := fs.Client()
	require.NoError(t, err)

	objHandle := client.
		Bucket(bucket).
		Object(utils.RemoveLeadingSlash(baseLoc.Path() + "myfolder/"))

	ctx := t.Context()

	writer := objHandle.NewWriter(ctx)
	_, err = writer.Write([]byte(""))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	f, err := baseLoc.NewFile("myfolder/file.txt")
	require.NoError(t, err)

	_, err = f.Write([]byte("some text"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	loc := f.Location()
	files, err := testsuite.CollectList(ctx, loc)
	require.NoError(t, err)
	assert.Len(t, files, 1, "check file count found")
	assert.Equal(t, "file.txt", files[0], "file.txt was found")

	require.NoError(t, loc.DeleteFile("file.txt"))
	require.NoError(t, objHandle.Delete(ctx))
}
