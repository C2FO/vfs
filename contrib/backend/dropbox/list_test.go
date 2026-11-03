package dropbox

import (
	"context"
	"errors"
	iofs "io/fs"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/auth"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/dropbox/mocks"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// ListEntriesTestSuite checks the native listing against the in-process fake Dropbox API.
type ListEntriesTestSuite struct {
	suite.Suite
	fake *fakeDropbox
	fs   *FileSystem
}

func TestListEntriesTestSuite(t *testing.T) { suite.Run(t, new(ListEntriesTestSuite)) }

func (s *ListEntriesTestSuite) SetupTest() {
	var client files.Client
	s.fake, client = newFakeClient(s.T())
	s.fs = NewFileSystem(WithClient(client))
	s.fake.seedFolder("/base")
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

func (s *ListEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location("/base/"))
}

func (s *ListEntriesTestSuite) TestFoldersAreReadPageByPage() {
	loc := s.location("/base/")
	names := make([]string, 0, 8)
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		names = append(names, n+".txt")
	}
	s.write(loc, names...)

	s.Equal(names, s.keys(loc))
	s.Equal(1, s.fake.calls("list_folder"))
	s.Equal(2, s.fake.calls("list_folder/continue"), "eight entries at three per page")
}

func (s *ListEntriesTestSuite) TestAFolderIsReadOnlyWhenTheListingReachesIt() {
	loc := s.location("/base/")
	s.write(loc, "a/1.txt", "b/2.txt", "c/3.txt")
	before := s.fake.calls("list_folder")

	next, stop := iterPull(list.Entries(s.T().Context(), loc, list.Recursive()))
	e, err, ok := next() // "a/"
	s.Require().True(ok)
	s.Require().NoError(err)
	s.Equal("a", e.RelPath())
	s.Equal(1, s.fake.calls("list_folder")-before, "only the location is read before the first folder is reached")
	stop()
	s.Equal(1, s.fake.calls("list_folder")-before, "stopping reads no further folder")
}

func (s *ListEntriesTestSuite) TestEachFolderIsReadOnce() {
	loc := s.location("/base/")
	s.write(loc, "a/1.txt", "b/2.txt", "c/d/3.txt")
	before := s.fake.calls("list_folder")

	s.Equal([]string{"a/", "a/1.txt", "b/", "b/2.txt", "c/", "c/d/", "c/d/3.txt"}, s.keys(loc, list.Recursive()))
	s.Equal(5, s.fake.calls("list_folder")-before, "the location and each of its four folders")
}

func (s *ListEntriesTestSuite) TestNonRecursiveListingReadsOnlyTheLocation() {
	loc := s.location("/base/")
	s.write(loc, "a/1.txt", "b/2.txt", "top.txt")
	before := s.fake.calls("list_folder")

	s.Equal([]string{"a/", "b/", "top.txt"}, s.keys(loc))
	s.Equal(1, s.fake.calls("list_folder")-before)
}

func (s *ListEntriesTestSuite) TestEntriesCarryTheListedMetadata() {
	loc := s.location("/base/")
	s.write(loc, "f.txt")
	f, err := loc.NewFile("f.txt")
	s.Require().NoError(err)
	modTime, err := f.LastModified()
	s.Require().NoError(err)
	before := s.fake.calls("get_metadata")

	entries, err := list.Collect(list.Entries(s.T().Context(), loc))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	info, err := entries[0].Info()
	s.Require().NoError(err)

	s.Equal(int64(5), info.Size())
	s.True(modTime.Equal(info.ModTime()), "listed %v, file reports %v", info.ModTime(), modTime)
	s.Zero(s.fake.calls("get_metadata")-before, "size and time come from the listing")
	_, isFile := entries[0].Sys().(*files.FileMetadata)
	s.True(isFile, "Sys returns the listed metadata")
}

func (s *ListEntriesTestSuite) TestLocationNeedNotMatchTheCaseOfTheFolder() {
	s.write(s.location("/base/"), "Docs/a.txt")

	nested, err := s.location("/base/").NewLocation("docs/")
	s.Require().NoError(err)
	s.Equal([]string{"a.txt"}, s.keys(nested), "Dropbox paths are case-insensitive")

	s.Equal([]string{"Docs/", "Docs/a.txt"}, s.keys(s.location("/BASE/"), list.Recursive()),
		"names are the display names Dropbox reports")
}

