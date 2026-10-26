package dropbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
)

const (
	fakeToken          = "test-token"
	fakeListPageSize   = 3
	fakeTimeFormat     = "2006-01-02T15:04:05Z"
	fakeStyleRPC       = "rpc"
	fakeStyleUpload    = "upload"
	fakeStyleDownload  = "download"
	fakeHostAPI        = "api"
	fakeHostContent    = "content"
	fakeWriteModeAdd   = "add"
	fakeWriteModeOvwr  = "overwrite"
	fakeTagNotFound    = "not_found"
	fakeTagConflict    = "conflict"
	fakeTagFile        = "file"
	fakeTagFolder      = "folder"
	fakeTagMalformed   = "malformed_path"
	fakeConflictFolder = "folder"
)

// fakeRoute describes which host and request style a route must arrive with.
type fakeRoute struct {
	host  string
	style string
}

var fakeRoutes = map[string]fakeRoute{
	"get_metadata":             {fakeHostAPI, fakeStyleRPC},
	"list_folder":              {fakeHostAPI, fakeStyleRPC},
	"list_folder/continue":     {fakeHostAPI, fakeStyleRPC},
	"copy_v2":                  {fakeHostAPI, fakeStyleRPC},
	"move_v2":                  {fakeHostAPI, fakeStyleRPC},
	"delete_v2":                {fakeHostAPI, fakeStyleRPC},
	"upload_session/start":     {fakeHostContent, fakeStyleUpload},
	"upload":                   {fakeHostContent, fakeStyleUpload},
	"upload_session/append_v2": {fakeHostContent, fakeStyleUpload},
	"upload_session/finish":    {fakeHostContent, fakeStyleUpload},
	"download":                 {fakeHostContent, fakeStyleDownload},
}

// fakeEntry is a stored file or folder.
type fakeEntry struct {
	displayPath    string
	isFolder       bool
	data           []byte
	clientModified time.Time
	serverModified time.Time
}

// fakeSession is an in-flight chunked upload.
type fakeSession struct {
	data []byte
}

// fakeDropbox is an in-process, stateful implementation of the subset of the
// Dropbox v2 files API used by this backend. It deliberately does not use any
// vfs backend for storage.
type fakeDropbox struct {
	mu       sync.Mutex
	entries  map[string]*fakeEntry // keyed by lower-cased path
	sessions map[string]*fakeSession
	cursors  map[string][]map[string]any
	nextID   int

	// routeCalls counts requests per route, so tests can assert which routes were exercised.
	routeCalls map[string]int
}

func newFakeDropbox() *fakeDropbox {
	return &fakeDropbox{
		entries:    map[string]*fakeEntry{},
		sessions:   map[string]*fakeSession{},
		cursors:    map[string][]map[string]any{},
		routeCalls: map[string]int{},
	}
}

// calls returns how many times a route has been requested.
func (f *fakeDropbox) calls(route string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.routeCalls[route]
}

// newFakeServer starts a fake server and returns it with a real files.Client pointed at it.
func newFakeServer(t *testing.T) (*fakeDropbox, *httptest.Server, files.Client) {
	t.Helper()
	fake := newFakeDropbox()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	cfg := dropbox.Config{
		Client: srv.Client(),
		URLGenerator: func(host, ns, route string) string {
			return fmt.Sprintf("%s/%s/2/%s/%s", srv.URL, host, ns, route)
		},
		HeaderGenerator: func(string, string, string) map[string]string {
			return map[string]string{"Authorization": "Bearer " + fakeToken}
		},
	}
	return fake, srv, files.New(cfg)
}

// newFakeClient is newFakeServer without the server handle.
func newFakeClient(t *testing.T) (*fakeDropbox, files.Client) {
	t.Helper()
	fake, _, client := newFakeServer(t)
	return fake, client
}

// ServeHTTP implements http.Handler.
func (f *fakeDropbox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+fakeToken {
		http.Error(w, `{"error_summary":"invalid_access_token/","error":{".tag":"invalid_access_token"}}`, http.StatusUnauthorized)
		return
	}

	// path: /{host}/2/files/{route...}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 4)
	if len(parts) != 4 || parts[1] != "2" || parts[2] != "files" {
		http.Error(w, "unknown endpoint "+r.URL.Path, http.StatusNotFound)
		return
	}
	host, route := parts[0], parts[3]

	spec, ok := fakeRoutes[route]
	if !ok {
		http.Error(w, "unknown route "+route, http.StatusNotFound)
		return
	}
	if spec.host != host {
		http.Error(w, fmt.Sprintf("route %s must be sent to host %s, got %s", route, spec.host, host), http.StatusBadRequest)
		return
	}

	argBytes, err := f.readArg(r, spec.style)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.routeCalls[route]++
	f.dispatch(w, r, route, argBytes)
}

