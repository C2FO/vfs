package s3

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/backend/s3/mocks"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/utils"
)

type writeBufferTestSuite struct {
	suite.Suite
	tmpDir string
}

// uploadCapture records what the mocked PutObject received.
type uploadCapture struct {
	mu    sync.Mutex
	body  []byte
	input *s3.PutObjectInput
	calls int
}

func (c *uploadCapture) contents() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.body)
}

func (c *uploadCapture) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (ts *writeBufferTestSuite) SetupTest() {
	// isolate temp files so tests can assert none were created
	ts.tmpDir = ts.T().TempDir()
	ts.T().Setenv("TMPDIR", ts.tmpDir)
	ts.T().Setenv("TMP", ts.tmpDir)
	ts.T().Setenv("TEMP", ts.tmpDir)
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

func (ts *writeBufferTestSuite) newClient() (*mocks.Client, *uploadCapture) {
	client := mocks.NewClient(ts.T())
	capture := &uploadCapture{}
	client.EXPECT().PutObject(matchContext, mock.IsType((*s3.PutObjectInput)(nil)), mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			b, err := io.ReadAll(input.Body)
			if err != nil {
				return nil, err
			}
			capture.mu.Lock()
			defer capture.mu.Unlock()
			capture.body = b
			capture.input = input
			capture.calls++
			return &s3.PutObjectOutput{}, nil
		}).Maybe()
	client.EXPECT().HeadObject(matchContext, mock.IsType((*s3.HeadObjectInput)(nil))).
		Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(5)}, nil).Maybe()
	client.EXPECT().GetObject(matchContext, mock.IsType((*s3.GetObjectInput)(nil))).
		RunAndReturn(func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("hello"))}, nil
		}).Maybe()
	return client, capture
}

func (ts *writeBufferTestSuite) newFile(client Client, opts Options, fileOpts ...options.NewFileOption) *File {
	fsys := &FileSystem{client: client, options: opts}
	f, err := fsys.NewFile("bucket", "/some/path/file.txt", fileOpts...)
	ts.Require().NoError(err)
	return f.(*File)
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
			f := ts.newFile(mocks.NewClient(ts.T()), tt.opts, tt.fileOpts...)
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
			client, capture := ts.newClient()
			f := ts.newFile(client, tt.opts, append(tt.fileOpts, newfile.WithContentType("text/plain"))...)

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
			ts.Equal("hello world", capture.contents())
			ts.Require().NotNil(capture.input)
			ts.Equal("text/plain", aws.ToString(capture.input.ContentType))
			ts.Empty(ts.tempFiles())
			ts.False(f.streaming)
			ts.Zero(f.cursorPos)
		})
	}
}

func (ts *writeBufferTestSuite) TestStreamingZeroByteWrite() {
	client, capture := ts.newClient()
	f := ts.newFile(client, Options{}, newfile.WithStreamingWrite(), newfile.WithContentType("application/x-empty"))

	n, err := f.Write([]byte{})
	ts.Require().NoError(err)
	ts.Zero(n)
	ts.Empty(ts.tempFiles())
	ts.Require().NoError(f.Close())

	ts.Equal(1, capture.callCount())
	ts.Empty(capture.contents())
	ts.Equal("application/x-empty", aws.ToString(capture.input.ContentType))
}

func (ts *writeBufferTestSuite) TestMixedIO() {
	tests := []struct {
		name     string
		ops      func(f *File) error
		expected string // contents committed on Close
	}{
		{
			name: "Write then Seek is rejected and keeps the upload alive",
			ops: func(f *File) error {
				if _, err := f.Write([]byte("abc")); err != nil {
					return err
				}
				_, err := f.Seek(0, io.SeekStart)
				if err == nil {
					return errors.New("expected seek error")
				}
				// the stream must still be usable after the rejected Seek
				if _, werr := f.Write([]byte("def")); werr != nil {
					return werr
				}
				return err
			},
			expected: "abcdef",
		},
		{
			name: "Write then Read is rejected and keeps the upload alive",
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
			ops: func(f *File) error {
				if _, err := f.Seek(0, io.SeekStart); err != nil {
					return err
				}
				_, err := f.Write([]byte("abc"))
				return err
			},
		},
		{
			name: "Read then Write is rejected",
			ops: func(f *File) error {
				if _, err := f.Read(make([]byte, 1)); err != nil {
					return err
				}
				_, err := f.Write([]byte("abc"))
				return err
			},
		},
	}
	for i := range tests {
		tt := &tests[i]
		ts.Run(tt.name, func() {
			client, capture := ts.newClient()
			f := ts.newFile(client, Options{DisableWriteBuffer: true})

			err := tt.ops(f)
			ts.Require().ErrorIs(err, vfs.ErrWriteBufferDisabled)
			ts.Empty(ts.tempFiles())

			ts.Require().NoError(f.Close())
			ts.Equal(tt.expected, capture.contents())
		})
	}
}

