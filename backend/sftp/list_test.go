package sftp

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/sftp/mocks"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils/authority"
)

var listModTime = time.Unix(1_700_000_000, 0)

type listEntriesTestSuite struct {
	suite.Suite
	client *mocks.Client
	fs     *FileSystem
}

func TestListEntriesTestSuite(t *testing.T) {
	suite.Run(t, new(listEntriesTestSuite))
}

func (s *listEntriesTestSuite) SetupTest() {
	s.client = mocks.NewClient(s.T())
	s.fs = &FileSystem{sftpclient: s.client}
}

func (s *listEntriesTestSuite) location() vfs.Location {
	s.T().Helper()
	loc, err := s.fs.NewLocation("host.com", "/dir1/")
	s.Require().NoError(err)
	return loc
}

func (s *listEntriesTestSuite) keys(loc vfs.Location, opts ...options.ListOption) []string {
	s.T().Helper()
	entries, err := list.Collect(list.Entries(s.T().Context(), loc, opts...))
	s.Require().NoError(err)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func (s *listEntriesTestSuite) info(name string, dir bool, size int64, mode os.FileMode) os.FileInfo {
	fi := mocks.NewFileInfo(s.T())
	fi.EXPECT().Name().Return(name).Maybe()
	fi.EXPECT().IsDir().Return(dir).Maybe()
	fi.EXPECT().Size().Return(size).Maybe()
	fi.EXPECT().ModTime().Return(listModTime).Maybe()
	fi.EXPECT().Mode().Return(mode).Maybe()
	return fi
}

func (s *listEntriesTestSuite) file(name string) os.FileInfo {
	return s.info(name, false, int64(len(name)), 0o644)
}
func (s *listEntriesTestSuite) dir(name string) os.FileInfo {
	return s.info(name, true, 0, os.ModeDir|0o755)
}

// expectDir expects one ReadDir of dir.
func (s *listEntriesTestSuite) expectDir(dir string, infos ...os.FileInfo) {
	s.client.EXPECT().ReadDir(dir).Return(infos, nil).Once()
}

func (s *listEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location())
}

func (s *listEntriesTestSuite) TestRecursiveListingReadsEachDirectoryOnce() {
	s.expectDir("/dir1", s.file("z.txt"), s.dir("logs"), s.file("a.txt"))
	s.expectDir("/dir1/logs", s.dir("old"), s.file("app.log"))
	s.expectDir("/dir1/logs/old", s.file("2023.log"))

	s.Equal([]string{"a.txt", "logs/", "logs/app.log", "logs/old/", "logs/old/2023.log", "z.txt"},
		s.keys(s.location(), list.Recursive()))
}

func (s *listEntriesTestSuite) TestNonRecursiveListingReadsOnlyTheLocation() {
	s.expectDir("/dir1", s.file("a.txt"), s.dir("logs"))

	s.Equal([]string{"a.txt", "logs/"}, s.keys(s.location()))
}

func (s *listEntriesTestSuite) TestAFileSortsBeforeADirectoryItSharesAPrefixWith() {
	s.expectDir("/dir1", s.dir("logs"), s.file("logs-2024.gz"))

	s.Equal([]string{"logs-2024.gz", "logs/"}, s.keys(s.location()))
}

func (s *listEntriesTestSuite) TestDirectoriesOutsideThePrefixAreNotRead() {
	s.expectDir("/dir1", s.dir("logs"), s.dir("other"))
	s.expectDir("/dir1/logs", s.file("app.log"))

	s.Equal([]string{"logs/", "logs/app.log"}, s.keys(s.location(), list.Recursive(), list.Prefix("logs")))
}

func (s *listEntriesTestSuite) TestStoppingEarlyReadsNoFurtherDirectories() {
	s.expectDir("/dir1", s.dir("a"), s.dir("b"))
	s.expectDir("/dir1/a", s.file("x.txt"))

	for e, err := range s.location().(vfs.EntryLister).ListEntries(s.T().Context(), list.Recursive()) {
		s.Require().NoError(err)
		if !e.IsDir() {
			break
		}
	}
	// The mock fails the test if /dir1/b is read, or if /dir1/a is not.
}

