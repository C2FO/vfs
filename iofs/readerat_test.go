package iofs

import (
	"io"
	"io/fs"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
)

// readerAt wraps a real file so that it also implements io.ReaderAt, which no core backend's file does.
type readerAt struct {
	vfs.File
	data string
}

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	return copy(p, r.data[off:]), nil
}

func TestNewFileExposesReaderAtOnlyWhenSupported(t *testing.T) {
	loc, err := mem.NewFileSystem().NewLocation("vol", "/root/")
	require.NoError(t, err)
	f, err := loc.NewFile("a.txt")
	require.NoError(t, err)

	plain := newFile(&fsys{}, f, vfs.Entry{}, "a.txt")
	_, ok := plain.(io.ReaderAt)
	require.False(t, ok)

	withReaderAt := newFile(&fsys{}, readerAt{File: f, data: "alpha"}, vfs.Entry{}, "a.txt")
	ra, ok := withReaderAt.(io.ReaderAt)
	require.True(t, ok)
	buf := make([]byte, 3)
	n, err := ra.ReadAt(buf, 2)
	require.NoError(t, err)
	require.Equal(t, "pha", string(buf[:n]))
}

func TestNewDirInfoUsesReportedPermissions(t *testing.T) {
	loc, err := mem.NewFileSystem().NewLocation("vol", "/root/")
	require.NoError(t, err)

	reported, err := vfs.NewDirEntry(loc, "sub", vfs.WithEntryMode(0o700))
	require.NoError(t, err)
	unreported, err := vfs.NewDirEntry(loc, "sub")
	require.NoError(t, err)

	require.Equal(t, fs.ModeDir|0o700, newDirInfo("sub", &reported).Mode())
	require.Equal(t, fs.ModeDir|0o555, newDirInfo("sub", &unreported).Mode())
	require.Equal(t, fs.ModeDir|0o555, newDirInfo(".", nil).Mode())
}

func TestValidName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "dot", input: ".", expected: true},
		{name: "nested", input: "a/b.txt", expected: true},
		{name: "rooted", input: "/a", expected: false},
		{name: "dotdot", input: "../a", expected: false},
		{name: "trailing slash", input: "a/", expected: false},
		{name: "backslash", input: `a\b`, expected: runtime.GOOS != "windows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, validName(tt.input))
		})
	}
}
