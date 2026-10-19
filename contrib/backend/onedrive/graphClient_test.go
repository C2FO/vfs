package onedrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/suite"
)

type recordedRequest struct {
	Method string
	Path   string // escaped path
	Query  string
	Header http.Header
	Body   string
}

type GraphClientTestSuite struct {
	suite.Suite
	server   *httptest.Server
	handler  http.HandlerFunc
	mu       sync.Mutex
	requests []recordedRequest
	sleeps   []time.Duration
	client   *graphClient
}

func TestGraphClientTestSuite(t *testing.T) {
	suite.Run(t, new(GraphClientTestSuite))
}

func (s *GraphClientTestSuite) SetupTest() {
	s.requests = nil
	s.sleeps = nil
	s.handler = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotImplemented) }
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.EscapedPath(),
			Query:  r.URL.RawQuery,
			Header: r.Header.Clone(),
			Body:   string(body),
		})
		s.mu.Unlock()
		s.handler(w, r)
	}))
	s.client = s.newClient(&Options{BaseURL: s.server.URL, DriveID: "drive1", RetryCount: 2})
}

func (s *GraphClientTestSuite) TearDownTest() {
	s.server.Close()
}

func (s *GraphClientTestSuite) newClient(opts *Options) *graphClient {
	c := newGraphClient(staticTokenCredential("test-token"), opts)
	c.sleep = func(_ context.Context, d time.Duration) error {
		s.sleeps = append(s.sleeps, d)
		return nil
	}
	return c
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

const itemJSON = `{"id":"item1","name":"file.txt","size":42,"lastModifiedDateTime":"2026-01-02T03:04:05Z","file":{}}`

func (s *GraphClientTestSuite) TestItemURL() {
	tests := []struct {
		name     string
		itemPath string
		suffix   string
		expected string
	}{
		{name: "root item", itemPath: "", expected: "/drives/drive1/root"},
		{name: "root with suffix", itemPath: "/", suffix: "/children", expected: "/drives/drive1/root/children"},
		{name: "nested item", itemPath: "a/b.txt", expected: "/drives/drive1/root:/a/b.txt"},
		{name: "nested item with suffix", itemPath: "/a/b/", suffix: "/content", expected: "/drives/drive1/root:/a/b:/content"},
		{
			name:     "special characters escaped",
			itemPath: "dir with space/100% #1?.txt",
			expected: "/drives/drive1/root:/dir%20with%20space/100%25%20%231%3F.txt",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			u, err := s.client.itemURL(context.Background(), tt.itemPath, tt.suffix)
			s.Require().NoError(err)
			s.Equal(s.server.URL+tt.expected, u)
		})
	}
}

func (s *GraphClientTestSuite) TestResolveDriveID() {
	tests := []struct {
		name          string
		opts          Options
		status        int
		body          string
		expectedPath  string
		expectedID    string
		expectedError string
	}{
		{name: "configured drive needs no request", opts: Options{DriveID: "given"}, expectedID: "given"},
		{name: "signed-in user drive", status: 200, body: `{"id":"me-drive"}`, expectedPath: "/me/drive", expectedID: "me-drive"},
		{name: "user drive", opts: Options{UserID: "a@b.com"}, status: 200, body: `{"id":"u"}`,
			expectedPath: "/users/a@b.com/drive", expectedID: "u"},
		{name: "site drive", opts: Options{SiteID: "site,1"}, status: 200, body: `{"id":"s"}`,
			expectedPath: "/sites/site%2C1/drive", expectedID: "s"},
		{name: "missing id", status: 200, body: `{}`,
			expectedPath: "/me/drive", expectedError: "drive response had no id"},
		{name: "api error", status: 403, body: `{"error":{"code":"accessDenied","message":"nope"}}`,
			expectedPath: "/me/drive", expectedError: "accessDenied"},
	}

	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.requests = nil
			s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, tt.status, tt.body) }
			opts := tt.opts
			opts.BaseURL = s.server.URL
			c := s.newClient(&opts)

			id, err := c.resolveDriveID(context.Background())
			if tt.expectedError != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expectedID, id)

			_, err = c.resolveDriveID(context.Background())
			s.Require().NoError(err)
			if tt.expectedPath == "" {
				s.Empty(s.requests)
			} else {
				s.Require().Len(s.requests, 1, "drive ID should be cached")
				s.Equal(tt.expectedPath, s.requests[0].Path)
			}
		})
	}
}

