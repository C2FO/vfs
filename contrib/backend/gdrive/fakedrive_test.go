package gdrive

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDrive is an in-process implementation of the subset of the Drive v3 API this backend uses. It
// mirrors Drive's data model where it matters for VFS semantics: files are addressed by ID, folders
// are files with a folder mime type, siblings may share a name, deleted files can be trashed, and
// uploads may be multipart or resumable.
type fakeDrive struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	files    map[string]*fakeFile
	sessions map[string]*fakeSession
	nextID   int
	requests []string
	faults   []*fakeFault
	pageSize int // when > 0, caps files.list pages to exercise pagination
	// failChunkAfterApply makes the next resumable chunk be stored and then answered with a 500.
	failChunkAfterApply bool
	// drives holds the IDs of shared drives; each acts as a root folder.
	drives map[string]bool
	// hook, when set, may take over a request before it is handled (without the lock held).
	hook func(w http.ResponseWriter, r *http.Request) bool
}

type fakeFile struct {
	id      string
	name    string
	mime    string
	parents []string
	data    []byte
	mod     time.Time
	trashed bool
}

type fakeSession struct {
	target   string // existing file ID, or "" to create
	name     string
	parent   string
	total    int64
	received []byte
}

type fakeFault struct {
	match  func(r *http.Request) bool
	status int
	reason string
	header map[string]string
	times  int
}

func newFakeDrive(t *testing.T) *fakeDrive {
	t.Helper()

	d := &fakeDrive{
		t:        t,
		files:    map[string]*fakeFile{},
		sessions: map[string]*fakeSession{},
		drives:   map[string]bool{},
	}
	d.srv = httptest.NewServer(http.HandlerFunc(d.serve))
	t.Cleanup(d.srv.Close)

	return d
}

func (d *fakeDrive) url() string { return d.srv.URL }

func (d *fakeDrive) newID() string {
	d.nextID++
	return "id" + strconv.Itoa(d.nextID)
}

func now() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// addFolder creates a top-level folder.
func (d *fakeDrive) addFolder(name string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.add("root", name, folderMimeType, nil)
}

// addSharedDrive registers a shared drive whose ID can be used as a parent.
func (d *fakeDrive) addSharedDrive(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.drives[id] = true
}

func (d *fakeDrive) setHook(h func(w http.ResponseWriter, r *http.Request) bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hook = h
}

// countNamed returns how many untrashed items are named name.
func (d *fakeDrive) countNamed(name string) int {
	d.mu.Lock()
	defer d.mu.Unlock()

	n := 0
	for _, f := range d.files {
		if !f.trashed && f.name == name {
			n++
		}
	}
	return n
}

// addFile creates a file under parentID, allowing duplicate names.
func (d *fakeDrive) addFile(parentID, name string, data []byte) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.add(parentID, name, "application/octet-stream", data)
}

// addMime creates a top-level item with the given mime type, e.g. a Google Doc.
func (d *fakeDrive) addMime(name, mimeType string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.add("root", name, mimeType, nil)
}

func (d *fakeDrive) add(parentID, name, mimeType string, data []byte) string {
	id := d.newID()
	d.files[id] = &fakeFile{id: id, name: name, mime: mimeType, parents: []string{parentID}, data: data, mod: now()}
	return id
}

// liveCount returns how many untrashed files exist, excluding folders.
func (d *fakeDrive) liveFiles() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	n := 0
	for _, f := range d.files {
		if !f.trashed && f.mime != folderMimeType {
			n++
		}
	}
	return n
}

func (d *fakeDrive) fault(f *fakeFault) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.faults = append(d.faults, f)
}

func (d *fakeDrive) requestLog() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.requests...)
}

func (d *fakeDrive) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	hook := d.hook
	d.mu.Unlock()
	if hook != nil && hook(w, r) {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	entry := r.Method + " " + r.URL.Path
	if cr := r.Header.Get("Content-Range"); cr != "" {
		entry += " " + cr
	}
	d.requests = append(d.requests, entry)

	if r.Header.Get("Authorization") == "" {
		writeErr(w, http.StatusUnauthorized, "authError", "missing credentials")
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/upload/session/") && r.URL.Query().Get("supportsAllDrives") != "true" {
		writeErr(w, http.StatusBadRequest, "badRequest", "supportsAllDrives=true is required")
		return
	}

	if d.injectFault(w, r) {
		return
	}

	switch {
	case strings.HasPrefix(r.URL.Path, "/upload/session/"):
		d.handleSession(w, r)
	case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files"):
		d.handleUpload(w, r)
	case strings.HasPrefix(r.URL.Path, "/drive/v3/files"):
		d.handleFiles(w, r)
	default:
		writeErr(w, http.StatusNotFound, "notFound", "no such route")
	}
}

