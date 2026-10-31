package os

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

type ListEntriesTestSuite struct {
	suite.Suite
	dir string
	loc vfs.Location
}

func TestListEntriesTestSuite(t *testing.T) {
	suite.Run(t, new(ListEntriesTestSuite))
}

func (s *ListEntriesTestSuite) SetupTest() {
	s.dir = filepath.Clean(s.T().TempDir()) + string(filepath.Separator)
	loc, err := NewFileSystem().NewLocation("", s.dir)
	s.Require().NoError(err)
	s.loc = loc
}

func (s *ListEntriesTestSuite) write(rel, body string) {
	s.T().Helper()
	full := filepath.Join(s.dir, filepath.FromSlash(rel))
	s.Require().NoError(os.MkdirAll(filepath.Dir(full), 0o750))
	s.Require().NoError(os.WriteFile(full, []byte(body), 0o600))
}

func (s *ListEntriesTestSuite) entries(opts ...options.ListOption) []vfs.Entry {
	s.T().Helper()
	entries, err := list.Collect(list.Entries(s.T().Context(), s.loc, opts...))
	s.Require().NoError(err)
	return entries
}

func keysOf(entries []vfs.Entry) []string {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func (s *ListEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.loc)
}

func (s *ListEntriesTestSuite) TestListsFilesAndDirectories() {
	s.write("a.txt", "a")
	s.write("a-b.txt", "ab")
	s.write("sub/b.txt", "bb")
	s.write("sub/deep/c.txt", "ccc")

	s.Equal([]string{"a-b.txt", "a.txt", "sub/", "sub/b.txt", "sub/deep/", "sub/deep/c.txt"}, keysOf(s.entries(list.Recursive())))
	s.Equal([]string{"a-b.txt", "a.txt", "sub/"}, keysOf(s.entries()))
}

func (s *ListEntriesTestSuite) TestEmptyDirectoryIsListed() {
	s.Require().NoError(os.MkdirAll(filepath.Join(s.dir, "empty"), 0o750))
	s.write("a.txt", "a")

	s.Equal([]string{"a.txt", "empty/"}, keysOf(s.entries(list.Recursive())))
}

func (s *ListEntriesTestSuite) TestFileEntriesCarryTheOperatingSystemsMetadata() {
	s.write("a.txt", "four")
	info, err := os.Stat(filepath.Join(s.dir, "a.txt"))
	s.Require().NoError(err)

	entries := s.entries()
	s.Require().Len(entries, 1)
	e := entries[0]
	s.True(e.HasMetadata())
	s.Equal(uint64(4), e.Size())
	s.True(info.ModTime().Equal(e.ModTime()))
	s.NotNil(e.Sys())
	if runtime.GOOS != "windows" {
		s.Equal(fs.FileMode(0o600), e.Mode())
	}
}