func (s *ListEntriesTestSuite) TestNonASCIINamesRoundTrip() {
	loc := s.location("/base/")
	s.write(loc, "ünï/cödé.txt", "😀/emoji.txt")

	s.Equal([]string{"ünï/", "ünï/cödé.txt", "😀/", "😀/emoji.txt"}, s.keys(loc, list.Recursive()))
}

func (s *ListEntriesTestSuite) TestMissingLocationListsNothing() {
	s.Empty(s.keys(s.location("/base/nothing/here/"), list.Recursive()))
}

func (s *ListEntriesTestSuite) TestLocationThatIsAFileIsAnError() {
	loc := s.location("/base/")
	s.write(loc, "f.txt")

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/base/f.txt/")))
	s.Require().Error(err)
	s.Require().NotErrorIs(err, iofs.ErrNotExist)
}

func (s *ListEntriesTestSuite) TestRootLocationLists() {
	s.write(s.location("/"), "r.txt")
	s.Contains(s.keys(s.location("/")), "r.txt")
}

// vanishingClient deletes a folder from the fake just before the folder is first read.
type vanishingClient struct {
	files.Client
	fake *fakeDropbox
	path string
}

func (c vanishingClient) ListFolder(arg *files.ListFolderArg) (*files.ListFolderResult, error) {
	if arg.Path == c.path {
		c.fake.mu.Lock()
		for k := range c.fake.entries {
			if k == normalize(c.path) || strings.HasPrefix(k, normalize(c.path)+"/") {
				delete(c.fake.entries, k)
			}
		}
		c.fake.mu.Unlock()
	}
	return c.Client.ListFolder(arg)
}

func (s *ListEntriesTestSuite) TestAFolderThatVanishesIsAPartialFailure() {
	loc := s.location("/base/")
	s.write(loc, "a/1.txt", "b/2.txt")
	client, err := s.fs.Client()
	s.Require().NoError(err)
	filesClient, ok := client.(files.Client)
	s.Require().True(ok)
	fsys := NewFileSystem(WithClient(vanishingClient{Client: filesClient, fake: s.fake, path: "/base/a"}))
	vanishing, err := fsys.NewLocation("", "/base/")
	s.Require().NoError(err)

	var keys []string
	var failed []vfs.Entry
	for e, err := range list.Entries(s.T().Context(), vanishing, list.Recursive()) {
		if err != nil {
			s.Require().ErrorIs(err, iofs.ErrNotExist)
			failed = append(failed, e)
			continue
		}
		keys = append(keys, list.OrderKey(e))
	}
	s.Equal([]string{"a/", "b/", "b/2.txt"}, keys)
	s.Require().Len(failed, 1)
	s.Equal("a", failed[0].RelPath())
}

// ListFailureTestSuite covers failures the fake cannot produce.
type ListFailureTestSuite struct {
	suite.Suite
	client   *mocks.Client
	location *Location
}

func TestListFailureTestSuite(t *testing.T) { suite.Run(t, new(ListFailureTestSuite)) }

func (s *ListFailureTestSuite) SetupTest() {
	s.client = mocks.NewClient(s.T())
	s.location = &Location{fileSystem: &FileSystem{client: s.client, options: NewOptions()}, path: "/test/path/"}
}

func (s *ListFailureTestSuite) folder(name string) *files.FolderMetadata {
	return &files.FolderMetadata{Metadata: files.Metadata{Name: name}}
}

func (s *ListFailureTestSuite) file(name string) *files.FileMetadata {
	return &files.FileMetadata{Metadata: files.Metadata{Name: name}, Size: 3, ServerModified: dropbox.DBXTime(time.Unix(1700000000, 0).UTC())}
}

