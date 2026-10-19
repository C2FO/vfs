//go:build vfsintegration

package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// wiremockImage is pinned by digest; bump it deliberately.
const (
	wiremockImage = "wiremock/wiremock:3.13.2@sha256:0d4ecb3e4dc8213fd7a4d37d6a78f6e6b553a6d2e15bd51b0999781282ac61b3"
	wiremockPort  = "8080/tcp"

	filesPath       = "/drive/v3/files"
	uploadFilesPath = "/upload/drive/v3/files"
)

// WireMockTestSuite drives the real FileSystem against Drive responses stubbed in a WireMock
// container. Each test registers the Drive exchanges for one scenario and then asserts on the
// requests WireMock received, so the HTTP contract (paths, query parameters, headers, bodies,
// pagination, retries, and resumable uploads) is exercised over a real network hop.
//
// WireMock is stateless apart from scenarios, so these tests cover wire-level protocol behavior;
// the in-process fake Drive in conformance_test.go covers the stateful VFS semantics.
type WireMockTestSuite struct {
	suite.Suite
	root string
	fs   *FileSystem
}

func TestWireMockTestSuite(t *testing.T) {
	suite.Run(t, new(WireMockTestSuite))
}

func (s *WireMockTestSuite) SetupSuite() {
	testcontainers.SkipIfProviderIsNotHealthy(s.T())
	ctx := context.Background()

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        wiremockImage,
			ExposedPorts: []string{wiremockPort},
			Cmd:          []string{"--disable-banner"},
			WaitingFor:   wait.ForHTTP("/__admin/health").WithPort(wiremockPort),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(s.T(), ctr)
	s.Require().NoError(err)

	host, err := ctr.Host(ctx)
	s.Require().NoError(err)
	port, err := ctr.MappedPort(ctx, wiremockPort)
	s.Require().NoError(err)
	s.root = "http://" + net.JoinHostPort(host, port.Port())
}

func (s *WireMockTestSuite) SetupTest() {
	s.admin(http.MethodPost, "/reset", nil, nil)
	s.fs = NewFileSystem(
		WithBaseURL(s.root),
		WithAccessToken("test-token"),
		WithTempDir(s.T().TempDir()),
		WithRetryCount(2),
		WithChunkSize(uploadChunkMultiple),
	)
}

// TearDownTest logs the requests no stub matched, which is the usual cause of a failing scenario.
func (s *WireMockTestSuite) TearDownTest() {
	if !s.T().Failed() {
		return
	}

	var out struct {
		Requests []loggedRequest `json:"requests"`
	}
	s.admin(http.MethodGet, "/requests/unmatched", nil, &out)
	for _, r := range out.Requests {
		s.T().Logf("unmatched request: %s %s", r.Method, r.URL)
	}
}

func (s *WireMockTestSuite) admin(method, path string, in, out any) {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		s.Require().NoError(err)
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, s.root+"/__admin"+path, body)
	s.Require().NoError(err)
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()
	s.Require().Less(resp.StatusCode, 300, "wiremock admin %s %s", method, path)

	if out != nil {
		s.Require().NoError(json.NewDecoder(resp.Body).Decode(out))
	}
}

type stubOpt func(req, resp, mapping map[string]any)

func withHeader(name, value string) stubOpt {
	return func(_, resp, _ map[string]any) { resp["headers"].(map[string]any)[name] = value }
}

func withQuery(name, value string) stubOpt {
	return func(req, _, _ map[string]any) {
		q, _ := req["queryParameters"].(map[string]any)
		if q == nil {
			q = map[string]any{}
			req["queryParameters"] = q
		}
		q[name] = map[string]any{"equalTo": value}
	}
}

func inScenario(name, requiredState, newState string) stubOpt {
	return func(_, _, mapping map[string]any) {
		mapping["scenarioName"] = name
		mapping["requiredScenarioState"] = requiredState
		if newState != "" {
			mapping["newScenarioState"] = newState
		}
	}
}

func (s *WireMockTestSuite) stub(method, path string, status int, body string, opts ...stubOpt) {
	req := map[string]any{"method": method, "urlPath": path}
	resp := map[string]any{"status": status, "headers": map[string]any{"Content-Type": "application/json"}}
	if body != "" {
		resp["body"] = body
	}
	mapping := map[string]any{"request": req, "response": resp}
	for _, opt := range opts {
		opt(req, resp, mapping)
	}
	s.admin(http.MethodPost, "/mappings", mapping, nil)
}

type loggedRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers"`
}

func (s *WireMockTestSuite) received(method, path string) []loggedRequest {
	var out struct {
		Requests []loggedRequest `json:"requests"`
	}
	s.admin(http.MethodPost, "/requests/find", map[string]any{"method": method, "urlPath": path}, &out)
	return out.Requests
}

func (s *WireMockTestSuite) file(p string) *File {
	f, err := s.fs.NewFile("", p)
	s.Require().NoError(err)
	return f.(*File)
}

