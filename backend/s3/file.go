package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/mocks"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/options/delete"
	"github.com/c2fo/vfs/v6/options/newfile"
	"github.com/c2fo/vfs/v6/utils"
)

const defaultPartitionSize = int64(32 * 1024 * 1024)

// File implements vfs.File interface for S3 fs.
type File struct {
	fileSystem *FileSystem
	bucket     string
	key        string
	opts       []options.NewFileOption

	// seek-related fields
	cursorPos  int64
	seekCalled bool

	// read-related fields
	reader      io.ReadCloser
	readCalled  bool
	readEOFSeen bool

	// write-related fields
	tempFileWriter     *os.File
	s3Writer           *io.PipeWriter
	cancelFunc         context.CancelFunc
	writeCalled        bool
	s3WriterCompleteCh chan error
}

// Info Functions

// LastModified returns the LastModified property of a HEAD request to the s3 object.
func (f *File) LastModified() (*time.Time, error) {
	head, err := f.getHeadObject()
	if err != nil {
		return nil, err
	}
	return head.LastModified, nil
}

// Name returns the name portion of the file's key property. IE: "file.txt" of "s3://some/path/to/file.txt
func (f *File) Name() string {
	return path.Base(f.key)
}

// Path return the directory portion of the file's key. IE: "path/to" of "s3://some/path/to/file.txt
func (f *File) Path() string {
	return utils.EnsureLeadingSlash(f.key)
}

// Exists returns whether (boolean) the object exists on s3, based on a call for
// the object's HEAD through the s3 API.
func (f *File) Exists() (bool, error) {
	_, err := f.getHeadObject()
	if err != nil {
		if errors.Is(err, vfs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Size returns the ContentLength value from an S3 HEAD request on the file's object.
func (f *File) Size() (uint64, error) {
	head, err := f.getHeadObject()
	if err != nil {
		return 0, err
	}
	return uint64(*head.ContentLength), nil
}

// Location returns a vfs.Location at the location of the object. IE: if file is at
// s3://bucket/here/is/the/file.txt the location points to s3://bucket/here/is/the/
func (f *File) Location() vfs.Location {
	return vfs.Location(&Location{
		fileSystem: f.fileSystem,
		prefix:     path.Dir(f.key),
		bucket:     f.bucket,
	})
}

// Move/Copy Operations

// CopyToFile puts the contents of File into the targetFile passed. Uses the S3 CopyObject
// method if the target file is also on S3, otherwise uses io.CopyBuffer.
func (f *File) CopyToFile(file vfs.File) (err error) {
	// Close file (f) reader regardless of an error
	defer func() {
		// close writer
		wErr := file.Close()
		// close reader
		rErr := f.Close()
		//
		if err == nil {
			if wErr != nil {
				err = wErr
			} else if rErr != nil {
				err = rErr
			}
		}
	}()
	// validate seek is at 0,0 before doing copy
	if f.cursorPos != 0 {
		return vfs.CopyToNotPossible
	}

	// if target is S3
	if tf, ok := file.(*File); ok {
		input := f.getCopyObjectInput(tf)
		// if input is not nil, use it to natively copy object
		if input != nil {
			client, err := f.fileSystem.Client()
			if err != nil {
				return err
			}
			_, err = client.CopyObject(context.Background(), input)
			return err
		}
	}

	// Otherwise, use TouchCopyBuffered using io.CopyBuffer
	fileBufferSize := 0

	if opts, ok := f.Location().FileSystem().(*FileSystem).options.(Options); ok {
		fileBufferSize = opts.FileBufferSize
	}

	if err := utils.TouchCopyBuffered(file, f, fileBufferSize); err != nil {
		return err
	}
	// Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if err := file.Close(); err != nil {
		return err
	}

	return err
}

// MoveToFile puts the contents of File into the targetFile passed using File.CopyToFile.
// If the copy succeeds, the source file is deleted. Any errors from the copy or delete are
// returned.
func (f *File) MoveToFile(file vfs.File) error {
	if err := f.CopyToFile(file); err != nil {
		return err
	}

	return f.Delete()
}

// MoveToLocation works by first calling File.CopyToLocation(vfs.Location) then, if that
// succeeds, it deletes the original file, returning the new file. If the copy process fails
// the error is returned, and the Delete isn't called. If the call to Delete fails, the error
// and the file generated by the copy are both returned.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := f.CopyToLocation(location)
	if err != nil {
		return nil, err
	}
	delErr := f.Delete()
	return newFile, delErr
}

// CopyToLocation creates a copy of *File, using the file's current name as the new file's
// name at the given location. If the given location is also s3, the AWS API for copying
// files will be utilized, otherwise, standard io.Copy will be done to the new file.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}

	return newFile, f.CopyToFile(newFile)
}

