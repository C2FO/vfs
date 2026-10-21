package gdrive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type DriveClientTestSuite struct {
	suite.Suite
	drive  *fakeDrive
	fs     *FileSystem
	client *driveClient
	ctx    context.Context
}

func TestDriveClientTestSuite(t *testing.T) {
	suite.Run(t, new(DriveClientTestSuite))
}

func (s *DriveClientTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.drive = newFakeDrive(s.T())
	s.fs = newTestFS(s.T(), s.drive)
	c, err := s.fs.Client("")
	s.Require().NoError(err)
	s.client = c.(*driveClient)
}

func (s *DriveClientTestSuite) upload(p, content string) {
	_, err := s.client.Upload(s.ctx, p, strings.NewReader(content))
	s.Require().NoError(err)
}

func (s *DriveClientTestSuite) read(p string) string {
	rc, err := s.client.Download(s.ctx, p)
	s.Require().NoError(err)
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	s.Require().NoError(err)
	return string(data)
}

func (s *DriveClientTestSuite) TestUploadCreatesParentsAndReplaces() {
	s.upload("a/b/c.txt", "one")
	s.Equal("one", s.read("a/b/c.txt"))

	s.upload("a/b/c.txt", "two")
	s.Equal("two", s.read("a/b/c.txt"))
	s.Equal(1, s.drive.liveFiles(), "replace must update in place rather than create a duplicate")

	item, err := s.client.GetItem(s.ctx, "a/b/c.txt")
	s.Require().NoError(err)
	s.Equal(int64(3), item.Size)
	s.False(item.IsFolder)
}

func (s *DriveClientTestSuite) TestGetItem() {
	s.upload("dir/file.txt", "x")

	tests := []struct {
		name        string
		path        string
		expectedErr error
		isFolder    bool
	}{
		{name: "root", path: "", isFolder: true},
		{name: "folder", path: "dir", isFolder: true},
		{name: "file", path: "dir/file.txt"},
		{name: "missing file", path: "dir/nope.txt", expectedErr: ErrNotFound},
		{name: "missing folder", path: "nope/file.txt", expectedErr: ErrNotFound},
		{name: "file used as folder", path: "dir/file.txt/child", expectedErr: ErrNotFolder},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			item, err := s.client.GetItem(s.ctx, tt.path)
			if tt.expectedErr != nil {
				s.Require().ErrorIs(err, tt.expectedErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.isFolder, item.IsFolder)
		})
	}
}

func (s *DriveClientTestSuite) TestDuplicateNamesAreAmbiguous() {
	folder := s.drive.addFolder("dup")
	s.drive.addFile(folder, "same.txt", []byte("1"))
	s.drive.addFile(folder, "same.txt", []byte("2"))
	s.drive.addFolder("twin")
	s.drive.addFolder("twin")

	_, err := s.client.GetItem(s.ctx, "dup/same.txt")
	s.Require().ErrorIs(err, ErrAmbiguous)

	_, err = s.client.Download(s.ctx, "dup/same.txt")
	s.Require().ErrorIs(err, ErrAmbiguous)

	s.Require().ErrorIs(s.client.Delete(s.ctx, "dup/same.txt"), ErrAmbiguous)

	_, err = s.client.Upload(s.ctx, "dup/same.txt", strings.NewReader("x"))
	s.Require().ErrorIs(err, ErrAmbiguous)

	_, err = s.client.CreateUploadSession(s.ctx, "dup/same.txt", 10)
	s.Require().ErrorIs(err, ErrAmbiguous)

	_, err = s.client.GetItem(s.ctx, "twin/file.txt")
	s.Require().ErrorIs(err, ErrAmbiguous, "ambiguous intermediate folder")

	_, err = s.client.ListChildren(s.ctx, "dup")
	s.Require().NoError(err)
	items, _ := s.client.ListChildren(s.ctx, "dup")
	s.Len(items, 2, "listing shows both duplicates")
}

