package gdrive

import (
	"context"
	iofs "io/fs"
	"iter"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/gdrive/mocks"
	"github.com/c2fo/vfs/contrib/backend/gdrive/types"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// ListEntriesTestSuite checks the native listing against the in-process fake Drive.
type ListEntriesTestSuite struct {
	suite.Suite
	drive *fakeDrive
	fs    *FileSystem
	base  string
}

func TestListEntriesTestSuite(t *testing.T) { suite.Run(t, new(ListEntriesTestSuite)) }

func (s *ListEntriesTestSuite) SetupTest() {
	s.drive = newFakeDrive(s.T())
	s.base = s.drive.addFolder("base")
	s.fs = newTestFS(s.T(), s.drive)
}

func (s *ListEntriesTestSuite) location(p string) vfs.Location {
	loc, err := s.fs.NewLocation("", p)
	s.Require().NoError(err)
	return loc
}

func (s *ListEntriesTestSuite) write(loc vfs.Location, paths ...string) {
	for _, p := range paths {
		f, err := loc.NewFile(p)
		s.Require().NoError(err)
		_, err = f.Write([]byte("hello"))
		s.Require().NoError(err)
		s.Require().NoError(f.Close())
	}
}

func (s *ListEntriesTestSuite) keys(loc vfs.Location, opts ...options.ListOption) []string {
	entries, err := list.Collect(list.Entries(s.T().Context(), loc, opts...))
	s.Require().NoError(err)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

// freshLocation returns a location on a new file system, so no folder is already known from earlier requests.
func (s *ListEntriesTestSuite) freshLocation(p string) vfs.Location {
	s.fs = newTestFS(s.T(), s.drive)
	return s.location(p)
}

func (s *ListEntriesTestSuite) listRequests() int {
	n := 0
	for _, r := range s.drive.requestLog() {
		if r == "GET /drive/v3/files" {
			n++
		}
	}
	return n
}

func (s *ListEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location("/base/"))
}

func (s *ListEntriesTestSuite) TestRecursiveListingIsInByteOrder() {
	loc := s.location("/base/")
	s.write(loc, "z.txt", "a.txt", "logs/app.log", "logs-2024.gz", "logs/old/2023.log")

	s.Equal([]string{
		"a.txt", "logs-2024.gz", "logs/", "logs/app.log", "logs/old/", "logs/old/2023.log", "z.txt",
	}, s.keys(loc, list.Recursive()))
}

func (s *ListEntriesTestSuite) TestPagesAreReadToTheEnd() {
	s.drive.pageSize = 2
	loc := s.location("/base/")
	s.write(loc, "a.txt", "b.txt", "c.txt", "d.txt", "e.txt")

	s.Equal([]string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"}, s.keys(loc))
}

func (s *ListEntriesTestSuite) TestAFolderIsReadOnlyWhenTheListingReachesIt() {
	s.write(s.location("/base/"), "a.txt", "sub/b.txt", "sub2/c.txt")
	loc := s.freshLocation("/base/")

	before := s.listRequests()
	next, stop := iter.Pull2(list.Entries(s.T().Context(), loc, list.Recursive()))
	defer stop()

	e, err, ok := next()
	s.Require().True(ok)
	s.Require().NoError(err)
	s.Equal("a.txt", list.OrderKey(e))
	// finding "base" and reading it is all a first entry costs: no subfolder has been read
	s.Equal(2, s.listRequests()-before)
}

func (s *ListEntriesTestSuite) TestSubfoldersAreReadWithoutLookingThemUpByName() {
	s.write(s.location("/base/"), "a.txt", "sub/b.txt", "sub/deep/c.txt", "sub2/d.txt")
	loc := s.freshLocation("/base/")

	before := s.listRequests()
	s.Equal([]string{
		"a.txt", "sub/", "sub/b.txt", "sub/deep/", "sub/deep/c.txt", "sub2/", "sub2/d.txt",
	}, s.keys(loc, list.Recursive()))
	// "base" is looked up and listed; each of the other three folders needs only its listing
	s.Equal(2+3, s.listRequests()-before)
}

func (s *ListEntriesTestSuite) TestNonRecursiveListingReadsOnlyTheLocation() {
	loc := s.location("/base/")
	s.write(loc, "a.txt", "sub/b.txt")

	before := s.listRequests()
	s.Equal([]string{"a.txt", "sub/"}, s.keys(loc))
	s.Equal(1, s.listRequests()-before) // the location is already known, so one listing is all it takes
}

func (s *ListEntriesTestSuite) TestEntriesCarryTheListedMetadata() {
	loc := s.location("/base/")
	s.write(loc, "a.txt")

	entries, err := list.Collect(list.Entries(s.T().Context(), loc))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]

	f, err := loc.NewFile("a.txt")
	s.Require().NoError(err)
	size, err := f.Size()
	s.Require().NoError(err)
	modified, err := f.LastModified()
	s.Require().NoError(err)

	s.Equal(size, e.Size())
	s.Equal(*modified, e.ModTime())
	item, ok := e.Sys().(*Item)
	s.Require().True(ok)
	s.Equal("a.txt", item.Name)
	s.Equal(int64(size), item.Size)
}

func (s *ListEntriesTestSuite) TestGoogleNativeFileIsListedWithSizeZero() {
	s.drive.addMime("doc", "application/vnd.google-apps.document")

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/")))
	s.Require().NoError(err)

	var found bool
	for _, e := range entries {
		if list.OrderKey(e) == "doc" {
			found = true
			s.Zero(e.Size())
		}
	}
	s.True(found)
}

func (s *ListEntriesTestSuite) TestFilesSharingANameAreEachListed() {
	s.drive.addFile(s.base, "dup.txt", []byte("one"))
	s.drive.addFile(s.base, "dup.txt", []byte("three"))

	s.Equal([]string{"dup.txt", "dup.txt"}, s.keys(s.location("/base/")))
}

func (s *ListEntriesTestSuite) TestMissingLocationListsNothing() {
	s.Empty(s.keys(s.location("/nope/deeper/")))
}

func (s *ListEntriesTestSuite) TestLocationThatIsAFileIsAnError() {
	s.write(s.location("/base/"), "f.txt")

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/f.txt/")))
	s.Require().Error(err)
	s.Require().NotErrorIs(err, iofs.ErrNotExist)
}

func (s *ListEntriesTestSuite) TestLocationBelowAFileListsNothing() {
	s.write(s.location("/base/"), "f.txt")

	s.Empty(s.keys(s.location("/base/f.txt/deeper/")))
}

func (s *ListEntriesTestSuite) TestAmbiguousFolderIsAPartialFailure() {
	s.drive.addFile(s.base, "a.txt", []byte("x"))
	s.drive.add(s.base, "dup", folderMimeType, nil)
	s.drive.add(s.base, "dup", folderMimeType, nil)
	s.drive.addFile(s.base, "z.txt", []byte("x"))

	var keys []string
	var failed []string
	for e, err := range list.Entries(s.T().Context(), s.location("/base/"), list.Recursive()) {
		if err != nil {
			s.Require().ErrorIs(err, ErrAmbiguous)
			failed = append(failed, list.OrderKey(e))
			continue
		}
		keys = append(keys, list.OrderKey(e))
	}

	// each ambiguous folder is listed as an entry, then reported as unreadable, and the listing goes on
	s.Equal([]string{"a.txt", "dup/", "dup/", "z.txt"}, keys)
	s.Len(failed, 2)
	s.Equal("dup/", failed[0])
}

func (s *ListEntriesTestSuite) TestDuplicateAddedAfterTheFolderWasCachedIsStillAmbiguous() {
	loc := s.location("/base/")
	s.write(loc, "dup/x.txt") // leaves "base/dup" in the folder cache
	s.drive.add(s.base, "dup", folderMimeType, nil)

	var keys []string
	var failed int
	for e, err := range list.Entries(s.T().Context(), loc, list.Recursive()) {
		if err != nil {
			s.Require().ErrorIs(err, ErrAmbiguous)
			failed++
			continue
		}
		keys = append(keys, list.OrderKey(e))
	}

	s.Equal([]string{"dup/", "dup/"}, keys, "neither folder is descended into, so x.txt is not listed twice")
	s.Equal(2, failed)
}

func (s *ListEntriesTestSuite) TestNameWithASlashIsReportedAsInvalid() {
	s.drive.addFile(s.base, "ok.txt", []byte("x"))
	s.drive.addFile(s.base, "a/b.txt", []byte("x"))

	var keys []string
	var invalid int
	for e, err := range list.Entries(s.T().Context(), s.location("/base/")) {
		if err != nil {
			s.Require().ErrorIs(err, iofs.ErrInvalid)
			invalid++
			continue
		}
		keys = append(keys, list.OrderKey(e))
	}

	s.Equal([]string{"ok.txt"}, keys)
	s.Equal(1, invalid)
}

func (s *ListEntriesTestSuite) TestCanceledContextStopsTheListing() {
	loc := s.location("/base/")
	s.write(loc, "a.txt", "sub/b.txt")

	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	before := s.listRequests()
	_, err := list.Collect(list.Entries(ctx, loc, list.Recursive()))
	s.Require().ErrorIs(err, context.Canceled)
	s.Equal(before, s.listRequests())
}

func (s *ListEntriesTestSuite) TestRootLocationLists() {
	s.drive.addFile("root", "r.txt", []byte("x"))

	s.Equal([]string{"base/", "r.txt"}, s.keys(s.location("/")))
}

// ListFailureTestSuite checks how the native listing reports failures, using a mocked client.
type ListFailureTestSuite struct {
	suite.Suite
	client *mocks.Client
	fs     *FileSystem
}

func TestListFailureTestSuite(t *testing.T) { suite.Run(t, new(ListFailureTestSuite)) }

func (s *ListFailureTestSuite) SetupTest() {
	s.client = mocks.NewClient(s.T())
	s.fs = NewFileSystem(WithClient(s.client))
}

func (s *ListFailureTestSuite) location(p string) vfs.Location {
	loc, err := s.fs.NewLocation("", p)
	s.Require().NoError(err)
	return loc
}

func (s *ListFailureTestSuite) TestEntriesCarryTheListedItem() {
	modified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return([]types.Item{
		{ID: "f1", Name: "a.txt", Size: 7, ModTime: modified},
		{ID: "d1", Name: "sub", IsFolder: true},
	}, nil)

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/")))
	s.Require().NoError(err)
	s.Require().Len(entries, 2)

	s.Equal("a.txt", list.OrderKey(entries[0]))
	s.Equal(uint64(7), entries[0].Size())
	s.Equal(modified, entries[0].ModTime())
	s.Equal("f1", entries[0].Sys().(*Item).ID)
	s.Equal("sub/", list.OrderKey(entries[1]))
}

func (s *ListFailureTestSuite) TestEveryItemKeepsItsOwnSys() {
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return([]types.Item{
		{ID: "f1", Name: "a.txt"}, {ID: "f2", Name: "b.txt"},
	}, nil)

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/")))
	s.Require().NoError(err)
	s.Require().Len(entries, 2)
	s.Equal("f1", entries[0].Sys().(*Item).ID)
	s.Equal("f2", entries[1].Sys().(*Item).ID)
}

func (s *ListFailureTestSuite) TestSubfolderIsReadByItsPath() {
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return([]types.Item{{Name: "sub", IsFolder: true}}, nil)
	s.client.EXPECT().ListChildren(mock.Anything, "base/sub").Return([]types.Item{{Name: "b.txt"}}, nil)

	keys := make([]string, 0, 2)
	for e, err := range list.Entries(s.T().Context(), s.location("/base/"), list.Recursive()) {
		s.Require().NoError(err)
		keys = append(keys, list.OrderKey(e))
	}
	s.Equal([]string{"sub/", "sub/b.txt"}, keys)
}

func (s *ListFailureTestSuite) TestPathThroughAFile() {
	tests := []struct {
		name        string
		getItem     func() (*types.Item, error)
		expectEmpty bool
	}{
		{
			name:        "Ancestor is a file - the location is missing",
			getItem:     func() (*types.Item, error) { return nil, ErrNotFolder },
			expectEmpty: true,
		},
		{
			name:        "Nothing at the location - missing",
			getItem:     func() (*types.Item, error) { return nil, ErrNotFound },
			expectEmpty: true,
		},
		{
			name:    "Location is itself a file - an error",
			getItem: func() (*types.Item, error) { return &types.Item{Name: "base"}, nil },
		},
		{
			name:    "Location cannot be checked - an error",
			getItem: func() (*types.Item, error) { return nil, errBoom },
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			client := mocks.NewClient(s.T())
			client.EXPECT().ListChildren(mock.Anything, "base").Return(nil, ErrNotFolder)
			client.EXPECT().GetItem(mock.Anything, "base").RunAndReturn(
				func(context.Context, string) (*types.Item, error) { return tt.getItem() })

			loc, err := NewFileSystem(WithClient(client)).NewLocation("", "/base/")
			s.Require().NoError(err)

			entries, err := list.Collect(list.Entries(s.T().Context(), loc))
			if tt.expectEmpty {
				s.Require().NoError(err)
				s.Empty(entries)
				return
			}
			s.Require().Error(err)
			s.Require().NotErrorIs(err, iofs.ErrNotExist)
		})
	}
}

