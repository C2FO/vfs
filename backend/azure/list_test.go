package azure

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/azure/mocks"
	"github.com/c2fo/vfs/v7/backend/azure/types"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// listingClient is a Client that can also list natively, which is what the default client looks like.
type listingClient struct {
	*mocks.Client
	*mocks.BlobLister
}

type ListEntriesTestSuite struct {
	suite.Suite
	client *listingClient
	pager  *mocks.BlobPager
	fs     *FileSystem
}

func TestListEntriesTestSuite(t *testing.T) {
	suite.Run(t, new(ListEntriesTestSuite))
}

func (s *ListEntriesTestSuite) SetupTest() {
	s.client = &listingClient{Client: mocks.NewClient(s.T()), BlobLister: mocks.NewBlobLister(s.T())}
	s.pager = mocks.NewBlobPager(s.T())
	s.fs = NewFileSystem(WithClient(s.client))
}

func (s *ListEntriesTestSuite) location(path string) vfs.Location {
	s.T().Helper()
	loc, err := s.fs.NewLocation("container", path)
	s.Require().NoError(err)
	return loc
}

// expectPager expects the pager to be created with exactly req.
func (s *ListEntriesTestSuite) expectPager(req types.BlobListRequest) {
	s.client.BlobLister.EXPECT().BlobPager(mock.Anything, "container", req).Return(s.pager, nil).Once()
}

func (s *ListEntriesTestSuite) keys(loc vfs.Location, opts ...options.ListOption) []string {
	s.T().Helper()
	entries, err := list.Collect(list.Entries(s.T().Context(), loc, opts...))
	s.Require().NoError(err)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func blobs(names ...string) []types.BlobItem {
	items := make([]types.BlobItem, len(names))
	for i, name := range names {
		items[i] = types.BlobItem{Name: name, Size: int64(len(name)), LastModified: time.Unix(1_700_000_000, 0), Native: name}
	}
	return items
}

func (s *ListEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location("/dir1/"))
}

func (s *ListEntriesTestSuite) TestAPageOfBlobsMakesOneRequest() {
	const total = 2500
	all := make([]string, total)
	for i := range all {
		all[i] = fmt.Sprintf("dir1/k%04d", i)
	}
	s.expectPager(types.BlobListRequest{Prefix: "dir1/", PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{Blobs: blobs(all[:1000]...)}, nil).Once()
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{Blobs: blobs(all[1000:2000]...)}, nil).Once()
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{Blobs: blobs(all[2000:]...), Done: true}, nil).Once()

	keys := s.keys(s.location("/dir1/"), list.Recursive())

	s.Require().Len(keys, total)
	s.Equal("k0000", keys[0])
	s.Equal("k2499", keys[total-1])
}

