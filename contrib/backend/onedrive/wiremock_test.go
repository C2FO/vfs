//go:build vfsintegration

package onedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// wiremockImage is pinned by digest; bump it deliberately.
const (
	wiremockImage = "wiremock/wiremock:3.13.2@sha256:0d4ecb3e4dc8213fd7a4d37d6a78f6e6b553a6d2e15bd51b0999781282ac61b3"
	wiremockPort  = "8080/tcp"

	drivePath = "/v1.0/drives/drive1"
)

// WireMockTestSuite drives the real FileSystem against Microsoft Graph responses stubbed in a
// WireMock container. Each test registers the Graph exchanges for one scenario and then asserts on
// the requests WireMock received, so the HTTP contract (paths, query parameters, headers, bodies,
// pagination, async copy polling, retries, and upload sessions) is exercised end to end.
//
// WireMock is stateless apart from scenarios, so these tests cover protocol behavior rather than
// the full conformance suite; conformance_test.go runs that against a real drive.
type WireMockTestSuite struct {
	suite.Suite
	root string // http://host:port
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
	s.stubJSON(http.MethodGet, "/v1.0/me/drive", 200, `{"id":"drive1"}`)
	s.fs = NewFileSystem(
		WithBaseURL(s.root+"/v1.0"),
		WithAccessToken("test-token"),
		WithTempDir(s.T().TempDir()),
		WithRetryCount(2),
	)
}

// admin calls the WireMock admin API.
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

type stubOpt func(req, resp map[string]any, mapping map[string]any)

func withHeader(name, value string) stubOpt {
	return func(_, resp, _ map[string]any) { resp["headers"].(map[string]any)[name] = value }
}

func withQuery(name, value string) stubOpt {
	return func(req, _, _ map[string]any) {
		req["queryParameters"] = map[string]any{name: map[string]any{"equalTo": value}}
	}
}

func withRequestHeaderMatching(name, regex string) stubOpt {
	return func(req, _, _ map[string]any) {
		req["headers"] = map[string]any{name: map[string]any{"matches": regex}}
	}
}

func withPriority(p int) stubOpt {
	return func(_, _, mapping map[string]any) { mapping["priority"] = p }
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

func (s *WireMockTestSuite) stubJSON(method, path string, status int, body string, opts ...stubOpt) {
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

func (s *WireMockTestSuite) location(p string) *Location {
	l, err := s.fs.NewLocation("", p)
	s.Require().NoError(err)
	return l.(*Location)
}

const reportJSON = `{"id":"r1","name":"report.txt","size":11,"lastModifiedDateTime":"2026-03-04T05:06:07Z","file":{}}`

func (s *WireMockTestSuite) TestMetadataAndDriveResolution() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt", 200, reportJSON)
	f := s.file("/docs/report.txt")

	exists, err := f.Exists()
	s.Require().NoError(err)
	s.True(exists)

	size, err := f.Size()
	s.Require().NoError(err)
	s.Equal(uint64(11), size)

	modified, err := f.LastModified()
	s.Require().NoError(err)
	s.Equal(time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC), *modified)

	s.Len(s.received(http.MethodGet, "/v1.0/me/drive"), 1, "drive ID is resolved once and cached")
	for _, r := range s.received(http.MethodGet, drivePath+"/root:/docs/report.txt") {
		s.Equal("Bearer test-token", r.Headers["Authorization"])
	}
}

func (s *WireMockTestSuite) TestMissingFile() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/missing.txt", 404,
		`{"error":{"code":"itemNotFound","message":"The resource could not be found."}}`)
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/missing.txt:/content", 404,
		`{"error":{"code":"itemNotFound","message":"The resource could not be found."}}`)
	f := s.file("/docs/missing.txt")

	exists, err := f.Exists()
	s.Require().NoError(err)
	s.False(exists)

	_, err = f.Read(make([]byte, 1))
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *WireMockTestSuite) TestReadFollowsDownloadRedirect() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt:/content", 302, "",
		withHeader("Location", s.root+"/blob/report"))
	s.stubJSON(http.MethodGet, "/blob/report", 200, "hello world")
	f := s.file("/docs/report.txt")

	data, err := io.ReadAll(f)
	s.Require().NoError(err)
	s.Equal("hello world", string(data))
	s.Require().NoError(f.Close())
}

