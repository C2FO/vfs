//go:build vfsintegration

package os

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c2fo/vfs/v8/backend/testsuite"
)

// TestConformance runs the v8 conformance suite against the local filesystem backend.
//
// Optional environment variables:
//   - VFS_OS_TEST_PATH: Base path for tests (default: system temp directory)
func TestConformance(t *testing.T) {
	t.Helper()

	testPath := getTestPath(t)
	fs := NewFileSystem()
	location, err := fs.NewLocation("", testPath)
	if err != nil {
		t.Fatalf("failed to create OS test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs scripted IO conformance tests.
func TestIOConformance(t *testing.T) {
	t.Helper()

	testPath := getTestPath(t)
	fs := NewFileSystem()
	location, err := fs.NewLocation("", testPath)
	if err != nil {
		t.Fatalf("failed to create OS test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}

func getTestPath(t *testing.T) string {
	t.Helper()
	if testPath := os.Getenv("VFS_OS_TEST_PATH"); testPath != "" {
		if err := os.MkdirAll(testPath, 0750); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		return testPath
	}
	return filepath.Clean(t.TempDir()) + string(filepath.Separator)
}