// CRUD Operations

// Delete clears any local temp file, or write buffer from read/writes to the file, then makes
// a DeleteObject call to s3 for the file. If delete.AllVersions option is provided,
// DeleteObject call is made to s3 for each version of the file. Returns any error returned by the API.
func (f *File) Delete(opts ...options.DeleteOption) error {
	if err := f.Close(); err != nil {
		return err
	}

	client, err := f.fileSystem.Client()
	if err != nil {
		return err
	}

	var allVersions bool
	for _, o := range opts {
		switch o.(type) {
		case delete.AllVersions, delete.DeleteAllVersions:
			allVersions = true
		default:
		}
	}

	_, err = client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Key:    &f.key,
		Bucket: &f.bucket,
	})
	if err != nil {
		return err
	}

	if allVersions {
		objectVersions, err := f.getAllObjectVersions(client)
		if err != nil {
			return err
		}

		for _, version := range objectVersions.Versions {
			if _, err = client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
				Key:       &f.key,
				Bucket:    &f.bucket,
				VersionId: version.VersionId,
			}); err != nil {
				return err
			}
		}
	}

	return err
}

// Close cleans up underlying mechanisms for reading from and writing to the file. Closes and removes the
// local temp file, and triggers a Write to S3 of anything in the f.writeBuffer if it has been created.
func (f *File) Close() error { //nolint:gocyclo
	defer func() {
		f.reader = nil
		f.cancelFunc = nil
		f.s3Writer = nil

		// reset state
		f.cursorPos = 0
		f.seekCalled = false
		f.readCalled = false
		f.writeCalled = false
		f.readEOFSeen = false
	}()

	// cleanup reader (unless reader is also the writer tempfile)
	if f.reader != nil && !f.writeCalled {
		// close reader
		if err := f.reader.Close(); err != nil {
			return utils.WrapCloseError(err)
		}
	}

	// finalize writer
	wroteFile := false
	if f.s3Writer != nil {
		// close s3Writer
		if err := f.s3Writer.Close(); err != nil {
			return utils.WrapCloseError(err)
		}
		wroteFile = true
	} else if f.tempFileWriter != nil { // s3Writer is nil but tempFileWriter is not nil (seek after write, write after seek)
		// write tempFileWriter to s3
		if err := f.tempToS3(); err != nil {
			return utils.WrapCloseError(err)
		}
		wroteFile = true
	}

	// cleanup tempFileWriter
	if f.tempFileWriter != nil {
		if err := f.cleanupTempFile(); err != nil {
			return utils.WrapCloseError(err)
		}
	}

	// wait for file to exist
	if wroteFile {
		// read s3WriterCompleteCh if it exists
		if f.writeCalled && f.s3Writer != nil && f.s3WriterCompleteCh != nil {
			// wait for s3Writer to complete
			err := <-f.s3WriterCompleteCh
			// close s3WriterCompleteCh channel
			close(f.s3WriterCompleteCh)
			if err != nil {
				return utils.WrapCloseError(err)
			}
		}
		err := waitUntilFileExists(f, 5)
		if err != nil {
			return utils.WrapCloseError(err)
		}
	}

	// close reader
	if f.reader != nil && !f.writeCalled {
		err := f.reader.Close()
		if err != nil {
			return utils.WrapCloseError(err)
		}
	}

	return nil
}

func (f *File) tempToS3() error {
	// ensure cursor is at 0
	if _, err := f.tempFileWriter.Seek(0, 0); err != nil {
		return err
	}

	// write tempFileWriter to s3
	client, err := f.fileSystem.Client()
	if err != nil {
		return err
	}

	uploader := manager.NewUploader(client, withUploadPartitionSize(f.getUploadPartitionSize()))
	uploadInput := uploadInput(f)
	uploadInput.Body = f.tempFileWriter

	_, err = uploader.Upload(context.Background(), uploadInput)
	if err != nil {
		return err
	}

	return nil
}