func (s *DriveClientTestSuite) TestFolderGuard() {
	s.upload("dir/file.txt", "x")
	s.upload("other/keep.txt", "keep")

	s.Require().ErrorIs(s.client.Delete(s.ctx, "dir"), ErrIsFolder)
	s.Require().ErrorIs(s.client.Copy(s.ctx, "dir", "other", "dir"), ErrIsFolder)
	_, err := s.client.Move(s.ctx, "dir", "other", "dir")
	s.Require().ErrorIs(err, ErrIsFolder)
	_, err = s.client.Download(s.ctx, "dir")
	s.Require().ErrorIs(err, ErrIsFolder)
	_, err = s.client.Upload(s.ctx, "dir", strings.NewReader("x"))
	s.Require().ErrorIs(err, ErrIsFolder)
	s.Require().ErrorIs(s.client.Touch(s.ctx, "dir"), ErrIsFolder)

	s.Equal("x", s.read("dir/file.txt"), "folder contents survive")

	s.upload("other/target", "t")
	s.Require().NoError(s.client.Delete(s.ctx, "other/target"))
	s.drive.addFolder("destfolder")
	s.Require().ErrorIs(s.client.Copy(s.ctx, "other/keep.txt", "", "destfolder"), ErrIsFolder, "destination is a folder")
}

func (s *DriveClientTestSuite) TestListChildrenPaginates() {
	s.drive.pageSize = 2
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		s.upload("dir/"+n, n)
	}

	items, err := s.client.ListChildren(s.ctx, "dir")
	s.Require().NoError(err)
	s.Len(items, 5)

	_, err = s.client.ListChildren(s.ctx, "missing")
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *DriveClientTestSuite) TestSpecialCharacterNames() {
	names := []string{
		"it's.txt", `back\slash.txt`, "100% done.txt", "a#b&c+d.txt", "with space.txt", "ünï©ode.txt",
		`quote'and\both.txt`,
	}

	for _, n := range names {
		s.Run(n, func() {
			p := "sp'ec/" + n
			s.upload(p, n)
			s.Equal(n, s.read(p))

			items, err := s.client.ListChildren(s.ctx, "sp'ec")
			s.Require().NoError(err)
			var found bool
			for _, it := range items {
				found = found || it.Name == n
			}
			s.True(found)
		})
	}
}

func (s *DriveClientTestSuite) TestCopyAndMove() {
	s.upload("src/a.txt", "A")
	s.upload("dst/existing.txt", "OLD")
	s.drive.addFolder("empty")

	// copy replaces an existing destination without leaving duplicates
	s.Require().NoError(s.client.Copy(s.ctx, "src/a.txt", "dst", "existing.txt"))
	s.Equal("A", s.read("dst/existing.txt"))
	s.Equal("A", s.read("src/a.txt"))
	s.Equal(2, s.drive.liveFiles())

	// copy to a new name, and onto itself
	s.Require().NoError(s.client.Copy(s.ctx, "src/a.txt", "dst", "fresh.txt"))
	s.Equal("A", s.read("dst/fresh.txt"))
	s.Require().NoError(s.client.Copy(s.ctx, "src/a.txt", "src", "a.txt"))

	// move replaces an existing destination and removes the source
	s.upload("src/m.txt", "M")
	item, err := s.client.Move(s.ctx, "src/m.txt", "dst", "existing.txt")
	s.Require().NoError(err)
	s.Equal("existing.txt", item.Name)
	s.Equal("M", s.read("dst/existing.txt"))
	_, err = s.client.GetItem(s.ctx, "src/m.txt")
	s.Require().ErrorIs(err, ErrNotFound)

	// rename within the same folder
	_, err = s.client.Move(s.ctx, "src/a.txt", "src", "renamed.txt")
	s.Require().NoError(err)
	s.Equal("A", s.read("src/renamed.txt"))

	// move onto itself
	_, err = s.client.Move(s.ctx, "src/renamed.txt", "src", "renamed.txt")
	s.Require().NoError(err)

	// missing destination folder and missing source
	s.Require().ErrorIs(s.client.Copy(s.ctx, "src/renamed.txt", "nope", "x"), ErrNotFound)
	_, err = s.client.Move(s.ctx, "src/renamed.txt", "nope", "x")
	s.Require().ErrorIs(err, ErrNotFound)
	s.Require().ErrorIs(s.client.Copy(s.ctx, "src/ghost.txt", "dst", "x"), ErrNotFound)
	_, err = s.client.Move(s.ctx, "ghost/ghost.txt", "dst", "x")
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *DriveClientTestSuite) TestDeleteAndTrash() {
	s.upload("d/a.txt", "a")
	s.Require().NoError(s.client.Delete(s.ctx, "d/a.txt"))
	_, err := s.client.GetItem(s.ctx, "d/a.txt")
	s.Require().ErrorIs(err, ErrNotFound)
	s.Require().ErrorIs(s.client.Delete(s.ctx, "d/a.txt"), ErrNotFound)

	s.client.trash = true
	s.upload("d/b.txt", "b")
	s.Require().NoError(s.client.Delete(s.ctx, "d/b.txt"))
	_, err = s.client.GetItem(s.ctx, "d/b.txt")
	s.Require().ErrorIs(err, ErrNotFound, "trashed files are invisible")
	s.Equal(0, s.drive.liveFiles())
	s.Len(s.drive.files, 2, "folder d plus the trashed file")
}

