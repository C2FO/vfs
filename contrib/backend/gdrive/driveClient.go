package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the Google API host used unless overridden with WithBaseURL.
	DefaultBaseURL = "https://www.googleapis.com"

	apiPath    = "/drive/v3"
	uploadPath = "/upload/drive/v3"

	itemFields = "id,name,mimeType,size,modifiedTime"
	listFields = "nextPageToken,files(" + itemFields + ")"

	folderCacheTTL = time.Minute

	// statusClientClosedRequest is what Drive answers to a canceled resumable upload session.
	statusClientClosedRequest = 499
)

// driveClient implements Client against the Google Drive v3 REST API for a single drive.
type driveClient struct {
	baseURL    string
	httpClient *http.Client
	retryCount int
	timeout    time.Duration
	logger     RequestLogger

	driveID    string // "" for My Drive
	rootID     string // "root" for My Drive, the drive ID for a shared drive
	trash      bool
	exportMime string

	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time

	mu      sync.Mutex
	folders map[string]folderEntry

	// createMu serializes folder creation so concurrent writers into a new folder share one folder
	// instead of each creating a same-named duplicate.
	createMu sync.Mutex
}

type folderEntry struct {
	id      string
	expires time.Time
}

func newDriveClient(httpClient *http.Client, opts *Options, driveID string) *driveClient {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	rootID := "root"
	if driveID != "" {
		rootID = driveID
	}

	return &driveClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: httpClient,
		retryCount: opts.RetryCount,
		timeout:    opts.RequestTimeout,
		logger:     opts.RequestLogger,
		driveID:    driveID,
		rootID:     rootID,
		trash:      opts.TrashOnDelete,
		exportMime: opts.ExportMimeType,
		sleep:      sleepContext,
		now:        time.Now,
		folders:    map[string]folderEntry{},
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type fileJSON struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	MimeType     string    `json:"mimeType"`
	Size         string    `json:"size"`
	ModifiedTime time.Time `json:"modifiedTime"`
	Trashed      bool      `json:"trashed"`
}

func (f *fileJSON) toItem() Item {
	size, _ := strconv.ParseInt(f.Size, 10, 64)
	return Item{
		ID:       f.ID,
		Name:     f.Name,
		MimeType: f.MimeType,
		Size:     size,
		ModTime:  f.ModifiedTime,
		IsFolder: f.MimeType == folderMimeType,
	}
}

type listPage struct {
	NextPageToken string     `json:"nextPageToken"`
	Files         []fileJSON `json:"files"`
}

func (c *driveClient) apiURL(p string, q url.Values) string {
	return c.withQuery(c.baseURL+apiPath+p, q)
}

func (c *driveClient) uploadURL(p string, q url.Values) string {
	return c.withQuery(c.baseURL+uploadPath+p, q)
}

func (c *driveClient) withQuery(u string, q url.Values) string {
	if q == nil {
		q = url.Values{}
	}
	q.Set("supportsAllDrives", "true")
	return u + "?" + q.Encode()
}

func (c *driveClient) fileURL(id string, q url.Values) string {
	return c.apiURL("/files/"+url.PathEscape(id), q)
}

// listQuery returns the query parameters every files.list call needs for this drive.
func (c *driveClient) listQuery(q string) url.Values {
	v := url.Values{
		"q":                         {q},
		"fields":                    {listFields},
		"pageSize":                  {"1000"},
		"includeItemsFromAllDrives": {"true"},
	}
	if c.driveID != "" {
		v.Set("corpora", "drive")
		v.Set("driveId", c.driveID)
	}
	return v
}

func escapeQuery(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}

// splitPath cleans a drive-relative path and splits it into its parent folder path and base name.
func splitPath(p string) (parent, base string) {
	p = strings.Trim(path.Clean("/"+p), "/")
	if p == "" {
		return "", ""
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i], p[i+1:]
	}
	return "", p
}