func (s *ListFailureTestSuite) TestFailedContinuationEndsTheListing() {
	boom := errors.New("connection reset by peer")
	s.client.EXPECT().ListFolder(mock.Anything).Return(&files.ListFolderResult{
		Entries: []files.IsMetadata{s.file("a.txt")}, Cursor: "c1", HasMore: true,
	}, nil).Once()
	s.client.EXPECT().ListFolderContinue(&files.ListFolderContinueArg{Cursor: "c1"}).Return(nil, boom).Once()

	var errs []error
	var got []vfs.Entry
	for e, err := range list.Entries(s.T().Context(), s.location) {
		got = append(got, e)
		errs = append(errs, err)
	}
	s.Require().Len(errs, 1, "no entry of the failed folder is yielded")
	s.Require().ErrorIs(errs[0], boom)
	s.Equal(vfs.Entry{}.RelPath(), got[0].RelPath())
}

func (s *ListFailureTestSuite) TestClientErrorEndsTheListing() {
	_, err := list.Collect(list.Entries(s.T().Context(), (&Location{fileSystem: &FileSystem{options: NewOptions()}, path: "/x/"})))
	s.Require().ErrorIs(err, errAccessTokenRequired)
}

func (s *ListFailureTestSuite) TestDeletedEntriesAreSkipped() {
	s.client.EXPECT().ListFolder(mock.Anything).Return(&files.ListFolderResult{Entries: []files.IsMetadata{
		s.file("a.txt"), &files.DeletedMetadata{Metadata: files.Metadata{Name: "gone.txt"}}, s.folder("d"),
	}}, nil).Once()

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location))
	s.Require().NoError(err)
	s.Require().Len(entries, 2)
	s.Equal("a.txt", entries[0].RelPath())
	s.Equal("d", entries[1].RelPath())
}

func (s *ListFailureTestSuite) TestFolderFailuresMapToStandardErrors() {
	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{name: "missing folder", err: errors.New("path/not_found/."), expected: iofs.ErrNotExist},
		{name: "restricted content", err: errors.New("path/restricted_content/."), expected: iofs.ErrPermission},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.EXPECT().ListFolder(&files.ListFolderArg{Path: "/test/path"}).Return(&files.ListFolderResult{
				Entries: []files.IsMetadata{s.folder("sub")},
			}, nil).Once()
			s.client.EXPECT().ListFolder(&files.ListFolderArg{Path: "/test/path/sub"}).Return(nil, tt.err).Once()

			var failed []vfs.Entry
			for e, err := range list.Entries(s.T().Context(), s.location, list.Recursive()) {
				if err != nil {
					s.Require().ErrorIs(err, tt.expected)
					failed = append(failed, e)
				}
			}
			s.Require().Len(failed, 1, "the folder is reported with its entry and the listing continues")
			s.Equal("sub", failed[0].RelPath())
		})
	}
}

func (s *ListFailureTestSuite) TestOtherFailuresAreNotMappedToStandardErrors() {
	boom := errors.New("too_many_requests")
	s.client.EXPECT().ListFolder(mock.Anything).Return(nil, boom).Once()

	_, err := list.Collect(list.Entries(s.T().Context(), s.location))
	s.Require().ErrorIs(err, boom)
	s.Require().NotErrorIs(err, iofs.ErrNotExist)
	s.Require().NotErrorIs(err, iofs.ErrPermission)
}

func (s *ListFailureTestSuite) TestAuthFailureEndsTheListing() {
	authErr := auth.AuthAPIError{APIError: dropbox.APIError{ErrorSummary: "expired_access_token/."}}
	s.client.EXPECT().ListFolder(&files.ListFolderArg{Path: "/test/path"}).
		Return(&files.ListFolderResult{Entries: []files.IsMetadata{s.folder("a"), s.folder("b")}}, nil).Once()
	s.client.EXPECT().ListFolder(&files.ListFolderArg{Path: "/test/path/a"}).Return(nil, authErr).Once()

	var errs []error
	for e, err := range list.Entries(s.T().Context(), s.location, list.Recursive()) {
		if err != nil {
			s.Equal(vfs.Entry{}.RelPath(), e.RelPath(), "the failure carries no entry")
			errs = append(errs, err)
		}
	}
	s.Require().Len(errs, 1, "folder b is never requested")
	s.Require().ErrorAs(errs[0], new(auth.AuthAPIError))
}