// Read implements the standard for io.Reader.
func (f *File) Read(p []byte) (n int, err error) {
	// s3 reader returns io.EOF when reading the last byte (but not past the last byte) to save on bandwidth,
	// but we want to return io.EOF only when reading past the last byte
	if f.readEOFSeen {
		return 0, io.EOF
	}

	// check/initialize for reader
	r, err := f.getReader()
	if err != nil {
		return 0, utils.WrapReadError(err)
	}

	read, err := r.Read(p)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return 0, utils.WrapReadError(err)
		}
		f.readEOFSeen = true
	}

	f.cursorPos += int64(read)
	f.readCalled = true

	return read, nil
}

func (f *File) cleanupTempFile() error {
	if f.tempFileWriter != nil {
		err := f.tempFileWriter.Close()
		if err != nil {
			return err
		}

		err = os.Remove(f.tempFileWriter.Name())
		if err != nil {
			return err
		}

		f.tempFileWriter = nil
	}

	return nil
}

// Seek implements the standard for io.Seeker.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	// get length of file
	var length uint64
	if f.writeCalled {
		// if write has been called, then the length is the cursorPos
		length = uint64(f.cursorPos)
	} else {
		var err error
		length, err = f.Size()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	// invalidate reader (if any)
	if f.reader != nil {
		err := f.reader.Close()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}

		f.reader = nil
	}

	// invalidate s3Writer
	if f.s3Writer != nil {
		// cancel s3Writer
		f.cancelFunc()
		f.cancelFunc = nil

		// close s3Writer
		err := f.s3Writer.Close()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}

		f.s3Writer = nil
	}

	// update seek position for tempFileWriter writer (if any)
	if f.tempFileWriter != nil {
		// seek tempFileWriter
		_, err := f.tempFileWriter.Seek(offset, whence)
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	// update cursorPos
	pos, err := utils.SeekTo(int64(length), f.cursorPos, offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}
	f.cursorPos = pos

	// set readEOFSeen if seeking to the end of the file
	f.readEOFSeen = f.cursorPos >= int64(length)
	f.seekCalled = true
	return f.cursorPos, nil
}