func folderSegments(p string) []string {
	p = strings.Trim(path.Clean("/"+p), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// lookup returns every live child of parentID named name.
func (c *driveClient) lookup(ctx context.Context, parentID, name string) ([]Item, error) {
	q := fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", escapeQuery(parentID), escapeQuery(name))
	return c.list(ctx, q)
}

func (c *driveClient) list(ctx context.Context, q string) ([]Item, error) {
	var items []Item
	query := c.listQuery(q)

	for {
		var page listPage
		if err := c.call(ctx, request{method: http.MethodGet, url: c.apiURL("/files", query)}, &page); err != nil {
			return nil, err
		}

		for i := range page.Files {
			items = append(items, page.Files[i].toItem())
		}

		if page.NextPageToken == "" {
			return items, nil
		}
		query.Set("pageToken", page.NextPageToken)
	}
}

func (c *driveClient) cachedFolder(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.folders[key]
	if !ok || c.now().After(e.expires) {
		delete(c.folders, key)
		return "", false
	}
	return e.id, true
}

func (c *driveClient) cacheFolder(key, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.folders[key] = folderEntry{id: id, expires: c.now().Add(folderCacheTTL)}
}

func (c *driveClient) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.folders)
}

// cachedPrefix returns the deepest cached folder ID along folderPath.
func (c *driveClient) cachedPrefix(folderPath string) (string, bool) {
	segs := folderSegments(folderPath)
	for i := len(segs); i > 0; i-- {
		if id, ok := c.cachedFolder(strings.Join(segs[:i], "/")); ok {
			return id, true
		}
	}
	return "", false
}

// folderChild returns the folder named name. The name must match exactly one item, and that item
// must be a folder: a folder that shares its name with a file is ErrAmbiguous, because a path cannot
// say which one it means.
func folderChild(items []Item, name string) (*Item, error) {
	switch len(items) {
	case 0:
		return nil, fmt.Errorf("%w: folder %q", ErrNotFound, name)
	case 1:
		if !items[0].IsFolder {
			return nil, fmt.Errorf("%w: %q", ErrNotFolder, name)
		}
		return &items[0], nil
	default:
		return nil, fmt.Errorf("%w: folder %q", ErrAmbiguous, name)
	}
}

// resolveFolder returns the ID of the folder at folderPath, and whether any part of it came from the
// cache (and so may be stale). With create, missing folders are made.
func (c *driveClient) resolveFolder(ctx context.Context, folderPath string, create bool) (string, bool, error) {
	segs := folderSegments(folderPath)

	id, start := c.rootID, 0
	for i := len(segs); i > 0; i-- {
		if cached, ok := c.cachedFolder(strings.Join(segs[:i], "/")); ok {
			id, start = cached, i
			break
		}
	}
	fromCache := start > 0

	for i := start; i < len(segs); i++ {
		key := strings.Join(segs[:i+1], "/")

		items, err := c.lookup(ctx, id, segs[i])
		if err != nil {
			return "", fromCache, err
		}

		folder, ferr := folderChild(items, segs[i])
		switch {
		case ferr == nil:
			id = folder.ID
		case create && errors.Is(ferr, ErrNotFound):
			id, err = c.createFolderOnce(ctx, id, segs[i], key)
			if err != nil {
				return "", fromCache, err
			}
		default:
			return "", fromCache, fmt.Errorf("gdrive: resolving %q: %w", key, ferr)
		}

		c.cacheFolder(key, id)
	}

	return id, fromCache, nil
}

