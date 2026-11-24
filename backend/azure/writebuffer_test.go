package azure

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/azure/mocks"
	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/utils"
)

const (
	wbContainer    = "test-container"
	wbSeedBlob     = "seed.txt"
	wbSeedContents = "hello world"
)

type fakeBlob struct {
	data         []byte
	contentType  string
	lastModified time.Time
	metadata     map[string]string
}

type fakeBlobServer struct {
	mu           sync.Mutex
	blobs        map[string]*fakeBlob
	stagedBlocks map[string]map[string][]byte
	commitCount  int
}

func newFakeBlobServer() *fakeBlobServer {
	s := &fakeBlobServer{
		blobs:        make(map[string]*fakeBlob),
		stagedBlocks: make(map[string]map[string][]byte),
	}
	s.blobs[wbContainer+"/"+wbSeedBlob] = &fakeBlob{
		data:         []byte(wbSeedContents),
		contentType:  "text/plain",
		lastModified: time.Now().UTC().Truncate(time.Second),
		metadata:     map[string]string{},
	}
	return s
}

func (s *fakeBlobServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Path format: /<account>/<container>/<blob...>
	trimmed := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 3 {
		http.Error(w, "invalid blob path", http.StatusBadRequest)
		return
	}
	blobKey := parts[1] + "/" + parts[2]
	comp := r.URL.Query().Get("comp")

	switch r.Method {
	case http.MethodPut:
		s.handlePut(w, r, blobKey, comp)
	case http.MethodHead:
		s.handleHead(w, blobKey)
	case http.MethodGet:
		s.handleGet(w, r, blobKey)
	case http.MethodDelete:
		s.handleDelete(w, blobKey)
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

func (s *fakeBlobServer) handlePut(w http.ResponseWriter, r *http.Request, blobKey, comp string) {
	switch comp {
	case "block":
		s.handlePutBlock(w, r, blobKey)
	case "blocklist":
		s.handlePutBlockList(w, r, blobKey)
	case "metadata":
		s.handlePutMetadata(w, r, blobKey)
	case "":
		if copySrc := r.Header.Get("x-ms-copy-source"); copySrc != "" {
			s.handlePutCopy(w, blobKey, copySrc)
			return
		}
		s.handlePutBlob(w, r, blobKey)
	default:
		http.Error(w, "unsupported comp "+comp, http.StatusBadRequest)
	}
}

func (s *fakeBlobServer) handlePutBlock(w http.ResponseWriter, r *http.Request, blobKey string) {
	blockID := r.URL.Query().Get("blockid")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if s.stagedBlocks[blobKey] == nil {
		s.stagedBlocks[blobKey] = make(map[string][]byte)
	}
	s.stagedBlocks[blobKey][blockID] = body
	s.mu.Unlock()
	w.Header().Set("Content-MD5", "")
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeBlobServer) handlePutBlockList(w http.ResponseWriter, r *http.Request, blobKey string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var blockList struct {
		Latest []string `xml:"Latest"`
	}
	if err := xml.Unmarshal(body, &blockList); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ct := blobContentTypeFromHeaders(r.Header)
	s.mu.Lock()
	var buf bytes.Buffer
	staged := s.stagedBlocks[blobKey]
	for _, id := range blockList.Latest {
		chunk, ok := staged[id]
		if !ok {
			s.mu.Unlock()
			w.Header().Set("x-ms-error-code", "InvalidBlockList")
			http.Error(w, "missing staged block "+id, http.StatusBadRequest)
			return
		}
		buf.Write(chunk)
	}
	delete(s.stagedBlocks, blobKey)
	s.blobs[blobKey] = &fakeBlob{
		data:         buf.Bytes(),
		contentType:  ct,
		lastModified: time.Now().UTC().Truncate(time.Second),
		metadata:     extractMetaHeaders(r.Header),
	}
	s.commitCount++
	s.mu.Unlock()
	w.Header().Set("ETag", `"0x1"`)
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusCreated)
}

func (s *fakeBlobServer) handlePutMetadata(w http.ResponseWriter, r *http.Request, blobKey string) {
	s.mu.Lock()
	b, ok := s.blobs[blobKey]
	if !ok {
		s.mu.Unlock()
		writeBlobNotFound(w)
		return
	}
	b.metadata = extractMetaHeaders(r.Header)
	b.lastModified = time.Now().UTC().Truncate(time.Second)
	s.mu.Unlock()
	w.Header().Set("ETag", `"0x2"`)
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
}

func (s *fakeBlobServer) handlePutCopy(w http.ResponseWriter, blobKey, copySrc string) {
	u, err := url.Parse(copySrc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	srcParts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
	srcKey := srcParts[1] + "/" + srcParts[2]
	s.mu.Lock()
	srcBlob, ok := s.blobs[srcKey]
	if !ok {
		s.mu.Unlock()
		writeBlobNotFound(w)
		return
	}
	s.blobs[blobKey] = &fakeBlob{
		data:         bytes.Clone(srcBlob.data),
		contentType:  srcBlob.contentType,
		lastModified: time.Now().UTC().Truncate(time.Second),
		metadata:     maps.Clone(srcBlob.metadata),
	}
	s.mu.Unlock()
	w.Header().Set("x-ms-copy-id", "copy-id-1")
	w.Header().Set("x-ms-copy-status", "success")
	w.Header().Set("ETag", `"0x3"`)
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusAccepted)
}

func (s *fakeBlobServer) handlePutBlob(w http.ResponseWriter, r *http.Request, blobKey string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.blobs[blobKey] = &fakeBlob{
		data:         body,
		contentType:  blobContentTypeFromHeaders(r.Header),
		lastModified: time.Now().UTC().Truncate(time.Second),
		metadata:     extractMetaHeaders(r.Header),
	}
	s.mu.Unlock()
	w.Header().Set("ETag", `"0x1"`)
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusCreated)
}