func (s *DriveClientTestSuite) TestTouch() {
	id := s.drive.addFile("root", "t.txt", []byte("x"))
	s.drive.files[id].mod = time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	before := s.drive.files[id].mod

	s.Require().NoError(s.client.Touch(s.ctx, "t.txt"))
	s.True(s.drive.files[id].mod.After(before))
	s.Equal("x", s.read("t.txt"), "touch leaves content alone")

	s.Require().ErrorIs(s.client.Touch(s.ctx, "missing.txt"), ErrNotFound)
}

func (s *DriveClientTestSuite) TestNativeItems() {
	s.drive.addMime("doc", "application/vnd.google-apps.document")

	_, err := s.client.Download(s.ctx, "doc")
	s.Require().ErrorIs(err, ErrNotDownloadable)

	item, err := s.client.GetItem(s.ctx, "doc")
	s.Require().NoError(err)
	s.True(item.IsNative())
	s.Zero(item.Size)

	s.client.exportMime = "application/pdf"
	s.Equal("exported:doc:application/pdf", s.read("doc"))
}

func (s *DriveClientTestSuite) TestResumableUpload() {
	data := bytes.Repeat([]byte("0123456789"), 100)

	session, err := s.client.CreateUploadSession(s.ctx, "big/file.bin", int64(len(data)))
	s.Require().NoError(err)

	item, err := session.UploadChunk(s.ctx, data[:400], 0)
	s.Require().NoError(err)
	s.Nil(item, "intermediate chunk returns no item")

	item, err = session.UploadChunk(s.ctx, data[400:], 400)
	s.Require().NoError(err)
	s.Require().NotNil(item)
	s.Equal(int64(len(data)), item.Size)
	s.Equal(string(data), s.read("big/file.bin"))

	// replacing an existing file goes through PATCH on the same ID
	session, err = s.client.CreateUploadSession(s.ctx, "big/file.bin", 4)
	s.Require().NoError(err)
	item, err = session.UploadChunk(s.ctx, []byte("abcd"), 0)
	s.Require().NoError(err)
	s.Require().NotNil(item)
	s.Equal("abcd", s.read("big/file.bin"))
	s.Equal(1, s.drive.liveFiles())
}

func (s *DriveClientTestSuite) TestResumableUploadRecoversFromServerError() {
	data := bytes.Repeat([]byte("x"), 300)

	session, err := s.client.CreateUploadSession(s.ctx, "r.bin", int64(len(data)))
	s.Require().NoError(err)

	// the chunk is stored, then the server answers 500: the client must ask where the session
	// stands rather than resending the bytes at a stale offset.
	s.drive.failChunkAfterApply = true
	item, err := session.UploadChunk(s.ctx, data[:100], 0)
	s.Require().NoError(err)
	s.Nil(item)

	item, err = session.UploadChunk(s.ctx, data[100:], 100)
	s.Require().NoError(err)
	s.Require().NotNil(item)
	s.Equal(string(data), s.read("r.bin"))

	s.Equal([]string{
		"PUT /upload/session/session1 bytes 0-99/300",
		"PUT /upload/session/session1 bytes */300",
		"PUT /upload/session/session1 bytes 100-299/300",
	}, s.sessionRequests(), "a failed chunk is followed by a status query, never a blind resend")
}

// sessionRequests returns the resumable session requests the fake received, with their Content-Range.
func (s *DriveClientTestSuite) sessionRequests() []string {
	var out []string
	for _, r := range s.drive.requestLog() {
		if strings.Contains(r, "/upload/session/") {
			out = append(out, r)
		}
	}
	return out
}