func (ts *writeBufferTestSuite) TestReuseAfterClose() {
	client, capture := ts.newClient()
	f := ts.newFile(client, Options{DisableWriteBuffer: true})

	_, err := f.Write([]byte("hello"))
	ts.Require().NoError(err)
	ts.Require().NoError(f.Close())
	ts.Equal("hello", capture.contents())

	pos, err := f.Seek(2, io.SeekStart)
	ts.Require().NoError(err)
	ts.Equal(int64(2), pos)

	b, err := io.ReadAll(f)
	ts.Require().NoError(err)
	ts.NotEmpty(b)
	ts.Require().NoError(f.Close())
}

func (ts *writeBufferTestSuite) TestReadOnlySessionUnaffected() {
	client, _ := ts.newClient()
	f := ts.newFile(client, Options{DisableWriteBuffer: true})

	_, err := f.Seek(1, io.SeekStart)
	ts.Require().NoError(err)
	_, err = f.Read(make([]byte, 2))
	ts.Require().NoError(err)
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

	client, _ := ts.newClient()
	f := ts.newFile(client, Options{})

	_, err := f.Write([]byte("abc"))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Nil(f.tempFileWriter)
	ts.Nil(f.s3Writer, "no upload should start when the local buffer cannot be created")

	// Close reports the failure instead of succeeding with nothing written
	err = f.Close()
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
}

func (ts *writeBufferTestSuite) TestTempWriteFailureAbortsUpload() {
	// PutObject is deliberately not expected: a mock call would fail the test
	client := mocks.NewClient(ts.T())
	f := ts.newFile(client, Options{})

	_, err := f.Write([]byte("abc"))
	ts.Require().NoError(err)
	ts.Require().NotNil(f.tempFileWriter)

	// swap in a read-only handle to the same temp file so the next local write fails
	orig := f.tempFileWriter
	readOnly, err := os.Open(orig.Name())
	ts.Require().NoError(err)
	f.tempFileWriter = readOnly
	// release the writable handle: Windows cannot remove a file that is still open
	ts.Require().NoError(orig.Close())

	_, err = f.Write([]byte("def"))
	ts.Require().Error(err)
	ts.Contains(err.Error(), "newfile.WithStreamingWrite()")
	ts.Nil(f.tempFileWriter)
	ts.Nil(f.s3Writer)
	ts.Nil(f.s3WriterCompleteCh)
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
	ts.Nil(f.s3Writer)
	ts.Nil(f.tempFileWriter)
}

func (ts *writeBufferTestSuite) TestTouchCreatesEmptyObjectWithoutTempFile() {
	client := mocks.NewClient(ts.T())
	var captured *s3.PutObjectInput
	var body []byte
	client.EXPECT().HeadObject(matchContext, mock.IsType((*s3.HeadObjectInput)(nil))).
		Return(nil, &types.NotFound{}).Once()
	client.EXPECT().HeadObject(matchContext, mock.IsType((*s3.HeadObjectInput)(nil))).
		Return(&s3.HeadObjectOutput{ContentLength: aws.Int64(0)}, nil)
	client.EXPECT().PutObject(matchContext, mock.IsType((*s3.PutObjectInput)(nil)), mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			captured = input
			var err error
			body, err = io.ReadAll(input.Body)
			return &s3.PutObjectOutput{}, err
		}).Once()

	f := ts.newFile(client, Options{ACL: types.ObjectCannedACLPrivate}, newfile.WithContentType("text/plain"))
	ts.Require().NoError(f.Touch())

	ts.Nil(f.tempFileWriter)
	ts.Empty(ts.tempFiles())
	ts.Require().NotNil(captured)
	ts.Empty(body)
	ts.Equal("text/plain", aws.ToString(captured.ContentType))
	ts.Equal(types.ObjectCannedACLPrivate, captured.ACL)
	ts.Equal(types.ServerSideEncryptionAes256, captured.ServerSideEncryption)
}

func (ts *writeBufferTestSuite) TestTouchErrors() {
	tests := []struct {
		name          string
		opts          Options
		putErr        error
		expectedError string
	}{
		{name: "upload fails", putErr: errors.New("boom"), expectedError: "boom"},
		{name: "undersized part size", opts: Options{UploadPartitionSize: 1}, expectedError: "upload partition size"},
	}
	for i := range tests {
		tt := &tests[i]
		ts.Run(tt.name, func() {
			client := mocks.NewClient(ts.T())
			client.EXPECT().HeadObject(matchContext, mock.IsType((*s3.HeadObjectInput)(nil))).
				Return(nil, &types.NotFound{}).Once()
			if tt.putErr != nil {
				client.EXPECT().PutObject(matchContext, mock.IsType((*s3.PutObjectInput)(nil)), mock.Anything, mock.Anything).
					Return(nil, tt.putErr).Once()
			}
			f := ts.newFile(client, tt.opts)
			err := f.Touch()
			ts.Require().Error(err)
			ts.Contains(err.Error(), tt.expectedError)
		})
	}
}

