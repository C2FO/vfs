package dropbox

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/options"
)

// testBasePath mirrors the pre-existing folder the credentialed integration test runs in.
const testBasePath = "/vfs-test/"

// conformanceModes are the upload strategies the shared suites are run against.
var conformanceModes = []struct {
	name string
	opts []options.NewFileSystemOption[FileSystem]
	// chunked is true when every write must go through upload_session/*.
	chunked bool
}{
	{name: "SimpleUpload"},
	{
		name: "ChunkedUpload",
		opts: []options.NewFileSystemOption[FileSystem]{
			// Small enough that the suites' tiny fixtures still go through upload_session/*,
			// and not a divisor of the fixtures so the final chunk is short.
			WithMaxSimpleUploadSize(8),
			WithChunkSize(5),
		},
		chunked: true,
	},
}

// TestFakeConformance runs the shared conformance suite against an in-process fake Dropbox API.
func TestFakeConformance(t *testing.T) {
	for _, mode := range conformanceModes {
		t.Run(mode.name, func(t *testing.T) {
			fake, client := newFakeClient(t)
			fs := NewFileSystem(append([]options.NewFileSystemOption[FileSystem]{WithClient(client)}, mode.opts...)...)
			fake.seedFolder(testBasePath)
			loc, err := fs.NewLocation("", testBasePath)
			if err != nil {
				t.Fatalf("failed to create test location: %v", err)
			}

			// Matches the credentialed integration test.
			testsuite.RunConformanceTests(t, loc, testsuite.ConformanceOptions{SkipTouchTimestampTest: true})

			assertUploadRoutes(t, fake, mode.chunked)
		})
	}
}

// TestFakeIOConformance runs the shared IO suite against an in-process fake Dropbox API.
func TestFakeIOConformance(t *testing.T) {
	for _, mode := range conformanceModes {
		t.Run(mode.name, func(t *testing.T) {
			fake, client := newFakeClient(t)
			fs := NewFileSystem(append([]options.NewFileSystemOption[FileSystem]{WithClient(client)}, mode.opts...)...)
			fake.seedFolder(testBasePath)
			loc, err := fs.NewLocation("", testBasePath)
			if err != nil {
				t.Fatalf("failed to create test location: %v", err)
			}

			testsuite.RunIOTests(t, loc)

			assertUploadRoutes(t, fake, mode.chunked)
		})
	}
}

func assertUploadRoutes(t *testing.T, fake *fakeDropbox, chunked bool) {
	t.Helper()
	if chunked {
		for _, route := range []string{"upload_session/start", "upload_session/append_v2", "upload_session/finish"} {
			if fake.calls(route) == 0 {
				t.Errorf("expected chunked path to exercise %s", route)
			}
		}
		return
	}
	if fake.calls("upload") == 0 {
		t.Error("expected simple path to exercise upload")
	}
	if fake.calls("upload_session/start") != 0 {
		t.Error("simple upload mode must not start upload sessions")
	}
}

// FakeServerTestSuite verifies the fake rejects what the real API would reject, so that
// drift between the fake and the backend shows up as a failure rather than a silent pass.
type FakeServerTestSuite struct {
	suite.Suite
	fake   *fakeDropbox
	srv    *httptest.Server
	client files.Client
}

func TestFakeServerTestSuite(t *testing.T) {
	suite.Run(t, new(FakeServerTestSuite))
}

func (s *FakeServerTestSuite) SetupTest() {
	s.fake, s.srv, s.client = newFakeServer(s.T())
}