func isChunkPut(r *http.Request) bool {
	return r.Method == http.MethodPut && !strings.HasPrefix(r.Header.Get("Content-Range"), "bytes */")
}

func (s *DriveClientTestSuite) TestResumableUploadRecovery() {
	tests := []struct {
		name        string
		status      int
		reason      string
		times       int
		expectedErr bool
	}{
		{name: "429 resumes from status", status: http.StatusTooManyRequests, times: 1},
		{name: "rate-limit 403 resumes from status", status: http.StatusForbidden, reason: "userRateLimitExceeded", times: 1},
		{name: "503 twice resumes within budget", status: http.StatusServiceUnavailable, times: 2},
		{name: "recovery budget exhausted", status: http.StatusInternalServerError, times: 10, expectedErr: true},
		{name: "plain 403 is final", status: http.StatusForbidden, reason: "forbidden", times: 1, expectedErr: true},
	}

	for i, tt := range tests {
		s.Run(tt.name, func() {
			p := fmt.Sprintf("rec%d.bin", i)
			session, err := s.client.CreateUploadSession(s.ctx, p, 6)
			s.Require().NoError(err)

			s.drive.faults = nil
			s.drive.fault(&fakeFault{match: isChunkPut, status: tt.status, reason: tt.reason, times: tt.times})
			item, err := session.UploadChunk(s.ctx, []byte("abcdef"), 0)
			s.drive.faults = nil

			if tt.expectedErr {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)
			s.Require().NotNil(item)
			s.Equal("abcdef", s.read(p))
		})
	}
}

func (s *DriveClientTestSuite) TestResumableUploadCompletesViaStatus() {
	data := []byte("complete")
	session, err := s.client.CreateUploadSession(s.ctx, "c.bin", int64(len(data)))
	s.Require().NoError(err)

	s.drive.failChunkAfterApply = true
	item, err := session.UploadChunk(s.ctx, data, 0)
	s.Require().NoError(err)
	s.Require().NotNil(item, "status query reports the finished upload")
	s.Equal("complete", s.read("c.bin"))
	s.Equal([]string{
		"PUT /upload/session/session1 bytes 0-7/8",
		"PUT /upload/session/session1 bytes */8",
	}, s.sessionRequests())
}

func (s *DriveClientTestSuite) TestResumableUploadErrors() {
	session, err := s.client.CreateUploadSession(s.ctx, "e.bin", 10)
	s.Require().NoError(err)

	// a 4xx is final and not retried through the status path
	s.drive.fault(&fakeFault{match: isPut, status: http.StatusBadRequest, reason: "badRequest", times: 1})
	_, err = session.UploadChunk(s.ctx, []byte("12345"), 0)
	s.Require().Error(err)

	// offsets that disagree with the session are rejected
	_, err = session.UploadChunk(s.ctx, []byte("12345"), 5)
	s.Require().Error(err)

	s.Require().NoError(session.Cancel(s.ctx))
	_, err = session.UploadChunk(s.ctx, []byte("12345"), 0)
	s.Require().ErrorIs(err, ErrNotFound)
}

func isPut(r *http.Request) bool { return r.Method == http.MethodPut }

func (s *DriveClientTestSuite) TestRetries() {
	s.upload("r/f.txt", "hello")

	tests := []struct {
		name        string
		match       func(*http.Request) bool
		status      int
		reason      string
		header      map[string]string
		times       int
		expectedErr bool
	}{
		{name: "429 retried", match: isGet, status: 429, times: 2},
		{name: "403 rate limit retried", match: isGet, status: 403, reason: "userRateLimitExceeded", times: 2},
		{name: "403 other not retried", match: isGet, status: 403, reason: "forbidden", times: 1, expectedErr: true},
		{name: "500 retried for GET", match: isGet, status: 500, times: 2},
		{name: "retries exhausted", match: isGet, status: 500, times: 10, expectedErr: true},
		{name: "retry-after honored", match: isGet, status: 429, header: map[string]string{"Retry-After": "1"}, times: 1},
		{name: "retry-after too long", match: isGet, status: 429, header: map[string]string{"Retry-After": "99999"}, times: 1, expectedErr: true},
		{name: "408 retried for GET", match: isGet, status: 408, times: 1},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var slept []time.Duration
			s.client.sleep = func(_ context.Context, d time.Duration) error {
				slept = append(slept, d)
				return nil
			}
			s.drive.faults = nil
			s.drive.fault(&fakeFault{match: tt.match, status: tt.status, reason: tt.reason, header: tt.header, times: tt.times})
			if tt.header["Retry-After"] == "1" {
				defer func() { s.Equal([]time.Duration{time.Second}, slept, "Retry-After replaces the backoff") }()
			}

			_, err := s.client.GetItem(s.ctx, "r/f.txt")
			if tt.expectedErr {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)
		})
	}
}