func (ts *writeBufferTestSuite) TestEphemeralStreamingViaTouchCopyBuffered() {
	tests := []struct {
		name          string
		content       string
		fileOpts      []options.NewFileOption
		expectedTemp  bool
		expectedBytes string
	}{
		{name: "non-empty copy skips temp file", content: "copied", expectedBytes: "copied"},
		{name: "empty copy skips temp file", content: ""},
		{
			name:          "explicit WithWriteBuffer keeps temp file",
			content:       "copied",
			fileOpts:      []options.NewFileOption{newfile.WithWriteBuffer()},
			expectedTemp:  true,
			expectedBytes: "copied",
		},
	}
	for i := range tests {
		tt := &tests[i]
		ts.Run(tt.name, func() {
			client, capture := ts.newClient()
			f := ts.newFile(client, Options{}, tt.fileOpts...)

			ts.Require().NoError(utils.TouchCopyBuffered(f, strings.NewReader(tt.content), 0))
			ts.Equal(tt.expectedTemp, f.tempFileWriter != nil)
			ts.False(f.ephemeralStream, "ephemeral flag should be reset after the copy")
			if !tt.expectedTemp {
				ts.Empty(ts.tempFiles())
			}

			ts.Require().NoError(f.Close())
			ts.Equal(tt.expectedBytes, capture.contents())
		})
	}
}

func (ts *writeBufferTestSuite) TestEphemeralNotClaimedMidSession() {
	client, _ := ts.newClient()
	f := ts.newFile(client, Options{})

	_, err := f.Write([]byte("abc"))
	ts.Require().NoError(err)
	ts.Require().NotNil(f.tempFileWriter)

	// a buffered session already in progress must keep its temp file
	ts.Require().NoError(utils.TouchCopyBuffered(f, strings.NewReader("def"), 0))
	ts.NotNil(f.tempFileWriter)
	ts.Require().NoError(f.Close())
}

func (ts *writeBufferTestSuite) TestCopyFromMemSkipsTempFile() {
	client, capture := ts.newClient()
	target := ts.newFile(client, Options{})

	memFile, err := mem.NewFileSystem().NewFile("", "/src.txt")
	ts.Require().NoError(err)
	_, err = memFile.Write([]byte("from mem"))
	ts.Require().NoError(err)
	ts.Require().NoError(memFile.Close())

	ts.Require().NoError(memFile.CopyToFile(target))
	ts.Equal("from mem", capture.contents())
	ts.Empty(ts.tempFiles())
}

func (ts *writeBufferTestSuite) TestCopyToLocationFromS3SkipsTempFile() {
	srcClient, _ := ts.newClient()
	src := ts.newFile(srcClient, Options{AccessKeyID: "src"})

	dstClient, capture := ts.newClient()
	dstFs := &FileSystem{client: dstClient, options: Options{AccessKeyID: "dst"}}
	dstLoc, err := dstFs.NewLocation("otherbucket", "/dest/")
	ts.Require().NoError(err)

	_, err = src.CopyToLocation(dstLoc)
	ts.Require().NoError(err)
	ts.Equal("hello", capture.contents())
	ts.Empty(ts.tempFiles())
}

func (ts *writeBufferTestSuite) TestNativeCopyWithDisableWriteBuffer() {
	client := mocks.NewClient(ts.T())
	opts := Options{DisableWriteBuffer: true}
	src := ts.newFile(client, opts)
	dst := ts.newFile(client, opts)

	input := src.getCopyObjectInput(dst)
	ts.Require().NotNil(input, "same-auth files should still use native CopyObject when DisableWriteBuffer is set")
}

func (ts *writeBufferTestSuite) TestAbortWriteClosesNetworkReader() {
	f := ts.newFile(mocks.NewClient(ts.T()), Options{})
	reader := &closeTracker{Reader: strings.NewReader("x")}
	f.reader = reader

	err := f.abortWrite(errors.New("disk full"))
	ts.Require().Error(err)
	ts.True(reader.closed, "a network reader must be closed when the write is aborted")
	ts.Nil(f.reader)
	ts.Require().Error(f.writeErr)
}

func (ts *writeBufferTestSuite) TestCloseReportsUploadErrorWhenNoWriteSucceeded() {
	// no HeadObject expectation: Close must return the upload error without polling for the object
	f := ts.newFile(mocks.NewClient(ts.T()), Options{})
	_, pw := io.Pipe()
	f.s3Writer = pw
	f.s3WriterCompleteCh = make(chan error, 1)
	f.s3WriterCompleteCh <- errors.New("access denied")

	err := f.Close()
	ts.Require().Error(err)
	ts.Contains(err.Error(), "access denied")
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
