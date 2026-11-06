package iofs_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
	vfsos "github.com/c2fo/vfs/v7/backend/os"
	"github.com/c2fo/vfs/v7/iofs"
	"github.com/c2fo/vfs/v7/mocks"
)

var tree = map[string]string{
	"a.txt":           "alpha",
	"b.txt":           "bravo!",
	"css/site.css":    "body{}",
	"css/img/x.png":   "png",
	"deep/er/er/f.go": "package f",
}

type IOFSTestSuite struct {
	suite.Suite
}

func TestIOFSTestSuite(t *testing.T) { suite.Run(t, new(IOFSTestSuite)) }

func (s *IOFSTestSuite) memLocation() vfs.Location {
	loc, err := mem.NewFileSystem().NewLocation("vol", "/root/")
	s.Require().NoError(err)
	s.populate(loc)
	return loc
}

func (s *IOFSTestSuite) osLocation() vfs.Location {
	loc, err := vfsos.NewFileSystem().NewLocation("", filepath.Clean(s.T().TempDir())+string(filepath.Separator))
	s.Require().NoError(err)
	s.populate(loc)
	return loc
}

func (s *IOFSTestSuite) populate(loc vfs.Location) {
	for name, content := range tree {
		f, err := loc.NewFile(name)
		s.Require().NoError(err)
		_, err = io.WriteString(f, content)
		s.Require().NoError(err)
		s.Require().NoError(f.Close())
	}
}

func (s *IOFSTestSuite) backends() map[string]vfs.Location {
	return map[string]vfs.Location{"mem": s.memLocation(), "os": s.osLocation()}
}

func (s *IOFSTestSuite) TestFSTest() {
	expected := make([]string, 0, len(tree))
	for name := range tree {
		expected = append(expected, name)
	}
	for name, loc := range s.backends() {
		s.Run(name, func() {
			s.Require().NoError(fstest.TestFS(iofs.New(loc), expected...))
		})
	}
}

func (s *IOFSTestSuite) TestReadFileAndStat() {
	tests := []struct {
		name         string
		path         string
		expectedData string
		expectedErr  error
		expectedDir  bool
	}{
		{name: "top level file", path: "a.txt", expectedData: "alpha"},
		{name: "nested file", path: "css/site.css", expectedData: "body{}"},
		{name: "deeply nested file", path: "deep/er/er/f.go", expectedData: "package f"},
		{name: "directory", path: "css", expectedDir: true},
		{name: "root", path: ".", expectedDir: true},
		{name: "missing file", path: "nope.txt", expectedErr: fs.ErrNotExist},
		{name: "missing in missing dir", path: "nope/nope.txt", expectedErr: fs.ErrNotExist},
		{name: "rooted name", path: "/a.txt", expectedErr: fs.ErrInvalid},
		{name: "dotdot name", path: "../a.txt", expectedErr: fs.ErrInvalid},
		{name: "trailing slash", path: "css/", expectedErr: fs.ErrInvalid},
	}
	for backend, loc := range s.backends() {
		fsys := iofs.New(loc)
		for _, tt := range tests {
			s.Run(backend+"/"+tt.name, func() {
				info, err := fs.Stat(fsys, tt.path)
				if tt.expectedErr != nil {
					s.Require().Error(err)
					s.Require().ErrorIs(err, tt.expectedErr)
					var pe *fs.PathError
					s.Require().ErrorAs(err, &pe)
					s.Equal(tt.path, pe.Path)
					return
				}
				s.Require().NoError(err)
				s.Equal(tt.expectedDir, info.IsDir())
				if tt.expectedDir {
					s.Equal(fs.ModeDir, info.Mode().Type())
					s.NotZero(info.Mode().Perm())
					if tt.path == "." {
						s.Equal(fs.ModeDir|0o555, info.Mode())
					}
					s.Nil(info.Sys())
					s.Zero(info.Size())
					s.True(info.ModTime().IsZero())
					return
				}
				s.Equal(path.Base(tt.path), info.Name())
				s.EqualValues(len(tt.expectedData), info.Size())
				s.NotZero(info.Mode().Perm())
				_, ok := info.Sys().(vfs.File)
				s.True(ok, "Sys should return the vfs.File")

				data, err := fs.ReadFile(fsys, tt.path)
				s.Require().NoError(err)
				s.Equal(tt.expectedData, string(data))
			})
		}
	}
}

func (s *IOFSTestSuite) TestReadFileOfDirectory() {
	for backend, loc := range s.backends() {
		s.Run(backend, func() {
			_, err := fs.ReadFile(iofs.New(loc), "css")
			s.Require().Error(err)
			var pe *fs.PathError
			s.Require().ErrorAs(err, &pe)
		})
	}
}