// Write implements the standard for io.Writer.  Note that writes are not committed to S3 until CLose() is called.
func (f *File) Write(data []byte) (int, error) {
	// check/initialize for writer
	err := f.initWriters()
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	// write to tempfile
	written, err := f.tempFileWriter.Write(data)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	// write to s3
	if f.s3Writer != nil {
		// write to s3
		s3written, err := f.s3Writer.Write(data)
		if err != nil {
			return 0, utils.WrapWriteError(err)
		}

		// ensure both writes are the same
		if written != s3written {
			return 0, utils.WrapWriteError(
				fmt.Errorf("local write and s3 write are different sizes: local=%d, s3=%d", written, s3written),
			)
		}
	}

	// update cursorPos
	f.cursorPos += int64(written)
	f.writeCalled = true

	return written, nil
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {
	// check if file exists
	exists, err := f.Exists()
	if err != nil {
		return err
	}

	// file doesn't already exist so create it
	if !exists {
		_, err = f.Write([]byte(""))
		if err != nil {
			return err
		}

		if err := f.Close(); err != nil {
			return err
		}
	} else {
		// file already exists so update its last modified date
		return utils.UpdateLastModifiedByMoving(f)
	}

	return nil
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	return utils.GetFileURI(f)
}

// String implement fmt.Stringer, returning the file's URI as the default string.
func (f *File) String() string {
	return f.URI()
}

/*
Private helper functions
*/
func (f *File) getAllObjectVersions(client Client) (*s3.ListObjectVersionsOutput, error) {
	prefix := utils.RemoveLeadingSlash(f.key)
	objVers, err := client.ListObjectVersions(context.Background(), &s3.ListObjectVersionsInput{
		Bucket: &f.bucket,
		Prefix: &prefix,
	})
	return objVers, err
}

func (f *File) getHeadObject() (*s3.HeadObjectOutput, error) {
	headObjectInput := &s3.HeadObjectInput{
		Key:    aws.String(f.key),
		Bucket: aws.String(f.bucket),
	}
	client, err := f.fileSystem.Client()
	if err != nil {
		return nil, err
	}

	head, err := client.HeadObject(context.Background(), headObjectInput)

	return head, handleExistsError(err)
}

// For copy from S3-to-S3 when credentials are the same between source and target, return *s3.CopyObjectInput or error
func (f *File) getCopyObjectInput(targetFile *File) *s3.CopyObjectInput {
	// If both files use the same account, copy with native library. Otherwise, copy to disk
	// first before pushing out to the target file's location.
	sameAuth, ACL := f.isSameAuth(targetFile)
	if !sameAuth {
		// return nil if credentials aren't the same
		return nil
	}

	// PathEscape ensures we url-encode as required by the API, including double-encoding literals
	copySourceKey := url.PathEscape(path.Join(f.bucket, f.key))

	copyInput := &s3.CopyObjectInput{
		ServerSideEncryption: types.ServerSideEncryptionAes256,
		ACL:                  ACL,
		Key:                  aws.String(targetFile.key),
		Bucket:               aws.String(targetFile.bucket),
		CopySource:           aws.String(copySourceKey),
	}

	// get content type from source
	var contentType string
	if targetFile.opts == nil && f.opts != nil {
		for _, o := range f.opts {
			switch o := o.(type) {
			case *newfile.ContentType:
				contentType = string(*o)
			default:
			}
		}
	}

	// set content type if it exists
	if contentType != "" {
		copyInput.ContentType = aws.String(contentType)
	}

	if f.fileSystem.options != nil && f.fileSystem.options.(Options).DisableServerSideEncryption {
		copyInput.ServerSideEncryption = ""
	}

	return copyInput
}

func (f *File) isSameAuth(targetFile *File) (bool, types.ObjectCannedACL) {
	fileOptions := f.Location().FileSystem().(*FileSystem).options
	targetOptions := targetFile.Location().FileSystem().(*FileSystem).options

	if fileOptions == nil && targetOptions == nil {
		// if both opts are nil, we must be using the default credentials
		return true, ""
	} else if opts, ok := fileOptions.(Options); ok {
		// use source ACL (even if empty), UNLESS target ACL is set
		ACL := opts.ACL
		if targetOpts, ok := targetOptions.(Options); ok {
			if targetOpts.ACL != "" {
				ACL = targetOpts.ACL
			}
			// since accesskey and session token are mutually exclusive, one will be nil
			// if both are the same, we're using the same credentials
			isSameAccount := (opts.AccessKeyID == targetOpts.AccessKeyID) && (opts.SessionToken == targetOpts.SessionToken)
			return isSameAccount, ACL
		}
		return false, ACL
	}
	return false, ""
}

func (f *File) copyS3ToLocalTempReader(tmpFile *os.File) error {
	client, err := f.fileSystem.Client()
	if err != nil {
		return err
	}

	// Download file
	input := &s3.GetObjectInput{
		Bucket: aws.String(f.bucket),
		Key:    aws.String(f.key),
	}
	opt := withDownloadPartitionSize(f.getDownloadPartitionSize())
	_, err = manager.NewDownloader(client, opt).
		Download(context.Background(), tmpFile, input)

	return err
}

// TODO: need to provide an implementation-agnostic container for providing config options such as SSE
func uploadInput(f *File) *s3.PutObjectInput {
	input := &s3.PutObjectInput{
		Bucket:               &f.bucket,
		Key:                  &f.key,
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	}

	if f.fileSystem.options == nil {
		f.fileSystem.options = Options{}
	}

	if f.fileSystem.options.(Options).DisableServerSideEncryption {
		input.ServerSideEncryption = ""
	}

	if opts, ok := f.fileSystem.options.(Options); ok {
		if opts.ACL != "" {
			input.ACL = opts.ACL
		}
	}

	for _, o := range f.opts {
		switch o := o.(type) {
		case *newfile.ContentType:
			input.ContentType = (*string)(o)
		default:
		}
	}

	return input
}

// WaitUntilFileExists attempts to ensure that a recently written file is available before moving on.  This is helpful for
// attempting to overcome race conditions withe S3's "eventual consistency".
// WaitUntilFileExists accepts vfs.File and an int representing the number of times to retry(once a second).
// error is returned if the file is still not available after the specified retries.
// nil is returned once the file is available.
func waitUntilFileExists(file vfs.File, retries int) error {
	// Ignore in-memory VFS files
	if _, ok := file.(*mocks.ReadWriteFile); ok {
		return nil
	}

	// Return as if file was found when retries is set to -1. Useful mainly for testing.
	if retries == -1 {
		return nil
	}
	var retryCount = 0
	for {
		if retryCount == retries {
			return fmt.Errorf("failed to find file %s after %d retries", file, retries)
		}

		// check for existing file
		found, err := file.Exists()
		if err != nil {
			return fmt.Errorf("unable to perform S3 exists on file %s: %s", file, err.Error())
		}

		if found {
			break
		}

		retryCount++
		time.Sleep(time.Second * 1)
	}

	return nil
}

func (f *File) getReader() (io.ReadCloser, error) {
	if f.reader == nil {
		if f.writeCalled && f.tempFileWriter != nil {
			// we've edited or truncated the file, so we need to read from the temp file which should already be at the
			// current cursor position
			f.reader = f.tempFileWriter
		} else {
			sz, err := f.Size()
			if err != nil {
				return nil, err
			}
			if sz == 0 {
				// can't set range on empty file, so just return an empty ReadCloser
				f.reader = io.NopCloser(strings.NewReader(""))
			} else {
				// Create the request to get the object
				input := &s3.GetObjectInput{
					Bucket: aws.String(f.bucket),
					Key:    aws.String(f.key),
					Range:  aws.String(fmt.Sprintf("bytes=%d-", f.cursorPos)),
				}

				// Get the client
				client, err := f.fileSystem.Client()
				if err != nil {
					return nil, err
				}

				// Request the object
				result, err := client.GetObject(context.Background(), input)
				if err != nil {
					return nil, err
				}

				// Set the reader to the body of the object
				f.reader = result.Body
			}
		}
	}
	return f.reader, nil
}

func handleExistsError(err error) error {
	if err != nil {
		var kerr *types.NotFound
		if errors.As(err, &kerr) {
			return vfs.ErrNotExist
		}
		return err
	}
	return nil
}

func (f *File) initWriters() error {
	if f.tempFileWriter == nil {
		// Create temp file
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("vfs_s3_%s.%d", f.Name(), time.Now().UnixNano()))
		if err != nil {
			return err
		}
		f.tempFileWriter = tmpFile
		if f.cursorPos != 0 {
			// if file exists(because cursor position is non-zero), we need to copy the existing s3 file to temp
			err := f.copyS3ToLocalTempReader(tmpFile)
			if err != nil {
				return err
			}

			// seek to cursorPos
			if _, err := f.tempFileWriter.Seek(f.cursorPos, 0); err != nil {
				return err
			}
		}
	}

	// if we haven't seeked yet, we need to get the s3Writer
	if f.s3Writer == nil {
		if !f.seekCalled && !f.readCalled {
			w, err := f.getS3Writer()
			if err != nil {
				return err
			}

			// Set the reader to the body of the object
			f.s3Writer = w
		}
	}

	return nil
}