func isGet(r *http.Request) bool { return r.Method == http.MethodGet }

func (s *DriveClientTestSuite) TestNonIdempotentRequestsRetryOnlyThrottling() {
	s.upload("r/f.txt", "x")

	// A POST that Drive applied and then answered 500, 429, or 503 must not be sent again.
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		s.drive.faults = nil
		s.drive.fault(&fakeFault{match: isPost, status: status, times: 1})
		before := s.countRequests("POST ")

		_, err := s.client.Upload(s.ctx, "r/new.txt", strings.NewReader("y"))
		s.Require().Error(err)
		s.Equal(1, s.countRequests("POST ")-before, "status %d", status)
	}
}

func isPost(r *http.Request) bool { return r.Method == http.MethodPost }

func (s *DriveClientTestSuite) TestStaleFolderCacheRecovers() {
	s.upload("cache/f.txt", "x")

	// delete the folder behind the client's back, then recreate it under a new ID
	s.drive.mu.Lock()
	for id, f := range s.drive.files {
		if f.name == "cache" {
			s.drive.deleteTree(id)
		}
	}
	s.drive.mu.Unlock()
	s.drive.addFolder("cache")

	_, err := s.client.GetItem(s.ctx, "cache/f.txt")
	s.Require().ErrorIs(err, ErrNotFound)

	s.upload("cache/g.txt", "y")
	s.Equal("y", s.read("cache/g.txt"))
}

func (s *DriveClientTestSuite) TestFolderCacheExpires() {
	s.upload("c/f.txt", "x")
	_, ok := s.client.cachedFolder("c")
	s.True(ok)

	base := time.Now()
	s.client.now = func() time.Time { return base.Add(2 * folderCacheTTL) }
	_, ok = s.client.cachedFolder("c")
	s.False(ok)
}

func (s *DriveClientTestSuite) TestContextCancelled() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()

	_, err := s.client.GetItem(ctx, "x/y.txt")
	s.Require().Error(err)
}

func (s *DriveClientTestSuite) TestAuthorizationHeaderSent() {
	s.upload("a.txt", "x")
	s.NotEmpty(s.drive.requestLog())
}

func (s *DriveClientTestSuite) TestParseReceived() {
	s.EqualValues(0, parseReceived(""))
	s.EqualValues(0, parseReceived("garbage"))
	s.EqualValues(0, parseReceived("bytes=0-x"))
	s.EqualValues(10, parseReceived("bytes=0-9"))
}

func (s *DriveClientTestSuite) TestSplitPath() {
	tests := []struct {
		in     string
		parent string
		base   string
	}{
		{in: "", base: ""},
		{in: "/", base: ""},
		{in: "a", base: "a"},
		{in: "a/b/c.txt", parent: "a/b", base: "c.txt"},
		{in: "/a//b/../c", parent: "a", base: "c"},
	}
	for _, tt := range tests {
		parent, base := splitPath(tt.in)
		s.Equal(tt.parent, parent, tt.in)
		s.Equal(tt.base, base, tt.in)
	}
}

func (s *DriveClientTestSuite) TestSharedDriveQueries() {
	c := newDriveClient(s.client.httpClient, &s.fs.options, "shared1")
	s.Equal("shared1", c.rootID)

	q := c.listQuery("x")
	s.Equal("drive", q.Get("corpora"))
	s.Equal("shared1", q.Get("driveId"))
	s.Equal("true", q.Get("includeItemsFromAllDrives"))

	my := s.client.listQuery("x")
	s.Empty(my.Get("corpora"))
	s.Equal("root", s.client.rootID)
}

