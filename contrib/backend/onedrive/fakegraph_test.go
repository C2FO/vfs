package onedrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const fakeDriveID = "drive1"

// fakeNode is one file or folder of the fake drive.
type fakeNode struct {
	id       string
	folder   bool
	data     []byte
	modified time.Time
}

// fakeGraph is a small stateful in-process Microsoft Graph server. It serves the requests the listing needs: drive
// resolution, path-addressed items, paged children, file content, upload and delete. It returns children in reverse
// name order, which Graph does not promise either way, so a listing that did not sort would be caught.
type fakeGraph struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	nodes    map[string]*fakeNode // keyed by drive-relative path, "" is the root
	pageSize int
	log      []string
	hook     func(w http.ResponseWriter, r *http.Request) bool
	nextID   int
}

func newFakeGraph(t *testing.T) *fakeGraph {
	t.Helper()
	g := &fakeGraph{
		t:        t,
		nodes:    map[string]*fakeNode{"": {id: "root", folder: true, modified: time.Now().UTC()}},
		pageSize: 3,
	}
	g.srv = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGraph) url() string { return g.srv.URL }

// setHook installs a function that may answer a request itself; it returns true when it did.
func (g *fakeGraph) setHook(h func(w http.ResponseWriter, r *http.Request) bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.hook = h
}

// requests returns "METHOD path" for every request received, with the drive prefix removed.
func (g *fakeGraph) requests() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.log)
}