// stubFolder registers the lookup of a folder named name under parent.
func (s *WireMockTestSuite) stubFolder(parent, name, id string) {
	s.stubChild(parent, name, fmt.Sprintf(`{"files":[{"id":%q,"name":%q,"mimeType":"application/vnd.google-apps.folder"}]}`, id, name))
}

// stubChild registers a name lookup under parent with the given list response body.
func (s *WireMockTestSuite) stubChild(parent, name, body string) {
	s.stub(http.MethodGet, filesPath, 200, body,
		withQuery("q", fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", parent, name)))
}

func (s *WireMockTestSuite) TestMetadataAndAuthorization() {
	s.stubFolder("root", "docs", "dirID")
	s.stubChild("dirID", "report.txt", `{"files":[{"id":"r1","name":"report.txt","mimeType":"text/plain",
		"size":"11","modifiedTime":"2026-03-04T05:06:07.000Z"}]}`)
	f := s.file("/docs/report.txt")

	exists, err := f.Exists()
	s.Require().NoError(err)
	s.True(exists)

	size, err := f.Size()
	s.Require().NoError(err)
	s.EqualValues(11, size)

	modified, err := f.LastModified()
	s.Require().NoError(err)
	s.Equal("2026-03-04T05:06:07Z", modified.UTC().Format("2006-01-02T15:04:05Z"))

	reqs := s.received(http.MethodGet, filesPath)
	s.Require().NotEmpty(reqs)
	for _, r := range reqs {
		s.Equal("Bearer test-token", r.Headers["Authorization"])
		s.Contains(r.URL, "supportsAllDrives=true")
	}
}

func (s *WireMockTestSuite) TestQuotesInNamesAreEscaped() {
	s.stubChild("root", `it\'s.txt`, `{"files":[]}`)

	exists, err := s.file("/it's.txt").Exists()
	s.Require().NoError(err)
	s.False(exists)
}

func (s *WireMockTestSuite) TestSharedDriveListingScopesToDrive() {
	s.stub(http.MethodGet, filesPath, 200, `{"files":[]}`,
		withQuery("corpora", "drive"), withQuery("driveId", "0ADrive"), withQuery("includeItemsFromAllDrives", "true"),
		withQuery("q", "'0ADrive' in parents and trashed = false"))

	loc, err := s.fs.NewLocation("0ADrive", "/")
	s.Require().NoError(err)
	names, err := loc.List()
	s.Require().NoError(err)
	s.Empty(names)
}

func (s *WireMockTestSuite) TestListFollowsPagination() {
	listQ := "'dirID' in parents and trashed = false"
	s.stubFolder("root", "docs", "dirID")
	s.stub(http.MethodGet, filesPath, 200,
		`{"files":[{"id":"1","name":"a.txt","mimeType":"text/plain"},`+
			`{"id":"2","name":"sub","mimeType":"application/vnd.google-apps.folder"}],"nextPageToken":"tok2"}`,
		withQuery("q", listQ), inScenario("list", "Started", "page2"))
	s.stub(http.MethodGet, filesPath, 200,
		`{"files":[{"id":"3","name":"b.txt","mimeType":"text/plain"}]}`,
		withQuery("q", listQ), withQuery("pageToken", "tok2"), inScenario("list", "page2", ""))

	loc, err := s.fs.NewLocation("", "/docs/")
	s.Require().NoError(err)
	names, err := loc.List()
	s.Require().NoError(err)
	s.Equal([]string{"a.txt", "b.txt"}, names, "folders excluded, both pages read")
}

func (s *WireMockTestSuite) TestDuplicateNamesFailWithAmbiguity() {
	s.stubChild("root", "dup.txt", `{"files":[{"id":"1","name":"dup.txt","mimeType":"text/plain"},`+
		`{"id":"2","name":"dup.txt","mimeType":"text/plain"}]}`)

	_, err := s.file("/dup.txt").Exists()
	s.Require().ErrorIs(err, ErrAmbiguous)
	s.Require().ErrorIs(s.file("/dup.txt").Delete(), ErrAmbiguous)
	s.Empty(s.received(http.MethodDelete, filesPath+"/1"), "nothing was deleted")
}

func (s *WireMockTestSuite) TestDeleteRefusesFolders() {
	s.stubFolder("root", "docs", "dirID")

	err := s.file("/docs").Delete()
	s.Require().ErrorIs(err, ErrIsFolder)
	s.Empty(s.received(http.MethodDelete, filesPath+"/dirID"))
}

func (s *WireMockTestSuite) TestReadDownloadsMedia() {
	s.stubChild("root", "a.txt", `{"files":[{"id":"fid","name":"a.txt","mimeType":"text/plain","size":"5"}]}`)
	s.stub(http.MethodGet, filesPath+"/fid", 200, "hello", withQuery("alt", "media"))

	data, err := io.ReadAll(s.file("/a.txt"))
	s.Require().NoError(err)
	s.Equal("hello", string(data))
}

func (s *WireMockTestSuite) TestSmallWriteIsOneMultipartRequest() {
	s.stubChild("root", "new.txt", `{"files":[]}`)
	s.stub(http.MethodPost, uploadFilesPath, 200, `{"id":"n1","name":"new.txt","mimeType":"application/octet-stream","size":"11"}`,
		withQuery("uploadType", "multipart"))

	f := s.file("/new.txt")
	_, err := f.Write([]byte("new content"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	posts := s.received(http.MethodPost, uploadFilesPath)
	s.Require().Len(posts, 1)
	s.True(strings.HasPrefix(posts[0].Headers["Content-Type"], "multipart/related; boundary="))
	s.Contains(posts[0].Body, `"name":"new.txt"`)
	s.Contains(posts[0].Body, `"parents":["root"]`)
	s.Contains(posts[0].Body, "new content")
}

func (s *WireMockTestSuite) TestOverwriteUpdatesExistingFileInPlace() {
	s.stubChild("root", "old.txt", `{"files":[{"id":"o1","name":"old.txt","mimeType":"text/plain"}]}`)
	s.stub(http.MethodPatch, uploadFilesPath+"/o1", 200, `{"id":"o1","name":"old.txt"}`, withQuery("uploadType", "multipart"))

	f := s.file("/old.txt")
	_, err := f.Write([]byte("v2"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	s.Len(s.received(http.MethodPatch, uploadFilesPath+"/o1"), 1)
	s.Empty(s.received(http.MethodPost, uploadFilesPath), "no duplicate was created")
}

func (s *WireMockTestSuite) TestLargeWriteUsesResumableUpload() {
	size := maxSimpleUploadSize + 10
	s.stubChild("root", "big.bin", `{"files":[]}`)
	s.stub(http.MethodPost, uploadFilesPath, 200, "",
		withQuery("uploadType", "resumable"), withHeader("Location", s.root+"/session/1"))
	s.stub(http.MethodPut, "/session/1", 308, "",
		withHeader("Range", fmt.Sprintf("bytes=0-%d", uploadChunkMultiple-1)), inScenario("up", "Started", "chunk2"))

	// every remaining chunk is also incomplete until the final one
	chunks := (size + uploadChunkMultiple - 1) / uploadChunkMultiple
	state := "chunk2"
	for i := 2; i < chunks; i++ {
		next := fmt.Sprintf("chunk%d", i+1)
		s.stub(http.MethodPut, "/session/1", 308, "",
			withHeader("Range", fmt.Sprintf("bytes=0-%d", i*uploadChunkMultiple-1)), inScenario("up", state, next))
		state = next
	}
	s.stub(http.MethodPut, "/session/1", 200, `{"id":"b1","name":"big.bin","size":"`+strconv.Itoa(size)+`"}`, inScenario("up", state, "done"))

	f := s.file("/big.bin")
	_, err := f.Write(bytes.Repeat([]byte("b"), size))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	puts := s.received(http.MethodPut, "/session/1")
	s.Require().Len(puts, chunks)
	s.Equal(fmt.Sprintf("bytes 0-%d/%d", uploadChunkMultiple-1, size), puts[0].Headers["Content-Range"])
	last := puts[len(puts)-1].Headers["Content-Range"]
	s.Equal(fmt.Sprintf("bytes %d-%d/%d", (chunks-1)*uploadChunkMultiple, size-1, size), last)
}

func (s *WireMockTestSuite) TestRateLimitIsRetried() {
	s.stubChild("root", "a.txt", `{"files":[{"id":"1","name":"a.txt","mimeType":"text/plain"}]}`)
	// replace the happy stub with a throttled-then-ok scenario at higher priority
	s.stub(http.MethodGet, filesPath, 403,
		`{"error":{"code":403,"message":"slow down","errors":[{"reason":"userRateLimitExceeded"}]}}`,
		withQuery("q", "'root' in parents and name = 'a.txt' and trashed = false"),
		withHeader("Retry-After", "0"), inScenario("rl", "Started", "ok"))
	s.stub(http.MethodGet, filesPath, 200, `{"files":[{"id":"1","name":"a.txt","mimeType":"text/plain"}]}`,
		withQuery("q", "'root' in parents and name = 'a.txt' and trashed = false"), inScenario("rl", "ok", ""))

	exists, err := s.file("/a.txt").Exists()
	s.Require().NoError(err)
	s.True(exists)
}

func (s *WireMockTestSuite) TestAuthFailureIsNotRetried() {
	s.stub(http.MethodGet, filesPath, 401,
		`{"error":{"code":401,"message":"Invalid Credentials","errors":[{"reason":"authError"}]}}`)

	_, err := s.file("/a.txt").Exists()
	s.Require().Error(err)
	s.Len(s.received(http.MethodGet, filesPath), 1)
}
