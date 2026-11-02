package ftp

import (
	"context"
	"errors"
	iofs "io/fs"
	"net"
	"net/textproto"
	"strconv"
	"testing"
	"time"

	_ftp "github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/ftp/mocks"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

var listModTime = time.Date(2024, 3, 4, 5, 6, 0, 0, time.UTC)

type listEntriesTestSuite struct {
	suite.Suite
	client *mocks.Client
	fs     *FileSystem
}

func TestListEntriesTestSuite(t *testing.T) {
	suite.Run(t, new(listEntriesTestSuite))
}

func (s *listEntriesTestSuite) SetupTest() {
	dataConnGetterFunc = getDataConn
	s.client = mocks.NewClient(s.T())
	s.fs = NewFileSystem().WithClient(s.client)
}

func (s *listEntriesTestSuite) location() vfs.Location {
	s.T().Helper()
	loc, err := s.fs.NewLocation("host.com", "/dir1/")
	s.Require().NoError(err)
	return loc
}

func (s *listEntriesTestSuite) keys(opts ...options.ListOption) []string {
	s.T().Helper()
	entries, err := list.Collect(list.Entries(s.T().Context(), s.location(), opts...))
	s.Require().NoError(err)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func file(name string) *_ftp.Entry {
	return &_ftp.Entry{Name: name, Type: _ftp.EntryTypeFile, Size: uint64(len(name)), Time: listModTime}
}

func folder(name string) *_ftp.Entry {
	return &_ftp.Entry{Name: name, Type: _ftp.EntryTypeFolder, Time: listModTime}
}

func (s *listEntriesTestSuite) expectDir(dir string, entries ...*_ftp.Entry) {
	s.client.EXPECT().List(dir).Return(entries, nil).Once()
}

func (s *listEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location())
}

func (s *listEntriesTestSuite) TestRecursiveListingListsEachDirectoryOnce() {
	s.expectDir("/dir1", file("z.txt"), folder("logs"), file("a.txt"))
	s.expectDir("/dir1/logs", folder("old"), file("app.log"))
	s.expectDir("/dir1/logs/old", file("2023.log"))

	s.Equal([]string{"a.txt", "logs/", "logs/app.log", "logs/old/", "logs/old/2023.log", "z.txt"}, s.keys(list.Recursive()))
}

func (s *listEntriesTestSuite) TestNonRecursiveListingListsOnlyTheLocation() {
	s.expectDir("/dir1", file("a.txt"), folder("logs"))

	s.Equal([]string{"a.txt", "logs/"}, s.keys())
}

func (s *listEntriesTestSuite) TestAFileSortsBeforeADirectoryItSharesAPrefixWith() {
	s.expectDir("/dir1", folder("logs"), file("logs-2024.gz"))

	s.Equal([]string{"logs-2024.gz", "logs/"}, s.keys())
}

func (s *listEntriesTestSuite) TestADirectoryIsListedOnlyWhenTheListingReachesIt() {
	s.expectDir("/dir1", folder("a"), folder("b"))

	for _, err := range s.location().(vfs.EntryLister).ListEntries(s.T().Context(), list.Recursive()) {
		s.Require().NoError(err)
		break
	}
	// The mock fails the test if /dir1/a or /dir1/b is listed.
}

func (s *listEntriesTestSuite) TestTheConnectionIsFreeWhileTheCallerHandlesAnEntry() {
	s.expectDir("/dir1", file("a.txt"), file("b.txt"))
	// The first entry is yielded only after the whole directory is listed, so a request made from the loop body
	// finds no listing in progress.
	s.client.EXPECT().Delete("/dir1/a.txt").Return(nil).Once()

	for e, err := range s.location().(vfs.EntryLister).ListEntries(s.T().Context()) {
		s.Require().NoError(err)
		s.Require().NoError(s.fs.ftpclient.Delete(e.Path()))
		break
	}
}

func (s *listEntriesTestSuite) TestEntriesCarryTheListingMetadata() {
	link := &_ftp.Entry{Name: "link", Type: _ftp.EntryTypeLink, Target: "a.txt", Size: 5, Time: listModTime}
	a := file("a.txt")
	a.Size = 1234
	s.expectDir("/dir1", a, link, folder("sub"))

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location()))
	s.Require().NoError(err)
	s.Require().Len(entries, 3)

	first, l, sub := entries[0], entries[1], entries[2]
	s.Equal("a.txt", first.RelPath())
	s.True(first.HasMetadata())
	s.Equal(uint64(1234), first.Size())
	s.True(listModTime.Equal(first.ModTime()))
	s.Equal(iofs.FileMode(0), first.Mode())
	s.Equal(a, first.Sys())
	info, err := first.Info()
	s.Require().NoError(err, "listed metadata needs no further request")
	s.Equal(int64(1234), info.Size())
	s.Equal("/dir1/a.txt", first.Path())

	s.Equal(iofs.ModeSymlink, l.Type())
	s.False(l.IsDir(), "a link is listed, not followed")
	s.True(sub.IsDir())
}

