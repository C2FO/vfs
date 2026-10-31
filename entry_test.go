package vfs_test

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/mocks"
)

type EntryTestSuite struct {
	suite.Suite
}

func TestEntryTestSuite(t *testing.T) {
	suite.Run(t, new(EntryTestSuite))
}

func (s *EntryTestSuite) parent(path string) *mocks.Location {
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().Path().Return(path).Maybe()
	return loc
}

func (s *EntryTestSuite) file(parent vfs.Location, rel string, opts ...vfs.EntryOption) vfs.Entry {
	s.T().Helper()
	e, err := vfs.NewFileEntry(parent, rel, opts...)
	s.Require().NoError(err)
	return e
}

func (s *EntryTestSuite) dir(parent vfs.Location, rel string, opts ...vfs.EntryOption) vfs.Entry {
	s.T().Helper()
	e, err := vfs.NewDirEntry(parent, rel, opts...)
	s.Require().NoError(err)
	return e
}

func (s *EntryTestSuite) TestNamesAndPaths() {
	tests := []struct {
		name         string
		parentPath   string
		rel          string
		dir          bool
		expectedName string
		expectedRel  string
		expectedPath string
	}{
		{
			name: "nested directory", parentPath: "/root/", rel: "a/b", dir: true,
			expectedName: "b", expectedRel: "a/b", expectedPath: "/root/a/b/",
		},
		{
			name: "directory with trailing slash input", parentPath: "/root/", rel: "a/b/", dir: true,
			expectedName: "b", expectedRel: "a/b", expectedPath: "/root/a/b/",
		},
		{
			name: "file in root location", parentPath: "/", rel: "f.txt",
			expectedName: "f.txt", expectedRel: "f.txt", expectedPath: "/f.txt",
		},
		{
			name: "nested file", parentPath: "/root/", rel: "a/f.txt",
			expectedName: "f.txt", expectedRel: "a/f.txt", expectedPath: "/root/a/f.txt",
		},
		{
			name: "file whose name looks like an extension", parentPath: "/root/", rel: ".hidden",
			expectedName: ".hidden", expectedRel: ".hidden", expectedPath: "/root/.hidden",
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			build := s.file
			if tt.dir {
				build = s.dir
			}
			e := build(s.parent(tt.parentPath), tt.rel)
			s.Equal(tt.expectedName, e.Name())
			s.Equal(tt.expectedRel, e.RelPath())
			s.Equal(tt.expectedPath, e.Path())
			s.Equal(tt.dir, e.IsDir())
			s.True(fs.ValidPath(e.RelPath()))
		})
	}
}

func (s *EntryTestSuite) TestZeroEntry() {
	var e vfs.Entry
	s.Empty(e.Name())
	s.Empty(e.RelPath())
	s.False(e.IsDir())
	s.Equal("/", e.Path())
	s.Zero(e.Mode())
	s.False(e.HasMetadata())
}

func (s *EntryTestSuite) TestConstructorValidation() {
	tests := []struct {
		name  string
		rel   string
		dir   bool
		valid bool
	}{
		{name: "file simple", rel: "a.txt", valid: true},
		{name: "file nested", rel: "a/b/c.txt", valid: true},
		{name: "directory trailing slash", rel: "a/", dir: true, valid: true},
		{name: "file empty", rel: ""},
		{name: "directory empty", rel: "", dir: true},
		{name: "directory only slash", rel: "/", dir: true},
		{name: "file dot", rel: "."},
		{name: "directory dot", rel: "./", dir: true},
		{name: "file parent escape", rel: "../x"},
		{name: "directory parent escape", rel: "a/../..", dir: true},
		{name: "file leading slash", rel: "/a.txt"},
		{name: "file trailing slash", rel: "a/"},
		{name: "directory two trailing slashes", rel: "a//", dir: true},
		{name: "empty middle segment", rel: "a//b"},
		{name: "dot segment", rel: "a/./b"},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			ctor := vfs.NewFileEntry
			if tt.dir {
				ctor = vfs.NewDirEntry
			}
			e, err := ctor(s.parent("/"), tt.rel)
			if tt.valid {
				s.Require().NoError(err)
				s.Equal(tt.dir, e.IsDir())
				return
			}
			s.Require().ErrorIs(err, fs.ErrInvalid)
			var pe *fs.PathError
			s.Require().ErrorAs(err, &pe)
			s.Equal("list", pe.Op)
			s.Equal(tt.rel, pe.Path)
			s.Equal(vfs.Entry{}.Name(), e.Name())
		})
	}

	s.Run("nil parent", func() {
		_, err := vfs.NewFileEntry(nil, "a.txt")
		s.Require().Error(err)
		var pe *fs.PathError
		s.Require().ErrorAs(err, &pe)
		s.Equal("list", pe.Op)
	})
}