func (s *ListFailureTestSuite) TestAmbiguousFolderIsNotMissing() {
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return(nil, ErrAmbiguous)

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/")))
	s.Require().ErrorIs(err, ErrAmbiguous)
	s.Require().NotErrorIs(err, iofs.ErrNotExist)
}

func (s *ListFailureTestSuite) TestClientErrorEndsTheListing() {
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return(nil, errBoom)

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/")))
	s.Require().ErrorIs(err, errBoom)
}

func (s *ListFailureTestSuite) TestMissingLocationListsNothing() {
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return(nil, ErrNotFound)

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/")))
	s.Require().NoError(err)
	s.Empty(entries)
}

func (s *ListFailureTestSuite) TestFolderPathHasNoSlashes() {
	s.client.EXPECT().ListChildren(mock.Anything, mock.MatchedBy(func(p string) bool {
		return !strings.HasPrefix(p, "/") && !strings.HasSuffix(p, "/")
	})).Return(nil, nil)

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/")))
	s.Require().NoError(err)
}

func (s *ListFailureTestSuite) TestNoClientIsAnError() {
	fs := NewFileSystem()
	loc, err := fs.NewLocation("", "/base/")
	s.Require().NoError(err)

	_, err = list.Collect(list.Entries(s.T().Context(), loc))
	s.Require().Error(err)
}