func (s *FakeServerTestSuite) post(path, token, argHeader, contentType string, body []byte) int {
	req, err := http.NewRequest(http.MethodPost, s.srv.URL+path, bytes.NewReader(body))
	s.Require().NoError(err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if argHeader != "" {
		req.Header.Set("Dropbox-API-Arg", argHeader)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := s.srv.Client().Do(req)
	s.Require().NoError(err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func (s *FakeServerTestSuite) TestRequestValidation() {
	tests := []struct {
		name        string
		path        string
		token       string
		argHeader   string
		contentType string
		body        []byte
		expected    int
	}{
		{
			name:        "valid rpc request",
			path:        "/api/2/files/get_metadata",
			token:       fakeToken,
			contentType: "application/json",
			body:        []byte(`{"path":"/nope"}`),
			expected:    http.StatusConflict,
		},
		{
			name:        "missing authorization",
			path:        "/api/2/files/get_metadata",
			contentType: "application/json",
			body:        []byte(`{"path":"/nope"}`),
			expected:    http.StatusUnauthorized,
		},
		{
			name:        "wrong token",
			path:        "/api/2/files/get_metadata",
			token:       "other",
			contentType: "application/json",
			body:        []byte(`{"path":"/nope"}`),
			expected:    http.StatusUnauthorized,
		},
		{
			name:        "unknown route",
			path:        "/api/2/files/no_such_route",
			token:       fakeToken,
			contentType: "application/json",
			body:        []byte(`{}`),
			expected:    http.StatusNotFound,
		},
		{
			name:        "rpc route sent to content host",
			path:        "/content/2/files/get_metadata",
			token:       fakeToken,
			contentType: "application/json",
			body:        []byte(`{"path":"/nope"}`),
			expected:    http.StatusBadRequest,
		},
		{
			name:      "content route sent to api host",
			path:      "/api/2/files/upload",
			token:     fakeToken,
			argHeader: `{"path":"/x"}`,
			expected:  http.StatusBadRequest,
		},
		{
			name:        "rpc route with arg in header",
			path:        "/api/2/files/get_metadata",
			token:       fakeToken,
			argHeader:   `{"path":"/nope"}`,
			contentType: "application/json",
			expected:    http.StatusBadRequest,
		},
		{
			name:        "rpc route without json content type",
			path:        "/api/2/files/get_metadata",
			token:       fakeToken,
			contentType: "text/plain",
			body:        []byte(`{"path":"/nope"}`),
			expected:    http.StatusBadRequest,
		},
		{
			name:     "upload route without arg header",
			path:     "/content/2/files/upload",
			token:    fakeToken,
			expected: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expected, s.post(tt.path, tt.token, tt.argHeader, tt.contentType, tt.body))
		})
	}
}

func (s *FakeServerTestSuite) TestRootMetadataRefused() {
	_, err := s.client.GetMetadata(files.NewGetMetadataArg(""))
	s.Require().Error(err)
	s.Contains(err.Error(), "malformed_path")
}

func (s *FakeServerTestSuite) TestAddModeConflicts() {
	arg := files.NewUploadArg("/a.txt")
	arg.Mode = &files.WriteMode{}
	arg.Mode.Tag = fakeWriteModeAdd

	_, err := s.client.Upload(arg, bytes.NewReader([]byte("one")))
	s.Require().NoError(err)

	_, err = s.client.Upload(arg, bytes.NewReader([]byte("two")))
	s.Require().Error(err)

	var apiErr files.UploadAPIError
	s.Require().ErrorAs(err, &apiErr)
	s.Require().NotNil(apiErr.EndpointError)
	s.Require().NotNil(apiErr.EndpointError.Path)
	s.Require().NotNil(apiErr.EndpointError.Path.Reason)
	s.Equal(files.WriteErrorConflict, apiErr.EndpointError.Path.Reason.Tag)
}

func (s *FakeServerTestSuite) TestListPagination() {
	fs := NewFileSystem(WithClient(s.client))
	loc, err := fs.NewLocation("", "/paged/")
	s.Require().NoError(err)

	expected := make([]string, 0, 8)
	for i := range 8 {
		name := fmt.Sprintf("file%d.txt", i)
		f, err := loc.NewFile(name)
		s.Require().NoError(err)
		_, err = f.Write([]byte("x"))
		s.Require().NoError(err)
		s.Require().NoError(f.Close())
		expected = append(expected, name)
	}
	// A subfolder must be skipped by List.
	sub, err := loc.NewFile("sub/inner.txt")
	s.Require().NoError(err)
	_, err = sub.Write([]byte("x"))
	s.Require().NoError(err)
	s.Require().NoError(sub.Close())

	got, err := loc.List()
	s.Require().NoError(err)
	s.ElementsMatch(expected, got, "every file exactly once across pages")
	s.Positive(s.fake.calls("list_folder/continue"))
}

func (s *FakeServerTestSuite) TestRelocationIntoItselfRefused() {
	up := files.NewUploadArg("/dir/a.txt")
	_, err := s.client.Upload(up, bytes.NewReader([]byte("a")))
	s.Require().NoError(err)

	_, err = s.client.MoveV2(files.NewRelocationArg("/dir", "/dir/sub"))
	s.Require().Error(err)
	s.Contains(err.Error(), "cant_move_folder_into_itself")
}

func (s *FakeServerTestSuite) TestNonASCIIArgHeaderRefused() {
	_, err := s.client.Upload(files.NewUploadArg("/caf\u00e9.txt"), bytes.NewReader([]byte("a")))
	s.Require().Error(err)
	s.Contains(err.Error(), "ASCII")
}

func (s *FakeServerTestSuite) TestSessionOffsetValidated() {
	start, err := s.client.UploadSessionStart(files.NewUploadSessionStartArg(), bytes.NewReader([]byte("abcd")))
	s.Require().NoError(err)

	err = s.client.UploadSessionAppendV2(
		files.NewUploadSessionAppendArg(files.NewUploadSessionCursor(start.SessionId, 99)),
		bytes.NewReader([]byte("ef")),
	)
	s.Require().Error(err)
	s.Contains(err.Error(), "incorrect_offset")
}

func (s *FakeServerTestSuite) TestChunkedRoundTrip() {
	fs := NewFileSystem(WithClient(s.client), WithMaxSimpleUploadSize(1024), WithChunkSize(256))

	payload := make([]byte, 10*1024+7)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	f, err := fs.NewFile("", "/big/payload.bin")
	s.Require().NoError(err)
	_, err = f.Write(payload)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	s.Positive(s.fake.calls("upload_session/append_v2"), "multiple chunks should be appended")
	s.Equal(0, s.fake.calls("upload"))

	got, err := fs.NewFile("", "/big/payload.bin")
	s.Require().NoError(err)
	read, err := io.ReadAll(got)
	s.Require().NoError(err)
	s.Require().NoError(got.Close())
	s.Equal(payload, read)
}