func (s *EntryTestSuite) TestTypeAndMode() {
	tests := []struct {
		name         string
		entry        vfs.Entry
		expectedType fs.FileMode
		expectedMode fs.FileMode
	}{
		{
			name:         "directory",
			entry:        s.dir(s.parent("/"), "d", vfs.WithEntryMode(0o755)),
			expectedType: fs.ModeDir, expectedMode: fs.ModeDir | 0o755,
		},
		{
			name:         "directory without reported mode",
			entry:        s.dir(s.parent("/"), "d"),
			expectedType: fs.ModeDir, expectedMode: fs.ModeDir,
		},
		{
			name:         "directory ignores a supplied dir bit",
			entry:        s.dir(s.parent("/"), "d", vfs.WithEntryMode(fs.ModeDir|0o700)),
			expectedType: fs.ModeDir, expectedMode: fs.ModeDir | 0o700,
		},
		{
			name:         "directory ignores a supplied symlink bit",
			entry:        s.dir(s.parent("/"), "d", vfs.WithEntryMode(fs.ModeSymlink|0o700)),
			expectedType: fs.ModeDir, expectedMode: fs.ModeDir | 0o700,
		},
		{
			name:         "regular file",
			entry:        s.file(s.parent("/"), "f", vfs.WithEntryMode(0o644)),
			expectedType: 0, expectedMode: 0o644,
		},
		{
			name:         "symlink",
			entry:        s.file(s.parent("/"), "l", vfs.WithEntryMode(fs.ModeSymlink|0o777)),
			expectedType: fs.ModeSymlink, expectedMode: fs.ModeSymlink | 0o777,
		},
		{
			name:         "setuid, setgid and sticky bits are kept",
			entry:        s.file(s.parent("/"), "f", vfs.WithEntryMode(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky|0o755)),
			expectedType: 0, expectedMode: fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky | 0o755,
		},
		{
			name:         "no mode reported",
			entry:        s.file(s.parent("/"), "f"),
			expectedType: 0, expectedMode: 0,
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.Equal(tt.expectedType, tt.entry.Type())
			s.Equal(tt.expectedMode, tt.entry.Mode())
		})
	}
}

func (s *EntryTestSuite) TestMetadataAccessors() {
	mod := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	native := struct{ Key string }{Key: "a/f.txt"}

	e := s.file(s.parent("/"), "a/f.txt", vfs.WithEntryMetadata(42, mod), vfs.WithEntrySys(native))
	s.True(e.HasMetadata())
	s.Equal(uint64(42), e.Size())
	s.Equal(mod, e.ModTime())
	s.Equal(native, e.Sys())

	info, err := e.Info()
	s.Require().NoError(err)
	s.Equal(native, info.Sys())

	bare := s.dir(s.parent("/"), "a")
	s.False(bare.HasMetadata())
	s.Zero(bare.Size())
	s.True(bare.ModTime().IsZero())
	s.Nil(bare.Sys())
}