func (s *ListEntriesTestSuite) TestNonRecursiveListingIsAHierarchyListing() {
	s.expectPager(types.BlobListRequest{Prefix: "dir1/", Delimited: true, PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{
		Blobs:    blobs("dir1/", "dir1/a.txt", "dir1/z.txt"),
		Prefixes: []string{"dir1/logs/", "dir1/sub/"},
		Done:     true,
	}, nil).Once()

	s.Equal([]string{"a.txt", "logs/", "sub/", "z.txt"}, s.keys(s.location("/dir1/")))
}

func (s *ListEntriesTestSuite) TestContainerRootHasNoPrefix() {
	s.expectPager(types.BlobListRequest{Delimited: true, PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{
		Blobs: blobs("a.txt"), Prefixes: []string{"logs/"}, Done: true,
	}, nil).Once()

	s.Equal([]string{"a.txt", "logs/"}, s.keys(s.location("/")))
}

func (s *ListEntriesTestSuite) TestDirectoriesAreSynthesizedFromNames() {
	s.expectPager(types.BlobListRequest{Prefix: "dir1/", PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{
		Blobs: blobs("dir1/a/b/c.txt", "dir1/folder/"), Done: true,
	}, nil).Once()

	s.Equal([]string{"a/", "a/b/", "a/b/c.txt", "folder/"}, s.keys(s.location("/dir1/"), list.Recursive()))
}

func (s *ListEntriesTestSuite) TestPrefixIsSentAndTheCursorIsAppliedLocally() {
	s.expectPager(types.BlobListRequest{Prefix: "dir1/logs/o", Delimited: true, PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{
		Blobs: blobs("dir1/logs/other.log"), Prefixes: []string{"dir1/logs/old/"}, Done: true,
	}, nil).Once()

	s.Equal([]string{"logs/other.log"}, s.keys(s.location("/dir1/"), list.Prefix("logs/o"), list.StartAfter("logs/old/")))
}

func (s *ListEntriesTestSuite) TestCursorInsideADirectoryDoesNotRepeatIt() {
	tests := []struct {
		name   string
		cursor string
	}{
		{name: "cursor on the directory itself", cursor: "logs/"},
		{name: "cursor deep inside the directory", cursor: "logs/old/2024.log"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.expectPager(types.BlobListRequest{Prefix: "dir1/", Delimited: true, PageSize: 1000})
			s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{
				Blobs: blobs("dir1/a.txt", "dir1/z.txt"), Prefixes: []string{"dir1/logs/"}, Done: true,
			}, nil).Once()

			s.Equal([]string{"z.txt"}, s.keys(s.location("/dir1/"), list.StartAfter(tt.cursor)))
		})
	}
}

func (s *ListEntriesTestSuite) TestEntriesCarryTheListedMetadata() {
	listed := blobs("dir1/a.txt")
	s.expectPager(types.BlobListRequest{Prefix: "dir1/", PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{Blobs: listed, Done: true}, nil).Once()

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/"), list.Recursive()))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]
	s.True(e.HasMetadata())
	s.Equal(uint64(len("dir1/a.txt")), e.Size())
	s.True(listed[0].LastModified.Equal(e.ModTime()))
	s.Equal("dir1/a.txt", e.Sys())
	info, err := e.Info()
	s.Require().NoError(err, "listed metadata needs no Properties request")
	s.Equal(int64(len("dir1/a.txt")), info.Size())
	s.Equal("/dir1/a.txt", e.Path())
}

func (s *ListEntriesTestSuite) TestStoppingEarlyStopsTheRequests() {
	s.expectPager(types.BlobListRequest{Prefix: "dir1/", PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{Blobs: blobs("dir1/a.txt", "dir1/b.txt")}, nil).Once()

	for _, err := range s.location("/dir1/").(vfs.EntryLister).ListEntries(s.T().Context(), list.Recursive()) {
		s.Require().NoError(err)
		break
	}
}

func (s *ListEntriesTestSuite) TestForbiddenMatchesErrPermission() {
	s.expectPager(types.BlobListRequest{Prefix: "dir1/", Delimited: true, PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{}, &azcore.ResponseError{StatusCode: http.StatusForbidden}).Once()

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/")))

	s.Require().ErrorIs(err, iofs.ErrPermission)
	var pe *iofs.PathError
	s.Require().ErrorAs(err, &pe)
	s.Equal("list", pe.Op)
}

func (s *ListEntriesTestSuite) TestMissingContainerIsAnError() {
	s.expectPager(types.BlobListRequest{Prefix: "dir1/", Delimited: true, PageSize: 1000})
	notFound := &azcore.ResponseError{StatusCode: http.StatusNotFound, ErrorCode: "ContainerNotFound"}
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{}, notFound).Once()

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/")))

	var respErr *azcore.ResponseError
	s.Require().ErrorAs(err, &respErr)
	s.NotErrorIs(err, iofs.ErrNotExist)
}

func (s *ListEntriesTestSuite) TestPagerCreationFailureIsReported() {
	boom := errors.New("boom")
	s.client.BlobLister.EXPECT().BlobPager(mock.Anything, "container", mock.Anything).Return(nil, boom).Once()

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/")))

	s.Require().ErrorIs(err, boom)
}

func (s *ListEntriesTestSuite) TestLaterPageFailureEndsTheListing() {
	boom := errors.New("boom")
	s.expectPager(types.BlobListRequest{Prefix: "dir1/", PageSize: 1000})
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{Blobs: blobs("dir1/a.txt")}, nil).Once()
	s.pager.EXPECT().NextPage(mock.Anything).Return(types.BlobPage{}, boom).Once()

	var keys []string
	var errs []error
	for e, err := range list.Entries(s.T().Context(), s.location("/dir1/"), list.Recursive()) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		keys = append(keys, e.RelPath())
	}

	s.Equal([]string{"a.txt"}, keys)
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], boom)
}

func (s *ListEntriesTestSuite) TestClientWithoutBlobListerIsUnsupported() {
	plain := NewFileSystem(WithClient(mocks.NewClient(s.T())))
	loc, err := plain.NewLocation("container", "/dir1/")
	s.Require().NoError(err)

	_, err = list.Collect(list.Entries(s.T().Context(), loc))

	s.Require().ErrorIs(err, errors.ErrUnsupported)
	s.Require().ErrorIs(err, errNoBlobLister)
}

func (s *ListEntriesTestSuite) TestInvalidOptionsMakeNoRequest() {
	var errs []error
	for _, err := range s.location("/dir1/").(vfs.EntryLister).ListEntries(s.T().Context(), list.FilesOnly(), list.DirsOnly()) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], list.ErrInvalidOption)
}

func (s *ListEntriesTestSuite) TestCanceledContextMakesNoRequest() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	var errs []error
	for _, err := range s.location("/dir1/").(vfs.EntryLister).ListEntries(ctx) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], context.Canceled)
}

func (s *ListEntriesTestSuite) TestClientCreationFailureIsReported() {
	fileSystem := NewFileSystem(WithOptions(Options{
		TenantID: "t", ClientID: "c", ClientSecret: "s", tokenCredentialFactory: failingTokenFactory,
	}))
	loc, err := fileSystem.NewLocation("container", "/dir1/")
	s.Require().NoError(err)

	for range 2 {
		_, err = list.Collect(list.Entries(s.T().Context(), loc))
		s.Require().ErrorContains(err, "no token", "a client that failed to build must fail every time, not once")
	}
}