func (s *ListEntriesTestSuite) TestRelativePathsUseForwardSlashes() {
	s.write("a/b/c.txt", "x")
	for _, e := range s.entries(list.Recursive()) {
		s.NotContains(e.RelPath(), `\`)
	}
}

func (s *ListEntriesTestSuite) TestSymlinksAreReportedAndNotFollowed() {
	if runtime.GOOS == "windows" {
		s.T().Skip("creating symlinks needs privileges on Windows")
	}
	s.write("target/inside.txt", "x")
	s.write("real.txt", "x")
	s.Require().NoError(os.Symlink(filepath.Join(s.dir, "real.txt"), filepath.Join(s.dir, "link.txt")))
	s.Require().NoError(os.Symlink(filepath.Join(s.dir, "target"), filepath.Join(s.dir, "linkdir")))

	entries := s.entries(list.Recursive())
	s.Equal([]string{"link.txt", "linkdir", "real.txt", "target/", "target/inside.txt"}, keysOf(entries))
	for _, e := range entries {
		switch e.RelPath() {
		case "link.txt", "linkdir":
			s.False(e.IsDir(), "%s: a link is not followed, so it is not a directory", e.RelPath())
			s.Equal(fs.ModeSymlink, e.Type(), e.RelPath())
		default:
			s.Zero(e.Type()&fs.ModeSymlink, e.RelPath())
		}
	}
}

func (s *ListEntriesTestSuite) TestUnreadableDirectoryIsAPartialFailure() {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		s.T().Skip("needs a user that permission bits apply to")
	}
	s.write("a/1.txt", "x")
	s.write("b/2.txt", "x")
	s.write("c/3.txt", "x")
	denied := filepath.Join(s.dir, "b")
	s.Require().NoError(os.Chmod(denied, 0o000))
	s.T().Cleanup(func() { _ = os.Chmod(denied, 0o750) }) //nolint:gosec // a directory needs its execute bit to be removed

	var keys []string
	var failed []vfs.Entry
	for e, err := range list.Entries(s.T().Context(), s.loc, list.Recursive()) {
		if err != nil {
			s.Require().ErrorIs(err, fs.ErrPermission)
			failed = append(failed, e)
			continue
		}
		keys = append(keys, list.OrderKey(e))
	}

	s.Equal([]string{"a/", "a/1.txt", "b/", "c/", "c/3.txt"}, keys, "the listing continues past the unreadable directory")
	s.Require().Len(failed, 1)
	s.Equal("b", failed[0].RelPath(), "the error comes with the directory that could not be read")
}

func (s *ListEntriesTestSuite) TestMissingLocationListsNothing() {
	loc, err := NewFileSystem().NewLocation("", filepath.Join(s.dir, "nowhere")+string(filepath.Separator))
	s.Require().NoError(err)

	entries, err := list.Collect(list.Entries(s.T().Context(), loc, list.Recursive()))
	s.Require().NoError(err)
	s.Empty(entries)
}

func (s *ListEntriesTestSuite) TestLocationThatIsAFileIsAnError() {
	s.write("file.txt", "x")
	loc, err := NewFileSystem().NewLocation("", filepath.Join(s.dir, "file.txt")+string(filepath.Separator))
	s.Require().NoError(err)

	_, err = list.Collect(list.Entries(s.T().Context(), loc))
	s.Require().Error(err)
	var pe *fs.PathError
	s.Require().ErrorAs(err, &pe)
	s.Equal("list", pe.Op)
}

func (s *ListEntriesTestSuite) TestInvalidOptionsAreReportedAsListErrors() {
	var errs []error
	for _, err := range s.loc.(vfs.EntryLister).ListEntries(s.T().Context(), list.FilesOnly(), list.DirsOnly()) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], list.ErrInvalidOption)
	var pe *fs.PathError
	s.Require().ErrorAs(errs[0], &pe)
	s.Equal("list", pe.Op)
}

func (s *ListEntriesTestSuite) TestCanceledContext() {
	s.write("a.txt", "x")
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	var errs []error
	for _, err := range s.loc.(vfs.EntryLister).ListEntries(ctx) {
		errs = append(errs, err)
	}
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], context.Canceled)
}

func (s *ListEntriesTestSuite) TestSubtreeNotRequestedIsNotRead() {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		s.T().Skip("needs a user that permission bits apply to")
	}
	s.write("a/1.txt", "x")
	s.write("b/2.txt", "x")
	denied := filepath.Join(s.dir, "b")
	s.Require().NoError(os.Chmod(denied, 0o000))
	s.T().Cleanup(func() { _ = os.Chmod(denied, 0o750) }) //nolint:gosec // a directory needs its execute bit to be removed

	// A prefix that excludes b, and a depth that stops above it, never try to read it.
	s.Equal([]string{"a/", "a/1.txt"}, keysOf(s.entries(list.Recursive(), list.Prefix("a"))))
	s.Equal([]string{"a/", "b/"}, keysOf(s.entries(list.Recursive(), list.MaxDepth(1))))
}