func (g *fakeGraph) count(prefix string) int {
	n := 0
	for _, r := range g.requests() {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

// put stores a file, creating any missing folders above it.
func (g *fakeGraph) put(p string, data []byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.putLocked(p, data)
}

func (g *fakeGraph) putLocked(p string, data []byte) *fakeNode {
	g.mkdirsLocked(path.Dir(p))
	n := &fakeNode{id: g.newIDLocked(), data: data, modified: time.Now().UTC().Truncate(time.Second)}
	g.nodes[p] = n
	return n
}

func (g *fakeGraph) mkdirsLocked(p string) {
	if p == "." || p == "" {
		return
	}
	if _, ok := g.nodes[p]; ok {
		return
	}
	g.mkdirsLocked(path.Dir(p))
	g.nodes[p] = &fakeNode{id: g.newIDLocked(), folder: true, modified: time.Now().UTC().Truncate(time.Second)}
}

func (g *fakeGraph) newIDLocked() string {
	g.nextID++
	return "id" + strconv.Itoa(g.nextID)
}

func (g *fakeGraph) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	hook := g.hook
	g.mu.Unlock()
	if hook != nil && hook(w, r) {
		return
	}

	if r.URL.Path == "/me/drive" {
		writeFakeBody(w, map[string]any{"id": fakeDriveID})
		return
	}

	rest, ok := strings.CutPrefix(r.URL.Path, "/drives/"+fakeDriveID+"/root")
	if !ok {
		writeFakeErr(w, http.StatusBadRequest, "invalidRequest", "unsupported path "+r.URL.Path)
		return
	}

	itemPath, suffix := "", rest
	if p, ok := strings.CutPrefix(rest, ":/"); ok {
		itemPath, suffix, _ = strings.Cut(p, ":")
		itemPath = strings.Trim(itemPath, "/")
	}

	g.mu.Lock()
	g.log = append(g.log, r.Method+" "+path.Join("/", itemPath)+suffix)
	g.mu.Unlock()

	g.route(w, r, itemPath, suffix)
}

func (g *fakeGraph) route(w http.ResponseWriter, r *http.Request, itemPath, suffix string) {
	switch {
	case r.Method == http.MethodGet && suffix == "":
		g.getItem(w, itemPath)
	case r.Method == http.MethodGet && suffix == "/children":
		g.children(w, r, itemPath)
	case r.Method == http.MethodGet && suffix == "/content":
		g.content(w, itemPath)
	case r.Method == http.MethodPut && suffix == "/content":
		g.upload(w, r, itemPath)
	case r.Method == http.MethodDelete && suffix == "":
		g.remove(w, itemPath)
	default:
		writeFakeErr(w, http.StatusBadRequest, "invalidRequest", fmt.Sprintf("unsupported %s %s", r.Method, r.URL.Path))
	}
}

func fakeItemJSON(name string, n *fakeNode) map[string]any {
	m := map[string]any{
		"id":                   n.id,
		"name":                 name,
		"size":                 len(n.data),
		"lastModifiedDateTime": n.modified.Format(time.RFC3339),
	}
	if n.folder {
		m["folder"] = map[string]any{}
	} else {
		m["file"] = map[string]any{}
	}
	return m
}

func (g *fakeGraph) getItem(w http.ResponseWriter, p string) {
	g.mu.Lock()
	n, ok := g.nodes[p]
	g.mu.Unlock()
	if !ok {
		writeFakeErr(w, http.StatusNotFound, "itemNotFound", "item not found")
		return
	}
	writeFakeBody(w, fakeItemJSON(path.Base(p), n))
}

func (g *fakeGraph) children(w http.ResponseWriter, r *http.Request, p string) {
	g.mu.Lock()
	n, ok := g.nodes[p]
	if !ok || !n.folder {
		g.mu.Unlock()
		writeFakeErr(w, http.StatusNotFound, "itemNotFound", "item not found")
		return
	}

	var names []string
	for child := range g.nodes {
		if child == "" {
			continue
		}
		if parent := path.Dir(child); parent == p || (parent == "." && p == "") {
			names = append(names, path.Base(child))
		}
	}
	slices.Sort(names)
	slices.Reverse(names)

	start, _ := strconv.Atoi(r.URL.Query().Get("$skiptoken"))
	end := min(start+g.pageSize, len(names))
	page := make([]map[string]any, 0, end-start)
	for _, name := range names[min(start, end):end] {
		page = append(page, fakeItemJSON(name, g.nodes[path.Join(p, name)]))
	}
	g.mu.Unlock()

	body := map[string]any{"value": page}
	if end < len(names) {
		next := g.url() + r.URL.Path + "?$skiptoken=" + strconv.Itoa(end)
		body["@odata.nextLink"] = next
	}
	writeFakeBody(w, body)
}

func (g *fakeGraph) content(w http.ResponseWriter, p string) {
	g.mu.Lock()
	n, ok := g.nodes[p]
	g.mu.Unlock()
	if !ok || n.folder {
		writeFakeErr(w, http.StatusNotFound, "itemNotFound", "item not found")
		return
	}
	_, _ = w.Write(n.data)
}

func (g *fakeGraph) upload(w http.ResponseWriter, r *http.Request, p string) {
	data, err := io.ReadAll(r.Body)
	require.NoError(g.t, err)
	g.mu.Lock()
	n := g.putLocked(p, data)
	g.mu.Unlock()
	writeFakeBody(w, fakeItemJSON(path.Base(p), n))
}

func (g *fakeGraph) remove(w http.ResponseWriter, p string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.nodes[p]; !ok {
		writeFakeErr(w, http.StatusNotFound, "itemNotFound", "item not found")
		return
	}
	for k := range g.nodes {
		if k == p || strings.HasPrefix(k, p+"/") {
			delete(g.nodes, k)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeFakeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg}})
}

func writeFakeBody(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// newFakeGraphFS returns a FileSystem wired to a fake Graph with instant retries.
func newFakeGraphFS(t *testing.T, g *fakeGraph) *FileSystem {
	t.Helper()
	fs := NewFileSystem(
		WithBaseURL(g.url()),
		WithAccessToken("test-token"),
		WithTempDir(t.TempDir()),
		WithRetryCount(2),
	)
	c, err := fs.Client()
	require.NoError(t, err)
	if gc, ok := c.(*graphClient); ok {
		gc.sleep = func(_ context.Context, _ time.Duration) error { return nil }
	}
	return fs
}