func (s *DriveClientTestSuite) TestErrorStrings() {
	s.Contains((&apiError{StatusCode: 500}).Error(), "status 500")
	s.Contains((&apiError{StatusCode: 403, Reason: "r", Message: "m"}).Error(), "(r): m")
	s.Require().ErrorIs(&apiError{StatusCode: 404}, ErrNotFound)
	s.NotErrorIs(&apiError{StatusCode: 500}, ErrNotFound)
}

func (s *DriveClientTestSuite) TestCreateUploadSessionWithoutLocationHeader() {
	s.drive.fault(&fakeFault{
		match:  func(r *http.Request) bool { return r.URL.Query().Get("uploadType") == "resumable" },
		status: 200, times: 1,
	})
	_, err := s.client.CreateUploadSession(s.ctx, "n.bin", 5)
	s.Require().Error(err)
}

func isFileDelete(r *http.Request) bool {
	return r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/drive/v3/files/")
}

func (s *DriveClientTestSuite) countRequests(prefix string) int {
	n := 0
	for _, r := range s.drive.requestLog() {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func (s *DriveClientTestSuite) TestParallelWritesShareNewFolders() {
	const writers = 16

	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Go(func() {
			_, errs[i] = s.client.Upload(s.ctx, fmt.Sprintf("fresh/nested/f%d.txt", i), strings.NewReader("x"))
		})
	}
	wg.Wait()

	for _, err := range errs {
		s.Require().NoError(err)
	}
	s.Equal(1, s.drive.countNamed("fresh"), "one folder, not one per writer")
	s.Equal(1, s.drive.countNamed("nested"))

	items, err := s.client.ListChildren(s.ctx, "fresh/nested")
	s.Require().NoError(err)
	s.Len(items, writers)
}

func (s *DriveClientTestSuite) TestMissingFileKeepsFolderCache() {
	s.upload("keep/f.txt", "x")
	before := len(s.drive.requestLog())

	_, err := s.client.GetItem(s.ctx, "keep/missing.txt")
	s.Require().ErrorIs(err, ErrNotFound)

	_, ok := s.client.cachedFolder("keep")
	s.True(ok, "a plain miss must not flush the folder cache")
	s.Len(s.drive.requestLog()[before:], 2, "one lookup plus one check that the cached folder still exists")

	// a miss in a freshly resolved folder needs no check at all
	s.client.invalidate()
	before = len(s.drive.requestLog())
	_, err = s.client.GetItem(s.ctx, "keep/missing.txt")
	s.Require().ErrorIs(err, ErrNotFound)
	s.Len(s.drive.requestLog()[before:], 2, "walk to the folder plus the lookup")
}

func (s *DriveClientTestSuite) TestMissingDescendantOfCachedFolderKeepsCache() {
	s.upload("keep/f.txt", "x")
	before := len(s.drive.requestLog())

	_, err := s.client.GetItem(s.ctx, "keep/missing/child.txt")
	s.Require().ErrorIs(err, ErrNotFound)

	_, ok := s.client.cachedFolder("keep")
	s.True(ok, "a missing child of a live cached folder must not flush the cache")
	s.Len(s.drive.requestLog()[before:], 2, "one lookup plus one check that the cached folder still exists")
}

func (s *DriveClientTestSuite) TestFolderSharingNameWithFileIsAmbiguous() {
	s.drive.addFolder("reports")
	s.drive.addFile("root", "reports", []byte("not a folder"))

	_, err := s.client.GetItem(s.ctx, "reports/q1.csv")
	s.Require().ErrorIs(err, ErrAmbiguous)

	_, err = s.client.Upload(s.ctx, "reports/q1.csv", strings.NewReader("x"))
	s.Require().ErrorIs(err, ErrAmbiguous)
	s.Equal(1, s.drive.liveFiles(), "the ambiguous name was not written into")
}

func (s *DriveClientTestSuite) TestStaleCachedFolderDetectedOnResolve() {
	s.upload("a/b/f.txt", "x")

	// delete the cached leaf folder: resolving its child folder from the cache now fails
	s.drive.mu.Lock()
	for id, f := range s.drive.files {
		if f.name == "b" {
			s.drive.deleteTree(id)
		}
	}
	s.drive.mu.Unlock()

	s.upload("a/b/c/g.txt", "y")
	s.Equal("y", s.read("a/b/c/g.txt"))
}

func (s *DriveClientTestSuite) TestTransferIsNotRepeatedAfterSuccess() {
	tests := []struct {
		name     string
		transfer func() error
		mutation string
	}{
		{
			name:     "move",
			transfer: func() error { _, err := s.client.Move(s.ctx, "src/a.txt", "dst", "b.txt"); return err },
			mutation: "PATCH /drive/v3/files/",
		},
		{
			name:     "copy",
			transfer: func() error { return s.client.Copy(s.ctx, "src/a.txt", "dst", "b.txt") },
			mutation: "POST /drive/v3/files/",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.upload("src/a.txt", "A")
			s.upload("dst/b.txt", "B")

			// someone else deletes the replaced file just before the transfer removes it
			s.drive.setHook(func(w http.ResponseWriter, r *http.Request) bool {
				if !isFileDelete(r) {
					return false
				}
				s.drive.mu.Lock()
				s.drive.deleteTree(strings.TrimPrefix(r.URL.Path, "/drive/v3/files/"))
				s.drive.mu.Unlock()
				writeErr(w, http.StatusNotFound, "notFound", "File not found")
				return true
			})
			defer s.drive.setHook(nil)
			before := s.countRequests(tt.mutation)

			s.Require().NoError(tt.transfer())
			s.Equal(1, s.countRequests(tt.mutation)-before, "the mutation runs exactly once")
			s.Equal("A", s.read("dst/b.txt"))
		})
	}
}

