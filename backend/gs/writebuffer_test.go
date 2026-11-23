package gs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/utils"
)

const (
	wbBucket       = "bucki"
	wbSeedObject   = "seed.txt"
	wbSeedContents = "hello"
)

type writeBufferTestSuite struct {
	suite.Suite
	tmpDir string
	server *fakestorage.Server
}

func (ts *writeBufferTestSuite) SetupTest() {
	// isolate temp files so tests can assert none were created
	ts.tmpDir = ts.T().TempDir()
	ts.T().Setenv("TMPDIR", ts.tmpDir)
	ts.T().Setenv("TMP", ts.tmpDir)
	ts.T().Setenv("TEMP", ts.tmpDir)

	ts.server = fakestorage.NewServer(Objects{
		fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{BucketName: wbBucket, Name: wbSeedObject},
			Content:     []byte(wbSeedContents),
		},
	})
}

func (ts *writeBufferTestSuite) TearDownTest() {
	ts.server.Stop()
}

func (ts *writeBufferTestSuite) tempFiles() []string {
	entries, err := os.ReadDir(ts.tmpDir)
	ts.Require().NoError(err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func (ts *writeBufferTestSuite) newFile(opts Options, key string, fileOpts ...options.NewFileOption) *File {
	fsys := NewFileSystem(WithClient(ts.server.Client()), WithOptions(opts))
	f, err := fsys.NewFile(wbBucket, key, fileOpts...)
	ts.Require().NoError(err)
	return f.(*File)
}

func (ts *writeBufferTestSuite) objectContents(name string) string {
	return string(mustReadObject(context.Background(), ts.server.Client().Bucket(wbBucket), name))
}

func (ts *writeBufferTestSuite) objectExists(name string) bool {
	return objectExists(context.Background(), ts.server.Client().Bucket(wbBucket), name)
}

func (ts *writeBufferTestSuite) TestUseWriteBuffer() {
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

func (ts *writeBufferTestSuite) TestOptionsJSON() {
	b, err := json.Marshal(Options{DisableWriteBuffer: true})
	ts.Require().NoError(err)
	ts.Contains(string(b), `"disableWriteBuffer":true`)

	b, err = json.Marshal(Options{})
	ts.Require().NoError(err)
	ts.NotContains(string(b), "disableWriteBuffer")

	var o Options
	ts.Require().NoError(json.Unmarshal([]byte(`{"disableWriteBuffer":true}`), &o))
	ts.True(o.DisableWriteBuffer)
}

func (ts *writeBufferTestSuite) TestStreamingWrite() {
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
			ts.Equal("hello world", ts.objectContents(key[1:]))
			attrs, err := ts.server.Client().Bucket(wbBucket).Object(key[1:]).Attrs(context.Background())
			ts.Require().NoError(err)
			ts.Equal("text/plain", attrs.ContentType)
			ts.Empty(ts.tempFiles())
			ts.False(f.streaming)
			ts.Zero(f.cursorPos)
		})
	}
}

func (ts *writeBufferTestSuite) TestStreamingZeroByteWrite() {
	f := ts.newFile(Options{}, "/empty.txt", newfile.WithStreamingWrite(), newfile.WithContentType("application/x-empty"))

	n, err := f.Write([]byte{})
	ts.Require().NoError(err)
	ts.Zero(n)
	ts.Empty(ts.tempFiles())
	ts.Require().NoError(f.Close())

	attrs, err := ts.server.Client().Bucket(wbBucket).Object("empty.txt").Attrs(context.Background())
	ts.Require().NoError(err)
	ts.Zero(attrs.Size)
	ts.Equal("application/x-empty", attrs.ContentType)
}

func (ts *writeBufferTestSuite) TestMixedIO() {
	tests := []struct {
		name     string
		key      string
		ops      func(f *File) error
		expected string // contents committed on Close; empty means no object is expected
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
			key:  wbSeedObject,
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
			key:  wbSeedObject,
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
			ts.Equal(tt.expected, ts.objectContents(tt.key))
		})
	}
}

func (ts *writeBufferTestSuite) TestReuseAfterClose() {
	f := ts.newFile(Options{DisableWriteBuffer: true}, "/reuse.txt")

	_, err := f.Write([]byte("hello"))
	ts.Require().NoError(err)
	ts.Require().NoError(f.Close())
	ts.Equal("hello", ts.objectContents("reuse.txt"))

	pos, err := f.Seek(2, io.SeekStart)
	ts.Require().NoError(err)
	ts.Equal(int64(2), pos)

	b, err := io.ReadAll(f)
	ts.Require().NoError(err)
	ts.Equal("llo", string(b))
	ts.Require().NoError(f.Close())
}

func (ts *writeBufferTestSuite) TestReadOnlySessionUnaffected() {
	f := ts.newFile(Options{DisableWriteBuffer: true}, "/"+wbSeedObject)

	_, err := f.Seek(1, io.SeekStart)
	ts.Require().NoError(err)
	b := make([]byte, 2)
	_, err = io.ReadFull(f, b)
	ts.Require().NoError(err)
	ts.Equal("el", string(b))
	ts.Require().NoError(f.Close())
}