// createFolderOnce creates the folder name under parentID unless a concurrent caller already did.
func (c *driveClient) createFolderOnce(ctx context.Context, parentID, name, key string) (string, error) {
	c.createMu.Lock()
	defer c.createMu.Unlock()

	if id, ok := c.cachedFolder(key); ok {
		return id, nil
	}

	items, err := c.lookup(ctx, parentID, name)
	if err != nil {
		return "", err
	}

	folder, ferr := folderChild(items, name)
	switch {
	case ferr == nil:
		return folder.ID, nil
	case !errors.Is(ferr, ErrNotFound):
		return "", fmt.Errorf("gdrive: resolving %q: %w", key, ferr)
	}

	req, err := jsonRequest(http.MethodPost, c.apiURL("/files", url.Values{"fields": {"id"}}), map[string]any{
		"name":     name,
		"mimeType": folderMimeType,
		"parents":  []string{parentID},
	})
	if err != nil {
		return "", err
	}

	var created fileJSON
	if err := c.call(ctx, req, &created); err != nil {
		return "", fmt.Errorf("gdrive: creating folder %q: %w", key, err)
	}
	return created.ID, nil
}

// folderGone reports whether the folder id has been deleted or trashed.
func (c *driveClient) folderGone(ctx context.Context, id string) (bool, error) {
	var f fileJSON
	err := c.call(ctx, request{method: http.MethodGet, url: c.fileURL(id, url.Values{"fields": {"id,trashed"}})}, &f)
	switch {
	case errors.Is(err, ErrNotFound):
		return true, nil
	case err != nil:
		return false, err
	}
	return f.Trashed, nil
}

// withFolder resolves folderPath and runs fn with its ID. When fn reports ErrNotFound against a
// folder ID that came from the cache, the folder is checked; only if it is really gone is the cache
// dropped and fn run once more. fn must therefore be safe to repeat after it failed with ErrNotFound.
func (c *driveClient) withFolder(ctx context.Context, folderPath string, create bool, fn func(folderID string) error) error {
	id, fromCache, err := c.resolveFolder(ctx, folderPath, create)
	if err != nil {
		stale, serr := c.cachedFolderStale(ctx, folderPath, fromCache, err)
		if serr != nil {
			return serr
		}
		if !stale {
			return err
		}
	} else {
		err = fn(id)
		if !fromCache || !errors.Is(err, ErrNotFound) {
			return err
		}

		gone, gerr := c.folderGone(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !gone {
			return err
		}
	}

	c.invalidate()
	id, _, err = c.resolveFolder(ctx, folderPath, create)
	if err != nil {
		return err
	}
	return fn(id)
}

// cachedFolderStale reports whether err is a not-found from a path that used a cached folder ID
// which has since been deleted. A cached folder that still exists is left in the cache.
func (c *driveClient) cachedFolderStale(ctx context.Context, folderPath string, fromCache bool, err error) (bool, error) {
	if !fromCache || !errors.Is(err, ErrNotFound) {
		return false, nil
	}
	cachedID, ok := c.cachedPrefix(folderPath)
	if !ok {
		return false, nil
	}
	return c.folderGone(ctx, cachedID)
}

// rootItem is the synthetic item for the drive root.
func (c *driveClient) rootItem() *Item {
	return &Item{ID: c.rootID, MimeType: folderMimeType, IsFolder: true}
}

// child returns the single item named base in parentID.
func (c *driveClient) child(ctx context.Context, parentID, base, itemPath string) (*Item, error) {
	found, err := c.lookup(ctx, parentID, base)
	if err != nil {
		return nil, err
	}

	switch len(found) {
	case 0:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, itemPath)
	case 1:
		return &found[0], nil
	}
	return nil, fmt.Errorf("%w: %s (%d items)", ErrAmbiguous, itemPath, len(found))
}

// GetItem implements Client.
func (c *driveClient) GetItem(ctx context.Context, itemPath string) (*Item, error) {
	parent, base := splitPath(itemPath)
	if base == "" {
		return c.rootItem(), nil
	}

	var item *Item
	err := c.withFolder(ctx, parent, false, func(parentID string) error {
		var err error
		item, err = c.child(ctx, parentID, base, itemPath)
		return err
	})
	if err != nil {
		return nil, err
	}

	return item, nil
}

// fileIn returns the file named base in parentID, failing with ErrIsFolder if it is a folder.
func (c *driveClient) fileIn(ctx context.Context, parentID, base, itemPath string) (*Item, error) {
	item, err := c.child(ctx, parentID, base, itemPath)
	if err != nil {
		return nil, err
	}
	if item.IsFolder {
		return nil, fmt.Errorf("%w: %s", ErrIsFolder, itemPath)
	}
	return item, nil
}