func (s *IOFSTestSuite) TestReadDir() {
	for backend, loc := range s.backends() {
		fsys := iofs.New(loc)
		s.Run(backend+"/root lists files and directories sorted", func() {
			entries, err := fs.ReadDir(fsys, ".")
			s.Require().NoError(err)
			var got []string
			for _, e := range entries {
				if e.IsDir() {
					got = append(got, e.Name()+"/")
				} else {
					got = append(got, e.Name())
				}
			}
			s.Equal([]string{"a.txt", "b.txt", "css/", "deep/"}, got)
		})
		s.Run(backend+"/file is not a directory", func() {
			_, err := fs.ReadDir(fsys, "a.txt")
			s.Require().Error(err)
		})
		s.Run(backend+"/missing directory", func() {
			_, err := fs.ReadDir(fsys, "nope")
			s.Require().ErrorIs(err, fs.ErrNotExist)
		})
		s.Run(backend+"/batched reads", func() {
			f, err := fsys.Open(".")
			s.Require().NoError(err)
			defer func() { s.Require().NoError(f.Close()) }()
			d, ok := f.(fs.ReadDirFile)
			s.Require().True(ok)

			var sizes []int
			for {
				entries, err := d.ReadDir(3)
				if errors.Is(err, io.EOF) {
					break
				}
				s.Require().NoError(err)
				sizes = append(sizes, len(entries))
			}
			s.Equal([]int{3, 1}, sizes)
		})
		s.Run(backend+"/read on directory fails", func() {
			f, err := fsys.Open("css")
			s.Require().NoError(err)
			defer func() { s.Require().NoError(f.Close()) }()
			_, err = f.Read(make([]byte, 1))
			s.Require().Error(err)
		})
		s.Run(backend+"/closed directory", func() {
			f, err := fsys.Open("css")
			s.Require().NoError(err)
			s.Require().NoError(f.Close())
			s.Require().NoError(f.Close())
			_, err = f.(fs.ReadDirFile).ReadDir(-1)
			s.Require().ErrorIs(err, fs.ErrClosed)
		})
		s.Run(backend+"/walk visits everything once", func() {
			var files []string
			s.Require().NoError(fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					files = append(files, p)
				}
				return err
			}))
			s.Len(files, len(tree))
		})
	}
}

func (s *IOFSTestSuite) TestSub() {
	for backend, loc := range s.backends() {
		fsys := iofs.New(loc)
		s.Run(backend+"/sub then open", func() {
			sub, err := fs.Sub(fsys, "css")
			s.Require().NoError(err)
			data, err := fs.ReadFile(sub, "site.css")
			s.Require().NoError(err)
			s.Equal("body{}", string(data))
		})
		s.Run(backend+"/sub dot returns same filesystem", func() {
			sub, err := fsys.(fs.SubFS).Sub(".")
			s.Require().NoError(err)
			s.Same(fsys, sub)
		})
		s.Run(backend+"/sub of invalid name", func() {
			_, err := fsys.(fs.SubFS).Sub("../x")
			s.Require().ErrorIs(err, fs.ErrInvalid)
		})
	}
}

func (s *IOFSTestSuite) TestFileSemantics() {
	for backend, loc := range s.backends() {
		fsys := iofs.New(loc)
		s.Run(backend+"/seek and read", func() {
			f, err := fsys.Open("a.txt")
			s.Require().NoError(err)
			defer func() { s.Require().NoError(f.Close()) }()
			seeker, ok := f.(io.Seeker)
			s.Require().True(ok)
			_, err = seeker.Seek(2, io.SeekStart)
			s.Require().NoError(err)
			data, err := io.ReadAll(f)
			s.Require().NoError(err)
			s.Equal("pha", string(data))
		})
		s.Run(backend+"/file is read only", func() {
			f, err := fsys.Open("a.txt")
			s.Require().NoError(err)
			defer func() { s.Require().NoError(f.Close()) }()
			_, ok := f.(io.Writer)
			s.False(ok)
		})
		s.Run(backend+"/http range request", func() {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/a.txt", http.NoBody)
			req.Header.Set("Range", "bytes=1-2")
			http.FileServerFS(fsys).ServeHTTP(rec, req)
			s.Equal(http.StatusPartialContent, rec.Code)
			s.Equal("lp", rec.Body.String())
		})
	}
}

func (s *IOFSTestSuite) TestContext() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for backend, loc := range s.backends() {
		s.Run(backend, func() {
			fsys := iofs.New(loc, iofs.WithContext(ctx))
			_, err := fs.Stat(fsys, "a.txt")
			s.Require().ErrorIs(err, context.Canceled)
			_, err = fs.ReadDir(fsys, ".")
			s.Require().ErrorIs(err, context.Canceled)
		})
	}
	s.Run("nil context keeps the default", func() {
		fsys := iofs.New(s.memLocation(), iofs.WithContext(nil)) //nolint:staticcheck // nil is deliberately passed
		_, err := fs.Stat(fsys, "a.txt")
		s.Require().NoError(err)
	})
}

