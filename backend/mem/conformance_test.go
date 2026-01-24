//go:build vfsintegration

package mem

import (
	"testing"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

// TestConformance runs the VFS conformance test suite against the in-memory backend.
// No environment variables are required as this backend operates entirely in memory.
func TestConformance(t *testing.T) {
	fs := NewFileSystem()
	location, err := fs.NewLocation("testvolume", "/vfs-integration-test/")
	if err != nil {
		t.Fatalf("failed to create in-memory test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs the IO conformance test suite against the in-memory backend.
func TestIOConformance(t *testing.T) {
	fs := NewFileSystem()
	location, err := fs.NewLocation("testvolume", "/vfs-integration-test/")
	if err != nil {
		t.Fatalf("failed to create in-memory test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}
