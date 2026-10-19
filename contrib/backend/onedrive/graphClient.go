package onedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

const (
	// DefaultBaseURL is the Microsoft Graph v1.0 endpoint used unless overridden with WithBaseURL.
	DefaultBaseURL = "https://graph.microsoft.com/v1.0"

	graphScope = "https://graph.microsoft.com/.default"

	// replaceOnConflict is appended to copy and move requests so they overwrite an existing target,
	// matching the overwrite semantics of the other vfs backends.
	replaceOnConflict = "@microsoft.graph.conflictBehavior=replace"

	defaultCopyPollInterval = 500 * time.Millisecond
	defaultCopyTimeout      = 10 * time.Minute
	maxRetryDelay           = 30 * time.Second
)

// graphClient implements Client against the Microsoft Graph REST API.
type graphClient struct {
	baseURL    string
	httpClient *http.Client
	cred       azcore.TokenCredential
	retryCount int

	driveID string
	userID  string
	siteID  string

	copyPollInterval time.Duration
	copyTimeout      time.Duration
	sleep            func(ctx context.Context, d time.Duration) error

	driveMu sync.Mutex
}

func newGraphClient(cred azcore.TokenCredential, opts *Options) *graphClient {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &graphClient{
		baseURL:          strings.TrimSuffix(baseURL, "/"),
		httpClient:       httpClient,
		cred:             cred,
		retryCount:       opts.RetryCount,
		driveID:          opts.DriveID,
		userID:           opts.UserID,
		siteID:           opts.SiteID,
		copyPollInterval: defaultCopyPollInterval,
		copyTimeout:      defaultCopyTimeout,
		sleep:            sleepContext,
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

// apiError is a non-2xx response from Graph.
type apiError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *apiError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("onedrive: graph request failed with status %d", e.StatusCode)
	}
	return fmt.Sprintf("onedrive: graph request failed with status %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// Is makes errors.Is(err, ErrNotFound) true for 404 responses.
func (e *apiError) Is(target error) bool {
	return target == ErrNotFound && e.StatusCode == http.StatusNotFound
}

type driveItemJSON struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Size                 int64     `json:"size"`
	LastModifiedDateTime time.Time `json:"lastModifiedDateTime"`
	Folder               *struct{} `json:"folder"`
}

func (d *driveItemJSON) toDriveItem() *DriveItem {
	return &DriveItem{
		ID:           d.ID,
		Name:         d.Name,
		Size:         d.Size,
		LastModified: d.LastModifiedDateTime,
		IsFolder:     d.Folder != nil,
	}
}

// childrenPage is one page of a driveItem children listing. The tag names are Graph's wire format.
type childrenPage struct {
	Value    []driveItemJSON `json:"value"`
	NextLink string          `json:"@odata.nextLink"` //nolint:tagliatelle // Graph's OData field name
}

// request describes one Graph HTTP request.
type request struct {
	method string
	url    string
	body   io.ReadSeeker
	header http.Header
	// anonymous omits the bearer token, as required for pre-authenticated upload-session and copy-monitor URLs.
	anonymous bool
	// client overrides graphClient.httpClient.
	client *http.Client
}

func jsonRequest(method, u string, v any) (request, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return request{}, fmt.Errorf("onedrive: encoding request: %w", err)
	}
	return request{
		method: method,
		url:    u,
		body:   bytes.NewReader(data),
		header: http.Header{"Content-Type": {"application/json"}},
	}, nil
}

// GetItem implements Client.
func (c *graphClient) GetItem(ctx context.Context, itemPath string) (*DriveItem, error) {
	u, err := c.itemURL(ctx, itemPath, "")
	if err != nil {
		return nil, err
	}

	var item driveItemJSON
	if _, _, err := c.call(ctx, request{method: http.MethodGet, url: u}, &item); err != nil {
		return nil, err
	}

	return item.toDriveItem(), nil
}

// ListChildren implements Client.
func (c *graphClient) ListChildren(ctx context.Context, folderPath string) ([]DriveItem, error) {
	u, err := c.itemURL(ctx, folderPath, "/children")
	if err != nil {
		return nil, err
	}

	var items []DriveItem
	for u != "" {
		var page childrenPage
		if _, _, err := c.call(ctx, request{method: http.MethodGet, url: u}, &page); err != nil {
			return nil, err
		}

		for i := range page.Value {
			items = append(items, *page.Value[i].toDriveItem())
		}
		u = page.NextLink
	}

	return items, nil
}