func (s *WireMockTestSuite) TestWriteUploads() {
	s.stubJSON(http.MethodPut, drivePath+"/root:/docs/new.txt:/content", 201, reportJSON)
	f := s.file("/docs/new.txt")

	_, err := f.Write([]byte("new content"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	puts := s.received(http.MethodPut, drivePath+"/root:/docs/new.txt:/content")
	s.Require().Len(puts, 1)
	s.Equal("new content", puts[0].Body)
	s.Equal("application/octet-stream", puts[0].Headers["Content-Type"])
}

func (s *WireMockTestSuite) TestSeekThenWritePreservesContent() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt", 200, `{"id":"r1","size":6}`)
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt:/content", 200, "hello!")
	s.stubJSON(http.MethodPut, drivePath+"/root:/docs/report.txt:/content", 200, reportJSON)
	f := s.file("/docs/report.txt")

	_, err := f.Seek(0, io.SeekStart)
	s.Require().NoError(err)
	_, err = f.Write([]byte("J"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	puts := s.received(http.MethodPut, drivePath+"/root:/docs/report.txt:/content")
	s.Require().Len(puts, 1)
	s.Equal("Jello!", puts[0].Body)
}

func (s *WireMockTestSuite) TestListFollowsPagination() {
	children := drivePath + "/root:/docs:/children"
	s.stubJSON(http.MethodGet, children, 200, fmt.Sprintf(
		`{"value":[{"name":"a.txt","file":{}},{"name":"sub","folder":{"childCount":1}}],"@odata.nextLink":%q}`,
		s.root+children+"?$skiptoken=page2",
	), withPriority(5))
	s.stubJSON(http.MethodGet, children, 200, `{"value":[{"name":"b.txt","file":{}}]}`,
		withQuery("$skiptoken", "page2"), withPriority(1))
	loc := s.location("/docs/")

	files, err := loc.List()
	s.Require().NoError(err)
	s.Equal([]string{"a.txt", "b.txt"}, files)
	s.Len(s.received(http.MethodGet, children), 2)
}

func (s *WireMockTestSuite) TestCopyPollsMonitorUntilComplete() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/archive", 200, `{"id":"archive1","folder":{}}`)
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt", 200, reportJSON)
	s.stubJSON(http.MethodPost, drivePath+"/root:/docs/report.txt:/copy", 202, "",
		withQuery("@microsoft.graph.conflictBehavior", "replace"),
		withHeader("Location", s.root+"/monitor/copy1"))
	s.stubJSON(http.MethodGet, "/monitor/copy1", 202, `{"status":"inProgress","percentageComplete":50}`,
		inScenario("copy", "Started", "done"))
	s.stubJSON(http.MethodGet, "/monitor/copy1", 200, `{"status":"completed","percentageComplete":100}`,
		inScenario("copy", "done", ""))

	newFile, err := s.file("/docs/report.txt").CopyToLocation(s.location("/archive/"))
	s.Require().NoError(err)
	s.Equal("/archive/report.txt", newFile.Path())

	copies := s.received(http.MethodPost, drivePath+"/root:/docs/report.txt:/copy")
	s.Require().Len(copies, 1)
	s.JSONEq(`{"parentReference":{"driveId":"drive1","id":"archive1"},"name":"report.txt"}`, copies[0].Body)

	monitor := s.received(http.MethodGet, "/monitor/copy1")
	s.Len(monitor, 2, "copy waits for the monitor to report completion")
	for _, r := range monitor {
		s.Empty(r.Headers["Authorization"], "monitor URLs must not receive the bearer token")
	}
}

func (s *WireMockTestSuite) TestCopyToMissingFolderStreams() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/new/folder", 404, `{"error":{"code":"itemNotFound"}}`)
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt:/content", 200, "payload")
	s.stubJSON(http.MethodPut, drivePath+"/root:/new/folder/report.txt:/content", 201, reportJSON)

	_, err := s.file("/docs/report.txt").CopyToLocation(s.location("/new/folder/"))
	s.Require().NoError(err)

	puts := s.received(http.MethodPut, drivePath+"/root:/new/folder/report.txt:/content")
	s.Require().Len(puts, 1)
	s.Equal("payload", puts[0].Body)
}

func (s *WireMockTestSuite) TestMove() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/archive", 200, `{"id":"archive1","folder":{}}`)
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt", 200, reportJSON)
	s.stubJSON(http.MethodPatch, drivePath+"/root:/docs/report.txt", 200, reportJSON,
		withQuery("@microsoft.graph.conflictBehavior", "replace"))

	newFile, err := s.file("/docs/report.txt").MoveToLocation(s.location("/archive/"))
	s.Require().NoError(err)
	s.Equal("/archive/report.txt", newFile.Path())

	patches := s.received(http.MethodPatch, drivePath+"/root:/docs/report.txt")
	s.Require().Len(patches, 1)
	s.JSONEq(`{"parentReference":{"id":"archive1"},"name":"report.txt"}`, patches[0].Body)
}