func (s *IOFSTestSuite) TestFileWinsOverDirectory() {
	loc := s.memLocation()
	for name, content := range map[string]string{"foo": "file", "foo/bar": "child"} {
		f, err := loc.NewFile(name)
		s.Require().NoError(err)
		_, err = io.WriteString(f, content)
		s.Require().NoError(err)
		s.Require().NoError(f.Close())
	}
	fsys := iofs.New(loc)
	info, err := fs.Stat(fsys, "foo")
	s.Require().NoError(err)
	s.False(info.IsDir())
	s.True(strings.HasPrefix(info.Name(), "foo"))

	entries, err := fs.ReadDir(fsys, ".")
	s.Require().NoError(err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	s.Equal(1, countOf(names, "foo"), "the name is listed once")
}

func (s *IOFSTestSuite) TestSymlinkIsNotFollowed() {
	if runtime.GOOS == "windows" {
		s.T().Skip("creating symbolic links needs privileges on Windows")
	}
	loc := s.osLocation()
	s.Require().NoError(os.Symlink(filepath.Join(loc.Path(), "css"), filepath.Join(loc.Path(), "link")))

	info, err := fs.Stat(iofs.New(loc), "link")
	s.Require().NoError(err)
	s.False(info.IsDir())
	s.NotZero(info.Mode() & fs.ModeSymlink)
}

func (s *IOFSTestSuite) TestMissingRoot() {
	loc, err := vfsos.NewFileSystem().NewLocation("", filepath.Join(filepath.Clean(s.T().TempDir()), "missing")+string(filepath.Separator))
	s.Require().NoError(err)
	fsys := iofs.New(loc)

	entries, err := fs.ReadDir(fsys, ".")
	s.Require().NoError(err)
	s.Empty(entries)
	_, err = fs.Stat(fsys, "a.txt")
	s.Require().ErrorIs(err, fs.ErrNotExist)
}

func (s *IOFSTestSuite) TestFileModeFromBackend() {
	if runtime.GOOS == "windows" {
		s.T().Skip("file permission bits are not reported on Windows")
	}
	loc := s.osLocation()
	s.Require().NoError(os.Chmod(filepath.Join(loc.Path(), "a.txt"), 0o600))

	info, err := fs.Stat(iofs.New(loc), "a.txt")
	s.Require().NoError(err)
	s.Equal(fs.FileMode(0o600), info.Mode())
}

// failingLocation returns a mock location with no native listing, so list.Entries uses its List and ListByPrefix.
func (s *IOFSTestSuite) failingLocation(listErr error) *mocks.Location {
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().Path().Return("/root/").Maybe()
	loc.EXPECT().List().Return(nil, listErr).Maybe()
	loc.EXPECT().ListByPrefix(mock.Anything).Return(nil, listErr).Maybe()
	return loc
}

func (s *IOFSTestSuite) TestListingErrors() {
	boom := errors.New("backend unavailable")
	fsys := iofs.New(s.failingLocation(boom))

	tests := []struct {
		name string
		call func() error
	}{
		{name: "open", call: func() error { _, err := fsys.Open("a.txt"); return err }},
		{name: "stat", call: func() error { _, err := fs.Stat(fsys, "a.txt"); return err }},
		{name: "read dir", call: func() error { _, err := fs.ReadDir(fsys, "."); return err }},
		{name: "read file", call: func() error { _, err := fs.ReadFile(fsys, "a.txt"); return err }},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.call()
			s.Require().ErrorIs(err, boom)
			var pe *fs.PathError
			s.Require().ErrorAs(err, &pe)
		})
	}
}

func (s *IOFSTestSuite) TestSubWithInvalidLocation() {
	boom := errors.New("bad path")
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().NewLocation("css/").Return(nil, boom)

	_, err := iofs.New(loc).(fs.SubFS).Sub("css")
	s.Require().ErrorIs(err, fs.ErrInvalid)
	s.Require().ErrorIs(err, boom)
}

func (s *IOFSTestSuite) TestLookupWithInvalidParent() {
	boom := errors.New("bad path")
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().NewLocation("css/").Return(nil, boom)

	_, err := fs.Stat(iofs.New(loc), "css/site.css")
	s.Require().ErrorIs(err, fs.ErrInvalid)
}

func (s *IOFSTestSuite) TestReadFileReadError() {
	boom := errors.New("read failed")
	file := mocks.NewFile(s.T())
	file.EXPECT().Read(mock.Anything).Return(0, boom)
	file.EXPECT().Close().Return(nil)

	loc := mocks.NewLocation(s.T())
	loc.EXPECT().Path().Return("/root/").Maybe()
	loc.EXPECT().ListByPrefix("a.txt").Return([]string{"a.txt"}, nil)
	loc.EXPECT().NewFile("a.txt").Return(file, nil)

	_, err := fs.ReadFile(iofs.New(loc), "a.txt")
	s.Require().ErrorIs(err, boom)
}

func countOf(names []string, name string) int {
	n := 0
	for _, x := range names {
		if x == name {
			n++
		}
	}
	return n
}