func (s *listEntriesTestSuite) TestMissingLocationListsNothing() {
	s.client.EXPECT().List("/dir1").Return(nil, errors.New("550 Failed to change directory.")).Once()

	s.Empty(s.keys(list.Recursive()))
}

func (s *listEntriesTestSuite) TestUnreadableDirectoryIsAPartialFailure() {
	s.expectDir("/dir1", folder("a"), folder("b"), folder("c"))
	s.expectDir("/dir1/a", file("1.txt"))
	s.client.EXPECT().List("/dir1/b").Return(nil, errors.New("550 Failed to change directory.")).Once()
	s.expectDir("/dir1/c", file("3.txt"))

	var keys []string
	var failed []vfs.Entry
	for e, err := range list.Entries(s.T().Context(), s.location(), list.Recursive()) {
		if err != nil {
			s.Require().ErrorIs(err, iofs.ErrNotExist)
			failed = append(failed, e)
			continue
		}
		keys = append(keys, list.OrderKey(e))
	}

	s.Equal([]string{"a/", "a/1.txt", "b/", "c/", "c/3.txt"}, keys, "the listing continues past the failed directory")
	s.Require().Len(failed, 1)
	s.Equal("b", failed[0].RelPath())
}

func (s *listEntriesTestSuite) TestListFailureOfTheLocationEndsTheListing() {
	boom := errors.New("421 service not available")
	s.client.EXPECT().List("/dir1").Return(nil, boom).Once()

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
	defaultClientGetter = clientGetterReturnsError

	loc, err := NewFileSystem().NewLocation("host.com", "/dir1/")
	s.Require().NoError(err)
	_, err = list.Collect(list.Entries(s.T().Context(), loc))

	s.Require().ErrorIs(err, errClientGetter)
}

func (s *listEntriesTestSuite) TestLostSessionInASubdirectoryEndsTheListing() {
	for _, code := range []int{421, 425, 426, 430, 434} {
		s.Run(strconv.Itoa(code), func() {
			s.SetupTest()
			s.expectDir("/dir1", folder("a"), folder("b"), folder("c"))
			s.expectDir("/dir1/a", file("1.txt"))
			s.client.EXPECT().List("/dir1/b").Return(nil, &textproto.Error{Code: code, Msg: "session lost"}).Once()

			var keys []string
			var errs []error
			for e, err := range list.Entries(s.T().Context(), s.location(), list.Recursive()) {
				if err != nil {
					errs = append(errs, err)
					continue
				}
				keys = append(keys, list.OrderKey(e))
			}

			s.Equal([]string{"a/", "a/1.txt", "b/"}, keys, "c is never listed once the session is lost")
			s.Require().Len(errs, 1)
			s.Require().ErrorIs(errs[0], net.ErrClosed)
		})
	}
}