func (s *GraphClientTestSuite) TestGetItem() {
	s.Run("decodes file metadata and sends bearer token", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, itemJSON) }

		item, err := s.client.GetItem(context.Background(), "dir/file.txt")
		s.Require().NoError(err)
		s.Equal(&DriveItem{
			ID: "item1", Name: "file.txt", Size: 42,
			LastModified: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}, item)
		s.Equal("/drives/drive1/root:/dir/file.txt", s.requests[0].Path)
		s.Equal("Bearer test-token", s.requests[0].Header.Get("Authorization"))
	})

	s.Run("folder facet sets IsFolder", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 200, `{"id":"f","name":"dir","folder":{"childCount":3}}`)
		}

		item, err := s.client.GetItem(context.Background(), "dir")
		s.Require().NoError(err)
		s.True(item.IsFolder)
	})

	s.Run("404 is ErrNotFound", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 404, `{"error":{"code":"itemNotFound","message":"The resource could not be found."}}`)
		}

		_, err := s.client.GetItem(context.Background(), "missing.txt")
		s.Require().Error(err)
		s.Require().ErrorIs(err, ErrNotFound)
		s.Contains(err.Error(), "itemNotFound")
	})

	s.Run("malformed JSON", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, `{`) }

		_, err := s.client.GetItem(context.Background(), "x")
		s.Require().Error(err)
		s.Contains(err.Error(), "decoding response")
	})
}

func (s *GraphClientTestSuite) TestListChildren() {
	s.Run("follows nextLink pagination", func() {
		s.handler = func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page") == "2" {
				writeJSON(w, 200, `{"value":[{"name":"c.txt"}]}`)
				return
			}
			writeJSON(w, 200, fmt.Sprintf(
				`{"value":[{"name":"a.txt"},{"name":"sub","folder":{}}],"@odata.nextLink":%q}`,
				s.server.URL+"/drives/drive1/root:/dir:/children?page=2",
			))
		}

		items, err := s.client.ListChildren(context.Background(), "dir")
		s.Require().NoError(err)
		s.Require().Len(items, 3)
		s.Equal("a.txt", items[0].Name)
		s.True(items[1].IsFolder)
		s.Equal("c.txt", items[2].Name)
		s.Equal("/drives/drive1/root:/dir:/children", s.requests[0].Path)
	})

	s.Run("error status", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 404, `{}`) }

		_, err := s.client.ListChildren(context.Background(), "missing")
		s.Require().ErrorIs(err, ErrNotFound)
	})

	s.Run("malformed page", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, `[`) }

		_, err := s.client.ListChildren(context.Background(), "dir")
		s.Require().Error(err)
	})
}

func (s *GraphClientTestSuite) TestDownload() {
	s.Run("follows redirect to content", func() {
		s.handler = func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/download/blob" {
				_, _ = io.WriteString(w, "hello")
				return
			}
			http.Redirect(w, r, "/download/blob", http.StatusFound)
		}

		rc, err := s.client.Download(context.Background(), "file.txt")
		s.Require().NoError(err)
		defer func() { _ = rc.Close() }()
		data, err := io.ReadAll(rc)
		s.Require().NoError(err)
		s.Equal("hello", string(data))
		s.Equal("/drives/drive1/root:/file.txt:/content", s.requests[0].Path)
	})

	s.Run("not found", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 404, `{}`) }

		_, err := s.client.Download(context.Background(), "missing.txt")
		s.Require().ErrorIs(err, ErrNotFound)
	})
}

func (s *GraphClientTestSuite) TestUpload() {
	s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 201, itemJSON) }

	item, err := s.client.Upload(context.Background(), "dir/file.txt", strings.NewReader("content"))
	s.Require().NoError(err)
	s.Equal("item1", item.ID)

	req := s.requests[0]
	s.Equal(http.MethodPut, req.Method)
	s.Equal("/drives/drive1/root:/dir/file.txt:/content", req.Path)
	s.Equal("content", req.Body)
	s.Equal("application/octet-stream", req.Header.Get("Content-Type"))
	s.Equal("7", req.Header.Get("Content-Length"))
}

type cannedResponse struct {
	status int
	body   string
}

// routes answers "METHOD /escaped/path" with successive canned responses, repeating the last one.
func (s *GraphClientTestSuite) routes(routes map[string][]cannedResponse) {
	calls := map[string]int{}
	s.handler = func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.EscapedPath()
		responses, ok := routes[key]
		if !ok {
			writeJSON(w, http.StatusNotImplemented, `{"error":{"code":"unexpected","message":"`+key+`"}}`)
			return
		}
		resp := responses[min(calls[key], len(responses)-1)]
		calls[key]++
		writeJSON(w, resp.status, resp.body)
	}
}

