package gdrive

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stall blocks until the client gives up on the request. The body is drained first: the server only
// notices a disconnected client once the request body has been consumed.
func stall(r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	<-r.Context().Done()
}

func (s *DriveClientTestSuite) TestRequestTimeout() {
	s.client.timeout = 50 * time.Millisecond
	s.drive.setHook(func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet {
			return false
		}
		stall(r)
		return true
	})

	start := time.Now()
	_, err := s.client.GetItem(s.ctx, "a/b.txt")
	s.Require().ErrorIs(err, ErrTimeout)
	s.Less(time.Since(start), 5*time.Second, "each attempt is bounded, including retries")
}

func (s *DriveClientTestSuite) TestChunkTimeoutRedactsSessionURL() {
	session, err := s.client.CreateUploadSession(s.ctx, "slow.bin", 4)
	s.Require().NoError(err)

	s.client.timeout = 50 * time.Millisecond
	s.client.retryCount = 0
	s.drive.setHook(func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPut {
			return false
		}
		stall(r)
		return true
	})

	_, err = session.UploadChunk(s.ctx, []byte("abcd"), 0)
	s.Require().ErrorIs(err, ErrTimeout)
	s.NotContains(err.Error(), "secret-session", "errors never include the session ID")
	s.Contains(err.Error(), "upload_id=REDACTED")
}

func (s *DriveClientTestSuite) TestDownloadIdleTimeout() {
	s.upload("dl.bin", "payload")

	tests := []struct {
		name        string
		chunks      int
		gap         time.Duration
		stallAfter  bool
		expectedErr error
	}{
		{name: "slow but steady download outlives the timeout", chunks: 20, gap: 20 * time.Millisecond},
		{name: "stalled download fails", chunks: 2, gap: time.Millisecond, stallAfter: true, expectedErr: ErrTimeout},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.timeout = 150 * time.Millisecond
			s.drive.setHook(func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Query().Get("alt") != "media" {
					return false
				}
				w.WriteHeader(http.StatusOK)
				for range tt.chunks {
					_, _ = w.Write([]byte("x"))
					w.(http.Flusher).Flush()
					time.Sleep(tt.gap)
				}
				if tt.stallAfter {
					stall(r)
				}
				return true
			})

			rc, err := s.client.Download(s.ctx, "dl.bin")
			s.Require().NoError(err)
			data, err := io.ReadAll(rc)
			s.Require().NoError(rc.Close())

			if tt.expectedErr != nil {
				s.Require().ErrorIs(err, tt.expectedErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(strings.Repeat("x", tt.chunks), string(data))
		})
	}
}

func (s *DriveClientTestSuite) TestTransportErrorRedactsSessionURL() {
	session, err := s.client.CreateUploadSession(s.ctx, "secret.bin", 4)
	s.Require().NoError(err)

	var recs []RequestLog
	s.client.logger = func(_ context.Context, rec RequestLog) { recs = append(recs, rec) }
	s.client.retryCount = 0
	s.client.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset")
	})

	_, err = session.UploadChunk(s.ctx, []byte("abcd"), 0)
	s.Require().Error(err)
	s.NotContains(err.Error(), "secret-session")
	s.Contains(err.Error(), "upload_id=REDACTED")
	s.Require().NotEmpty(recs)
	for _, rec := range recs {
		s.NotContains(rec.URL, "secret-session")
		if rec.Err != nil {
			s.NotContains(rec.Err.Error(), "secret-session")
		}
	}
}

func (s *DriveClientTestSuite) TestResumableUploadStopsWhenChunkMakesNoProgress() {
	session, err := s.client.CreateUploadSession(s.ctx, "stuck.bin", 8)
	s.Require().NoError(err)

	s.drive.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/upload/session/") {
			return false
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusPermanentRedirect)
		return true
	})

	_, err = session.UploadChunk(s.ctx, []byte("12345678"), 0)
	s.Require().Error(err)
	s.Contains(err.Error(), "no progress")
	s.LessOrEqual(len(s.sessionRequests()), s.client.retryCount+2)
}

func (s *DriveClientTestSuite) TestResumableUploadRequiresFileWhenComplete() {
	session, err := s.client.CreateUploadSession(s.ctx, "nofile.bin", 4)
	s.Require().NoError(err)

	s.drive.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPut || strings.HasPrefix(r.Header.Get("Content-Range"), "bytes */") {
			return false
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Range", "bytes=0-3")
		w.WriteHeader(http.StatusPermanentRedirect)
		return true
	})

	_, err = session.UploadChunk(s.ctx, []byte("abcd"), 0)
	s.Require().Error(err)
	s.Contains(err.Error(), "did not finish")
}

func (s *DriveClientTestSuite) TestRequestTimeoutDisabled() {
	s.client.timeout = 0
	s.upload("nt/f.txt", "no timeout")
	s.Equal("no timeout", s.read("nt/f.txt"))
}

func (s *DriveClientTestSuite) TestBodyReadAfterTimeoutFails() {
	s.client.timeout = 100 * time.Millisecond
	s.drive.setHook(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet || r.URL.Query().Get("alt") == "media" {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"files":[`))
		w.(http.Flusher).Flush()
		stall(r)
		return true
	})

	_, err := s.client.ListChildren(s.ctx, "")
	s.Require().ErrorIs(err, ErrTimeout, "a JSON response must arrive in full within the timeout")
}

func (s *DriveClientTestSuite) TestDefaultTimeoutApplied() {
	s.Equal(DefaultRequestTimeout, s.client.timeout)

	fs := NewFileSystem(WithAccessToken("t"), WithRequestTimeout(time.Second), WithRequestLogger(nil))
	c, err := fs.Client("")
	s.Require().NoError(err)
	s.Equal(time.Second, c.(*driveClient).timeout)
	s.Nil(c.(*driveClient).logger)
}