func (s *ListFailureTestSuite) TestCancellationStopsTheRequests() {
	ctx, cancel := context.WithCancel(s.T().Context())
	s.client.EXPECT().ListFolder(mock.Anything).RunAndReturn(func(*files.ListFolderArg) (*files.ListFolderResult, error) {
		cancel()
		return &files.ListFolderResult{Entries: []files.IsMetadata{s.file("a.txt")}, Cursor: "c1", HasMore: true}, nil
	}).Once()

	_, err := list.Collect(list.Entries(ctx, s.location))
	s.Require().ErrorIs(err, context.Canceled)
}

func (s *ListFailureTestSuite) TestFolderPathHasNoTrailingSlash() {
	s.client.EXPECT().ListFolder(&files.ListFolderArg{Path: "/test/path"}).
		Return(&files.ListFolderResult{Entries: []files.IsMetadata{s.folder("sub")}}, nil).Once()
	s.client.EXPECT().ListFolder(&files.ListFolderArg{Path: "/test/path/sub"}).
		Return(&files.ListFolderResult{}, nil).Once()

	_, err := list.Collect(list.Entries(s.T().Context(), s.location, list.Recursive()))
	s.Require().NoError(err)
}

// TransportTestSuite checks the escaping of the Dropbox-API-Arg header.
type TransportTestSuite struct{ suite.Suite }

func TestTransportTestSuite(t *testing.T) { suite.Run(t, new(TransportTestSuite)) }

func (s *TransportTestSuite) TestHeaderSafeJSON() {
	tests := []struct {
		name, in, expected string
	}{
		{name: "ASCII is unchanged", in: `{"path":"/a b.txt"}`, expected: `{"path":"/a b.txt"}`},
		{name: "a Latin-1 letter", in: "{\"path\":\"/café\"}", expected: `{"path":"/caf\u00e9"}`},
		{name: "a character in the Basic Multilingual Plane", in: "日", expected: `\u65e5`},
		{name: "a character above U+FFFF is a surrogate pair", in: "😀", expected: `\ud83d\ude00`},
		{name: "DEL is escaped", in: "\x7f", expected: `\u007f`},
		{name: "empty", in: "", expected: ""},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() { s.Equal(tt.expected, headerSafeJSON(tt.in)) })
	}
}

func (s *TransportTestSuite) TestRoundTripAuthenticatesAndEscapesWithoutModifyingTheRequest() {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r }))
	defer srv.Close()

	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodPost, srv.URL, http.NoBody)
	s.Require().NoError(err)
	req.Header.Set(apiArgHeader, "{\"path\":\"/ü\"}")

	resp, err := headerSafeTransport{token: "tok"}.RoundTrip(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())

	s.Equal("Bearer tok", got.Header.Get("Authorization"))
	s.Equal(`{"path":"/\u00fc"}`, got.Header.Get(apiArgHeader)) //nolint:testifylint // the exact escaped text is what is tested
	s.Equal("{\"path\":\"/ü\"}", req.Header.Get(apiArgHeader),  //nolint:testifylint // the exact text is what is tested
		"the caller's request is untouched")
	s.Empty(req.Header.Get("Authorization"))
}

func (s *TransportTestSuite) TestNewHTTPClientUsesTheTransport() {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r }))
	defer srv.Close()
	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodPost, srv.URL, http.NoBody)
	s.Require().NoError(err)
	req.Header.Set(apiArgHeader, "é")

	resp, err := newHTTPClient("tok", nil).Do(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())
	s.Equal("Bearer tok", got.Header.Get("Authorization"))
	s.Equal(`\u00e9`, got.Header.Get(apiArgHeader))
}

func (s *TransportTestSuite) TestRequestsWithoutAnArgumentHeaderAreNotGivenOne() {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r }))
	defer srv.Close()
	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodPost, srv.URL, http.NoBody)
	s.Require().NoError(err)

	resp, err := headerSafeTransport{token: "tok"}.RoundTrip(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())
	s.NotContains(got.Header, apiArgHeader)
}

// iterPull turns a sequence into pull-style next and stop functions.
func iterPull[K, V any](seq iter.Seq2[K, V]) (next func() (K, V, bool), stop func()) {
	return iter.Pull2(seq)
}