func (f *fakeDropbox) dispatch(w http.ResponseWriter, r *http.Request, route string, argBytes []byte) {
	switch route {
	case "get_metadata":
		f.getMetadata(w, argBytes)
	case "list_folder":
		f.listFolder(w, argBytes)
	case "list_folder/continue":
		f.listFolderContinue(w, argBytes)
	case "download":
		f.download(w, argBytes)
	case "upload":
		f.upload(w, r, argBytes)
	case "upload_session/start":
		f.sessionStart(w, r)
	case "upload_session/append_v2":
		f.sessionAppend(w, r, argBytes)
	case "upload_session/finish":
		f.sessionFinish(w, r, argBytes)
	case "copy_v2":
		f.relocate(w, argBytes, false)
	case "move_v2":
		f.relocate(w, argBytes, true)
	case "delete_v2":
		f.deletePath(w, argBytes)
	}
}

// readArg extracts the JSON argument according to the route's request style. RPC routes carry
// the argument in the body; upload/download routes carry it in the Dropbox-API-Arg header.
func (f *fakeDropbox) readArg(r *http.Request, style string) ([]byte, error) {
	headerArg := r.Header.Get("Dropbox-API-Arg")
	switch style {
	case fakeStyleRPC:
		if headerArg != "" {
			return nil, errors.New("rpc route received Dropbox-API-Arg header")
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			return nil, fmt.Errorf("rpc route requires application/json, got %q", ct)
		}
		return io.ReadAll(r.Body)
	default:
		if headerArg == "" {
			return nil, fmt.Errorf("%s route missing Dropbox-API-Arg header", style)
		}
		// The real API requires non-ASCII characters in this header to be \uXXXX-escaped.
		for _, c := range []byte(headerArg) {
			if c >= 0x80 {
				return nil, errors.New("Dropbox-API-Arg header must be ASCII; non-ASCII characters must be JSON-escaped")
			}
		}
		return []byte(headerArg), nil
	}
}

// ---- helpers ----

func normalize(p string) string { return strings.ToLower(strings.TrimSuffix(p, "/")) }

func parentOf(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return ""
	}
	return p[:idx]
}

func baseOf(p string) string { return p[strings.LastIndex(p, "/")+1:] }

// validPath reports whether p is acceptable to the API: empty (root) or beginning with "/".
func validPath(p string) bool { return p == "" || strings.HasPrefix(p, "/") }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiError writes a 409 with the real Dropbox error body shape.
func apiError(w http.ResponseWriter, summary string, errBody map[string]any) {
	writeJSON(w, http.StatusConflict, map[string]any{"error_summary": summary, "error": errBody})
}

func lookupNotFound(w http.ResponseWriter, tag string) {
	apiError(w, tag+"/not_found/.", map[string]any{".tag": tag, tag: map[string]any{".tag": fakeTagNotFound}})
}

func lookupMalformed(w http.ResponseWriter, tag string) {
	apiError(w, tag+"/malformed_path/.", map[string]any{".tag": tag, tag: map[string]any{".tag": fakeTagMalformed}})
}

func (e *fakeEntry) metadata(id string) map[string]any {
	m := map[string]any{
		".tag":         fakeTagFolder,
		"name":         baseOf(e.displayPath),
		"path_lower":   strings.ToLower(e.displayPath),
		"path_display": e.displayPath,
		"id":           id,
	}
	if e.isFolder {
		return m
	}
	m[".tag"] = fakeTagFile
	m["client_modified"] = e.clientModified.UTC().Format(fakeTimeFormat)
	m["server_modified"] = e.serverModified.UTC().Format(fakeTimeFormat)
	m["rev"] = "0123456789abcdef"
	m["size"] = len(e.data)
	return m
}

func (f *fakeDropbox) idFor(key string) string { return "id:" + key }

