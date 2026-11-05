package onedrive

import (
	"context"
	"errors"
	iofs "io/fs"
	"iter"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/onedrive/mocks"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

var errListBoom = errors.New("boom")

// TestListingConformanceFakeGraph runs the shared listing suite against the in-process fake Graph.
func TestListingConformanceFakeGraph(t *testing.T) {
	g := newFakeGraph(t)
	g.put("vfs-test/.keep", nil) // the suite expects its base location to already exist
	fs := newFakeGraphFS(t, g)

	loc, err := fs.NewLocation("", "/vfs-test/")
	require.NoError(t, err)

	testsuite.RunListingTests(t, loc)
}

// ListEntriesTestSuite checks the native listing against the in-process fake Graph.
type ListEntriesTestSuite struct {
	suite.Suite
	graph *fakeGraph
	fs    *FileSystem
}

func TestListEntriesTestSuite(t *testing.T) { suite.Run(t, new(ListEntriesTestSuite)) }

func (s *ListEntriesTestSuite) SetupTest() {
	s.graph = newFakeGraph(s.T())
	s.fs = newFakeGraphFS(s.T(), s.graph)
}

func (s *ListEntriesTestSuite) location(p string) vfs.Location {
	loc, err := s.fs.NewLocation("", p)
	s.Require().NoError(err)
	return loc
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

func (s *ListEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location("/base/"))
}

func (s *ListEntriesTestSuite) TestRecursiveListingIsInByteOrder() {
	for _, p := range []string{"base/z.txt", "base/a.txt", "base/logs/app.log", "base/logs-2024.gz", "base/logs/old/2023.log"} {
		s.graph.put(p, []byte("x"))
	}

	s.Equal([]string{
		"a.txt", "logs-2024.gz", "logs/", "logs/app.log", "logs/old/", "logs/old/2023.log", "z.txt",
	}, s.keys(s.location("/base/"), list.Recursive()))
}

func (s *ListEntriesTestSuite) TestPagesAreReadToTheEnd() {
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		s.graph.put("base/"+n+".txt", nil)
	}

	s.Equal([]string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt", "g.txt"}, s.keys(s.location("/base/")))
	s.Equal(3, s.graph.count("GET /base/children"), "seven children at three per page take three requests")
}

func (s *ListEntriesTestSuite) TestAFolderIsReadOnlyWhenTheListingReachesIt() {
	s.graph.put("base/a.txt", nil)
	s.graph.put("base/sub/b.txt", nil)
	s.graph.put("base/sub2/c.txt", nil)

	next, stop := iter.Pull2(list.Entries(s.T().Context(), s.location("/base/"), list.Recursive()))
	defer stop()

	e, err, ok := next()
	s.Require().True(ok)
	s.Require().NoError(err)
	s.Equal("a.txt", list.OrderKey(e))
	s.Equal(0, s.graph.count("GET /base/sub"), "no subfolder has been read")
}

func (s *ListEntriesTestSuite) TestRootLocationLists() {
	s.graph.put("r.txt", nil)
	s.graph.put("base/a.txt", nil)

	s.Equal([]string{"base/", "r.txt"}, s.keys(s.location("/")))
}

func (s *ListEntriesTestSuite) TestEntriesCarryTheListedMetadata() {
	s.graph.put("base/a.txt", []byte("hello"))
	loc := s.location("/base/")

	before := s.graph.count("GET /base/a.txt")
	entries, err := list.Collect(list.Entries(s.T().Context(), loc))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]
	s.Equal(before, s.graph.count("GET /base/a.txt"), "listing a file does not request it")

	f, err := loc.NewFile("a.txt")
	s.Require().NoError(err)
	size, err := f.Size()
	s.Require().NoError(err)
	modified, err := f.LastModified()
	s.Require().NoError(err)

	s.Equal(size, e.Size())
	s.True(modified.Equal(e.ModTime()))
	item, ok := e.Sys().(*DriveItem)
	s.Require().True(ok)
	s.Equal("a.txt", item.Name)
	s.Equal(int64(size), item.Size)
}

func (s *ListEntriesTestSuite) TestMissingLocationListsNothing() {
	s.Empty(s.keys(s.location("/nope/deeper/")))
}

func (s *ListEntriesTestSuite) TestLocationThatIsAFileIsAnError() {
	s.graph.put("base/f.txt", nil)

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/f.txt/")))
	s.Require().ErrorIs(err, errNotFolder)
	s.Require().NotErrorIs(err, iofs.ErrNotExist)
}

func (s *ListEntriesTestSuite) TestLocationBelowAFileListsNothing() {
	s.graph.put("base/f.txt", nil)

	s.Empty(s.keys(s.location("/base/f.txt/deeper/")))
}

func (s *ListEntriesTestSuite) TestCanceledContextStopsTheListing() {
	s.graph.put("base/a.txt", nil)
	s.graph.put("base/sub/b.txt", nil)

	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	_, err := list.Collect(list.Entries(ctx, s.location("/base/"), list.Recursive()))
	s.Require().ErrorIs(err, context.Canceled)
	s.Equal(0, s.graph.count("GET /base/children"))
}