func (s *EntryTestSuite) TestResolution() {
	s.Run("file resolves to its file without further requests", func() {
		loc := s.parent("/root/")
		f := mocks.NewFile(s.T())
		loc.EXPECT().NewFile("a/f.txt").Return(f, nil).Once()

		got, err := s.file(loc, "a/f.txt").File()
		s.Require().NoError(err)
		s.Same(f, got)
	})
	s.Run("directory resolves to its location", func() {
		loc := s.parent("/root/")
		sub := mocks.NewLocation(s.T())
		loc.EXPECT().NewLocation("a/b/").Return(sub, nil).Once()

		got, err := s.dir(loc, "a/b").Location()
		s.Require().NoError(err)
		s.Same(sub, got)
	})
	s.Run("File on a directory errors", func() {
		_, err := s.dir(s.parent("/"), "d").File()
		s.Require().Error(err)
	})
	s.Run("Location on a file errors", func() {
		_, err := s.file(s.parent("/"), "f").Location()
		s.Require().Error(err)
	})
	s.Run("zero entry errors", func() {
		_, err := vfs.Entry{}.File()
		s.Require().Error(err)
		_, err = vfs.Entry{}.Location()
		s.Require().Error(err)
	})
}

func (s *EntryTestSuite) TestInfoMakesNoRequestWhenMetadataListed() {
	mod := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	loc := s.parent("/root/") // any NewFile call would fail: no expectation is set

	tests := []struct {
		name         string
		entry        vfs.Entry
		expectedSize int64
		expectedMod  time.Time
		expectedDir  bool
	}{
		{
			name:         "listed file",
			entry:        s.file(loc, "f.txt", vfs.WithEntryMetadata(7, mod), vfs.WithEntryMode(0o600)),
			expectedSize: 7, expectedMod: mod,
		},
		{name: "directory", entry: s.dir(loc, "d"), expectedDir: true},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			info, err := tt.entry.Info()
			s.Require().NoError(err)
			s.Equal(tt.entry.Name(), info.Name())
			s.Equal(tt.expectedSize, info.Size())
			s.Equal(tt.expectedMod, info.ModTime())
			s.Equal(tt.expectedDir, info.IsDir())
			s.Equal(tt.entry.Mode(), info.Mode())
		})
	}
}

func (s *EntryTestSuite) TestInfoFetchesOnDemand() {
	mod := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		modTime     *time.Time
		expectedMod time.Time
	}{
		{name: "size and modification time", modTime: &mod, expectedMod: mod},
		{name: "backend reports no modification time", modTime: nil},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			loc := s.parent("/root/")
			f := mocks.NewFile(s.T())
			loc.EXPECT().NewFile("f.txt").Return(f, nil).Once()
			f.EXPECT().Size().Return(uint64(11), nil).Once()
			f.EXPECT().LastModified().Return(tt.modTime, nil).Once()

			info, err := s.file(loc, "f.txt").Info()
			s.Require().NoError(err)
			s.Equal(int64(11), info.Size())
			s.Equal(tt.expectedMod, info.ModTime())
			s.False(info.IsDir())
		})
	}
}

