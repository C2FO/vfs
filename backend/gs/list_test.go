package gs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	iofs "io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/suite"
	"google.golang.org/api/option"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// countingTransport records the object-list requests that reach the server, and can answer them with a failure.
type countingTransport struct {
	base http.RoundTripper

	mu       sync.Mutex
	requests []string
	status   int
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/o") {
		c.mu.Lock()
		c.requests = append(c.requests, req.URL.RawQuery)
		status := c.status
		c.mu.Unlock()
		if status != 0 {
			return &http.Response{
				StatusCode: status,
				Status:     http.StatusText(status),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}
	}
	return c.base.RoundTrip(req)
}

func (c *countingTransport) listRequests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.requests...)
}

type listEntriesTestSuite struct {
	suite.Suite
	server    *fakestorage.Server
	transport *countingTransport
	fs        *FileSystem
}

func TestListEntriesTestSuite(t *testing.T) {
	suite.Run(t, new(listEntriesTestSuite))
}

const listBucket = "list-bucket"

func (s *listEntriesTestSuite) SetupTest() {
	s.server = fakestorage.NewServer(nil)
	s.server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: listBucket})
	s.T().Cleanup(s.server.Stop)

	s.transport = &countingTransport{base: s.server.HTTPClient().Transport}
	client, err := storage.NewClient(s.T().Context(),
		option.WithHTTPClient(&http.Client{Transport: s.transport}), option.WithoutAuthentication())
	s.Require().NoError(err)
	s.fs = NewFileSystem(WithClient(client))
}

func (s *listEntriesTestSuite) put(names ...string) {
	for _, name := range names {
		s.server.CreateObject(fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: listBucket, Name: name},
			Content:     []byte(name),
		})
	}
}

func (s *listEntriesTestSuite) location(path string) vfs.Location {
	s.T().Helper()
	loc, err := s.fs.NewLocation(listBucket, path)
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

func (s *listEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location("/dir1/"))
}

func (s *listEntriesTestSuite) TestRecursiveListing() {
	s.put("dir1/a.txt", "dir1/logs/app.log", "dir1/logs/old/2023.log", "dir1/z.txt", "other/x.txt", "dir10/y.txt")

	s.Equal([]string{"a.txt", "logs/", "logs/app.log", "logs/old/", "logs/old/2023.log", "z.txt"},
		s.keys(s.location("/dir1/"), list.Recursive()))
}

func (s *listEntriesTestSuite) TestNonRecursiveListingRollsUpDirectories() {
	s.put("dir1/a.txt", "dir1/logs/app.log", "dir1/logs/old/2023.log", "dir1/z.txt")

	s.Equal([]string{"a.txt", "logs/", "z.txt"}, s.keys(s.location("/dir1/")))
}

func (s *listEntriesTestSuite) TestFileSortsBeforeADirectoryItSharesAPrefixWith() {
	s.put("dir1/logs-2024.gz", "dir1/logs/app.log")

	s.Equal([]string{"logs-2024.gz", "logs/"}, s.keys(s.location("/dir1/")))
}

func (s *listEntriesTestSuite) TestBucketRoot() {
	s.put("a.txt", "logs/app.log")

	s.Equal([]string{"a.txt", "logs/"}, s.keys(s.location("/")))
	s.Equal([]string{"a.txt", "logs/", "logs/app.log"}, s.keys(s.location("/"), list.Recursive()))
}

func (s *listEntriesTestSuite) TestFolderPlaceholdersAreFoldedIntoDirectories() {
	s.put("dir1/", "dir1/folder/", "dir1/folder/a.txt", "dir1/empty/")

	s.Equal([]string{"empty/", "folder/", "folder/a.txt"}, s.keys(s.location("/dir1/"), list.Recursive()))
}

func (s *listEntriesTestSuite) TestPrefixAndCursor() {
	s.put("dir1/logs/app.log", "dir1/logs/old/2023.log", "dir1/logs/other.log", "dir1/z.txt")

	s.Equal([]string{"logs/other.log"}, s.keys(s.location("/dir1/"), list.Prefix("logs/o"), list.StartAfter("logs/old/")))
	s.Equal([]string{"logs/old/", "logs/old/2023.log", "logs/other.log", "z.txt"},
		s.keys(s.location("/dir1/"), list.Recursive(), list.StartAfter("logs/app.log")))
}

func (s *listEntriesTestSuite) TestCursorInsideADirectoryDoesNotRepeatIt() {
	s.put("dir1/a.txt", "dir1/logs/app.log", "dir1/logs/old/2024.log", "dir1/z.txt")

	s.Equal([]string{"z.txt"}, s.keys(s.location("/dir1/"), list.StartAfter("logs/")))
	s.Equal([]string{"z.txt"}, s.keys(s.location("/dir1/"), list.StartAfter("logs/old/2024.log")))
}

func (s *listEntriesTestSuite) TestEntriesCarryTheListedMetadata() {
	s.put("dir1/a.txt")

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/"), list.Recursive()))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]
	s.True(e.HasMetadata())
	s.Equal(uint64(len("dir1/a.txt")), e.Size())
	s.False(e.ModTime().IsZero())
	attrs, ok := e.Sys().(*storage.ObjectAttrs)
	s.Require().True(ok, "Sys is the listed object")
	s.Equal("dir1/a.txt", attrs.Name)

	requests := len(s.transport.listRequests())
	info, err := e.Info()
	s.Require().NoError(err)
	s.Equal(int64(len("dir1/a.txt")), info.Size())
	s.Len(s.transport.listRequests(), requests, "listed metadata needs no request")
	s.Equal("/dir1/a.txt", e.Path())
}

