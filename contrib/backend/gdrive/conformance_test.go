package gdrive

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// newTestFS returns a FileSystem wired to a fake Drive with instant retries.
func newTestFS(t *testing.T, d *fakeDrive, opts ...func(*FileSystem)) *FileSystem {
	t.Helper()

	fs := NewFileSystem(
		WithBaseURL(d.url()),
		WithAccessToken("test-token"),
		WithTempDir(t.TempDir()),
		WithRetryCount(2),
	)
	for _, o := range opts {
		o(fs)
	}

	stubSleep(t, fs, "")
	return fs
}

// stubSleep makes the drive client for driveID retry without waiting.
func stubSleep(t *testing.T, fs *FileSystem, driveID string) {
	t.Helper()

	c, err := fs.Client(driveID)
	require.NoError(t, err)
	if dc, ok := c.(*driveClient); ok {
		dc.sleep = func(_ context.Context, _ time.Duration) error { return nil }
	}
}

// TestConformanceFakeDrive runs the shared VFS conformance and IO suites against the in-process
// fake Drive, so the backend's semantics are verified on every test run. realdrive_test.go runs the
// same suites against a real Drive.
func TestConformanceFakeDrive(t *testing.T) {
	d := newFakeDrive(t)
	d.addFolder("vfs-test") // the suite expects its base location to already exist
	fs := newTestFS(t, d)

	loc, err := fs.NewLocation("", "/vfs-test/")
	require.NoError(t, err)

	t.Run("conformance", func(t *testing.T) { testsuite.RunConformanceTests(t, loc) })
	t.Run("io", func(t *testing.T) { testsuite.RunIOTests(t, loc) })
}

// TestConformanceFakeSharedDrive runs the conformance suite against a shared drive addressed by the
// location authority, as a content manager: permanent deletes are refused, so every delete and
// every overwrite by copy or move has to fall back to trashing.
func TestConformanceFakeSharedDrive(t *testing.T) {
	const driveID = "shared1"

	d := newFakeDrive(t)
	d.addSharedDrive(driveID)
	d.mu.Lock()
	d.add(driveID, "vfs-test", folderMimeType, nil)
	d.mu.Unlock()
	d.fault(&fakeFault{
		match:  isFileDelete,
		status: http.StatusNotFound, reason: "notFound", times: 1 << 30,
	})

	fs := newTestFS(t, d)
	stubSleep(t, fs, driveID)

	loc, err := fs.NewLocation(driveID, "/vfs-test/")
	require.NoError(t, err)

	t.Run("conformance", func(t *testing.T) { testsuite.RunConformanceTests(t, loc) })
}