func blobContentTypeFromHeaders(h http.Header) string {
	if ct := h.Get("X-Ms-Blob-Content-Type"); ct != "" {
		return ct
	}
	return h.Get("Content-Type")
}

func (s *fakeBlobServer) handleHead(w http.ResponseWriter, blobKey string) {
	s.mu.Lock()
	b, ok := s.blobs[blobKey]
	if !ok {
		s.mu.Unlock()
		writeBlobNotFound(w)
		return
	}
	dataLen := len(b.data)
	ct := b.contentType
	lm := b.lastModified
	meta := maps.Clone(b.metadata)
	s.mu.Unlock()

	for k, v := range meta {
		w.Header().Set("x-ms-meta-"+k, v)
	}
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Content-Length", strconv.Itoa(dataLen))
	w.Header().Set("Last-Modified", lm.UTC().Format(http.TimeFormat))
	w.Header().Set("ETag", `"0x1"`)
	w.Header().Set("x-ms-blob-type", "BlockBlob")
	w.WriteHeader(http.StatusOK)
}

func (s *fakeBlobServer) handleGet(w http.ResponseWriter, r *http.Request, blobKey string) {
	s.mu.Lock()
	b, ok := s.blobs[blobKey]
	if !ok {
		s.mu.Unlock()
		writeBlobNotFound(w)
		return
	}
	data := bytes.Clone(b.data)
	ct := b.contentType
	lm := b.lastModified
	s.mu.Unlock()

	rangeHdr := r.Header.Get("x-ms-range")
	if rangeHdr == "" {
		rangeHdr = r.Header.Get("Range")
	}
	if rangeHdr != "" {
		// Enforce real Azure Blob Storage behavior: any Range request against an empty blob or
		// starting at/past EOF fails with 416 InvalidRange.
		rest := strings.TrimPrefix(rangeHdr, "bytes=")
		parts := strings.SplitN(rest, "-", 2)
		start, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || len(data) == 0 || start < 0 || start >= int64(len(data)) {
			w.Header().Set("x-ms-error-code", "InvalidRange")
			http.Error(w, "InvalidRange", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		slice := data[start:]
		if parts[1] != "" {
			end, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil && end < int64(len(data))-1 {
				slice = data[start : end+1]
			}
		}
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+int64(len(slice))-1, len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(slice)))
		w.Header().Set("Last-Modified", lm.UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"0x1"`)
		w.Header().Set("x-ms-blob-type", "BlockBlob")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(slice)
		return
	}

	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Last-Modified", lm.UTC().Format(http.TimeFormat))
	w.Header().Set("ETag", `"0x1"`)
	w.Header().Set("x-ms-blob-type", "BlockBlob")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *fakeBlobServer) handleDelete(w http.ResponseWriter, blobKey string) {
	s.mu.Lock()
	_, ok := s.blobs[blobKey]
	delete(s.blobs, blobKey)
	s.mu.Unlock()
	if !ok {
		writeBlobNotFound(w)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func extractMetaHeaders(h http.Header) map[string]string {
	out := make(map[string]string)
	for k, vals := range h {
		lower := strings.ToLower(k)
		if after, ok := strings.CutPrefix(lower, "x-ms-meta-"); ok && len(vals) > 0 {
			out[after] = vals[0]
		}
	}
	return out
}

func writeBlobNotFound(w http.ResponseWriter) {
	w.Header().Set("x-ms-error-code", "BlobNotFound")
	http.Error(w, "BlobNotFound", http.StatusNotFound)
}

type WriteBufferTestSuite struct {
	suite.Suite
	tmpDir string
	fake   *fakeBlobServer
	srv    *httptest.Server
	keyB64 string
}

func (ts *WriteBufferTestSuite) SetupTest() {
	ts.tmpDir = ts.T().TempDir()
	ts.T().Setenv("TMPDIR", ts.tmpDir)
	ts.T().Setenv("TMP", ts.tmpDir)
	ts.T().Setenv("TEMP", ts.tmpDir)

	ts.fake = newFakeBlobServer()
	ts.srv = httptest.NewServer(ts.fake)
	ts.keyB64 = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
}

func (ts *WriteBufferTestSuite) TearDownTest() {
	ts.srv.Close()
}

func (ts *WriteBufferTestSuite) baseOptions(extra Options) Options {
	if extra.AccountName == "" {
		extra.AccountName = "devstoreaccount1"
	}
	if extra.AccountKey == "" {
		extra.AccountKey = ts.keyB64
	}
	if extra.ServiceURL == "" {
		extra.ServiceURL = ts.srv.URL + "/" + extra.AccountName + "/"
	}
	return extra
}

func (ts *WriteBufferTestSuite) newFile(opts Options, key string, fileOpts ...options.NewFileOption) *File {
	resolvedOpts := ts.baseOptions(opts)
	cli, err := NewClient(&resolvedOpts)
	ts.Require().NoError(err)
	// Use 1 MiB blocks in the write-buffer suite so multi-block StageBlock/CommitBlockList and mid-stream abort
	// paths are exercised quickly with ~1.5 MiB payloads (default 8 MiB block size is verified in client_test.go).
	cli.uploadStreamBlockSize = 1024 * 1024
	fsys := NewFileSystem(WithClient(cli), WithOptions(resolvedOpts))
	f, err := fsys.NewFile(wbContainer, key, fileOpts...)
	ts.Require().NoError(err)
	return f.(*File)
}

func (ts *WriteBufferTestSuite) tempFiles() []string {
	entries, err := os.ReadDir(ts.tmpDir)
	ts.Require().NoError(err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func (ts *WriteBufferTestSuite) blobContents(name string) string {
	ts.fake.mu.Lock()
	defer ts.fake.mu.Unlock()
	b, ok := ts.fake.blobs[wbContainer+"/"+name]
	ts.Require().True(ok, "expected blob %s to exist", name)
	return string(b.data)
}

func (ts *WriteBufferTestSuite) blobContentType(name string) string {
	ts.fake.mu.Lock()
	defer ts.fake.mu.Unlock()
	b, ok := ts.fake.blobs[wbContainer+"/"+name]
	ts.Require().True(ok, "expected blob %s to exist", name)
	return b.contentType
}

func (ts *WriteBufferTestSuite) blobExists(name string) bool {
	ts.fake.mu.Lock()
	defer ts.fake.mu.Unlock()
	_, ok := ts.fake.blobs[wbContainer+"/"+name]
	return ok
}

func (ts *WriteBufferTestSuite) TestUseWriteBuffer() {
	tests := []struct {
		name     string
		opts     Options
		fileOpts []options.NewFileOption
		expected bool
	}{
		{name: "default buffers", expected: true},
		{name: "DisableWriteBuffer streams", opts: Options{DisableWriteBuffer: true}, expected: false},
		{name: "WithStreamingWrite streams", fileOpts: []options.NewFileOption{newfile.WithStreamingWrite()}, expected: false},
		{
			name:     "WithWriteBuffer overrides DisableWriteBuffer",
			opts:     Options{DisableWriteBuffer: true},
			fileOpts: []options.NewFileOption{newfile.WithWriteBuffer()},
			expected: true,
		},
		{
			name:     "last option wins - streaming last",
			fileOpts: []options.NewFileOption{newfile.WithWriteBuffer(), newfile.WithStreamingWrite()},
			expected: false,
		},
		{
			name:     "last option wins - buffer last",
			opts:     Options{DisableWriteBuffer: true},
			fileOpts: []options.NewFileOption{newfile.WithStreamingWrite(), newfile.WithWriteBuffer()},
			expected: true,
		},
	}
	for i := range tests {
		tt := &tests[i]
		ts.Run(tt.name, func() {
			f := ts.newFile(tt.opts, "/f.txt", tt.fileOpts...)
			ts.Equal(tt.expected, f.useWriteBuffer())
		})
	}
}

func (ts *WriteBufferTestSuite) TestStreamingWrite() {
	tests := []struct {
		name         string
		opts         Options
		fileOpts     []options.NewFileOption
		expectedTemp bool
	}{
		{name: "DisableWriteBuffer", opts: Options{DisableWriteBuffer: true}},
		{name: "WithStreamingWrite", fileOpts: []options.NewFileOption{newfile.WithStreamingWrite()}},
		{
			name:         "WithWriteBuffer overrides DisableWriteBuffer",
			opts:         Options{DisableWriteBuffer: true},
			fileOpts:     []options.NewFileOption{newfile.WithWriteBuffer()},
			expectedTemp: true,
		},
	}
	for i := range tests {
		tt := &tests[i]
		ts.Run(tt.name, func() {
			key := "/stream" + string(rune('a'+i)) + ".txt"
			f := ts.newFile(tt.opts, key, append(tt.fileOpts, newfile.WithContentType("text/plain"))...)

			n, err := f.Write([]byte("hello "))
			ts.Require().NoError(err)
			ts.Equal(6, n)
			n, err = f.Write([]byte("world"))
			ts.Require().NoError(err)
			ts.Equal(5, n)

			ts.Equal(tt.expectedTemp, f.tempFileWriter != nil)
			if tt.expectedTemp {
				ts.NotEmpty(ts.tempFiles())
			} else {
				ts.Empty(ts.tempFiles(), "no local temp file should be created")
			}
			ts.Equal(int64(11), f.cursorPos)

			ts.Require().NoError(f.Close())
			ts.Equal("hello world", ts.blobContents(key[1:]))
			ts.Equal("text/plain", ts.blobContentType(key[1:]))
			ts.Empty(ts.tempFiles())
			ts.False(f.streaming)
			ts.Zero(f.cursorPos)
		})
	}
}

func (ts *WriteBufferTestSuite) TestStreamingZeroByteWrite() {
	f := ts.newFile(Options{}, "/empty.txt", newfile.WithStreamingWrite(), newfile.WithContentType("application/x-empty"))

	n, err := f.Write([]byte{})
	ts.Require().NoError(err)
	ts.Zero(n)
	ts.Empty(ts.tempFiles())
	ts.Require().NoError(f.Close())

	ts.Empty(ts.blobContents("empty.txt"))
	ts.Equal("application/x-empty", ts.blobContentType("empty.txt"))

	// Reading a 0-byte blob must not trigger HTTP 416 InvalidRange
	data, err := io.ReadAll(f)
	ts.Require().NoError(err)
	ts.Empty(data)
	ts.Empty(ts.tempFiles())
	ts.Require().NoError(f.Close())
}

func (ts *WriteBufferTestSuite) TestStreamingMultiBlockWrite() {
	// 1.5 MiB exceeds azblob's default 1 MiB BlockSize, exercising StageBlock + CommitBlockList without a temp file.
	payload := bytes.Repeat([]byte("z"), 1536*1024)
	f := ts.newFile(Options{DisableWriteBuffer: true}, "/multiblock.bin", newfile.WithContentType("application/octet-stream"))

	n, err := f.Write(payload[:800*1024])
	ts.Require().NoError(err)
	ts.Equal(800*1024, n)
	n, err = f.Write(payload[800*1024:])
	ts.Require().NoError(err)
	ts.Equal(len(payload)-800*1024, n)
	ts.Empty(ts.tempFiles())

	ts.Require().NoError(f.Close())
	ts.Len(ts.blobContents("multiblock.bin"), len(payload))
	ts.Equal("application/octet-stream", ts.blobContentType("multiblock.bin"))
	ts.GreaterOrEqual(ts.fake.commitCount, 1)
}

func (ts *WriteBufferTestSuite) TestMixedIO() {
	tests := []struct {
		name     string
		key      string
		ops      func(f *File) error
		expected string
	}{
		{
			name: "Write then Seek is rejected and keeps the upload alive",
			key:  "mixed-ws.txt",
			ops: func(f *File) error {
				if _, err := f.Write([]byte("abc")); err != nil {
					return err
				}
				_, err := f.Seek(0, io.SeekStart)
				if err == nil {
					return errors.New("expected seek error")
				}
				if _, werr := f.Write([]byte("def")); werr != nil {
					return werr
				}
				return err
			},
			expected: "abcdef",
		},
		{
			name: "Write then Read is rejected and keeps the upload alive",
			key:  "mixed-wr.txt",
			ops: func(f *File) error {
				if _, err := f.Write([]byte("abc")); err != nil {
					return err
				}
				_, err := f.Read(make([]byte, 1))
				if err == nil {
					return errors.New("expected read error")
				}
				if _, werr := f.Write([]byte("def")); werr != nil {
					return werr
				}
				return err
			},
			expected: "abcdef",
		},
		{
			name: "Seek then Write is rejected",
			key:  wbSeedBlob,
			ops: func(f *File) error {
				if _, err := f.Seek(0, io.SeekStart); err != nil {
					return err
				}
				_, err := f.Write([]byte("abc"))
				return err
			},
			expected: wbSeedContents,
		},
		{
			name: "Read then Write is rejected",
			key:  wbSeedBlob,
			ops: func(f *File) error {
				if _, err := f.Read(make([]byte, 1)); err != nil {
					return err
				}
				_, err := f.Write([]byte("abc"))
				return err
			},
			expected: wbSeedContents,
		},
	}
	for i := range tests {
		tt := &tests[i]
		ts.Run(tt.name, func() {
			f := ts.newFile(Options{DisableWriteBuffer: true}, "/"+tt.key)

			err := tt.ops(f)
			ts.Require().ErrorIs(err, vfs.ErrWriteBufferDisabled)
			ts.Empty(ts.tempFiles())

			ts.Require().NoError(f.Close())
			ts.Equal(tt.expected, ts.blobContents(tt.key))
		})
	}
}

func (ts *WriteBufferTestSuite) TestBufferedMixedIO() {
	ts.Run("Write then Seek then Write commits edited temp file and cleans up", func() {
		f := ts.newFile(Options{}, "/edit-wsw.txt")
		_, err := f.Write([]byte("hello world"))
		ts.Require().NoError(err)
		_, err = f.Seek(6, io.SeekStart)
		ts.Require().NoError(err)
		_, err = f.Write([]byte("azure"))
		ts.Require().NoError(err)
		ts.Require().NoError(f.Close())

		ts.Equal("hello azure", ts.blobContents("edit-wsw.txt"))
		ts.Empty(ts.tempFiles())
	})

	ts.Run("Write then Seek then Read then Seek then Read keeps tempFileWriter open until Close", func() {
		f := ts.newFile(Options{}, "/edit-wsrsr.txt")
		_, err := f.Write([]byte("hello world"))
		ts.Require().NoError(err)
		_, err = f.Seek(0, io.SeekStart)
		ts.Require().NoError(err)
		buf := make([]byte, 5)
		_, err = io.ReadFull(f, buf)
		ts.Require().NoError(err)
		ts.Equal("hello", string(buf))

		_, err = f.Seek(6, io.SeekStart)
		ts.Require().NoError(err)
		rest, err := io.ReadAll(f)
		ts.Require().NoError(err)
		ts.Equal("world", string(rest))

		ts.Require().NoError(f.Close())
		ts.Equal("hello world", ts.blobContents("edit-wsrsr.txt"))
		ts.Empty(ts.tempFiles())
	})

	ts.Run("Seek(0,0) then Write downloads existing blob and patches prefix", func() {
		f := ts.newFile(Options{}, "/patch0.txt")
		_, err := f.Write([]byte("hello world"))
		ts.Require().NoError(err)
		ts.Require().NoError(f.Close())

		_, err = f.Seek(0, io.SeekStart)
		ts.Require().NoError(err)
		ts.Empty(ts.tempFiles(), "Seek alone must not create a temp file")

		_, err = f.Write([]byte("HELLO"))
		ts.Require().NoError(err)
		ts.NotEmpty(ts.tempFiles(), "buffered Write after Seek creates a temp file")
		ts.Require().NoError(f.Close())

		ts.Equal("HELLO world", ts.blobContents("patch0.txt"))
		ts.Empty(ts.tempFiles())
	})

	ts.Run("Read then Write downloads existing blob and overwrites from cursor", func() {
		f := ts.newFile(Options{}, "/patch-rw.txt")
		_, err := f.Write([]byte("hello world"))
		ts.Require().NoError(err)
		ts.Require().NoError(f.Close())

		buf := make([]byte, 6)
		_, err = io.ReadFull(f, buf)
		ts.Require().NoError(err)
		ts.Equal("hello ", string(buf))
		ts.Empty(ts.tempFiles(), "Read alone must not create a temp file")

		_, err = f.Write([]byte("azure"))
		ts.Require().NoError(err)
		ts.Require().NoError(f.Close())

		ts.Equal("hello azure", ts.blobContents("patch-rw.txt"))
		ts.Empty(ts.tempFiles())
	})

	ts.Run("Read then Write then Read without Seek reads from edited temp file at updated cursor", func() {
		f := ts.newFile(Options{}, "/patch-rwr.txt")
		_, err := f.Write([]byte("hello world"))
		ts.Require().NoError(err)
		ts.Require().NoError(f.Close())

		buf := make([]byte, 5)
		_, err = io.ReadFull(f, buf)
		ts.Require().NoError(err)
		ts.Equal("hello", string(buf))

		// Overwrite byte at index 5 (' ' -> 'X'), advancing cursor to 6
		_, err = f.Write([]byte("X"))
		ts.Require().NoError(err)

		// Without calling Seek, ReadAll must read from offset 6 of tempFileWriter ("world"), not stale remote offset 5 (" world")
		rest, err := io.ReadAll(f)
		ts.Require().NoError(err)
		ts.Equal("world", string(rest))

		ts.Require().NoError(f.Close())
		ts.Equal("helloXworld", ts.blobContents("patch-rwr.txt"))
		ts.Empty(ts.tempFiles())
	})
}

func (ts *WriteBufferTestSuite) TestReadAndSeekStreamWithoutTempFile() {
	f := ts.newFile(Options{}, "/"+wbSeedBlob)

	pos, err := f.Seek(6, io.SeekStart)
	ts.Require().NoError(err)
	ts.Equal(int64(6), pos)
	ts.Empty(ts.tempFiles())

	b := make([]byte, 5)
	_, err = io.ReadFull(f, b)
	ts.Require().NoError(err)
	ts.Equal("world", string(b))
	ts.Empty(ts.tempFiles())

	// Seek to exact EOF and Read must return 0, io.EOF without HTTP 416 InvalidRange
	pos, err = f.Seek(0, io.SeekEnd)
	ts.Require().NoError(err)
	ts.Equal(int64(len(wbSeedContents)), pos)
	n, err := f.Read(b)
	ts.Equal(0, n)
	ts.Require().ErrorIs(err, io.EOF)
	ts.Empty(ts.tempFiles())

	ts.Require().NoError(f.Close())
}

func (ts *WriteBufferTestSuite) TestReuseAfterClose() {
	f := ts.newFile(Options{DisableWriteBuffer: true}, "/reuse.txt")

	_, err := f.Write([]byte("hello"))
	ts.Require().NoError(err)
	ts.Require().NoError(f.Close())
	ts.Equal("hello", ts.blobContents("reuse.txt"))

	pos, err := f.Seek(2, io.SeekStart)
	ts.Require().NoError(err)
	ts.Equal(int64(2), pos)

	b, err := io.ReadAll(f)
	ts.Require().NoError(err)
	ts.Equal("llo", string(b))
	ts.Empty(ts.tempFiles())
	ts.Require().NoError(f.Close())
}

func (ts *WriteBufferTestSuite) TestTouchWithoutTempFile() {
	f := ts.newFile(Options{}, "/touched.txt", newfile.WithContentType("text/plain"))
	ts.Require().NoError(f.Touch())
	ts.Empty(ts.tempFiles())
	ts.Empty(ts.blobContents("touched.txt"))
	ts.Equal("text/plain", ts.blobContentType("touched.txt"))

	// Touching an existing blob also uses zero temp files
	ts.Require().NoError(f.Touch())
	ts.Empty(ts.tempFiles())
}

func (ts *WriteBufferTestSuite) TestErrLocalWriteBuffer() {
	cause := errors.New("no space left on device")
	err := errLocalWriteBuffer(cause)
	ts.Require().ErrorIs(err, cause)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Contains(err.Error(), "DisableWriteBuffer")
}

func (ts *WriteBufferTestSuite) TestCreateTempFailureHint() {
	missing := filepath.Join(ts.tmpDir, "does-not-exist")
	ts.T().Setenv("TMPDIR", missing)
	ts.T().Setenv("TMP", missing)
	ts.T().Setenv("TEMP", missing)

	f := ts.newFile(Options{}, "/nospace.txt")

	_, err := f.Write([]byte("abc"))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Nil(f.tempFileWriter)
	ts.Nil(f.azWriter, "no upload should start when the local buffer cannot be created")

	err = f.Close()
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.False(ts.blobExists("nospace.txt"))
}

func (ts *WriteBufferTestSuite) TestTempWriteFailureAbortsUpload() {
	f := ts.newFile(Options{}, "/aborted.txt")

	// Write > 1 MiB first so at least one block is staged in Azure's uncommitted area
	_, err := f.Write(bytes.Repeat([]byte("a"), 1100*1024))
	ts.Require().NoError(err)
	ts.Require().NotNil(f.tempFileWriter)

	orig := f.tempFileWriter
	readOnly, err := os.Open(orig.Name())
	ts.Require().NoError(err)
	f.tempFileWriter = readOnly
	ts.Require().NoError(orig.Close())

	_, err = f.Write([]byte("def"))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Nil(f.tempFileWriter)
	ts.Nil(f.azWriter)
	ts.Empty(ts.tempFiles(), "temp file should be removed immediately")

	_, err = f.Write([]byte("ghi"))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	_, err = f.Seek(0, io.SeekStart)
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	_, err = f.Read(make([]byte, 1))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Nil(f.reader, "aborted handle must not open a remote reader")

	err = f.Close()
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Require().NoError(f.writeErr)
	ts.False(f.writeCalled)
	ts.Zero(f.cursorPos)
	ts.Nil(f.azWriter)
	ts.Nil(f.tempFileWriter)

	for range 15 {
		ts.Require().False(ts.blobExists("aborted.txt"), "no truncated blob may be committed")
		time.Sleep(10 * time.Millisecond)
	}
}

func (ts *WriteBufferTestSuite) TestEphemeralStreamingViaTouchCopyBuffered() {
	tests := []struct {
		name         string
		key          string
		content      string
		fileOpts     []options.NewFileOption
		expectedTemp bool
	}{
		{name: "non-empty copy skips temp file", key: "eph1.txt", content: "copied"},
		{name: "empty copy skips temp file", key: "eph2.txt"},
		{
			name:         "explicit WithWriteBuffer keeps temp file",
			key:          "eph3.txt",
			content:      "copied",
			fileOpts:     []options.NewFileOption{newfile.WithWriteBuffer()},
			expectedTemp: true,
		},
	}
	for i := range tests {
		tt := &tests[i]
		ts.Run(tt.name, func() {
			f := ts.newFile(Options{}, "/"+tt.key, tt.fileOpts...)

			ts.Require().NoError(utils.TouchCopyBuffered(f, strings.NewReader(tt.content), 0))
			ts.Equal(tt.expectedTemp, f.tempFileWriter != nil)
			ts.False(f.ephemeralStream, "ephemeral flag should be reset after the copy")
			if !tt.expectedTemp {
				ts.Empty(ts.tempFiles())
			}

			ts.Require().NoError(f.Close())
			ts.Equal(tt.content, ts.blobContents(tt.key))
		})
	}
}

func (ts *WriteBufferTestSuite) TestEphemeralNotClaimedMidSession() {
	f := ts.newFile(Options{}, "/midsession.txt")

	_, err := f.Write([]byte("abc"))
	ts.Require().NoError(err)
	ts.Require().NotNil(f.tempFileWriter)

	ts.Require().NoError(utils.TouchCopyBuffered(f, strings.NewReader("def"), 0))
	ts.NotNil(f.tempFileWriter)
	ts.Require().NoError(f.Close())
	ts.Equal("abcdef", ts.blobContents("midsession.txt"))
}

func (ts *WriteBufferTestSuite) TestCopyFromMemSkipsTempFile() {
	target := ts.newFile(Options{}, "/frommem.txt")

	memFile, err := mem.NewFileSystem().NewFile("", "/src.txt")
	ts.Require().NoError(err)
	_, err = memFile.Write([]byte("from mem"))
	ts.Require().NoError(err)
	ts.Require().NoError(memFile.Close())

	ts.Require().NoError(memFile.CopyToFile(target))
	ts.Equal("from mem", ts.blobContents("frommem.txt"))
	ts.Empty(ts.tempFiles())
}

func (ts *WriteBufferTestSuite) TestCopyToLocationWithoutTempFileAndOptionIsolation() {
	// Cross-auth CopyToLocation must stream without a local temp file and must NOT leak WithWriteBuffer onto the target.
	src := ts.newFile(Options{}, "/"+wbSeedBlob, newfile.WithWriteBuffer(), newfile.WithContentType("text/csv"))
	altKey := base64.StdEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210"))
	dstFS := NewFileSystem(WithOptions(ts.baseOptions(Options{AccountKey: altKey})))
	dstLoc, err := dstFS.NewLocation(wbContainer, "/dest/")
	ts.Require().NoError(err)

	copied, err := src.CopyToLocation(dstLoc)
	ts.Require().NoError(err)
	ts.Empty(ts.tempFiles(), "cross-auth CopyToLocation must not stage to a local temp file")
	ts.Equal(wbSeedContents, ts.blobContents("dest/"+wbSeedBlob))
	ts.Equal("text/csv", ts.blobContentType("dest/"+wbSeedBlob))
	ts.False(newfile.UseWriteBuffer(false, copied.(*File).opts), "WithWriteBuffer must not leak to target file opts")
}

func (ts *WriteBufferTestSuite) TestCopyToFilePreservesTargetOptionsAndPropagatesContentType() {
	altKey := base64.StdEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210"))
	tests := []struct {
		name                string
		sameAuth            bool
		srcOpts             []options.NewFileOption
		dstOpts             []options.NewFileOption
		expectedContentType string
		expectedBuffered    bool
	}{
		{
			name:                "cross-auth plain source preserves target WithWriteBuffer and ContentType",
			srcOpts:             nil,
			dstOpts:             []options.NewFileOption{newfile.WithWriteBuffer(), newfile.WithContentType("application/json")},
			expectedContentType: "application/json",
			expectedBuffered:    true,
		},
		{
			name:                "cross-auth source ContentType propagates even when target has streaming option",
			srcOpts:             []options.NewFileOption{newfile.WithContentType("text/csv"), newfile.WithWriteBuffer()},
			dstOpts:             []options.NewFileOption{newfile.WithStreamingWrite()},
			expectedContentType: "text/csv",
			expectedBuffered:    false,
		},
		{
			name:                "cross-auth target ContentType overrides source ContentType",
			srcOpts:             []options.NewFileOption{newfile.WithContentType("text/csv")},
			dstOpts:             []options.NewFileOption{newfile.WithContentType("text/plain")},
			expectedContentType: "text/plain",
			expectedBuffered:    false,
		},
		{
			name:                "same-auth native copy preserves blob without temp file",
			sameAuth:            true,
			srcOpts:             nil,
			dstOpts:             nil,
			expectedContentType: "text/plain",
		},
	}

	for i := range tests {
		tt := &tests[i]
		ts.Run(tt.name, func() {
			dstKey := ts.keyB64
			if !tt.sameAuth {
				dstKey = altKey
			}
			src := ts.newFile(Options{}, "/"+wbSeedBlob, tt.srcOpts...)
			targetPath := "/copy-opts/" + strconv.Itoa(i) + ".txt"
			dst := ts.newFile(Options{AccountKey: dstKey}, targetPath, tt.dstOpts...)
			origDstOptsLen := len(dst.opts)

			ts.Require().NoError(src.CopyToFile(dst))

			ts.Len(dst.opts, origDstOptsLen, "CopyToFile must not permanently mutate target opts")
			ts.Equal(tt.expectedBuffered, newfile.UseWriteBuffer(false, dst.opts))
			ts.Equal(wbSeedContents, ts.blobContents(targetPath[1:]))
			ts.Equal(tt.expectedContentType, ts.blobContentType(targetPath[1:]))
		})
	}
}

func (ts *WriteBufferTestSuite) TestAbortWriteClosesNetworkReader() {
	f := ts.newFile(Options{}, "/f.txt")
	reader := &closeTracker{Reader: strings.NewReader("x")}
	f.reader = reader

	err := f.abortWrite(errors.New("disk full"))
	ts.Require().Error(err)
	ts.True(reader.closed, "a network reader must be closed when the write is aborted")
	ts.Nil(f.reader)
	ts.Require().Error(f.writeErr)
}

func (ts *WriteBufferTestSuite) TestTouchCopyBufferedReaderErrorAbortsUpload() {
	f := ts.newFile(Options{}, "/copy-aborted.txt")
	boom := errors.New("source connection reset")

	// Send 1.2 MiB before failing so a block is staged in Azure's uncommitted area, then verify it is never committed.
	err := utils.TouchCopyBuffered(f, &partialErrReader{chunk: strings.Repeat("p", 1200*1024), err: boom}, 0)
	ts.Require().ErrorIs(err, boom)
	ts.False(f.ephemeralStream)
	ts.Nil(f.azWriter, "in-flight upload must be torn down when the copy fails")
	ts.Empty(ts.tempFiles())

	closeErr := f.Close()
	ts.Require().ErrorIs(closeErr, boom)

	for range 15 {
		ts.Require().False(ts.blobExists("copy-aborted.txt"), "no truncated blob may be committed after a failed copy")
		time.Sleep(10 * time.Millisecond)
	}
}

func (ts *WriteBufferTestSuite) TestNestedBeginSequentialWrite() {
	f := ts.newFile(Options{}, "/nested.txt")

	outerEnd := utils.BeginSequentialWrite(f)
	ts.False(f.useWriteBuffer(), "ephemeral stream must disable write buffer")
	utils.AbortWrite(f, nil)
	innerEnd := utils.BeginSequentialWrite(f)
	utils.AbortWrite(f, errors.New("before first write"))
	innerEnd()
	ts.True(f.ephemeralStream, "inner end must not clear the outer sequential write scope")
	ts.Require().NoError(f.writeErr)

	_, err := f.Write([]byte("streamed"))
	ts.Require().NoError(err)
	ts.Empty(ts.tempFiles())

	outerEnd()
	ts.False(f.ephemeralStream)
	ts.Require().NoError(f.Close())
	ts.Equal("streamed", ts.blobContents("nested.txt"))
}

func (ts *WriteBufferTestSuite) TestCleanupTempFileRemovesWhenAlreadyClosed() {
	f := ts.newFile(Options{}, "/f.txt")
	tmp, err := os.CreateTemp(ts.tmpDir, "vfs_az_closed")
	ts.Require().NoError(err)
	ts.Require().NoError(tmp.Close())
	f.tempFileWriter = tmp

	err = f.cleanupTempFile()
	ts.Require().ErrorIs(err, os.ErrClosed)
	ts.Nil(f.tempFileWriter, "tempFileWriter must be cleared even when Close fails")
	ts.Empty(ts.tempFiles(), "temp file must still be removed when Close fails")
}

func (ts *WriteBufferTestSuite) TestCloseCancelsWriteContext() {
	f := ts.newFile(Options{DisableWriteBuffer: true}, "/ctx.txt")
	_, err := f.Write([]byte("hello"))
	ts.Require().NoError(err)
	ts.Require().NotNil(f.cancelFunc)

	var canceled bool
	orig := f.cancelFunc
	f.cancelFunc = func() {
		canceled = true
		orig()
	}

	ts.Require().NoError(f.Close())
	ts.True(canceled, "Close must cancel the per-write context to release resources")
	ts.Equal("hello", ts.blobContents("ctx.txt"))
}

func (ts *WriteBufferTestSuite) TestCloseReaderErrorCleansUpWriterAndTempFile() {
	f := ts.newFile(Options{}, "/reader-close-err.txt")
	_, err := f.Write([]byte("abc"))
	ts.Require().NoError(err)
	ts.Require().NotNil(f.tempFileWriter)
	ts.Require().NotNil(f.azWriter)

	readerBoom := errors.New("reader close boom")
	f.reader = &errCloseReader{Reader: strings.NewReader("x"), err: readerBoom}

	err = f.Close()
	ts.Require().ErrorIs(err, readerBoom)
	ts.Nil(f.tempFileWriter, "Close must clean up tempFileWriter even when reader.Close fails")
	ts.Nil(f.azWriter, "Close must cancel and clear azWriter even when reader.Close fails")
	ts.Empty(ts.tempFiles(), "Close must remove local temp file even when reader.Close fails")
	ts.False(ts.blobExists("reader-close-err.txt"), "failed Close must not commit blob")
}

func (ts *WriteBufferTestSuite) TestInitWritersErrorCleansUpTempFile() {
	f := ts.newFile(Options{}, "/"+wbSeedBlob)
	_, err := f.Seek(0, io.SeekStart)
	ts.Require().NoError(err)

	// Remove the remote blob so copyToLocalTempReader fails after CreateTemp succeeded
	ts.fake.mu.Lock()
	delete(ts.fake.blobs, wbContainer+"/"+wbSeedBlob)
	ts.fake.mu.Unlock()

	_, err = f.Write([]byte("patch"))
	ts.Require().Error(err)
	ts.Nil(f.tempFileWriter, "failed initWriters must clean up tempFileWriter")
	ts.Empty(ts.tempFiles(), "failed initWriters must remove the temp file")

	ts.Require().NoError(f.Close())
	ts.False(ts.blobExists(wbSeedBlob))
}

func (ts *WriteBufferTestSuite) TestCustomNonStreamingClientFallbackAndRejection() {
	ts.Run("buffered write falls back to tempFileWriter and Upload and cleans up temp file", func() {
		client := mocks.NewClient(ts.T())
		fsys := NewFileSystem(WithClient(client))
		f, err := fsys.NewFile(wbContainer, "/custom.txt", newfile.WithContentType("text/plain"))
		ts.Require().NoError(err)

		client.EXPECT().Upload(mock.Anything, mock.Anything, "text/plain").RunAndReturn(
			func(_ vfs.File, content io.ReadSeeker, _ string) error {
				b, rerr := io.ReadAll(content)
				ts.Require().NoError(rerr)
				ts.Equal("buffered fallback", string(b))
				return nil
			},
		)

		_, err = f.Write([]byte("buffered fallback"))
		ts.Require().NoError(err)
		ts.NotEmpty(ts.tempFiles())
		ts.Require().NoError(f.Close())
		ts.Empty(ts.tempFiles(), "Close must remove the temp file")
	})

	ts.Run("TouchCopyBuffered falls back to buffered Upload on non-streaming Client", func() {
		client := mocks.NewClient(ts.T())
		fsys := NewFileSystem(WithClient(client))
		f, err := fsys.NewFile(wbContainer, "/custom-copy.txt")
		ts.Require().NoError(err)

		client.EXPECT().Upload(mock.Anything, mock.Anything, "").Return(nil)

		ts.Require().NoError(utils.TouchCopyBuffered(f, strings.NewReader("copied"), 0))
		ts.Require().NoError(f.Close())
		ts.Empty(ts.tempFiles())
	})

	ts.Run("explicit WithStreamingWrite rejects non-streaming Client without temp file", func() {
		client := mocks.NewClient(ts.T())
		fsys := NewFileSystem(WithClient(client))
		f, err := fsys.NewFile(wbContainer, "/custom-stream.txt", newfile.WithStreamingWrite())
		ts.Require().NoError(err)

		_, err = f.Write([]byte("nope"))
		ts.Require().Error(err)
		ts.Contains(err.Error(), "BlobStreamer")
		ts.Empty(ts.tempFiles())
	})

	ts.Run("DisableWriteBuffer rejects non-streaming Client without temp file", func() {
		client := mocks.NewClient(ts.T())
		fsys := NewFileSystem(WithClient(client), WithOptions(Options{DisableWriteBuffer: true}))
		f, err := fsys.NewFile(wbContainer, "/custom-stream2.txt")
		ts.Require().NoError(err)

		_, err = f.Write([]byte("nope"))
		ts.Require().Error(err)
		ts.Contains(err.Error(), "BlobStreamer")
		ts.Empty(ts.tempFiles())
	})
}

type partialErrReader struct {
	chunk string
	err   error
	sent  bool
}

func (r *partialErrReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.chunk), nil
	}
	return 0, r.err
}

type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error {
	c.closed = true
	return nil
}

type errCloseReader struct {
	io.Reader
	err error
}

func (r *errCloseReader) Close() error {
	return r.err
}

func TestWriteBufferTestSuite(t *testing.T) {
	suite.Run(t, new(WriteBufferTestSuite))
}