// injectFault answers the request with the first matching registered fault, if any.
func (d *fakeDrive) injectFault(w http.ResponseWriter, r *http.Request) bool {
	for i, f := range d.faults {
		if !f.match(r) {
			continue
		}

		f.times--
		if f.times <= 0 {
			d.faults = append(d.faults[:i], d.faults[i+1:]...)
		}
		for k, v := range f.header {
			w.Header().Set(k, v)
		}
		writeErr(w, f.status, f.reason, "injected fault")
		return true
	}
	return false
}

func writeErr(w http.ResponseWriter, status int, reason, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code": status, "message": msg, "errors": []map[string]string{{"reason": reason}},
	}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeFile) json() map[string]any {
	m := map[string]any{"id": f.id, "name": f.name, "mimeType": f.mime, "modifiedTime": f.mod.Format(time.RFC3339Nano)}
	if f.mime != folderMimeType && !strings.HasPrefix(f.mime, "application/vnd.google-apps.") {
		m["size"] = strconv.Itoa(len(f.data))
	}
	return m
}

var listQueryRE = regexp.MustCompile(`^'((?:\\.|[^'\\])*)' in parents(?: and name = '((?:\\.|[^'\\])*)')? and trashed = false$`)

func unescapeQuery(s string) string {
	return strings.NewReplacer(`\'`, `'`, `\\`, `\`).Replace(s)
}

func (d *fakeDrive) handleFiles(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/drive/v3/files")
	rest = strings.TrimPrefix(rest, "/")

	switch {
	case rest == "" && r.Method == http.MethodGet:
		d.list(w, r)
	case rest == "" && r.Method == http.MethodPost:
		d.create(w, r)
	case strings.HasSuffix(rest, "/copy") && r.Method == http.MethodPost:
		d.copyFile(w, r, strings.TrimSuffix(rest, "/copy"))
	case strings.HasSuffix(rest, "/export") && r.Method == http.MethodGet:
		d.export(w, r, strings.TrimSuffix(rest, "/export"))
	case r.Method == http.MethodGet:
		d.get(w, r, rest)
	case r.Method == http.MethodPatch:
		d.patch(w, r, rest)
	case r.Method == http.MethodDelete:
		d.remove(w, rest)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "badRequest", "unsupported")
	}
}

// checkCorpus enforces the query parameters Drive requires to list within a shared drive.
func (d *fakeDrive) checkCorpus(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()
	driveID := q.Get("driveId")
	switch {
	case driveID == "":
		return true
	case q.Get("corpora") != "drive" || q.Get("includeItemsFromAllDrives") != "true":
		writeErr(w, http.StatusBadRequest, "badRequest", "driveId requires corpora=drive and includeItemsFromAllDrives")
		return false
	case !d.drives[driveID]:
		writeErr(w, http.StatusNotFound, "notFound", "Shared drive not found: "+driveID)
		return false
	}
	return true
}

func (d *fakeDrive) list(w http.ResponseWriter, r *http.Request) {
	if !d.checkCorpus(w, r) {
		return
	}

	m := listQueryRE.FindStringSubmatch(r.URL.Query().Get("q"))
	if m == nil {
		writeErr(w, http.StatusBadRequest, "invalidQuery", "unsupported query: "+r.URL.Query().Get("q"))
		return
	}
	parent, name := unescapeQuery(m[1]), unescapeQuery(m[2])
	byName := strings.Contains(r.URL.Query().Get("q"), " and name = ")

	var matches []map[string]any
	for _, f := range d.sortedFiles() {
		if f.trashed || !contains(f.parents, parent) || (byName && f.name != name) {
			continue
		}
		matches = append(matches, f.json())
	}

	start, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if d.pageSize > 0 && (size == 0 || size > d.pageSize) {
		size = d.pageSize
	}
	if size == 0 {
		size = len(matches)
	}

	end := min(start+size, len(matches))
	resp := map[string]any{"files": matches[start:end]}
	if end < len(matches) {
		resp["nextPageToken"] = strconv.Itoa(end)
	}
	writeJSON(w, resp)
}

func (d *fakeDrive) sortedFiles() []*fakeFile {
	out := make([]*fakeFile, 0, len(d.files))
	for i := 1; i <= d.nextID; i++ {
		if f, ok := d.files["id"+strconv.Itoa(i)]; ok {
			out = append(out, f)
		}
	}
	return out
}

func contains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

func (d *fakeDrive) lookupFile(w http.ResponseWriter, id string) *fakeFile {
	f, ok := d.files[id]
	if !ok {
		writeErr(w, http.StatusNotFound, "notFound", "File not found: "+id)
		return nil
	}
	return f
}

func (d *fakeDrive) get(w http.ResponseWriter, r *http.Request, id string) {
	f := d.lookupFile(w, id)
	if f == nil {
		return
	}

	if r.URL.Query().Get("alt") == "media" {
		if strings.HasPrefix(f.mime, "application/vnd.google-apps.") {
			writeErr(w, http.StatusForbidden, "fileNotDownloadable", "Only files with binary content can be downloaded")
			return
		}
		_, _ = w.Write(f.data)
		return
	}
	if strings.Contains(r.URL.Query().Get("fields"), "trashed") {
		m := f.json()
		m["trashed"] = f.trashed
		writeJSON(w, m)
		return
	}
	writeJSON(w, f.json())
}

func (d *fakeDrive) export(w http.ResponseWriter, r *http.Request, id string) {
	f := d.lookupFile(w, id)
	if f == nil {
		return
	}
	w.Header().Set("Content-Type", r.URL.Query().Get("mimeType"))
	_, _ = fmt.Fprintf(w, "exported:%s:%s", f.name, r.URL.Query().Get("mimeType"))
}

func (d *fakeDrive) parentOK(w http.ResponseWriter, parents []string) bool {
	for _, p := range parents {
		if p == "root" || d.drives[p] {
			continue
		}
		if f, ok := d.files[p]; !ok || f.mime != folderMimeType {
			writeErr(w, http.StatusNotFound, "notFound", "Parent not found: "+p)
			return false
		}
	}
	return true
}

func (d *fakeDrive) create(w http.ResponseWriter, r *http.Request) {
	var meta struct {
		Name     string   `json:"name"`
		MimeType string   `json:"mimeType"`
		Parents  []string `json:"parents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil || !d.parentOK(w, meta.Parents) {
		if err != nil {
			writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
		}
		return
	}

	id := d.add(meta.Parents[0], meta.Name, meta.MimeType, nil)
	writeJSON(w, d.files[id].json())
}