func (s *DriveClientTestSuite) TestOverwriteReportsUnremovableReplacedFile() {
	s.upload("src/a.txt", "A")
	s.upload("dst/b.txt", "B")

	s.drive.fault(&fakeFault{match: isFileDelete, status: http.StatusForbidden, reason: "insufficientFilePermissions", times: 1})
	err := s.client.Copy(s.ctx, "src/a.txt", "dst", "b.txt")
	s.Require().Error(err)
	s.Contains(err.Error(), "leaving two files", "My Drive does not fall back to trash")
}

func (s *DriveClientTestSuite) TestSharedDriveDeleteFallsBackToTrash() {
	s.drive.addSharedDrive("shared1")
	c := newDriveClient(s.client.httpClient, &s.fs.options, "shared1")
	c.sleep = s.client.sleep

	_, err := c.Upload(s.ctx, "dir/f.txt", strings.NewReader("x"))
	s.Require().NoError(err)

	s.drive.fault(&fakeFault{match: isFileDelete, status: http.StatusNotFound, reason: "notFound", times: 1})
	s.Require().NoError(c.Delete(s.ctx, "dir/f.txt"))

	_, err = c.GetItem(s.ctx, "dir/f.txt")
	s.Require().ErrorIs(err, ErrNotFound)
	s.Equal(1, s.drive.countNamed("dir"))
	s.Equal(0, s.drive.liveFiles())

	// the 403 reason named in Drive's documentation is accepted as well
	_, err = c.Upload(s.ctx, "dir/g.txt", strings.NewReader("x"))
	s.Require().NoError(err)
	s.drive.fault(&fakeFault{match: isFileDelete, status: http.StatusForbidden, reason: "insufficientFilePermissions", times: 1})
	s.Require().NoError(c.Delete(s.ctx, "dir/g.txt"))

	// other permission errors are not masked
	_, err = c.Upload(s.ctx, "dir/h.txt", strings.NewReader("x"))
	s.Require().NoError(err)
	s.drive.fault(&fakeFault{match: isFileDelete, status: http.StatusForbidden, reason: "forbidden", times: 1})
	s.Require().Error(c.Delete(s.ctx, "dir/h.txt"))
}

func (s *DriveClientTestSuite) TestSharedDriveRequiresDriveQuery() {
	s.drive.addSharedDrive("shared1")
	c := newDriveClient(s.client.httpClient, &s.fs.options, "shared1")
	_, err := c.ListChildren(s.ctx, "")
	s.Require().NoError(err)

	// listing is scoped to the drive: an unknown drive ID is rejected rather than silently searched
	c.driveID = "unknown"
	_, err = c.ListChildren(s.ctx, "")
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *DriveClientTestSuite) TestWritingOverNativeFileIsRefused() {
	s.drive.addMime("doc", "application/vnd.google-apps.document")
	s.upload("src.txt", "x")

	_, err := s.client.Upload(s.ctx, "doc", strings.NewReader("x"))
	s.Require().ErrorIs(err, ErrNotDownloadable)

	_, err = s.client.CreateUploadSession(s.ctx, "doc", 10)
	s.Require().ErrorIs(err, ErrNotDownloadable)

	s.Require().ErrorIs(s.client.Copy(s.ctx, "src.txt", "", "doc"), ErrNotDownloadable)
	_, err = s.client.Move(s.ctx, "src.txt", "", "doc")
	s.Require().ErrorIs(err, ErrNotDownloadable)

	item, err := s.client.GetItem(s.ctx, "doc")
	s.Require().NoError(err)
	s.True(item.IsNative(), "the document is untouched")
}