func (s *listEntriesTestSuite) TestListingMatchesFileMetadata() {
	s.put("dir1/a.txt")
	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/")))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)

	f, err := entries[0].File()
	s.Require().NoError(err)
	modTime, err := f.LastModified()
	s.Require().NoError(err)
	s.True(modTime.Equal(entries[0].ModTime()))
}

// pagedTransport answers object-list requests with synthetic pages of up to 1,000 objects, as GCS does, because the fake
// server returns every object in one response.
type pagedTransport struct {
	total int

	mu       sync.Mutex
	requests []string
}

func (p *pagedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req.URL.Query().Get("pageToken"))
	p.mu.Unlock()

	start := 0
	if token := req.URL.Query().Get("pageToken"); token != "" {
		start, _ = strconv.Atoi(token)
	}
	end := min(start+listPageSize, p.total)
	items := make([]map[string]string, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, map[string]string{
			"name": fmt.Sprintf("dir1/k%04d", i), "bucket": listBucket, "size": "10", "updated": "2026-01-01T00:00:00Z",
		})
	}
	body := map[string]any{"kind": "storage#objects", "items": items}
	if end < p.total {
		body["nextPageToken"] = strconv.Itoa(end)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
		Request:    req,
	}, nil
}

func (s *listEntriesTestSuite) pagedLocation(total int) (vfs.Location, *pagedTransport) {
	s.T().Helper()
	transport := &pagedTransport{total: total}
	client, err := storage.NewClient(s.T().Context(),
		option.WithHTTPClient(&http.Client{Transport: transport}), option.WithoutAuthentication())
	s.Require().NoError(err)
	loc, err := NewFileSystem(WithClient(client)).NewLocation(listBucket, "/dir1/")
	s.Require().NoError(err)
	return loc, transport
}

func (s *listEntriesTestSuite) TestAThousandObjectsMakeOneRequest() {
	loc, transport := s.pagedLocation(2500)

	keys := s.keys(loc, list.Recursive())

	s.Require().Len(keys, 2500)
	s.Equal("k0000", keys[0])
	s.Equal("k2499", keys[2499])
	s.Equal([]string{"", "1000", "2000"}, transport.requests, "2,500 objects take exactly three list requests")
}

func (s *listEntriesTestSuite) TestStoppingEarlyStopsTheRequests() {
	loc, transport := s.pagedLocation(2500)

	for _, err := range loc.(vfs.EntryLister).ListEntries(s.T().Context(), list.Recursive()) {
		s.Require().NoError(err)
		break
	}

	s.Equal([]string{""}, transport.requests, "stopping after the first entry stops at the first page")
}

func (s *listEntriesTestSuite) TestPageBoundaryFallsBetweenEntries() {
	loc, transport := s.pagedLocation(1000)

	s.Len(s.keys(loc, list.Recursive()), 1000)
	s.Equal([]string{""}, transport.requests, "a full final page needs no further request")
}

func (s *listEntriesTestSuite) TestMissingBucketIsAnError() {
	loc, err := s.fs.NewLocation("no-such-bucket", "/dir1/")
	s.Require().NoError(err)

	_, err = list.Collect(list.Entries(s.T().Context(), loc))

	s.Require().ErrorIs(err, storage.ErrBucketNotExist)
	s.NotErrorIs(err, iofs.ErrNotExist)
}

func (s *listEntriesTestSuite) TestForbiddenMatchesErrPermission() {
	s.transport.status = http.StatusForbidden

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/")))

	s.Require().ErrorIs(err, iofs.ErrPermission)
	var pe *iofs.PathError
	s.Require().ErrorAs(err, &pe)
	s.Equal("list", pe.Op)
}

func (s *listEntriesTestSuite) TestEmptyLocation() {
	s.put("other/a.txt")

	s.Empty(s.keys(s.location("/dir1/"), list.Recursive()))
}

func (s *listEntriesTestSuite) TestInvalidOptionsMakeNoRequest() {
	var errs []error
	for _, err := range s.location("/dir1/").(vfs.EntryLister).ListEntries(s.T().Context(), list.FilesOnly(), list.DirsOnly()) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], list.ErrInvalidOption)
	s.Empty(s.transport.listRequests())
}

func (s *listEntriesTestSuite) TestCanceledContextMakesNoRequest() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	var errs []error
	for _, err := range s.location("/dir1/").(vfs.EntryLister).ListEntries(ctx) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], context.Canceled)
	s.Empty(s.transport.listRequests())
}

func (s *listEntriesTestSuite) TestClientErrorIsReported() {
	fileSystem := &FileSystem{clientCreator: &mockClientCreatorWithError{}, ctx: s.T().Context()}
	loc, err := fileSystem.NewLocation(listBucket, "/dir1/")
	s.Require().NoError(err)

	for range 2 {
		_, err = list.Collect(list.Entries(s.T().Context(), loc))
		s.Require().ErrorContains(err, "mock error")
	}
}