func (s *GraphClientTestSuite) TestUploadUnderPercentFolder() {
	notFound := cannedResponse{404, `{"error":{"code":"itemNotFound"}}`}
	tests := []struct {
		name          string
		itemPath      string
		routes        map[string][]cannedResponse
		expectedCalls []string
		expectedBody  []string // JSON bodies of folder-creation requests, in order
		expectedError string
	}{
		{
			name:     "existing parent is addressed by ID",
			itemPath: "a%20b/f.txt",
			routes: map[string][]cannedResponse{
				"GET /drives/drive1/root:/a%2520b":            {{200, `{"id":"p1","folder":{}}`}},
				"PUT /drives/drive1/items/p1:/f.txt:/content": {{201, itemJSON}},
			},
			expectedCalls: []string{
				"GET /drives/drive1/root:/a%2520b",
				"PUT /drives/drive1/items/p1:/f.txt:/content",
			},
		},
		{
			name:     "missing ancestors are created literally",
			itemPath: "x/a%20b/f%20g.txt",
			routes: map[string][]cannedResponse{
				"GET /drives/drive1/root:/x/a%2520b":                 {notFound},
				"GET /drives/drive1/root:/x":                         {notFound},
				"POST /drives/drive1/items/root/children":            {{201, `{"id":"px","folder":{}}`}},
				"POST /drives/drive1/items/px/children":              {{201, `{"id":"pab","folder":{}}`}},
				"PUT /drives/drive1/items/pab:/f%2520g.txt:/content": {{201, itemJSON}},
			},
			expectedCalls: []string{
				"GET /drives/drive1/root:/x/a%2520b",
				"GET /drives/drive1/root:/x",
				"POST /drives/drive1/items/root/children",
				"POST /drives/drive1/items/px/children",
				"PUT /drives/drive1/items/pab:/f%2520g.txt:/content",
			},
			expectedBody: []string{
				`{"name":"x","folder":{},"@microsoft.graph.conflictBehavior":"fail"}`,
				`{"name":"a%20b","folder":{},"@microsoft.graph.conflictBehavior":"fail"}`,
			},
		},
		{
			name:     "folder created concurrently is looked up again",
			itemPath: "a%20b/f.txt",
			routes: map[string][]cannedResponse{
				"GET /drives/drive1/root:/a%2520b":            {notFound, {200, `{"id":"p1","folder":{}}`}},
				"POST /drives/drive1/items/root/children":     {{409, `{"error":{"code":"nameAlreadyExists"}}`}},
				"PUT /drives/drive1/items/p1:/f.txt:/content": {{201, itemJSON}},
			},
			expectedCalls: []string{
				"GET /drives/drive1/root:/a%2520b",
				"POST /drives/drive1/items/root/children",
				"GET /drives/drive1/root:/a%2520b",
				"PUT /drives/drive1/items/p1:/f.txt:/content",
			},
			expectedBody: []string{`{"name":"a%20b","folder":{},"@microsoft.graph.conflictBehavior":"fail"}`},
		},
		{
			name:     "parent that is a file is an error",
			itemPath: "a%20b/f.txt",
			routes: map[string][]cannedResponse{
				"GET /drives/drive1/root:/a%2520b": {{200, `{"id":"p1","file":{}}`}},
			},
			expectedError: "a%20b exists and is not a folder",
		},
		{
			name:     "lookup failure is returned",
			itemPath: "a%20b/f.txt",
			routes: map[string][]cannedResponse{
				"GET /drives/drive1/root:/a%2520b": {{403, `{"error":{"code":"accessDenied","message":"no"}}`}},
			},
			expectedError: "accessDenied",
		},
		{
			name:     "folder creation failure is returned",
			itemPath: "a%20b/f.txt",
			routes: map[string][]cannedResponse{
				"GET /drives/drive1/root:/a%2520b":        {notFound},
				"POST /drives/drive1/items/root/children": {{403, `{"error":{"code":"accessDenied","message":"no"}}`}},
			},
			expectedError: "creating folder a%20b",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.requests = nil
			s.routes(tt.routes)

			_, err := s.client.Upload(context.Background(), tt.itemPath, strings.NewReader("content"))
			if tt.expectedError != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tt.expectedError)
				return
			}
			s.Require().NoError(err)

			var calls, bodies []string
			for _, r := range s.requests {
				calls = append(calls, r.Method+" "+r.Path)
				if r.Method == http.MethodPost {
					bodies = append(bodies, r.Body)
				}
			}
			s.Equal(tt.expectedCalls, calls)
			s.Require().Len(bodies, len(tt.expectedBody))
			for i := range bodies {
				s.JSONEq(tt.expectedBody[i], bodies[i])
			}
		})
	}
}

