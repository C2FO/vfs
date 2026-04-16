package v7adapter

import (
	"context"
	"io/fs"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfsv8 "github.com/c2fo/vfs/v8"
	vfsv8opt "github.com/c2fo/vfs/v8/options"
)

func TestFileFromV7_AsV7File(t *testing.T) {
	t.Parallel()

	mfs := mem.NewFileSystem()
	fv7, err := mfs.NewFile("vol", "/a/b.txt")
	require.NoError(t, err)
	require.NoError(t, fv7.Touch())

	f8 := FileFromV7(fv7)
	require.NotNil(t, f8)

	back, ok := AsV7File(f8)
	require.True(t, ok)
	require.Equal(t, fv7, back)
}

func TestFile_Stat(t *testing.T) {
	t.Parallel()

	mfs := mem.NewFileSystem()
	fv7, err := mfs.NewFile("vol", "/x/y.txt")
	require.NoError(t, err)
	require.NoError(t, fv7.Touch())

	f8 := FileFromV7(fv7).(*File)
	info, err := f8.Stat()
	require.NoError(t, err)
	require.Equal(t, "y.txt", info.Name())
	require.False(t, info.IsDir())
}

func TestFile_Stat_NotExist(t *testing.T) {
	t.Parallel()

	mfs := mem.NewFileSystem()
	fv7, err := mfs.NewFile("vol", "/only/path.txt")
	require.NoError(t, err)
	// no Touch — mem file not linked

	f8 := FileFromV7(fv7).(*File)
	_, err = f8.Stat()
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestLocation_List_Matcher(t *testing.T) {
	t.Parallel()

	mfs := mem.NewFileSystem()
	locV7, err := mfs.NewLocation("v", "/dir/")
	require.NoError(t, err)

	for _, name := range []string{"a.txt", "b.txt", "c.log"} {
		f, ferr := mfs.NewFile("v", "/dir/"+name)
		require.NoError(t, ferr)
		require.NoError(t, f.Touch())
		require.NoError(t, f.Close())
	}

	loc8 := LocationFromV7(locV7).(*Location)
	var names []string
	for ent, lerr := range loc8.List(context.Background(), vfsv8.WithNameMatcher(func(s string) bool {
		return s != "" && s[len(s)-1] == 't'
	})) {
		require.NoError(t, lerr)
		names = append(names, ent.Name)
	}
	require.ElementsMatch(t, []string{"a.txt", "b.txt"}, names)
}

func TestLocation_List_RecursiveUnsupported(t *testing.T) {
	t.Parallel()

	mfs := mem.NewFileSystem()
	locV7, err := mfs.NewLocation("v", "/")
	require.NoError(t, err)

	loc8 := LocationFromV7(locV7).(*Location)
	for _, lerr := range loc8.List(context.Background(), vfsv8.WithRecursive(true)) {
		require.ErrorIs(t, lerr, vfsv8.ErrNotSupported)
	}
}

func TestFileSystem_RoundTrip(t *testing.T) {
	t.Parallel()

	mfs := mem.NewFileSystem()
	fs8 := FileSystemFromV7(mfs).(*FileSystem)

	back, ok := AsV7FileSystem(fs8)
	require.True(t, ok)
	require.Equal(t, mfs, back)

	loc, err := fs8.NewLocation("v", "/p/")
	require.NoError(t, err)
	require.NotNil(t, loc)

	f, err := fs8.NewFile("v", "/p/q.txt")
	require.NoError(t, err)
	require.NoError(t, f.Touch())

	st, err := f.Stat()
	require.NoError(t, err)
	require.Equal(t, "q.txt", st.Name())
}

func TestFileTransfers_CopyToLocation_NilDestination(t *testing.T) {
	t.Parallel()

	mfs := mem.NewFileSystem()
	src, err := mfs.NewFile("v", "/s.txt")
	require.NoError(t, err)
	require.NoError(t, src.Touch())

	tr := FileTransfersFromV7(src)
	_, err = tr.CopyToLocation(context.Background(), nil)
	require.Error(t, err)
}

func TestFileTransfers_CopyToLocation_NonAdapter(t *testing.T) {
	t.Parallel()

	mfs := mem.NewFileSystem()
	src, err := mfs.NewFile("v", "/s.txt")
	require.NoError(t, err)
	require.NoError(t, src.Touch())

	tr := FileTransfersFromV7(src)
	_, err = tr.CopyToLocation(context.Background(), nonAdapterLocation{})
	require.Error(t, err)
}

// nonAdapterLocation implements vfsv8.Location but is not from v7adapter.
type nonAdapterLocation struct{}

func (nonAdapterLocation) String() string { return "bogus://" }
func (nonAdapterLocation) URI() string    { return "bogus://" }
func (nonAdapterLocation) Path() string   { return "/" }
func (nonAdapterLocation) Authority() authority.Authority {
	return authority.Authority{}
}
func (nonAdapterLocation) List(context.Context, ...vfsv8.ListOption) iter.Seq2[vfsv8.Entry, error] {
	return func(yield func(vfsv8.Entry, error) bool) {}
}
func (nonAdapterLocation) Exists() (bool, error) { return true, nil }
func (nonAdapterLocation) NewLocation(string) (vfsv8.Location, error) {
	return nonAdapterLocation{}, nil
}
func (nonAdapterLocation) NewFile(string, ...vfsv8opt.NewFileOption) (vfsv8.File, error) {
	return nil, nil
}
func (nonAdapterLocation) DeleteFile(string, ...vfsv8opt.DeleteOption) error { return nil }
func (nonAdapterLocation) FileSystem() vfsv8.FileSystem                      { return nil }
