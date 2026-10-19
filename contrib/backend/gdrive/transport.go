package gdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	maxRetryDelay  = 30 * time.Second
	maxRetryAfter  = 5 * time.Minute
	baseRetryDelay = 500 * time.Millisecond
)

// ErrTimeout is returned (wrapped) when a request makes no progress within the request timeout.
var ErrTimeout = errors.New("gdrive: request timed out")

// apiError is a non-2xx response from Drive.
type apiError struct {
	StatusCode int
	Reason     string
	Message    string
}

func (e *apiError) Error() string {
	if e.Reason == "" && e.Message == "" {
		return fmt.Sprintf("gdrive: request failed with status %d", e.StatusCode)
	}
	return fmt.Sprintf("gdrive: request failed with status %d (%s): %s", e.StatusCode, e.Reason, e.Message)
}

// Is makes errors.Is(err, ErrNotFound) true for 404 responses.
func (e *apiError) Is(target error) bool {
	return target == ErrNotFound && e.StatusCode == http.StatusNotFound
}

// request describes one Drive HTTP request.
type request struct {
	method string
	url    string
	body   io.ReadSeeker
	size   int64
	header http.Header
	// accept308 treats 308 Resume Incomplete as success (resumable upload chunks and status queries).
	accept308 bool
	// noRetry sends the request once; the caller handles recovery (resumable chunks).
	noRetry bool
	// stream applies the request timeout to the wait for headers and then as an idle timeout while
	// the body is read, instead of as a deadline for the whole exchange (downloads).
	stream bool
}

func jsonRequest(method, u string, v any) (request, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return request{}, fmt.Errorf("gdrive: encoding request: %w", err)
	}
	return request{
		method: method,
		url:    u,
		body:   bytes.NewReader(data),
		size:   int64(len(data)),
		header: http.Header{"Content-Type": {"application/json"}},
	}, nil
}

// do sends a request, retrying transient failures, and returns a 2xx response whose body the caller
// must close. Only idempotent methods retry transport errors, 5xx and 408; every method retries 429,
// 503 and Drive's rate-limit 403s.
func (c *driveClient) do(ctx context.Context, r request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, r, attempt)

		var apiErr *apiError
		switch {
		case err == nil && (resp.StatusCode/100 == 2 || (r.accept308 && resp.StatusCode == http.StatusPermanentRedirect)):
			return resp, nil
		case err == nil:
			apiErr = readAPIError(resp)
		case ctx.Err() != nil || !isIdempotent(r.method):
			return nil, fmt.Errorf("gdrive: %s %s: %w", r.method, redactURL(r.url), err)
		}

		if r.noRetry || attempt >= c.retryCount {
			return nil, c.finalError(r, apiErr, err)
		}

		delay, ok := retryDelay(attempt, r.method, apiErr, resp)
		if !ok {
			return nil, c.finalError(r, apiErr, err)
		}

		if serr := c.sleep(ctx, delay); serr != nil {
			return nil, serr
		}
	}
}

func (c *driveClient) finalError(r request, apiErr *apiError, err error) error {
	if apiErr != nil {
		return apiErr
	}
	return fmt.Errorf("gdrive: %s %s: %w", r.method, redactURL(r.url), err)
}

// send performs one HTTP exchange under the request timeout and records it in the request log.
func (c *driveClient) send(ctx context.Context, r request, attempt int) (*http.Response, error) {
	start := time.Now()
	resp, err := c.sendTimed(ctx, r)
	c.logRequest(ctx, r, attempt, start, resp, err)
	return resp, err
}

func (c *driveClient) sendTimed(ctx context.Context, r request) (*http.Response, error) {
	var body io.Reader = http.NoBody
	if r.body != nil && r.size > 0 {
		if _, err := r.body.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("rewinding request body: %w", err)
		}
		body = r.body
	}

	if c.timeout <= 0 {
		return c.roundTrip(ctx, r, body)
	}

	ctx, cancel := context.WithCancel(ctx)
	tb := &timeoutBody{cancel: cancel, idle: r.stream, timeout: c.timeout}
	tb.timer = time.AfterFunc(c.timeout, tb.expire)

	resp, err := c.roundTrip(ctx, r, body)
	if err != nil {
		tb.stop()
		if tb.fired.Load() {
			return nil, fmt.Errorf("%w after %s", ErrTimeout, c.timeout)
		}
		return nil, err
	}

	if r.stream {
		tb.timer.Reset(c.timeout)
	}
	tb.rc = resp.Body
	resp.Body = tb
	return resp, nil
}