func (ts *writeBufferTestSuite) TestErrLocalWriteBuffer() {
	cause := errors.New("no space left on device")
	err := errLocalWriteBuffer(cause)
	ts.Require().ErrorIs(err, cause)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Contains(err.Error(), "DisableWriteBuffer")
}

func (ts *writeBufferTestSuite) TestCreateTempFailureHint() {
	missing := filepath.Join(ts.tmpDir, "does-not-exist")
	ts.T().Setenv("TMPDIR", missing)
	ts.T().Setenv("TMP", missing)
	ts.T().Setenv("TEMP", missing)

	f := ts.newFile(Options{}, "/nospace.txt")

	_, err := f.Write([]byte("abc"))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Nil(f.tempFileWriter)
	ts.Nil(f.gcsWriter, "no upload should start when the local buffer cannot be created")
}

func (ts *writeBufferTestSuite) TestTempWriteFailureAbortsUpload() {
	f := ts.newFile(Options{}, "/aborted.txt")

	_, err := f.Write([]byte("abc"))
	ts.Require().NoError(err)
	ts.Require().NotNil(f.tempFileWriter)

	// swap in a read-only handle to the same temp file so the next local write fails
	orig := f.tempFileWriter
	readOnly, err := os.Open(orig.Name())
	ts.Require().NoError(err)
	f.tempFileWriter = readOnly
	ts.T().Cleanup(func() { _ = orig.Close() })

	_, err = f.Write([]byte("def"))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Nil(f.tempFileWriter)
	ts.Nil(f.gcsWriter)
	ts.Empty(ts.tempFiles(), "temp file should be removed immediately")

	// later writes fail with the latched error
	_, err = f.Write([]byte("ghi"))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")

	// Close reports the failure without committing and resets state
	err = f.Close()
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Require().NoError(f.writeErr)
	ts.False(f.writeCalled)
	ts.Zero(f.cursorPos)
	ts.Nil(f.gcsWriter)
	ts.Nil(f.tempFileWriter)

	// cancellation is asynchronous, so watch for a short while rather than checking once
	for range 15 {
		ts.Require().False(ts.objectExists("aborted.txt"), "no truncated object may be committed")
		time.Sleep(20 * time.Millisecond)
	}
}

func (ts *writeBufferTestSuite) TestEphemeralStreamingViaTouchCopyBuffered() {
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
			ts.Equal(tt.content, ts.objectContents(tt.key))
		})
	}
}

func (ts *writeBufferTestSuite) TestEphemeralNotClaimedMidSession() {
	f := ts.newFile(Options{}, "/midsession.txt")

	_, err := f.Write([]byte("abc"))
	ts.Require().NoError(err)
	ts.Require().NotNil(f.tempFileWriter)

	// a buffered session already in progress must keep its temp file
	ts.Require().NoError(utils.TouchCopyBuffered(f, strings.NewReader("def"), 0))
	ts.NotNil(f.tempFileWriter)
	ts.Require().NoError(f.Close())
	ts.Equal("abcdef", ts.objectContents("midsession.txt"))
}

func (ts *writeBufferTestSuite) TestCopyFromMemSkipsTempFile() {
	target := ts.newFile(Options{}, "/frommem.txt")

	memFile, err := mem.NewFileSystem().NewFile("", "/src.txt")
	ts.Require().NoError(err)
	_, err = memFile.Write([]byte("from mem"))
	ts.Require().NoError(err)
	ts.Require().NoError(memFile.Close())

	ts.Require().NoError(memFile.CopyToFile(target))
	ts.Equal("from mem", ts.objectContents("frommem.txt"))
	ts.Empty(ts.tempFiles())
}

func (ts *writeBufferTestSuite) TestCopyToLocationFromOtherAuthSkipsTempFile() {
	src := ts.newFile(Options{CredentialFile: "src-creds"}, "/"+wbSeedObject)
	dstFs := NewFileSystem(WithClient(ts.server.Client()), WithOptions(Options{CredentialFile: "dst-creds"}))
	dstLoc, err := dstFs.NewLocation(wbBucket, "/dest/")
	ts.Require().NoError(err)

	_, err = src.CopyToLocation(dstLoc)
	ts.Require().NoError(err)
	ts.Equal(wbSeedContents, ts.objectContents("dest/"+wbSeedObject))
	ts.Empty(ts.tempFiles())
}

func (ts *writeBufferTestSuite) TestAbortWriteClosesNetworkReader() {
	f := ts.newFile(Options{}, "/f.txt")
	reader := &closeTracker{Reader: strings.NewReader("x")}
	f.reader = reader

	err := f.abortWrite(errors.New("disk full"))
	ts.Require().Error(err)
	ts.True(reader.closed, "a network reader must be closed when the write is aborted")
	ts.Nil(f.reader)
	ts.Require().Error(f.writeErr)
}

// closeTracker is a reader whose Close is observable.
type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error {
	c.closed = true
	return nil
}

func TestWriteBufferTestSuite(t *testing.T) {
	suite.Run(t, new(writeBufferTestSuite))
}