func (s *ListFailureTestSuite) TestAuthFailureEndsTheListing() {
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return([]types.Item{
		{Name: "a", IsFolder: true}, {Name: "b", IsFolder: true},
	}, nil)
	// "base/b" is never read: the mock fails the test if it is
	s.client.EXPECT().ListChildren(mock.Anything, "base/a").
		Return(nil, &apiError{StatusCode: http.StatusUnauthorized, Reason: "authError"})

	var keys []string
	var errs []error
	for e, err := range list.Entries(s.T().Context(), s.location("/base/"), list.Recursive()) {
		if err != nil {
			errs = append(errs, err)
			s.Zero(e, "an authentication failure is reported without an entry")
			continue
		}
		keys = append(keys, list.OrderKey(e))
	}

	s.Equal([]string{"a/"}, keys)
	s.Require().Len(errs, 1)
	var apiErr *apiError
	s.Require().ErrorAs(errs[0], &apiErr)
	s.Equal(http.StatusUnauthorized, apiErr.StatusCode)
}

func (s *ListFailureTestSuite) TestPermissionFailuresMatchErrPermission() {
	tests := []struct {
		name       string
		err        error
		permission bool
	}{
		{name: "Insufficient file permissions", err: &apiError{StatusCode: 403, Reason: "insufficientFilePermissions"}, permission: true},
		{name: "Forbidden", err: &apiError{StatusCode: 403, Reason: "forbidden"}, permission: true},
		{name: "App not authorized to file", err: &apiError{StatusCode: 403, Reason: "appNotAuthorizedToFile"}, permission: true},
		{name: "Rate limit also answers 403", err: &apiError{StatusCode: 403, Reason: "userRateLimitExceeded"}},
		{name: "Same reason on another status", err: &apiError{StatusCode: 500, Reason: "forbidden"}},
		{name: "Not an API error", err: errBoom},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			client := mocks.NewClient(s.T())
			client.EXPECT().ListChildren(mock.Anything, "base").Return(nil, tt.err)

			loc, err := NewFileSystem(WithClient(client)).NewLocation("", "/base/")
			s.Require().NoError(err)

			_, err = list.Collect(list.Entries(s.T().Context(), loc))
			s.Require().ErrorIs(err, tt.err, "the original error is kept")
			if tt.permission {
				s.Require().ErrorIs(err, iofs.ErrPermission)
			} else {
				s.Require().NotErrorIs(err, iofs.ErrPermission)
			}
		})
	}
}
