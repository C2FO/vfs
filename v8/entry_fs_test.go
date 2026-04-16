package vfs

import (
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/utils/authority"
)

type stubFileInfo struct {
	name string
	size int64
}

func (s stubFileInfo) Name() string       { return s.name }
func (s stubFileInfo) Size() int64        { return s.size }
func (s stubFileInfo) Mode() fs.FileMode  { return 0644 }
func (s stubFileInfo) ModTime() time.Time { return time.Time{} }
func (s stubFileInfo) IsDir() bool        { return false }
func (s stubFileInfo) Sys() any           { return nil }

type stubFile struct {
	info fs.FileInfo
}

func (s stubFile) Read(_ []byte) (int, error) { return 0, fs.ErrClosed }
func (s stubFile) Write(_ []byte) (int, error) {
	return 0, fs.ErrClosed
}
func (s stubFile) Close() error   { return nil }
func (s stubFile) String() string { return "stub" }

func (s stubFile) URI() string  { return "mem:///x" }
func (s stubFile) Path() string { return "/x" }
func (s stubFile) Name() string { return s.info.Name() }
func (s stubFile) Authority() authority.Authority {
	return authority.Authority{}
}

func (s stubFile) Location() Location { return nil }

func (s stubFile) Exists() (bool, error) { return true, nil }
func (s stubFile) Stat() (fs.FileInfo, error) {
	return s.info, nil
}
func (s stubFile) LastModified() (*time.Time, error) { return nil, nil }
func (s stubFile) Size() (uint64, error)             { return uint64(s.info.Size()), nil }
func (s stubFile) Touch() error                      { return nil }

func TestStubFileImplementsFile(t *testing.T) {
	t.Parallel()

	var _ File = stubFile{info: stubFileInfo{name: "n", size: 0}}
}

func TestEntry_DirEntry_File(t *testing.T) {
	t.Parallel()

	info := stubFileInfo{name: "a.txt", size: 3}
	f := stubFile{info: info}
	e := Entry{Kind: EntryFile, Name: "a.txt", File: f}

	de := e.DirEntry()
	require.Equal(t, "a.txt", de.Name())
	require.False(t, de.IsDir())

	got, err := de.Info()
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Size())
}

func TestEntry_DirEntry_Location(t *testing.T) {
	t.Parallel()

	e := Entry{Kind: EntryLocation, Name: "dir"}

	de := e.DirEntry()
	require.True(t, de.IsDir())
	require.Equal(t, fs.ModeDir, de.Type()&fs.ModeDir)
}

func TestEntry_DirEntry_NoInfo(t *testing.T) {
	t.Parallel()

	e := Entry{Kind: EntryBlob, Name: "only"}

	de := e.DirEntry()
	_, err := de.Info()
	require.ErrorIs(t, err, fs.ErrNotExist)
}
