//go:build vfsintegration

package mem

import (
	"testing"

	"github.com/c2fo/vfs/v8/backend/testsuite"
)

// TestConformance runs the v8 conformance suite against the in-memory backend.
func TestConformance(t *testing.T) {
	t.Helper()

	fs := NewFileSystem()
	location, err := fs.NewLocation("testvolume", "/vfs-integration-test/")
	if err != nil {
		t.Fatalf("failed to create in-memory test location: %v", err)
	}

	testsuite.RunConformanceTests(t, location)
}

// TestIOConformance runs scripted IO conformance tests.
func TestIOConformance(t *testing.T) {
	t.Helper()

	fs := NewFileSystem()
	location, err := fs.NewLocation("testvolume", "/vfs-integration-test/")
	if err != nil {
		t.Fatalf("failed to create in-memory test location: %v", err)
	}

	testsuite.RunIOTests(t, location)
}