func (s *GraphClientTestSuite) TestCreateUploadSessionUnderPercentFolder() {
	s.routes(map[string][]cannedResponse{
		"GET /drives/drive1/root:/a%2520b":                           {{200, `{"id":"p1","folder":{}}`}},
		"POST /drives/drive1/items/p1:/big.bin:/createUploadSession": {{200, `{"uploadUrl":"` + s.server.URL + `/up"}`}},
	})

	_, err := s.client.CreateUploadSession(context.Background(), "a%20b/big.bin")
	s.Require().NoError(err)
	s.Equal("/drives/drive1/items/p1:/big.bin:/createUploadSession", s.requests[1].Path)
}

func (s *GraphClientTestSuite) TestUploadSession() {
	s.handler = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":/createUploadSession"):
			writeJSON(w, 200, fmt.Sprintf(`{"uploadUrl":%q}`, s.server.URL+"/upload/session1"))
		case r.Method == http.MethodPut && strings.Contains(r.Header.Get("Content-Range"), "/10"):
			if strings.HasPrefix(r.Header.Get("Content-Range"), "bytes 0-") {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			writeJSON(w, 201, itemJSON)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}

	session, err := s.client.CreateUploadSession(context.Background(), "big.bin")
	s.Require().NoError(err)
	s.JSONEq(`{"item":{"@microsoft.graph.conflictBehavior":"replace"}}`, s.requests[0].Body)

	item, err := session.UploadChunk(context.Background(), []byte("01234"), 0, 10)
	s.Require().NoError(err)
	s.Nil(item, "intermediate chunk returns no item")

	item, err = session.UploadChunk(context.Background(), []byte("56789"), 5, 10)
	s.Require().NoError(err)
	s.Equal("item1", item.ID)

	chunk := s.requests[2]
	s.Equal("/upload/session1", chunk.Path)
	s.Equal("bytes 5-9/10", chunk.Header.Get("Content-Range"))
	s.Empty(chunk.Header.Get("Authorization"), "upload URLs are pre-authenticated")

	s.Require().NoError(session.Cancel(context.Background()))
	s.Equal(http.MethodDelete, s.requests[3].Method)
}

func (s *GraphClientTestSuite) TestUploadSessionErrors() {
	s.Run("missing uploadUrl", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, `{}`) }

		_, err := s.client.CreateUploadSession(context.Background(), "big.bin")
		s.Require().Error(err)
		s.Contains(err.Error(), "no uploadUrl")
	})

	s.Run("cancel of expired session is not an error", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 404, `{}`) }
		session := &graphUploadSession{client: s.client, uploadURL: s.server.URL + "/upload/gone"}

		s.NoError(session.Cancel(context.Background()))
	})

	s.Run("cancel failure", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 400, `{}`) }
		session := &graphUploadSession{client: s.client, uploadURL: s.server.URL + "/upload/bad"}

		s.Require().Error(session.Cancel(context.Background()))
	})

	s.Run("chunk rejected", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 416, `{}`) }
		session := &graphUploadSession{client: s.client, uploadURL: s.server.URL + "/upload/bad"}

		_, err := session.UploadChunk(context.Background(), []byte("x"), 0, 1)
		s.Require().Error(err)
	})
}