func (s *listEntriesTestSuite) TestADirectoryIsReadOnlyWhenTheListingReachesIt() {
	s.expectDir("/dir1", s.dir("a"), s.dir("b"))

	for _, err := range s.location().(vfs.EntryLister).ListEntries(s.T().Context(), list.Recursive()) {
		s.Require().NoError(err)
		break
	}
	// Stopping at the first entry, the directory "a", reads only the location itself.
}

func (s *listEntriesTestSuite) TestEntriesCarryTheDirectoryReadMetadata() {
	link := s.info("link", false, 5, os.ModeSymlink|0o777)
	file := s.info("a.txt", false, 1234, 0o640)
	s.expectDir("/dir1", file, link, s.dir("sub"))

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location()))
	s.Require().NoError(err)
	s.Require().Len(entries, 3)

	a, l, sub := entries[0], entries[1], entries[2]
	s.Equal("a.txt", a.RelPath())
	s.True(a.HasMetadata())
	s.Equal(uint64(1234), a.Size())
	s.True(listModTime.Equal(a.ModTime()))
	s.Equal(iofs.FileMode(0o640), a.Mode())
	s.Equal(file, a.Sys())
	info, err := a.Info()
	s.Require().NoError(err, "listed metadata needs no Stat request")
	s.Equal(int64(1234), info.Size())

	s.Equal("link", l.RelPath())
	s.Equal(iofs.ModeSymlink, l.Type())
	s.False(l.IsDir(), "a link is listed, not followed")
	s.True(sub.IsDir())
	s.Equal("/dir1/a.txt", a.Path())
}

func (s *listEntriesTestSuite) TestMissingLocationListsNothing() {
	s.client.EXPECT().ReadDir("/dir1").Return(nil, os.ErrNotExist).Once()

	s.Empty(s.keys(s.location(), list.Recursive()))
}

func (s *listEntriesTestSuite) TestUnreadableDirectoryIsAPartialFailure() {
	s.expectDir("/dir1", s.dir("a"), s.dir("b"), s.dir("c"))
	s.expectDir("/dir1/a", s.file("1.txt"))
	s.client.EXPECT().ReadDir("/dir1/b").Return(nil, os.ErrPermission).Once()
	s.expectDir("/dir1/c", s.file("3.txt"))

	var keys []string
	var failed []vfs.Entry
	for e, err := range list.Entries(s.T().Context(), s.location(), list.Recursive()) {
		if err != nil {
			s.Require().ErrorIs(err, iofs.ErrPermission)
			failed = append(failed, e)
			continue
		}
		keys = append(keys, list.OrderKey(e))
	}

	s.Equal([]string{"a/", "a/1.txt", "b/", "c/", "c/3.txt"}, keys, "the listing continues past the unreadable directory")
	s.Require().Len(failed, 1)
	s.Equal("b", failed[0].RelPath())
}

func (s *listEntriesTestSuite) TestReadFailureOfTheLocationEndsTheListing() {
	boom := errors.New("connection reset")
	s.client.EXPECT().ReadDir("/dir1").Return(nil, boom).Once()

	_, err := list.Collect(list.Entries(s.T().Context(), s.location()))

	s.Require().ErrorIs(err, boom)
	var pe *iofs.PathError
	s.Require().ErrorAs(err, &pe)
	s.Equal("list", pe.Op)
}

func (s *listEntriesTestSuite) TestInvalidOptionsMakeNoRequest() {
	var errs []error
	for _, err := range s.location().(vfs.EntryLister).ListEntries(s.T().Context(), list.FilesOnly(), list.DirsOnly()) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], list.ErrInvalidOption)
}

func (s *listEntriesTestSuite) TestCanceledContextMakesNoRequest() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	var errs []error
	for _, err := range s.location().(vfs.EntryLister).ListEntries(ctx) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], context.Canceled)
}

func (s *listEntriesTestSuite) TestConnectionFailureIsReported() {
	original := defaultClientGetter
	s.T().Cleanup(func() { defaultClientGetter = original })
	defaultClientGetter = func(authority.Authority, Options) (Client, io.Closer, error) {
		return nil, nil, errors.New("dial failed")
	}
	loc, err := (&FileSystem{}).NewLocation("host.com", "/dir1/")
	s.Require().NoError(err)

	_, err = list.Collect(list.Entries(s.T().Context(), loc))

	s.Require().ErrorContains(err, "dial failed")
}