func (f *File) getS3Writer() (*io.PipeWriter, error) {
	f.s3WriterCompleteCh = make(chan error, 1)
	pr, pw := io.Pipe()

	client, err := f.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	uploader := manager.NewUploader(client, withUploadPartitionSize(f.getUploadPartitionSize()))
	ctx, cancel := context.WithCancel(context.Background())
	f.cancelFunc = cancel
	uploadInput := uploadInput(f)
	uploadInput.Body = pr

	go func(input *s3.PutObjectInput) {
		defer cancel()
		_, err := uploader.Upload(ctx, input)
		if err != nil {
			_ = pw.CloseWithError(err)
		}
		f.s3WriterCompleteCh <- err
	}(uploadInput)

	return pw, nil
}

func (f *File) getUploadPartitionSize() int64 {
	partSize := defaultPartitionSize
	if f.fileSystem.options != nil {
		if opts, ok := f.fileSystem.options.(Options); ok {
			if opts.UploadPartitionSize != 0 {
				partSize = opts.UploadPartitionSize
			}
		}
	}
	return partSize
}

func (f *File) getDownloadPartitionSize() int64 {
	partSize := defaultPartitionSize
	if f.fileSystem.options != nil {
		if opts, ok := f.fileSystem.options.(Options); ok {
			if opts.DownloadPartitionSize != 0 {
				partSize = opts.DownloadPartitionSize
			}
		}
	}
	return partSize
}

func withDownloadPartitionSize(partSize int64) func(*manager.Downloader) {
	return func(d *manager.Downloader) {
		d.PartSize = partSize
	}
}

func withUploadPartitionSize(partSize int64) func(*manager.Uploader) {
	return func(u *manager.Uploader) {
		u.PartSize = partSize
	}
}