func (c *driveClient) roundTrip(ctx context.Context, r request, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, r.method, r.url, body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = r.size
	for k, v := range r.header {
		req.Header[k] = v
	}

	return c.httpClient.Do(req)
}

// timeoutBody enforces the request timeout while a response body is read. For streamed responses the
// timer restarts on every read, so a large download only fails when it stalls.
type timeoutBody struct {
	rc      io.ReadCloser
	timer   *time.Timer
	cancel  context.CancelFunc
	fired   atomic.Bool
	idle    bool
	timeout time.Duration
}

func (b *timeoutBody) expire() {
	b.fired.Store(true)
	b.cancel()
}

func (b *timeoutBody) stop() {
	b.timer.Stop()
	b.cancel()
}

func (b *timeoutBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 && b.idle && !b.fired.Load() {
		b.timer.Reset(b.timeout)
	}
	if err != nil && !errors.Is(err, io.EOF) && b.fired.Load() {
		return n, fmt.Errorf("%w after %s", ErrTimeout, b.timeout)
	}
	return n, err
}

func (b *timeoutBody) Close() error {
	b.stop()
	return b.rc.Close()
}

// RequestLogger receives one record per HTTP attempt. Nil disables request logging.
type RequestLogger func(ctx context.Context, rec RequestLog)

// RequestLog describes one HTTP attempt. URL has upload session IDs redacted. Status is 0 when the
// attempt failed before a response arrived. Credentials are never included.
type RequestLog struct {
	Method       string
	URL          string
	Attempt      int
	Status       int
	Duration     time.Duration
	RequestBytes int64
	Err          error
}

// logRequest records one exchange when a request logger is configured.
func (c *driveClient) logRequest(ctx context.Context, r request, attempt int, start time.Time, resp *http.Response, err error) {
	if c.logger == nil {
		return
	}

	rec := RequestLog{
		Method:       r.method,
		URL:          redactURL(r.url),
		Attempt:      attempt + 1,
		Duration:     time.Since(start),
		RequestBytes: r.size,
		Err:          err,
	}
	if resp != nil {
		rec.Status = resp.StatusCode
	}
	c.logger(ctx, rec)
}

// redactURL drops the upload session ID, which authorizes writes to the session, from a URL meant
// for logs or error messages.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable url>"
	}

	q := u.Query()
	if q.Has("upload_id") {
		q.Set("upload_id", "REDACTED")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// retryDelay decides whether a failure is retryable and how long to wait. resp is nil for transport errors.
func retryDelay(attempt int, method string, apiErr *apiError, resp *http.Response) (time.Duration, bool) {
	if apiErr != nil && !retryableStatus(method, apiErr) {
		return 0, false
	}

	delay := backoff(attempt)

	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			secs, err := strconv.Atoi(ra)
			if err == nil && secs >= 0 {
				d := time.Duration(secs) * time.Second
				if d > maxRetryAfter {
					return 0, false
				}
				delay = d
			}
		}
	}

	return delay, true
}

// backoff returns an exponential delay with jitter for the given attempt.
func backoff(attempt int) time.Duration {
	delay := min(baseRetryDelay<<attempt, maxRetryDelay)
	return delay + time.Duration(rand.Int64N(int64(delay)/2+1)) //nolint:gosec // jitter, not security sensitive
}

func retryableStatus(method string, e *apiError) bool {
	switch e.StatusCode {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	case http.StatusForbidden:
		return isRateLimitReason(e.Reason)
	case http.StatusRequestTimeout:
		return isIdempotent(method)
	}
	return e.StatusCode >= 500 && isIdempotent(method)
}

func isRateLimitReason(reason string) bool {
	switch reason {
	case "rateLimitExceeded", "userRateLimitExceeded", "sharingRateLimitExceeded", "backendError":
		return true
	}
	return false
}

func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// readAPIError consumes and closes resp, returning its Drive error details.
func readAPIError(resp *http.Response) *apiError {
	defer func() { _ = resp.Body.Close() }()

	e := &apiError{StatusCode: resp.StatusCode}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	var body struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil {
		e.Message = body.Error.Message
		if len(body.Error.Errors) > 0 {
			e.Reason = body.Error.Errors[0].Reason
		}
	}

	return e
}

// call sends the request and decodes a JSON response into out (when non-nil).
func (c *driveClient) call(ctx context.Context, r request, out any) error {
	resp, err := c.do(ctx, r)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("gdrive: decoding response: %w", err)
	}
	return nil
}
