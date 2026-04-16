package os

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/utils"
)

func TestFile_Stat_NotExist(t *testing.T) {
	t.Helper()

	fs := NewFileSystem()
	dir := utils.EnsureTrailingSlash(t.TempDir())
	loc, err := fs.NewLocation("", dir)
	require.NoError(t, err)

	f, err := loc.NewFile("does-not-exist.txt")
	require.NoError(t, err)
	_, err = f.Stat()
	require.Error(t, err)
}

func TestFile_WriteReadSeek(t *testing.T) {
	t.Helper()

	fs := NewFileSystem()
	dir := utils.EnsureTrailingSlash(t.TempDir())
	loc, err := fs.NewLocation("", dir)
	require.NoError(t, err)

	f, err := loc.NewFile("rw.txt")
	require.NoError(t, err)
	_, err = f.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	f2, err := loc.NewFile("rw.txt")
	require.NoError(t, err)
	sk, ok := f2.(io.Seeker)
	require.True(t, ok)
	_, err = sk.Seek(0, 0)
	require.NoError(t, err)
	buf := make([]byte, 5)
	n, err := f2.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, "hello", string(buf))
	require.NoError(t, f2.Close())
}