func (s *WireMockTestSuite) TestDelete() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt", 200, reportJSON)
	s.stubJSON(http.MethodDelete, drivePath+"/root:/docs/report.txt", 204, "")

	s.Require().NoError(s.file("/docs/report.txt").Delete())
	s.Len(s.received(http.MethodDelete, drivePath+"/root:/docs/report.txt"), 1)
}

func (s *WireMockTestSuite) TestFolderIsNeverDeletedMovedOrCopied() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs", 200, `{"id":"d1","name":"docs","folder":{"childCount":2}}`)
	s.stubJSON(http.MethodGet, drivePath+"/root:/archive", 200, `{"id":"archive1","folder":{}}`)
	s.stubJSON(http.MethodDelete, drivePath+"/root:/docs", 204, "")
	s.stubJSON(http.MethodPatch, drivePath+"/root:/docs", 200, reportJSON)
	s.stubJSON(http.MethodPost, drivePath+"/root:/docs:/copy", 202, "")
	f := s.file("/docs")

	exists, err := f.Exists()
	s.Require().NoError(err)
	s.False(exists, "a folder is not a file")

	s.Require().ErrorIs(f.Delete(), errIsFolder)
	_, err = f.MoveToLocation(s.location("/archive/"))
	s.Require().ErrorIs(err, errIsFolder)
	_, err = f.CopyToLocation(s.location("/archive/"))
	s.Require().ErrorIs(err, errIsFolder)

	s.Empty(s.received(http.MethodDelete, drivePath+"/root:/docs"))
	s.Empty(s.received(http.MethodPatch, drivePath+"/root:/docs"))
	s.Empty(s.received(http.MethodPost, drivePath+"/root:/docs:/copy"))
}

func (s *WireMockTestSuite) TestTouchReuploadsExistingContent() {
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt", 200, reportJSON)
	s.stubJSON(http.MethodGet, drivePath+"/root:/docs/report.txt:/content", 200, "hello world")
	s.stubJSON(http.MethodPut, drivePath+"/root:/docs/report.txt:/content", 200, reportJSON)

	s.Require().NoError(s.file("/docs/report.txt").Touch())

	puts := s.received(http.MethodPut, drivePath+"/root:/docs/report.txt:/content")
	s.Require().Len(puts, 1)
	s.Equal("hello world", puts[0].Body)
}

func (s *WireMockTestSuite) TestThrottledRequestIsRetried() {
	path := drivePath + "/root:/docs/report.txt"
	s.stubJSON(http.MethodGet, path, 429, `{"error":{"code":"activityLimitReached"}}`,
		withHeader("Retry-After", "1"), inScenario("throttle", "Started", "recovered"))
	s.stubJSON(http.MethodGet, path, 200, reportJSON, inScenario("throttle", "recovered", ""))

	exists, err := s.file("/docs/report.txt").Exists()
	s.Require().NoError(err)
	s.True(exists)
	s.Len(s.received(http.MethodGet, path), 2)
}

func (s *WireMockTestSuite) TestLargeFileUsesUploadSession() {
	const size = maxSimpleUploadSize + 1000
	s.fs.options.ChunkSize = uploadChunkMultiple
	uploadPath := "/upload/session1"

	s.stubJSON(http.MethodPost, drivePath+"/root:/big.bin:/createUploadSession", 200,
		fmt.Sprintf(`{"uploadUrl":%q}`, s.root+uploadPath))
	s.stubJSON(http.MethodPut, uploadPath, 202, `{"nextExpectedRanges":[]}`, withPriority(5))
	s.stubJSON(http.MethodPut, uploadPath, 201, `{"id":"big1","name":"big.bin","size":4195304,"file":{}}`,
		withRequestHeaderMatching("Content-Range", fmt.Sprintf(`bytes \d+-%d/%d`, size-1, size)), withPriority(1))

	f := s.file("/big.bin")
	_, err := f.Write(bytes.Repeat([]byte("x"), size))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	sessions := s.received(http.MethodPost, drivePath+"/root:/big.bin:/createUploadSession")
	s.Require().Len(sessions, 1)
	s.JSONEq(`{"item":{"@microsoft.graph.conflictBehavior":"replace"}}`, sessions[0].Body)

	chunks := s.received(http.MethodPut, uploadPath)
	expectedChunks := (size + uploadChunkMultiple - 1) / uploadChunkMultiple
	s.Require().Len(chunks, expectedChunks)
	ranges := make([]string, 0, len(chunks))
	for _, c := range chunks {
		s.Empty(c.Headers["Authorization"], "upload URLs must not receive the bearer token")
		ranges = append(ranges, c.Headers["Content-Range"])
	}
	s.Contains(ranges, fmt.Sprintf("bytes 0-%d/%d", uploadChunkMultiple-1, size))
	s.Contains(strings.Join(ranges, ","), fmt.Sprintf("-%d/%d", size-1, size))
}