func (s *DriveClientTestSuite) TestFileUsedAsFolder() {
	s.upload("f.txt", "x")

	_, err := s.client.Upload(s.ctx, "f.txt/child.txt", strings.NewReader("x"))
	s.Require().ErrorIs(err, ErrNotFolder)
	s.Require().NotErrorIs(err, ErrNotFound)

	_, err = s.client.ListChildren(s.ctx, "f.txt")
	s.Require().ErrorIs(err, ErrNotFolder)

	s.Require().ErrorIs(s.client.Copy(s.ctx, "f.txt", "f.txt", "x"), ErrNotFolder)
	s.Equal(1, s.drive.liveFiles())
}

func (s *DriveClientTestSuite) TestFolderGoneCheckErrors() {
	s.upload("g/f.txt", "x")

	s.drive.fault(&fakeFault{
		match:  func(r *http.Request) bool { return strings.Contains(r.URL.Query().Get("fields"), "trashed") },
		status: http.StatusBadRequest, reason: "badRequest", times: 1,
	})
	_, err := s.client.GetItem(s.ctx, "g/missing.txt")
	var apiErr *apiError
	s.Require().ErrorAs(err, &apiErr)
	s.Equal(http.StatusBadRequest, apiErr.StatusCode)
}

func (s *DriveClientTestSuite) TestCreateFolderFailure() {
	s.drive.fault(&fakeFault{
		match:  func(r *http.Request) bool { return r.Method == http.MethodPost && r.URL.Path == "/drive/v3/files" },
		status: http.StatusForbidden, reason: "forbidden", times: 1,
	})
	_, err := s.client.Upload(s.ctx, "newdir/f.txt", strings.NewReader("x"))
	s.Require().Error(err)
	s.Contains(err.Error(), `creating folder "newdir"`)
}

func (s *DriveClientTestSuite) TestRequestLoggerOutput() {
	var recs []RequestLog
	s.client.logger = func(_ context.Context, rec RequestLog) { recs = append(recs, rec) }

	data := bytes.Repeat([]byte("z"), 10)
	session, err := s.client.CreateUploadSession(s.ctx, "logged.bin", int64(len(data)))
	s.Require().NoError(err)
	_, err = session.UploadChunk(s.ctx, data[:5], 0)
	s.Require().NoError(err)
	_, err = session.UploadChunk(s.ctx, data[5:], 5)
	s.Require().NoError(err)

	s.drive.fault(&fakeFault{match: isGet, status: http.StatusTooManyRequests, times: 1})
	_, err = s.client.GetItem(s.ctx, "logged.bin")
	s.Require().NoError(err)

	var retried, chunk *RequestLog
	for i := range recs {
		s.NotContains(recs[i].URL, "secret-session", "upload session IDs are never logged")
		s.NotContains(recs[i].URL, "test-token", "credentials are never logged")
		switch {
		case recs[i].Status == http.StatusPermanentRedirect:
			chunk = &recs[i]
		case recs[i].Attempt == 2:
			retried = &recs[i]
		}
	}
	s.Require().NotNil(chunk)
	s.Equal(http.MethodPut, chunk.Method)
	s.Contains(chunk.URL, "upload_id=REDACTED")
	s.Positive(chunk.RequestBytes)
	s.Require().NotNil(retried)
	s.Equal(http.StatusOK, retried.Status)
	s.NoError(retried.Err)
}

func (s *DriveClientTestSuite) TestRedactURL() {
	s.Equal("https://h/upload?upload_id=REDACTED&x=1", redactURL("https://h/upload?upload_id=abc&x=1"))
	s.Equal("https://h/files?q=a", redactURL("https://h/files?q=a"))
	s.Equal("<unparseable url>", redactURL("://bad"))
}
