package mem

import (
	"context"
	"io"
	iofs "io/fs"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	vfs "github.com/c2fo/vfs/v8"
)

type memSuite struct {
	suite.Suite
	fs *FileSystem
}

func (s *memSuite) SetupTest() {
	s.fs = NewFileSystem()
}

func TestMemSuite(t *testing.T) {
	suite.Run(t, new(memSuite))
}

func (s *memSuite) TestNewFile_Touch_Read() {
	f, err := s.fs.NewFile("vol", "/dir/a.txt")
	s.Require().NoError(err)
	s.Require().NoError(f.Touch())

	n, err := f.Write([]byte("hello"))
	s.Require().NoError(err)
	s.Require().Equal(5, n)
	s.Require().NoError(f.Close())

	ok, err := f.Exists()
	s.Require().NoError(err)
	s.Require().True(ok)

	st, err := f.Stat()
	s.Require().NoError(err)
	s.Require().Equal("a.txt", st.Name())
	s.Require().Equal(int64(5), st.Size())
}

func (s *memSuite) TestLocation_List() {
	loc, err := s.fs.NewLocation("v", "/data/")
	s.Require().NoError(err)

	f, err := s.fs.NewFile("v", "/data/x.txt")
	s.Require().NoError(err)
	s.Require().NoError(f.Touch())
	s.Require().NoError(f.Close())

	var names []string
	for ent, err := range loc.List(context.Background()) {
		s.Require().NoError(err)
		names = append(names, ent.Name)
	}
	s.Require().Equal([]string{"x.txt"}, names)
}

func (s *memSuite) TestLocation_DeleteFile() {
	loc, err := s.fs.NewLocation("v", "/d/")
	s.Require().NoError(err)

	f, err := loc.NewFile("gone.txt")
	s.Require().NoError(err)
	s.Require().NoError(f.Touch())
	s.Require().NoError(f.Close())

	s.Require().NoError(loc.DeleteFile("gone.txt"))

	f2, err := loc.NewFile("gone.txt")
	s.Require().NoError(err)
	ok, err := f2.Exists()
	s.Require().NoError(err)
	s.Require().False(ok)
}

func (s *memSuite) TestRead_NotExist() {
	f, err := s.fs.NewFile("v", "/nope.txt")
	s.Require().NoError(err)

	_, err = f.Read(make([]byte, 10))
	s.Require().Error(err)
	s.Require().ErrorIs(err, iofs.ErrNotExist)
}

func TestFile_Stat_NotExist(t *testing.T) {
	t.Parallel()

	mfs := NewFileSystem()
	f, err := mfs.NewFile("v", "/only.txt")
	require.NoError(t, err)

	_, err = f.Stat()
	require.ErrorIs(t, err, iofs.ErrNotExist)
}

func TestLocation_List_RecursiveUnsupported(t *testing.T) {
	t.Parallel()

	fs := NewFileSystem()
	loc, err := fs.NewLocation("v", "/")
	require.NoError(t, err)

	for _, err := range loc.List(context.Background(), vfs.WithRecursive(true)) {
		require.ErrorIs(t, err, vfs.ErrNotSupported)
	}
}

func TestFile_Seek_Read(t *testing.T) {
	t.Parallel()

	fs := NewFileSystem()
	f, err := fs.NewFile("v", "/s.txt")
	require.NoError(t, err)
	require.NoError(t, f.Touch())
	_, err = f.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.NoError(t, f.Touch())
	seeker, ok := f.(io.Seeker)
	require.True(t, ok)
	_, err = seeker.Seek(0, io.SeekStart)
	require.NoError(t, err)

	buf := make([]byte, 5)
	n, err := f.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, "hello", string(buf))
}
