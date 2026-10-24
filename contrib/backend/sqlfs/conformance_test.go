package sqlfs_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/backend/testsuite"
)

func TestConformance(t *testing.T) {
	fsys := newSQLiteFS(t)
	loc, err := fsys.NewLocation("documents", "/conformance/")
	require.NoError(t, err)
	testsuite.RunConformanceTests(t, loc)
}

func TestConformanceSmallChunks(t *testing.T) {
	// A tiny chunk size forces every multi-byte operation across chunk boundaries.
	fsys := newSQLiteFS(t, sqlfsWithChunkSize(4096))
	loc, err := fsys.NewLocation("documents", "/conformance/")
	require.NoError(t, err)
	testsuite.RunConformanceTests(t, loc)
}

func TestIOConformance(t *testing.T) {
	fsys := newSQLiteFS(t)
	loc, err := fsys.NewLocation("documents", "/io/")
	require.NoError(t, err)
	testsuite.RunIOTests(t, loc)
}