func (d *fakeDrive) copyFile(w http.ResponseWriter, r *http.Request, id string) {
	src := d.lookupFile(w, id)
	if src == nil {
		return
	}

	var meta struct {
		Name    string   `json:"name"`
		Parents []string `json:"parents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil || !d.parentOK(w, meta.Parents) {
		if err != nil {
			writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
		}
		return
	}

	nid := d.add(meta.Parents[0], meta.Name, src.mime, append([]byte(nil), src.data...))
	writeJSON(w, d.files[nid].json())
}

func (d *fakeDrive) patch(w http.ResponseWriter, r *http.Request, id string) {
	f := d.lookupFile(w, id)
	if f == nil {
		return
	}

	var meta struct {
		Name         *string `json:"name"`
		Trashed      *bool   `json:"trashed"`
		ModifiedTime *string `json:"modifiedTime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
		return
	}

	q := r.URL.Query()
	if add := q.Get("addParents"); add != "" {
		if !d.parentOK(w, []string{add}) {
			return
		}
		f.parents = append(f.parents, add)
	}
	if rm := q.Get("removeParents"); rm != "" {
		kept := f.parents[:0]
		for _, p := range f.parents {
			if p != rm {
				kept = append(kept, p)
			}
		}
		f.parents = kept
	}
	if meta.Name != nil {
		f.name = *meta.Name
	}
	if meta.Trashed != nil {
		f.trashed = *meta.Trashed
	}
	if meta.ModifiedTime != nil {
		t, err := time.Parse(time.RFC3339Nano, *meta.ModifiedTime)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
			return
		}
		f.mod = t.UTC().Truncate(time.Millisecond)
	}

	writeJSON(w, f.json())
}

func (d *fakeDrive) remove(w http.ResponseWriter, id string) {
	if d.lookupFile(w, id) == nil {
		return
	}
	// Deleting a folder deletes its subtree, as Drive does.
	d.deleteTree(id)
	w.WriteHeader(http.StatusNoContent)
}