// Download implements Client. Graph answers with a redirect to a pre-authenticated URL, which
// http.Client follows; it drops the Authorization header when the redirect leaves the Graph host.
func (c *graphClient) Download(ctx context.Context, itemPath string) (io.ReadCloser, error) {
	u, err := c.itemURL(ctx, itemPath, "/content")
	if err != nil {
		return nil, err
	}

	resp, err := c.send(ctx, request{method: http.MethodGet, url: u})
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// Upload implements Client.
func (c *graphClient) Upload(ctx context.Context, itemPath string, content io.ReadSeeker) (*DriveItem, error) {
	u, err := c.uploadURL(ctx, itemPath, "/content")
	if err != nil {
		return nil, err
	}

	req := request{
		method: http.MethodPut,
		url:    u,
		body:   content,
		header: http.Header{"Content-Type": {"application/octet-stream"}},
	}
	var item driveItemJSON
	if _, _, err := c.call(ctx, req, &item); err != nil {
		return nil, err
	}

	return item.toDriveItem(), nil
}

// CreateUploadSession implements Client.
func (c *graphClient) CreateUploadSession(ctx context.Context, itemPath string) (UploadSession, error) {
	u, err := c.uploadURL(ctx, itemPath, "/createUploadSession")
	if err != nil {
		return nil, err
	}

	req, err := jsonRequest(http.MethodPost, u, map[string]any{
		"item": map[string]any{"@microsoft.graph.conflictBehavior": "replace"},
	})
	if err != nil {
		return nil, err
	}

	var session struct {
		UploadURL string `json:"uploadUrl"`
	}
	if _, _, err := c.call(ctx, req, &session); err != nil {
		return nil, err
	}
	if session.UploadURL == "" {
		return nil, errors.New("onedrive: upload session response had no uploadUrl")
	}

	return &graphUploadSession{client: c, uploadURL: session.UploadURL}, nil
}

// Copy implements Client. Graph copies asynchronously, answering 202 with a monitor URL that is
// polled until the copy completes.
func (c *graphClient) Copy(ctx context.Context, sourcePath, destParentPath, destName string) error {
	driveID, err := c.resolveDriveID(ctx)
	if err != nil {
		return err
	}

	parent, err := c.GetItem(ctx, destParentPath)
	if err != nil {
		return fmt.Errorf("onedrive: copy destination folder: %w", err)
	}

	u, err := c.itemURL(ctx, sourcePath, "/copy")
	if err != nil {
		return err
	}

	req, err := jsonRequest(http.MethodPost, u+"?"+replaceOnConflict, map[string]any{
		"parentReference": map[string]string{"driveId": driveID, "id": parent.ID},
		"name":            destName,
	})
	if err != nil {
		return err
	}

	_, header, err := c.call(ctx, req, nil)
	if err != nil {
		return err
	}

	monitorURL := header.Get("Location")
	if monitorURL == "" {
		return errors.New("onedrive: copy response had no monitor Location header")
	}

	return c.waitForCopy(ctx, monitorURL)
}

func (c *graphClient) waitForCopy(ctx context.Context, monitorURL string) error {
	ctx, cancel := context.WithTimeout(ctx, c.copyTimeout)
	defer cancel()

	// The monitor redirects to the new item when done; that redirect is the completion signal.
	noRedirect := *c.httpClient
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req := request{method: http.MethodGet, url: monitorURL, anonymous: true, client: &noRedirect}

	for {
		var status struct {
			Status string `json:"status"`
			Error  struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		code, _, err := c.call(ctx, req, &status)
		if err != nil {
			return fmt.Errorf("onedrive: polling copy status: %w", err)
		}

		if code == http.StatusSeeOther || code == http.StatusFound || status.Status == "completed" {
			return nil
		}
		if status.Status == "failed" {
			return fmt.Errorf("onedrive: copy failed (%s): %s", status.Error.Code, status.Error.Message)
		}

		if err := c.sleep(ctx, c.copyPollInterval); err != nil {
			return fmt.Errorf("onedrive: waiting for copy: %w", err)
		}
	}
}

// Move implements Client.
func (c *graphClient) Move(ctx context.Context, sourcePath, destParentPath, destName string) (*DriveItem, error) {
	parent, err := c.GetItem(ctx, destParentPath)
	if err != nil {
		return nil, fmt.Errorf("onedrive: move destination folder: %w", err)
	}

	u, err := c.itemURL(ctx, sourcePath, "")
	if err != nil {
		return nil, err
	}

	req, err := jsonRequest(http.MethodPatch, u+"?"+replaceOnConflict, map[string]any{
		"parentReference": map[string]string{"id": parent.ID},
		"name":            destName,
	})
	if err != nil {
		return nil, err
	}

	var item driveItemJSON
	if _, _, err := c.call(ctx, req, &item); err != nil {
		return nil, err
	}

	return item.toDriveItem(), nil
}

// Delete implements Client.
func (c *graphClient) Delete(ctx context.Context, itemPath string) error {
	u, err := c.itemURL(ctx, itemPath, "")
	if err != nil {
		return err
	}

	_, _, err = c.call(ctx, request{method: http.MethodDelete, url: u}, nil)
	return err
}

// graphUploadSession implements UploadSession.
type graphUploadSession struct {
	client    *graphClient
	uploadURL string
}

// UploadChunk implements UploadSession.
func (s *graphUploadSession) UploadChunk(ctx context.Context, data []byte, offset, totalSize int64) (*DriveItem, error) {
	end := offset + int64(len(data)) - 1
	req := request{
		method:    http.MethodPut,
		url:       s.uploadURL,
		body:      bytes.NewReader(data),
		header:    http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", offset, end, totalSize)}},
		anonymous: true,
	}

	var item driveItemJSON
	code, _, err := s.client.call(ctx, req, &item)
	if err != nil {
		return nil, err
	}
	if code == http.StatusAccepted {
		return nil, nil
	}

	return item.toDriveItem(), nil
}

// Cancel implements UploadSession.
func (s *graphUploadSession) Cancel(ctx context.Context) error {
	_, _, err := s.client.call(ctx, request{method: http.MethodDelete, url: s.uploadURL, anonymous: true}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// resolveDriveID returns the configured drive ID, or looks up and caches the default drive of the
// configured site, user, or signed-in user.
func (c *graphClient) resolveDriveID(ctx context.Context) (string, error) {
	c.driveMu.Lock()
	defer c.driveMu.Unlock()

	if c.driveID != "" {
		return c.driveID, nil
	}

	var u string
	switch {
	case c.siteID != "":
		u = c.baseURL + "/sites/" + url.PathEscape(c.siteID) + "/drive"
	case c.userID != "":
		u = c.baseURL + "/users/" + url.PathEscape(c.userID) + "/drive"
	default:
		u = c.baseURL + "/me/drive"
	}

	var drive struct {
		ID string `json:"id"`
	}
	if _, _, err := c.call(ctx, request{method: http.MethodGet, url: u}, &drive); err != nil {
		return "", fmt.Errorf("onedrive: resolving drive: %w", err)
	}
	if drive.ID == "" {
		return "", errors.New("onedrive: drive response had no id")
	}

	c.driveID = drive.ID
	return c.driveID, nil
}

// itemURL builds a path-addressed driveItem URL, e.g. {base}/drives/{id}/root:/a/b.txt:/content.
func (c *graphClient) itemURL(ctx context.Context, itemPath, suffix string) (string, error) {
	driveID, err := c.resolveDriveID(ctx)
	if err != nil {
		return "", err
	}

	root := c.baseURL + "/drives/" + url.PathEscape(driveID) + "/root"
	itemPath = strings.Trim(itemPath, "/")
	if itemPath == "" {
		return root + suffix, nil
	}

	segments := strings.Split(itemPath, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	escaped := strings.Join(segments, "/")

	if suffix == "" {
		return root + ":/" + escaped, nil
	}
	return root + ":/" + escaped + ":" + suffix, nil
}

// uploadURL returns the URL to upload itemPath to. When an upload auto-creates parent folders,
// Graph percent-decodes their names twice ("a%20b/" becomes "a b/") while lookups decode once, so
// a file uploaded under such a folder can't be found again by its path. If any parent segment
// contains '%', the parents are created explicitly and the upload is addressed by parent ID.
func (c *graphClient) uploadURL(ctx context.Context, itemPath, suffix string) (string, error) {
	dir, name := path.Split(strings.Trim(itemPath, "/"))
	if !strings.Contains(dir, "%") {
		return c.itemURL(ctx, itemPath, suffix)
	}

	parentID, err := c.ensureFolder(ctx, strings.TrimSuffix(dir, "/"))
	if err != nil {
		return "", err
	}

	driveID, err := c.resolveDriveID(ctx)
	if err != nil {
		return "", err
	}

	return c.baseURL + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(parentID) +
		":/" + url.PathEscape(name) + ":" + suffix, nil
}

// ensureFolder returns the ID of the folder at folderPath, creating it and any missing ancestors.
func (c *graphClient) ensureFolder(ctx context.Context, folderPath string) (string, error) {
	if folderPath == "" {
		return "root", nil
	}

	item, err := c.GetItem(ctx, folderPath)
	switch {
	case err == nil && item.IsFolder:
		return item.ID, nil
	case err == nil:
		return "", fmt.Errorf("onedrive: %s exists and is not a folder", folderPath)
	case !errors.Is(err, ErrNotFound):
		return "", err
	}

	parent, name := path.Split(folderPath)
	parentID, err := c.ensureFolder(ctx, strings.TrimSuffix(parent, "/"))
	if err != nil {
		return "", err
	}

	driveID, err := c.resolveDriveID(ctx)
	if err != nil {
		return "", err
	}

	u := c.baseURL + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(parentID) + "/children"
	req, err := jsonRequest(http.MethodPost, u, map[string]any{
		"name":                              name,
		"folder":                            map[string]any{},
		"@microsoft.graph.conflictBehavior": "fail",
	})
	if err != nil {
		return "", err
	}

	var created driveItemJSON
	_, _, err = c.call(ctx, req, &created)
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		// Created concurrently by another writer.
		item, err := c.GetItem(ctx, folderPath)
		if err != nil {
			return "", err
		}
		return item.ID, nil
	}
	if err != nil {
		return "", fmt.Errorf("onedrive: creating folder %s: %w", folderPath, err)
	}

	return created.ID, nil
}

// call sends a request and, when out is non-nil and the response has a body, decodes it as JSON.
// It returns the response status and headers.
func (c *graphClient) call(ctx context.Context, r request, out any) (int, http.Header, error) {
	resp, err := c.send(ctx, r)
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
			return 0, nil, fmt.Errorf("onedrive: decoding response: %w", err)
		}
	}

	return resp.StatusCode, resp.Header, nil
}

// send performs a request, retrying throttling and transient server errors up to retryCount times.
// It returns an *apiError for any status >= 400; on success the caller owns resp.Body.
func (c *graphClient) send(ctx context.Context, r request) (*http.Response, error) {
	httpClient := r.client
	if httpClient == nil {
		httpClient = c.httpClient
	}

	for attempt := 0; ; attempt++ {
		req, err := c.newRequest(ctx, r)
		if err != nil {
			return nil, err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("onedrive: %s request failed: %w", r.method, err)
		}

		if resp.StatusCode < http.StatusBadRequest {
			return resp, nil
		}

		retry := isRetryable(resp.StatusCode) && attempt < c.retryCount
		delay := retryDelay(resp, attempt)
		apiErr := newAPIError(resp)
		if !retry {
			return nil, apiErr
		}

		if err := c.sleep(ctx, delay); err != nil {
			return nil, fmt.Errorf("onedrive: waiting to retry: %w", err)
		}
	}
}

func (c *graphClient) newRequest(ctx context.Context, r request) (*http.Request, error) {
	var reqBody io.Reader
	var size int64
	if r.body != nil {
		var err error
		if size, err = r.body.Seek(0, io.SeekEnd); err != nil {
			return nil, fmt.Errorf("onedrive: sizing request body: %w", err)
		}
		if _, err := r.body.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("onedrive: rewinding request body: %w", err)
		}
		// NopCloser keeps http.Client from closing caller-owned bodies such as temp files.
		reqBody = io.NopCloser(r.body)
	}

	req, err := http.NewRequestWithContext(ctx, r.method, r.url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("onedrive: building request: %w", err)
	}
	if r.body != nil {
		req.ContentLength = size
	}

	for k, v := range r.header {
		req.Header[k] = v
	}

	if !r.anonymous {
		token, err := c.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{graphScope}})
		if err != nil {
			return nil, fmt.Errorf("onedrive: acquiring access token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token.Token)
	}

	return req, nil
}

func isRetryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func retryDelay(resp *http.Response, attempt int) time.Duration {
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs >= 0 {
		return min(time.Duration(secs)*time.Second, maxRetryDelay)
	}
	return min(time.Second<<attempt, maxRetryDelay)
}

// newAPIError reads and closes an error response.
func newAPIError(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()

	apiErr := &apiError{StatusCode: resp.StatusCode}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)); err == nil && json.Unmarshal(data, &body) == nil {
		apiErr.Code = body.Error.Code
		apiErr.Message = body.Error.Message
	}

	return apiErr
}