// requireFile returns the file at itemPath and the ID of its parent folder.
func (c *driveClient) requireFile(ctx context.Context, itemPath string) (item *Item, parentID string, err error) {
	parent, base := splitPath(itemPath)
	if base == "" {
		return nil, "", fmt.Errorf("%w: %s", ErrIsFolder, itemPath)
	}

	err = c.withFolder(ctx, parent, false, func(id string) error {
		parentID = id
		item, err = c.fileIn(ctx, id, base, itemPath)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return item, parentID, nil
}

// ListChildren implements Client.
func (c *driveClient) ListChildren(ctx context.Context, folderPath string) ([]Item, error) {
	var items []Item
	err := c.withFolder(ctx, folderPath, false, func(id string) error {
		var err error
		items, err = c.list(ctx, fmt.Sprintf("'%s' in parents and trashed = false", escapeQuery(id)))
		return err
	})
	if err != nil {
		return nil, err
	}

	return items, nil
}

// Download implements Client.
func (c *driveClient) Download(ctx context.Context, itemPath string) (io.ReadCloser, error) {
	item, _, err := c.requireFile(ctx, itemPath)
	if err != nil {
		return nil, err
	}

	var u string
	switch {
	case !item.IsNative():
		u = c.fileURL(item.ID, url.Values{"alt": {"media"}})
	case c.exportMime != "":
		u = c.apiURL("/files/"+url.PathEscape(item.ID)+"/export", url.Values{"mimeType": {c.exportMime}})
	default:
		return nil, fmt.Errorf("%w: %s (%s)", ErrNotDownloadable, itemPath, item.MimeType)
	}

	resp, err := c.do(ctx, request{method: http.MethodGet, url: u, stream: true})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// target is where bytes are written: an existing file to replace, or a name in a folder to create.
type target struct {
	existingID string
	parentID   string
	name       string
}

// writableTarget looks up base in parentID as a destination for new bytes. Folders and Google-native
// files are refused: neither can hold the content.
func (c *driveClient) writableTarget(ctx context.Context, parentID, base, itemPath string) (*target, *Item, error) {
	found, err := c.lookup(ctx, parentID, base)
	if err != nil {
		return nil, nil, err
	}

	t := &target{parentID: parentID, name: base}
	switch len(found) {
	case 0:
		return t, nil, nil
	case 1:
	default:
		return nil, nil, fmt.Errorf("%w: %s (%d items)", ErrAmbiguous, itemPath, len(found))
	}

	existing := &found[0]
	switch {
	case existing.IsFolder:
		return nil, nil, fmt.Errorf("%w: %s", ErrIsFolder, itemPath)
	case existing.IsNative():
		return nil, nil, fmt.Errorf("%w: %s is a Google-native file (%s) and cannot be overwritten",
			ErrNotDownloadable, itemPath, existing.MimeType)
	}

	t.existingID = existing.ID
	return t, existing, nil
}

func (t *target) metadata() map[string]any {
	if t.existingID != "" {
		return map[string]any{}
	}
	return map[string]any{"name": t.name, "parents": []string{t.parentID}}
}

func (c *driveClient) uploadEndpoint(t *target, uploadType string) (method, u string) {
	q := url.Values{"uploadType": {uploadType}, "fields": {itemFields}}
	if t.existingID != "" {
		return http.MethodPatch, c.uploadURL("/files/"+url.PathEscape(t.existingID), q)
	}
	return http.MethodPost, c.uploadURL("/files", q)
}

// withUploadTarget resolves (creating folders as needed) where itemPath's bytes go and runs fn.
func (c *driveClient) withUploadTarget(ctx context.Context, itemPath string, fn func(t *target) error) error {
	parent, base := splitPath(itemPath)
	if base == "" {
		return fmt.Errorf("gdrive: invalid file path %q", itemPath)
	}

	return c.withFolder(ctx, parent, true, func(parentID string) error {
		t, _, err := c.writableTarget(ctx, parentID, base, itemPath)
		if err != nil {
			return err
		}
		return fn(t)
	})
}

// Upload implements Client.
func (c *driveClient) Upload(ctx context.Context, itemPath string, content io.ReadSeeker) (*Item, error) {
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("gdrive: rewinding upload content: %w", err)
	}

	data, err := io.ReadAll(content)
	if err != nil {
		return nil, fmt.Errorf("gdrive: reading upload content: %w", err)
	}

	var item *Item
	err = c.withUploadTarget(ctx, itemPath, func(t *target) error {
		var uerr error
		item, uerr = c.uploadBytes(ctx, t, data)
		return uerr
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (c *driveClient) uploadBytes(ctx context.Context, t *target, data []byte) (*Item, error) {
	meta, err := json.Marshal(t.metadata())
	if err != nil {
		return nil, fmt.Errorf("gdrive: encoding metadata: %w", err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := writePart(mw, "application/json; charset=UTF-8", meta); err != nil {
		return nil, err
	}
	if err := writePart(mw, "application/octet-stream", data); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("gdrive: building multipart body: %w", err)
	}

	method, u := c.uploadEndpoint(t, "multipart")
	var out fileJSON
	err = c.call(ctx, request{
		method: method,
		url:    u,
		body:   bytes.NewReader(buf.Bytes()),
		size:   int64(buf.Len()),
		header: http.Header{"Content-Type": {"multipart/related; boundary=" + mw.Boundary()}},
	}, &out)
	if err != nil {
		return nil, err
	}

	item := out.toItem()
	return &item, nil
}

func writePart(mw *multipart.Writer, contentType string, data []byte) error {
	w, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {contentType}})
	if err != nil {
		return fmt.Errorf("gdrive: building multipart body: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("gdrive: building multipart body: %w", err)
	}
	return nil
}

// CreateUploadSession implements Client.
func (c *driveClient) CreateUploadSession(ctx context.Context, itemPath string, totalSize int64) (UploadSession, error) {
	var session UploadSession
	err := c.withUploadTarget(ctx, itemPath, func(t *target) error {
		var serr error
		session, serr = c.createSession(ctx, t, totalSize)
		return serr
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (c *driveClient) createSession(ctx context.Context, t *target, totalSize int64) (UploadSession, error) {
	method, u := c.uploadEndpoint(t, "resumable")
	req, err := jsonRequest(method, u, t.metadata())
	if err != nil {
		return nil, err
	}
	req.header.Set("X-Upload-Content-Length", strconv.FormatInt(totalSize, 10))

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	sessionURL := resp.Header.Get("Location")
	if sessionURL == "" {
		return nil, errors.New("gdrive: resumable upload response had no session URL")
	}

	return &uploadSession{client: c, url: sessionURL, total: totalSize}, nil
}

// uploadSession implements UploadSession for a Drive resumable upload.
type uploadSession struct {
	client *driveClient
	url    string
	total  int64
}

// UploadChunk implements UploadSession. Chunk PUTs are never blindly resent: after a failure the
// session is asked how much it already has and only the remainder is sent, up to the retry count.
func (s *uploadSession) UploadChunk(ctx context.Context, data []byte, offset int64) (*Item, error) {
	base := offset
	end := offset + int64(len(data))
	failures := 0

	for offset < end {
		item, next, err := s.put(ctx, data[offset-base:], offset)
		if err != nil {
			if failures >= s.client.retryCount || !recoverable(ctx, err) {
				return nil, err
			}
			failures++

			if serr := s.client.sleep(ctx, backoff(failures-1)); serr != nil {
				return nil, serr
			}

			var done *Item
			next, done, err = s.status(ctx)
			if err != nil {
				return nil, err
			}
			if done != nil {
				return done, nil
			}
		} else if item != nil {
			return item, nil
		} else if next == offset {
			// A 308 that reports no new bytes, including one with no Range header, must not spin.
			// A status query after a failed PUT can legitimately report the same offset.
			if failures >= s.client.retryCount {
				return nil, fmt.Errorf("gdrive: upload session made no progress at offset %d", offset)
			}
			failures++
			if serr := s.client.sleep(ctx, backoff(failures-1)); serr != nil {
				return nil, serr
			}
			continue
		}

		if next < offset || next > end {
			return nil, fmt.Errorf("gdrive: upload session reports %d bytes received, outside chunk [%d,%d)", next, offset, end)
		}
		offset = next
	}

	if end >= s.total {
		return nil, fmt.Errorf("gdrive: upload session accepted %d bytes but did not finish the file", s.total)
	}
	return nil, nil
}

// recoverable reports whether a failed chunk is worth resuming: the server or the connection failed
// transiently, so part of it may have been applied.
func recoverable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}

	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return true
	}

	switch apiErr.StatusCode {
	case http.StatusTooManyRequests, http.StatusRequestTimeout:
		return true
	case http.StatusForbidden:
		return isRateLimitReason(apiErr.Reason)
	}
	return apiErr.StatusCode >= 500
}

// put sends data at offset, once. It returns the item when the upload completes, otherwise the next
// offset Drive expects.
func (s *uploadSession) put(ctx context.Context, data []byte, offset int64) (*Item, int64, error) {
	last := offset + int64(len(data)) - 1
	resp, err := s.client.do(ctx, request{ //nolint:bodyclose // closed by parse
		method: http.MethodPut,
		url:    s.url,
		body:   bytes.NewReader(data),
		size:   int64(len(data)),
		header: http.Header{
			"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", offset, last, s.total)},
			"Content-Type":  {"application/octet-stream"},
		},
		accept308: true,
		noRetry:   true,
	})
	if err != nil {
		return nil, 0, err
	}
	return s.parse(resp)
}

// status asks the session how much of the file it has received.
func (s *uploadSession) status(ctx context.Context) (int64, *Item, error) {
	resp, err := s.client.do(ctx, request{ //nolint:bodyclose // closed by parse
		method:    http.MethodPut,
		url:       s.url,
		header:    http.Header{"Content-Range": {fmt.Sprintf("bytes */%d", s.total)}},
		accept308: true,
	})
	if err != nil {
		return 0, nil, err
	}

	item, next, err := s.parse(resp)
	return next, item, err
}

func (s *uploadSession) parse(resp *http.Response) (*Item, int64, error) {
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusPermanentRedirect {
		return nil, parseReceived(resp.Header.Get("Range")), nil
	}

	var out fileJSON
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("gdrive: decoding upload response: %w", err)
	}

	item := out.toItem()
	return &item, s.total, nil
}

// parseReceived converts a "bytes=0-N" Range header into the next expected offset.
func parseReceived(rangeHeader string) int64 {
	_, last, ok := strings.Cut(strings.TrimPrefix(rangeHeader, "bytes="), "-")
	if !ok {
		return 0
	}
	n, err := strconv.ParseInt(last, 10, 64)
	if err != nil {
		return 0
	}
	return n + 1
}

// Cancel implements UploadSession.
func (s *uploadSession) Cancel(ctx context.Context) error {
	resp, err := s.client.send(ctx, request{method: http.MethodDelete, url: s.url}, 0)
	if err != nil {
		return fmt.Errorf("gdrive: canceling upload session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 == 2 || resp.StatusCode == statusClientClosedRequest {
		return nil
	}
	return fmt.Errorf("gdrive: canceling upload session: status %d", resp.StatusCode)
}

// Touch implements Client.
func (c *driveClient) Touch(ctx context.Context, itemPath string) error {
	item, _, err := c.requireFile(ctx, itemPath)
	if err != nil {
		return err
	}

	req, err := jsonRequest(http.MethodPatch, c.fileURL(item.ID, url.Values{"fields": {"id"}}),
		map[string]any{"modifiedTime": c.now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}

	return c.call(ctx, req, nil)
}

// transfer runs a copy or move of src into destParentPath/destName. mutate performs the single Drive
// write; it is repeated only when it failed with ErrNotFound because the cached destination folder
// was gone, in which case nothing was written. A file the transfer replaces is removed afterwards,
// so a failure never loses the old destination.
func (c *driveClient) transfer(ctx context.Context, src *Item, destParentPath, destName string,
	mutate func(parentID string) error,
) error {
	destPath := path.Join(destParentPath, destName)

	var replaced *Item
	err := c.withFolder(ctx, destParentPath, false, func(parentID string) error {
		_, existing, err := c.writableTarget(ctx, parentID, destName, destPath)
		if err != nil {
			return err
		}
		if existing != nil && existing.ID == src.ID {
			return nil
		}
		replaced = existing
		return mutate(parentID)
	})
	if err != nil || replaced == nil {
		return err
	}

	if err := c.remove(ctx, replaced.ID); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("gdrive: %s was written but the file it replaces (id %s) could not be removed, "+
			"leaving two files with that name: %w", destPath, replaced.ID, err)
	}
	return nil
}

// Copy implements Client.
func (c *driveClient) Copy(ctx context.Context, sourcePath, destParentPath, destName string) error {
	src, _, err := c.requireFile(ctx, sourcePath)
	if err != nil {
		return err
	}

	return c.transfer(ctx, src, destParentPath, destName, func(parentID string) error {
		req, err := jsonRequest(http.MethodPost,
			c.apiURL("/files/"+url.PathEscape(src.ID)+"/copy", url.Values{"fields": {"id"}}),
			map[string]any{"name": destName, "parents": []string{parentID}})
		if err != nil {
			return err
		}
		return c.call(ctx, req, nil)
	})
}

// Move implements Client.
func (c *driveClient) Move(ctx context.Context, sourcePath, destParentPath, destName string) (*Item, error) {
	src, srcParentID, err := c.requireFile(ctx, sourcePath)
	if err != nil {
		return nil, err
	}

	moved := src
	err = c.transfer(ctx, src, destParentPath, destName, func(parentID string) error {
		q := url.Values{"fields": {itemFields}}
		if parentID != srcParentID {
			q.Set("addParents", parentID)
			q.Set("removeParents", srcParentID)
		}
		req, err := jsonRequest(http.MethodPatch, c.fileURL(src.ID, q), map[string]any{"name": destName})
		if err != nil {
			return err
		}

		var out fileJSON
		if err := c.call(ctx, req, &out); err != nil {
			return err
		}
		item := out.toItem()
		moved = &item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return moved, nil
}

// Delete implements Client.
func (c *driveClient) Delete(ctx context.Context, itemPath string) error {
	item, _, err := c.requireFile(ctx, itemPath)
	if err != nil {
		return err
	}

	return c.remove(ctx, item.ID)
}

// remove deletes an item by ID, or trashes it when configured to. In a shared drive, members below
// the organizer role may trash but not permanently delete. Drive reports that refusal as a 404
// notFound from files.delete (confirmed against the live API); a 403 insufficientFilePermissions
// is accepted too. Either one falls back to trashing.
func (c *driveClient) remove(ctx context.Context, id string) error {
	if !c.trash {
		err := c.call(ctx, request{method: http.MethodDelete, url: c.fileURL(id, nil)}, nil)
		if !c.canTrashInstead(err) {
			return err
		}
	}

	req, err := jsonRequest(http.MethodPatch, c.fileURL(id, url.Values{"fields": {"id"}}), map[string]any{"trashed": true})
	if err != nil {
		return err
	}
	return c.call(ctx, req, nil)
}

func (c *driveClient) canTrashInstead(err error) bool {
	var apiErr *apiError
	if c.driveID == "" || !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusNotFound:
		return true
	case http.StatusForbidden:
		return apiErr.Reason == "insufficientFilePermissions"
	}
	return false
}