func (d *fakeDrive) deleteTree(id string) {
	delete(d.files, id)
	for cid, f := range d.files {
		if contains(f.parents, id) {
			d.deleteTree(cid)
		}
	}
}

func (d *fakeDrive) handleUpload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/upload/drive/v3/files"), "/")
	if (id == "") != (r.Method == http.MethodPost) {
		writeErr(w, http.StatusMethodNotAllowed, "badRequest", "unsupported")
		return
	}
	if id != "" && d.lookupFile(w, id) == nil {
		return
	}

	switch r.URL.Query().Get("uploadType") {
	case "multipart":
		d.multipart(w, r, id)
	case "resumable":
		d.startSession(w, r, id)
	default:
		writeErr(w, http.StatusBadRequest, "badRequest", "unsupported uploadType")
	}
}

func (d *fakeDrive) multipart(w http.ResponseWriter, r *http.Request, id string) {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
		return
	}

	mr := multipart.NewReader(r.Body, params["boundary"])
	metaPart, err := mr.NextPart()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
		return
	}
	var meta struct {
		Name    string   `json:"name"`
		Parents []string `json:"parents"`
	}
	if err := json.NewDecoder(metaPart).Decode(&meta); err != nil {
		writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
		return
	}
	dataPart, err := mr.NextPart()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
		return
	}
	data, _ := io.ReadAll(dataPart)

	d.store(w, id, meta.Name, meta.Parents, data)
}

// store creates or replaces a file's content and responds with its metadata.
func (d *fakeDrive) store(w http.ResponseWriter, id, name string, parents []string, data []byte) {
	if id != "" {
		f := d.files[id]
		f.data, f.mod = append([]byte(nil), data...), now()
		writeJSON(w, f.json())
		return
	}

	if !d.parentOK(w, parents) {
		return
	}
	nid := d.add(parents[0], name, "application/octet-stream", append([]byte(nil), data...))
	writeJSON(w, d.files[nid].json())
}

func (d *fakeDrive) startSession(w http.ResponseWriter, r *http.Request, id string) {
	var meta struct {
		Name    string   `json:"name"`
		Parents []string `json:"parents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		writeErr(w, http.StatusBadRequest, "badRequest", err.Error())
		return
	}
	total, _ := strconv.ParseInt(r.Header.Get("X-Upload-Content-Length"), 10, 64)

	s := &fakeSession{target: id, name: meta.Name, total: total}
	if len(meta.Parents) > 0 {
		s.parent = meta.Parents[0]
	}
	sid := "session" + strconv.Itoa(len(d.sessions)+1)
	d.sessions[sid] = s

	w.Header().Set("Location", d.srv.URL+"/upload/session/"+sid+"?upload_id=secret-"+sid)
	w.WriteHeader(http.StatusOK)
}

var contentRangeRE = regexp.MustCompile(`^bytes (?:(\d+)-(\d+)|\*)/(\d+)$`)

func (d *fakeDrive) handleSession(w http.ResponseWriter, r *http.Request) {
	sid := strings.TrimPrefix(r.URL.Path, "/upload/session/")
	s, ok := d.sessions[sid]
	if !ok {
		writeErr(w, http.StatusNotFound, "notFound", "unknown session")
		return
	}

	if r.Method == http.MethodDelete {
		delete(d.sessions, sid)
		w.WriteHeader(statusClientClosedRequest)
		return
	}

	m := contentRangeRE.FindStringSubmatch(r.Header.Get("Content-Range"))
	if m == nil {
		writeErr(w, http.StatusBadRequest, "badRequest", "bad Content-Range")
		return
	}

	if m[1] != "" {
		first, _ := strconv.ParseInt(m[1], 10, 64)
		if first == int64(len(s.received)) {
			data, _ := io.ReadAll(r.Body)
			s.received = append(s.received, data...)

			if d.failChunkAfterApply {
				d.failChunkAfterApply = false
				writeErr(w, http.StatusInternalServerError, "backendError", "applied but failed")
				return
			}
		}
	}

	if int64(len(s.received)) >= s.total {
		delete(d.sessions, sid)
		d.store(w, s.target, s.name, []string{s.parent}, s.received)
		return
	}

	if len(s.received) > 0 {
		w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", len(s.received)-1))
	}
	w.WriteHeader(http.StatusPermanentRedirect)
}