// ensureParents creates implicit parent folders, as the real API does on upload and relocation.
func (f *fakeDropbox) ensureParents(displayPath string) {
	for p := parentOf(displayPath); p != ""; p = parentOf(p) {
		key := normalize(p)
		if _, ok := f.entries[key]; ok {
			continue
		}
		f.entries[key] = &fakeEntry{displayPath: p, isFolder: true}
	}
}

// hasFileAncestor reports whether any ancestor of displayPath is a file.
func (f *fakeDropbox) hasFileAncestor(displayPath string) bool {
	for p := parentOf(displayPath); p != ""; p = parentOf(p) {
		if e, ok := f.entries[normalize(p)]; ok && !e.isFolder {
			return true
		}
	}
	return false
}

// childrenOf returns the direct children of the folder at key (""=root), sorted by name.
func (f *fakeDropbox) childrenOf(key string) []*fakeEntry {
	var out []*fakeEntry
	for k, e := range f.entries {
		if k != key && parentOf(k) == key {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].displayPath < out[j].displayPath })
	return out
}

// ---- RPC routes ----

func (f *fakeDropbox) getMetadata(w http.ResponseWriter, arg []byte) {
	var in files.GetMetadataArg
	if err := json.Unmarshal(arg, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validPath(in.Path) {
		lookupMalformed(w, "path")
		return
	}
	if in.Path == "" {
		// The real API refuses to return metadata for the root folder.
		lookupMalformed(w, "path")
		return
	}
	e, ok := f.entries[normalize(in.Path)]
	if !ok {
		lookupNotFound(w, "path")
		return
	}
	writeJSON(w, http.StatusOK, e.metadata(f.idFor(normalize(in.Path))))
}

func (f *fakeDropbox) listFolder(w http.ResponseWriter, arg []byte) {
	var in files.ListFolderArg
	if err := json.Unmarshal(arg, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validPath(in.Path) || strings.HasSuffix(in.Path, "/") {
		lookupMalformed(w, "path")
		return
	}
	key := normalize(in.Path)
	if key != "" {
		e, ok := f.entries[key]
		if !ok {
			lookupNotFound(w, "path")
			return
		}
		if !e.isFolder {
			apiError(w, "path/not_folder/.", map[string]any{".tag": "path", "path": map[string]any{".tag": "not_folder"}})
			return
		}
	}

	var all []map[string]any
	for _, c := range f.childrenOf(key) {
		all = append(all, c.metadata(f.idFor(normalize(c.displayPath))))
	}
	f.writeListPage(w, all)
}

func (f *fakeDropbox) listFolderContinue(w http.ResponseWriter, arg []byte) {
	var in files.ListFolderContinueArg
	if err := json.Unmarshal(arg, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rest, ok := f.cursors[in.Cursor]
	if !ok {
		apiError(w, "reset/.", map[string]any{".tag": "reset"})
		return
	}
	delete(f.cursors, in.Cursor)
	f.writeListPage(w, rest)
}

// writeListPage emits up to fakeListPageSize entries, stashing the remainder under a new cursor.
func (f *fakeDropbox) writeListPage(w http.ResponseWriter, all []map[string]any) {
	page := all
	hasMore := false
	if len(all) > fakeListPageSize {
		page, hasMore = all[:fakeListPageSize], true
	}
	f.nextID++
	cursor := fmt.Sprintf("cursor-%d", f.nextID)
	if hasMore {
		f.cursors[cursor] = all[fakeListPageSize:]
	}

	writeJSON(w, http.StatusOK, map[string]any{"entries": page, "cursor": cursor, "has_more": hasMore})
}

func (f *fakeDropbox) relocate(w http.ResponseWriter, arg []byte, move bool) {
	var in files.RelocationArg
	if err := json.Unmarshal(arg, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if summary, body := f.checkRelocation(in.FromPath, in.ToPath); body != nil {
		apiError(w, summary, body)
		return
	}

	fromKey, toKey := normalize(in.FromPath), normalize(in.ToPath)
	f.ensureParents(in.ToPath)

	// Relocate the entry and, for folders, every descendant.
	moved := map[string]*fakeEntry{}
	for k, e := range f.entries {
		if !isSelfOrDescendant(k, fromKey) {
			continue
		}
		dup := *e
		dup.displayPath = in.ToPath + e.displayPath[len(in.FromPath):]
		dup.data = append([]byte(nil), e.data...)
		moved[normalize(dup.displayPath)] = &dup
	}
	if move {
		for k := range f.entries {
			if isSelfOrDescendant(k, fromKey) {
				delete(f.entries, k)
			}
		}
	}
	for k, e := range moved {
		f.entries[k] = e
	}

	writeJSON(w, http.StatusOK, map[string]any{"metadata": moved[toKey].metadata(f.idFor(toKey))})
}

func isSelfOrDescendant(key, ancestor string) bool {
	return key == ancestor || strings.HasPrefix(key, ancestor+"/")
}

// checkRelocation validates a copy/move, returning a non-nil error body when the real API would refuse.
func (f *fakeDropbox) checkRelocation(from, to string) (string, map[string]any) {
	if !validPath(from) || !validPath(to) || from == "" || to == "" {
		return "from_lookup/malformed_path/.", map[string]any{
			".tag": "from_lookup", "from_lookup": map[string]any{".tag": fakeTagMalformed},
		}
	}
	if _, ok := f.entries[normalize(from)]; !ok {
		return "from_lookup/not_found/.", map[string]any{
			".tag": "from_lookup", "from_lookup": map[string]any{".tag": fakeTagNotFound},
		}
	}

	if isSelfOrDescendant(normalize(to), normalize(from)) {
		return "cant_move_folder_into_itself/.", map[string]any{".tag": "cant_move_folder_into_itself"}
	}

	conflict := ""
	switch dst, exists := f.entries[normalize(to)]; {
	case exists && dst.isFolder:
		conflict = fakeConflictFolder
	case exists, f.hasFileAncestor(to):
		conflict = fakeTagFile
	}
	if conflict == "" {
		return "", nil
	}
	return "to/conflict/" + conflict + "/.", map[string]any{
		".tag": "to",
		"to":   map[string]any{".tag": fakeTagConflict, fakeTagConflict: map[string]any{".tag": conflict}},
	}
}

func (f *fakeDropbox) deletePath(w http.ResponseWriter, arg []byte) {
	var in files.DeleteArg
	if err := json.Unmarshal(arg, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validPath(in.Path) || in.Path == "" {
		lookupMalformed(w, "path_lookup")
		return
	}
	key := normalize(in.Path)
	e, ok := f.entries[key]
	if !ok {
		apiError(w, "path_lookup/not_found/.", map[string]any{".tag": "path_lookup", "path_lookup": map[string]any{".tag": fakeTagNotFound}})
		return
	}
	meta := e.metadata(f.idFor(key))
	for k := range f.entries {
		if k == key || strings.HasPrefix(k, key+"/") {
			delete(f.entries, k)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"metadata": meta})
}

// ---- content routes ----

func (f *fakeDropbox) download(w http.ResponseWriter, arg []byte) {
	var in files.DownloadArg
	if err := json.Unmarshal(arg, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e, ok := f.entries[normalize(in.Path)]
	if !ok || e.isFolder {
		lookupNotFound(w, "path")
		return
	}
	meta, _ := json.Marshal(e.metadata(f.idFor(normalize(in.Path))))
	w.Header().Set("Dropbox-API-Result", string(meta))
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(e.data)
}

// commitError writes a 409 for a failed commit. The real API nests the WriteError differently per
// route: upload wraps it in UploadWriteFailed ({"path":{"reason":...}} flattened under the tag),
// while upload_session/finish carries it directly under "path".
func commitError(w http.ResponseWriter, session bool, summary string, writeErr map[string]any) {
	if session {
		apiError(w, summary, map[string]any{".tag": "path", "path": writeErr})
		return
	}
	apiError(w, summary, map[string]any{".tag": "path", "reason": writeErr})
}

// commit writes data to path honoring mode, returning false after writing a conflict error.
// session selects the error shape of upload_session/finish rather than upload.
func (f *fakeDropbox) commit(w http.ResponseWriter, session bool, path, mode string, clientModified *time.Time, data []byte) bool {
	if !validPath(path) || path == "" {
		commitError(w, session, "path/malformed_path/.", map[string]any{".tag": fakeTagMalformed})
		return false
	}
	key := normalize(path)
	existing, exists := f.entries[key]
	if f.hasFileAncestor(path) || (exists && existing.isFolder) || (exists && mode != fakeWriteModeOvwr) {
		conflict := fakeTagFile
		if exists && existing.isFolder {
			conflict = fakeConflictFolder
		}
		commitError(w, session, "path/conflict/"+conflict+"/..",
			map[string]any{".tag": fakeTagConflict, fakeTagConflict: map[string]any{".tag": conflict}})
		return false
	}

	f.ensureParents(path)
	now := time.Now().UTC().Truncate(time.Second)
	cm := now
	if clientModified != nil {
		cm = clientModified.UTC()
	}
	entry := &fakeEntry{displayPath: path, data: append([]byte(nil), data...), clientModified: cm, serverModified: now}
	f.entries[key] = entry
	return true
}

// modeOf extracts the write mode tag, defaulting to add as the real API does.
func modeOf(m *files.WriteMode) string {
	if m == nil || m.Tag == "" {
		return fakeWriteModeAdd
	}
	return m.Tag
}

func clientModifiedOf(t *dropbox.DBXTime) *time.Time {
	if t == nil {
		return nil
	}
	v := time.Time(*t)
	return &v
}

func (f *fakeDropbox) upload(w http.ResponseWriter, r *http.Request, arg []byte) {
	var in files.UploadArg
	if err := json.Unmarshal(arg, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !f.commit(w, false, in.Path, modeOf(in.Mode), clientModifiedOf(in.ClientModified), body) {
		return
	}
	writeJSON(w, http.StatusOK, f.entries[normalize(in.Path)].metadata(f.idFor(normalize(in.Path))))
}

func (f *fakeDropbox) sessionStart(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.nextID++
	id := fmt.Sprintf("session-%d", f.nextID)
	f.sessions[id] = &fakeSession{data: append([]byte(nil), body...)}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": id})
}

func (f *fakeDropbox) sessionAppend(w http.ResponseWriter, r *http.Request, arg []byte) {
	var in files.UploadSessionAppendArg
	if err := json.Unmarshal(arg, &in); err != nil || in.Cursor == nil {
		http.Error(w, "invalid append arg", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sess, ok := f.sessions[in.Cursor.SessionId]
	if !ok {
		apiError(w, "not_found/.", map[string]any{".tag": fakeTagNotFound})
		return
	}
	if in.Cursor.Offset != uint64(len(sess.data)) {
		apiError(w, "incorrect_offset/.", map[string]any{".tag": "incorrect_offset", "correct_offset": len(sess.data)})
		return
	}
	sess.data = append(sess.data, body...)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeDropbox) sessionFinish(w http.ResponseWriter, r *http.Request, arg []byte) {
	var in files.UploadSessionFinishArg
	if err := json.Unmarshal(arg, &in); err != nil || in.Cursor == nil || in.Commit == nil {
		http.Error(w, "invalid finish arg", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sess, ok := f.sessions[in.Cursor.SessionId]
	if !ok {
		apiError(w, "lookup_failed/not_found/.", map[string]any{
			".tag":          "lookup_failed",
			"lookup_failed": map[string]any{".tag": fakeTagNotFound},
		})
		return
	}
	if in.Cursor.Offset != uint64(len(sess.data)) {
		apiError(w, "lookup_failed/incorrect_offset/.", map[string]any{
			".tag":          "lookup_failed",
			"lookup_failed": map[string]any{".tag": "incorrect_offset", "correct_offset": len(sess.data)},
		})
		return
	}
	data := append(append([]byte(nil), sess.data...), body...)
	if !f.commit(w, true, in.Commit.Path, modeOf(in.Commit.Mode), clientModifiedOf(in.Commit.ClientModified), data) {
		return
	}
	delete(f.sessions, in.Cursor.SessionId)
	writeJSON(w, http.StatusOK, f.entries[normalize(in.Commit.Path)].metadata(f.idFor(normalize(in.Commit.Path))))
}

// seedFolder creates a folder (and its parents) as if it already existed in the account.
func (f *fakeDropbox) seedFolder(displayPath string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	displayPath = strings.TrimSuffix(displayPath, "/")
	f.ensureParents(displayPath)
	f.entries[normalize(displayPath)] = &fakeEntry{displayPath: displayPath, isFolder: true}
}

// clientModified returns the stored client_modified of a file, for asserting Touch behavior.
func (f *fakeDropbox) clientModified(displayPath string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[normalize(displayPath)]
	if !ok || e.isFolder {
		return time.Time{}, false
	}
	return e.clientModified, true
}