// copyHandler serves the destination-folder lookup, the copy request, and a monitor that reports
// the given statuses in order.
func (s *GraphClientTestSuite) copyHandler(monitor []func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	calls := 0
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/drives/drive1/root:/dest":
			writeJSON(w, 200, `{"id":"parent1","folder":{}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":/copy"):
			w.Header().Set("Location", s.server.URL+"/monitor/1")
			w.WriteHeader(http.StatusAccepted)
		case r.URL.Path == "/monitor/1":
			monitor[min(calls, len(monitor)-1)](w, r)
			calls++
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

func (s *GraphClientTestSuite) TestCopy() {
	type monitorStep = func(http.ResponseWriter, *http.Request)
	inProgress := func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 202, `{"status":"inProgress"}`) }
	completed := func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, `{"status":"completed"}`) }
	redirect := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/drives/drive1/items/new")
		w.WriteHeader(http.StatusSeeOther)
	}
	failed := func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, `{"status":"failed","error":{"code":"quotaLimitReached","message":"full"}}`)
	}

	tests := []struct {
		name           string
		monitor        []monitorStep
		expectedSleeps int
		expectedError  string
	}{
		{name: "polls until completed", monitor: []monitorStep{inProgress, inProgress, completed}, expectedSleeps: 2},
		{name: "redirect means completed", monitor: []monitorStep{inProgress, redirect}, expectedSleeps: 1},
		{name: "failed status", monitor: []monitorStep{failed}, expectedError: "quotaLimitReached"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.requests, s.sleeps = nil, nil
			s.handler = s.copyHandler(tt.monitor)

			err := s.client.Copy(context.Background(), "src/a.txt", "dest", "b.txt")
			if tt.expectedError != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Len(s.sleeps, tt.expectedSleeps)

			copyReq := s.requests[1]
			s.Equal("/drives/drive1/root:/src/a.txt:/copy", copyReq.Path)
			s.Equal(replaceOnConflict, copyReq.Query)
			s.JSONEq(`{"parentReference":{"driveId":"drive1","id":"parent1"},"name":"b.txt"}`, copyReq.Body)
			for _, r := range s.requests[2:] {
				s.Empty(r.Header.Get("Authorization"), "monitor URLs are unauthenticated")
			}
		})
	}
}

func (s *GraphClientTestSuite) TestCopyErrors() {
	s.Run("destination folder missing", func() {
		s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 404, `{}`) }

		err := s.client.Copy(context.Background(), "a.txt", "nope", "a.txt")
		s.Require().ErrorIs(err, ErrNotFound)
	})

	s.Run("no monitor location", func() {
		s.handler = func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				writeJSON(w, 200, `{"id":"p"}`)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		}

		err := s.client.Copy(context.Background(), "a.txt", "dest", "a.txt")
		s.Require().Error(err)
		s.Contains(err.Error(), "no monitor Location")
	})

	s.Run("copy times out", func() {
		s.handler = s.copyHandler([]func(http.ResponseWriter, *http.Request){
			func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 202, `{"status":"inProgress"}`) },
		})
		c := s.newClient(&Options{BaseURL: s.server.URL, DriveID: "drive1"})
		c.copyTimeout = time.Nanosecond
		c.sleep = sleepContext

		err := c.Copy(context.Background(), "a.txt", "dest", "a.txt")
		s.Require().ErrorIs(err, context.DeadlineExceeded)
	})
}

func (s *GraphClientTestSuite) TestMove() {
	s.handler = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, 200, `{"id":"parent1","folder":{}}`)
			return
		}
		writeJSON(w, 200, itemJSON)
	}

	item, err := s.client.Move(context.Background(), "src/a.txt", "dest", "b.txt")
	s.Require().NoError(err)
	s.Equal("item1", item.ID)

	req := s.requests[1]
	s.Equal(http.MethodPatch, req.Method)
	s.Equal("/drives/drive1/root:/src/a.txt", req.Path)
	s.Equal(replaceOnConflict, req.Query)
	s.JSONEq(`{"parentReference":{"id":"parent1"},"name":"b.txt"}`, req.Body)
}

func (s *GraphClientTestSuite) TestMoveDestinationMissing() {
	s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 404, `{}`) }

	_, err := s.client.Move(context.Background(), "a.txt", "nope", "a.txt")
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *GraphClientTestSuite) TestDelete() {
	tests := []struct {
		name          string
		status        int
		expectedError error
	}{
		{name: "deleted", status: 204},
		{name: "not found", status: 404, expectedError: ErrNotFound},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.requests = nil
			s.handler = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.status) }

			err := s.client.Delete(context.Background(), "a.txt")
			if tt.expectedError != nil {
				s.Require().ErrorIs(err, tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Equal(http.MethodDelete, s.requests[0].Method)
		})
	}
}

func (s *GraphClientTestSuite) TestRetries() {
	tests := []struct {
		name           string
		statuses       []int
		retryAfter     string
		expectedCalls  int
		expectedSleeps []time.Duration
		expectedStatus int
	}{
		{name: "throttled then succeeds, honoring Retry-After", statuses: []int{429, 201}, retryAfter: "7",
			expectedCalls: 2, expectedSleeps: []time.Duration{7 * time.Second}},
		{name: "exponential backoff without Retry-After", statuses: []int{503, 502, 201},
			expectedCalls: 3, expectedSleeps: []time.Duration{time.Second, 2 * time.Second}},
		{name: "gives up after RetryCount retries", statuses: []int{503, 503, 503, 503},
			expectedCalls: 3, expectedSleeps: []time.Duration{time.Second, 2 * time.Second}, expectedStatus: 503},
		{name: "client errors are not retried", statuses: []int{400},
			expectedCalls: 1, expectedStatus: 400},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.requests, s.sleeps = nil, nil
			call := 0
			s.handler = func(w http.ResponseWriter, _ *http.Request) {
				status := tt.statuses[call]
				call++
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				writeJSON(w, status, itemJSON)
			}

			_, err := s.client.Upload(context.Background(), "a.txt", strings.NewReader("payload"))
			s.Len(s.requests, tt.expectedCalls)
			s.Equal(tt.expectedSleeps, s.sleeps)
			for _, r := range s.requests {
				s.Equal("payload", r.Body, "body is resent in full on retry")
			}

			if tt.expectedStatus != 0 {
				var apiErr *apiError
				s.Require().ErrorAs(err, &apiErr)
				s.Equal(tt.expectedStatus, apiErr.StatusCode)
				return
			}
			s.Require().NoError(err)
		})
	}
}

func (s *GraphClientTestSuite) TestRetryWaitCanceled() {
	s.handler = func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 503, `{}`) }
	s.client.sleep = func(context.Context, time.Duration) error { return context.Canceled }

	_, err := s.client.GetItem(context.Background(), "a.txt")
	s.Require().ErrorIs(err, context.Canceled)
}

type failingCredential struct{}

func (failingCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, errors.New("token boom")
}

func (s *GraphClientTestSuite) TestTokenFailure() {
	c := newGraphClient(failingCredential{}, &Options{BaseURL: s.server.URL, DriveID: "drive1"})

	_, err := c.GetItem(context.Background(), "a.txt")
	s.Require().Error(err)
	s.Contains(err.Error(), "token boom")
	s.Empty(s.requests)
}

func (s *GraphClientTestSuite) TestTransportFailure() {
	s.server.Close()

	_, err := s.client.GetItem(context.Background(), "a.txt")
	s.Require().Error(err)
	s.Contains(err.Error(), "GET request failed")
}

func (s *GraphClientTestSuite) TestNewGraphClientDefaults() {
	c := newGraphClient(staticTokenCredential("t"), &Options{BaseURL: "https://example.test/v1.0/"})
	s.Equal("https://example.test/v1.0", c.baseURL)
	s.Same(http.DefaultClient, c.httpClient)

	c = newGraphClient(staticTokenCredential("t"), &Options{})
	s.Equal(DefaultBaseURL, c.baseURL)
}

func (s *GraphClientTestSuite) TestAPIErrorMessage() {
	s.Equal("onedrive: graph request failed with status 500", (&apiError{StatusCode: 500}).Error())
	s.Equal(
		"onedrive: graph request failed with status 409 (nameAlreadyExists): exists",
		(&apiError{StatusCode: 409, Code: "nameAlreadyExists", Message: "exists"}).Error(),
	)
	s.NotErrorIs(&apiError{StatusCode: 409}, ErrNotFound)
}

func (s *GraphClientTestSuite) TestRetryDelay() {
	tests := []struct {
		name       string
		retryAfter string
		attempt    int
		expected   time.Duration
	}{
		{name: "Retry-After seconds", retryAfter: "3", expected: 3 * time.Second},
		{name: "Retry-After capped", retryAfter: "3600", expected: maxRetryDelay},
		{name: "unparseable Retry-After falls back to backoff", retryAfter: "soon", attempt: 2, expected: 4 * time.Second},
		{name: "backoff capped", attempt: 10, expected: maxRetryDelay},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			resp := &http.Response{Header: http.Header{}}
			if tt.retryAfter != "" {
				resp.Header.Set("Retry-After", tt.retryAfter)
			}
			s.Equal(tt.expected, retryDelay(resp, tt.attempt))
		})
	}
}

func (s *GraphClientTestSuite) TestSleepContext() {
	s.Require().NoError(sleepContext(context.Background(), time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Require().ErrorIs(sleepContext(ctx, time.Hour), context.Canceled)
}