func (s *ListEntriesTestSuite) TestTokenExpiringMidListingEndsIt() {
	s.graph.put("base/a/x.txt", nil)
	s.graph.put("base/b/y.txt", nil)
	s.graph.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.Contains(r.URL.Path, "/base/a:/children") {
			return false
		}
		writeFakeErr(w, http.StatusUnauthorized, "InvalidAuthenticationToken", "token expired")
		return true
	})

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

	s.Equal([]string{"a/"}, keys, "base/b is never read")
	s.Require().Len(errs, 1)
	var apiErr *apiError
	s.Require().ErrorAs(errs[0], &apiErr)
	s.Equal(http.StatusUnauthorized, apiErr.StatusCode)
	s.Equal(0, s.graph.count("GET /base/b"))
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

func (s *ListFailureTestSuite) TestEveryItemKeepsItsOwnSys() {
	modified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return([]DriveItem{
		{ID: "f1", Name: "a.txt", Size: 7, LastModified: modified}, {ID: "f2", Name: "b.txt"}, {ID: "d1", Name: "sub", IsFolder: true},
	}, nil)

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/")))
	s.Require().NoError(err)
	s.Require().Len(entries, 3)
	s.Equal(uint64(7), entries[0].Size())
	s.Equal(modified, entries[0].ModTime())
	s.Equal("f1", entries[0].Sys().(*DriveItem).ID)
	s.Equal("f2", entries[1].Sys().(*DriveItem).ID)
	s.Equal("sub/", list.OrderKey(entries[2]))
}

func (s *ListFailureTestSuite) TestSubfolderIsReadByItsPath() {
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return([]DriveItem{{Name: "sub", IsFolder: true}}, nil)
	s.client.EXPECT().ListChildren(mock.Anything, "base/sub").Return([]DriveItem{{Name: "b.txt"}}, nil)

	keys := make([]string, 0, 2)
	for e, err := range list.Entries(s.T().Context(), s.location("/base/"), list.Recursive()) {
		s.Require().NoError(err)
		keys = append(keys, list.OrderKey(e))
	}
	s.Equal([]string{"sub/", "sub/b.txt"}, keys)
}

func (s *ListFailureTestSuite) TestFolderPathHasNoSlashes() {
	s.client.EXPECT().ListChildren(mock.Anything, mock.MatchedBy(func(p string) bool {
		return !strings.HasPrefix(p, "/") && !strings.HasSuffix(p, "/")
	})).Return(nil, nil)

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/")))
	s.Require().NoError(err)
}

func (s *ListFailureTestSuite) TestNotFoundIsClassifiedByLookingAtTheItem() {
	tests := []struct {
		name        string
		getItem     func() (*DriveItem, error)
		expectEmpty bool
		expectErr   error
	}{
		{
			name:        "Nothing at the location - missing",
			getItem:     func() (*DriveItem, error) { return nil, ErrNotFound },
			expectEmpty: true,
		},
		{
			name:        "Location is itself a file - an error",
			getItem:     func() (*DriveItem, error) { return &DriveItem{Name: "base"}, nil },
			expectEmpty: false,
		},
		{
			name:        "Location is a folder that listing could not find - missing",
			getItem:     func() (*DriveItem, error) { return &DriveItem{Name: "base", IsFolder: true}, nil },
			expectEmpty: true,
		},
		{
			name:      "Lookup is refused - the failure matches ErrPermission",
			getItem:   func() (*DriveItem, error) { return nil, &apiError{StatusCode: 403, Code: "accessDenied"} },
			expectErr: iofs.ErrPermission,
		},
		{
			name:      "Lookup fails - the failure is reported, not read as missing",
			getItem:   func() (*DriveItem, error) { return nil, errListBoom },
			expectErr: errListBoom,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			client := mocks.NewClient(s.T())
			client.EXPECT().ListChildren(mock.Anything, "base").Return(nil, ErrNotFound)
			client.EXPECT().GetItem(mock.Anything, "base").RunAndReturn(
				func(context.Context, string) (*DriveItem, error) { return tt.getItem() })

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
			if tt.expectErr != nil {
				s.Require().ErrorIs(err, tt.expectErr)
				return
			}
			s.Require().ErrorIs(err, errNotFolder)
		})
	}
}

func (s *ListFailureTestSuite) TestClientErrorEndsTheListing() {
	s.client.EXPECT().ListChildren(mock.Anything, "base").Return(nil, errListBoom)

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/")))
	s.Require().ErrorIs(err, errListBoom)
}

func (s *ListFailureTestSuite) TestNoClientIsAnError() {
	s.T().Setenv("VFS_ONEDRIVE_ACCESS_TOKEN", "")
	s.T().Setenv("VFS_ONEDRIVE_TENANT_ID", "")
	loc, err := NewFileSystem().NewLocation("", "/base/")
	s.Require().NoError(err)

	_, err = list.Collect(list.Entries(s.T().Context(), loc))
	s.Require().ErrorIs(err, errCredentialsRequired)
}

func (s *ListFailureTestSuite) TestPermissionFailuresMatchErrPermission() {
	tests := []struct {
		name       string
		err        error
		permission bool
	}{
		{name: "Access denied", err: &apiError{StatusCode: 403, Code: "accessDenied"}, permission: true},
		{name: "Other 403 code", err: &apiError{StatusCode: 403, Code: "activityLimitReached"}},
		{name: "Same code on another status", err: &apiError{StatusCode: 500, Code: "accessDenied"}},
		{name: "Not an API error", err: errListBoom},
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