func failingTokenFactory(_, _, _ string) (azcore.TokenCredential, error) {
	return nil, errors.New("no token")
}

// BlobPagerTestSuite runs the default client's pagers against a server that answers like the Blob service.
type BlobPagerTestSuite struct {
	suite.Suite
	requests []string
	status   int
	client   *DefaultClient
}

func TestBlobPagerTestSuite(t *testing.T) {
	suite.Run(t, new(BlobPagerTestSuite))
}

// listResponse answers a container list request with pages of the 2,500 blobs "dir1/k0000"... and, for a hierarchy
// request, one virtual directory on the last page.
func (s *BlobPagerTestSuite) listResponse(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	s.requests = append(s.requests, query.Get("marker"))
	if s.status != 0 {
		w.Header().Set("x-ms-error-code", "AuthorizationFailure")
		w.WriteHeader(s.status)
		return
	}

	const total = 2500
	start := 0
	if marker := query.Get("marker"); marker != "" {
		start, _ = strconv.Atoi(marker)
	}
	end := min(start+1000, total)

	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?><EnumerationResults ContainerName="container"><Blobs>`)
	for i := start; i < end; i++ {
		fmt.Fprintf(&body, `<Blob><Name>dir1/k%04d</Name><Properties><Last-Modified>Thu, 01 Jan 2026 00:00:00 GMT</Last-Modified>`+
			`<Content-Length>%d</Content-Length></Properties></Blob>`, i, i)
	}
	if query.Get("delimiter") != "" && end == total {
		body.WriteString(`<BlobPrefix><Name>dir1/sub/</Name></BlobPrefix>`)
	}
	body.WriteString(`</Blobs>`)
	if end < total {
		fmt.Fprintf(&body, `<NextMarker>%d</NextMarker>`, end)
	} else {
		body.WriteString(`<NextMarker/>`)
	}
	body.WriteString(`</EnumerationResults>`)

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(body.String()))
}

func (s *BlobPagerTestSuite) SetupTest() {
	s.requests, s.status = nil, 0
	server := httptest.NewServer(http.HandlerFunc(s.listResponse))
	s.T().Cleanup(server.Close)

	client, err := NewClient(&Options{ServiceURL: server.URL})
	s.Require().NoError(err)
	s.client = client
}

func (s *BlobPagerTestSuite) TestFlatListingPagesThroughTheMarker() {
	pager, err := s.client.BlobPager(s.T().Context(), "container", types.BlobListRequest{Prefix: "dir1/", PageSize: 1000})
	s.Require().NoError(err)

	var names []string
	for done := false; !done; {
		page, err := pager.NextPage(s.T().Context())
		s.Require().NoError(err)
		for _, blob := range page.Blobs {
			names = append(names, blob.Name)
		}
		s.Empty(page.Prefixes, "a flat listing has no prefixes")
		done = page.Done
	}

	s.Len(names, 2500)
	s.Equal([]string{"", "1000", "2000"}, s.requests, "2,500 blobs take exactly three requests")

	page, err := pager.NextPage(s.T().Context())
	s.Require().NoError(err)
	s.True(page.Done)
	s.Empty(page.Blobs)
	s.Len(s.requests, 3, "a finished pager makes no request")
}

func (s *BlobPagerTestSuite) TestHierarchyListingReturnsPrefixesAndMetadata() {
	pager, err := s.client.BlobPager(s.T().Context(), "container", types.BlobListRequest{Prefix: "dir1/", Delimited: true})
	s.Require().NoError(err)

	var last types.BlobPage
	for !last.Done {
		last, err = pager.NextPage(s.T().Context())
		s.Require().NoError(err)
	}

	s.Equal([]string{"dir1/sub/"}, last.Prefixes)
	s.Require().NotEmpty(last.Blobs)
	blob := last.Blobs[len(last.Blobs)-1]
	s.Equal("dir1/k2499", blob.Name)
	s.Equal(int64(2499), blob.Size)
	s.True(blob.LastModified.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	s.IsType(&container.BlobItem{}, blob.Native)
}

func (s *BlobPagerTestSuite) TestFailedRequestsAreReturnedByBothPagers() {
	s.status = http.StatusForbidden

	for _, delimited := range []bool{false, true} {
		pager, err := s.client.BlobPager(s.T().Context(), "container", types.BlobListRequest{Delimited: delimited})
		s.Require().NoError(err)

		_, err = pager.NextPage(s.T().Context())

		var respErr *azcore.ResponseError
		s.Require().ErrorAs(err, &respErr, "delimited %v", delimited)
		s.Equal(http.StatusForbidden, respErr.StatusCode)
		s.Require().ErrorIs(classifyListError(err), iofs.ErrPermission)
	}
}

func (s *BlobPagerTestSuite) TestBlobItemToleratesMissingFields() {
	s.Equal(types.BlobItem{}, blobItem(nil))

	item := blobItem(&container.BlobItem{})
	s.Empty(item.Name)
	s.Zero(item.Size)
	s.True(item.LastModified.IsZero())
}
