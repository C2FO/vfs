package testsuite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v8/backend/mem"
)

func TestCollectList(t *testing.T) {
	t.Helper()

	fs := mem.NewFileSystem()
	loc, err := fs.NewLocation("vol", "/collect-test/")
	require.NoError(t, err)

	f, err := loc.NewFile("alpha.txt")
	require.NoError(t, err)
	_, err = f.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	names, err := CollectList(context.Background(), loc)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha.txt"}, names)
}