func (s *EntryTestSuite) TestInfoErrors() {
	statErr := errors.New("backend unavailable")
	tests := []struct {
		name       string
		setup      func(f *mocks.File)
		expectedIs []error
	}{
		{
			name: "deleted after listing keeps the backend error too",
			setup: func(f *mocks.File) {
				f.EXPECT().Size().Return(uint64(0), statErr).Once()
				f.EXPECT().Exists().Return(false, nil).Once()
			},
			expectedIs: []error{fs.ErrNotExist, statErr},
		},
		{
			name: "backend error while file exists",
			setup: func(f *mocks.File) {
				f.EXPECT().Size().Return(uint64(0), statErr).Once()
				f.EXPECT().Exists().Return(true, nil).Once()
			},
			expectedIs: []error{statErr},
		},
		{
			name: "backend error and existence unknown",
			setup: func(f *mocks.File) {
				f.EXPECT().Size().Return(uint64(0), statErr).Once()
				f.EXPECT().Exists().Return(false, errors.New("also failing")).Once()
			},
			expectedIs: []error{statErr},
		},
		{
			name: "modification time fails",
			setup: func(f *mocks.File) {
				f.EXPECT().Size().Return(uint64(1), nil).Once()
				f.EXPECT().LastModified().Return(nil, statErr).Once()
				f.EXPECT().Exists().Return(true, nil).Once()
			},
			expectedIs: []error{statErr},
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			loc := s.parent("/root/")
			f := mocks.NewFile(s.T())
			loc.EXPECT().NewFile("f.txt").Return(f, nil).Once()
			tt.setup(f)

			_, err := s.file(loc, "f.txt").Info()
			s.Require().Error(err)
			for _, target := range tt.expectedIs {
				s.Require().ErrorIs(err, target)
			}

			var pe *fs.PathError
			s.Require().ErrorAs(err, &pe)
			s.Equal("stat", pe.Op)
			s.Equal("/root/f.txt", pe.Path)
		})
	}

	s.Run("file cannot be created", func() {
		loc := s.parent("/root/")
		newFileErr := errors.New("bad name")
		loc.EXPECT().NewFile("f.txt").Return(nil, newFileErr).Once()

		_, err := s.file(loc, "f.txt").Info()
		s.Require().ErrorIs(err, newFileErr)
		var pe *fs.PathError
		s.Require().ErrorAs(err, &pe)
		s.Equal("stat", pe.Op)
	})

	s.Run("canceled context makes no request", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		cancel()

		_, err := s.file(s.parent("/root/"), "f.txt").InfoContext(ctx)
		s.Require().ErrorIs(err, context.Canceled)
		var pe *fs.PathError
		s.Require().ErrorAs(err, &pe)
		s.Equal("stat", pe.Op)
	})

	s.Run("cancellation during Size starts no further request", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		defer cancel()
		loc := s.parent("/root/")
		f := mocks.NewFile(s.T())
		loc.EXPECT().NewFile("f.txt").Return(f, nil).Once()
		f.EXPECT().Size().RunAndReturn(func() (uint64, error) {
			cancel()
			return 3, nil
		}).Once()

		_, err := s.file(loc, "f.txt").InfoContext(ctx)

		s.Require().ErrorIs(err, context.Canceled)
		var pe *fs.PathError
		s.Require().ErrorAs(err, &pe)
		s.Equal("stat", pe.Op)
	})

	s.Run("cancellation during LastModified does not check whether the file exists", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		defer cancel()
		boom := errors.New("request canceled")
		loc := s.parent("/root/")
		f := mocks.NewFile(s.T())
		loc.EXPECT().NewFile("f.txt").Return(f, nil).Once()
		f.EXPECT().Size().Return(3, nil).Once()
		f.EXPECT().LastModified().RunAndReturn(func() (*time.Time, error) {
			cancel()
			return nil, boom
		}).Once()

		_, err := s.file(loc, "f.txt").InfoContext(ctx)

		s.Require().ErrorIs(err, context.Canceled)
		s.Require().ErrorIs(err, boom)
	})

	s.Run("canceled context ignored when metadata was listed", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		cancel()

		e := s.file(s.parent("/root/"), "f.txt", vfs.WithEntryMetadata(3, time.Now()))
		info, err := e.InfoContext(ctx)
		s.Require().NoError(err)
		s.Equal(int64(3), info.Size())
	})

	s.Run("size beyond int64", func() {
		loc := s.parent("/root/")
		f := mocks.NewFile(s.T())
		loc.EXPECT().NewFile("big.bin").Return(f, nil).Once()
		f.EXPECT().Size().Return(uint64(math.MaxInt64)+1, nil).Once()
		mod := time.Now()
		f.EXPECT().LastModified().Return(&mod, nil).Once()

		_, err := s.file(loc, "big.bin").Info()
		s.Require().Error(err)
		var pe *fs.PathError
		s.Require().ErrorAs(err, &pe)
		s.Equal("stat", pe.Op)
	})

	s.Run("listed size beyond int64", func() {
		e := s.file(s.parent("/"), "big.bin", vfs.WithEntryMetadata(uint64(math.MaxInt64)+1, time.Now()))
		_, err := e.Info()
		s.Require().Error(err)
	})

	s.Run("zero entry", func() {
		_, err := vfs.Entry{}.Info()
		s.Require().Error(err)
	})
}
