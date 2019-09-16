package s3

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/mocks"
	"github.com/c2fo/vfs/v5/utils"
)

//File implements vfs.File interface for S3 fs.
type File struct {
	fileSystem  *FileSystem
	bucket      string
	key         string
	tempFile    *os.File
	writeBuffer *bytes.Buffer
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

// Exists returns a boolean of whether or not the object exists on s3, based on a call for
// the object's HEAD through the s3 API.
func (f *File) Exists() (bool, error) {
	_, err := f.getHeadObject()
	code := ""
	if err != nil {
		code = err.(awserr.Error).Code()
	}
	if err != nil && (code == s3.ErrCodeNoSuchKey || code == "NotFound") {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// Size returns the ContentLength value from an s3 HEAD request on the file's object.
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
// method if the target file is also on S3, otherwise uses io.Copy.
func (f *File) CopyToFile(file vfs.File) error {
	if tf, ok := file.(*File); ok {
		return f.copyFile(tf)
	}

	if err := utils.TouchCopy(file, f); err != nil {
		return err
	}
	//Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := file.Close(); cerr != nil {
		return cerr
	}
	//Close file (f) reader
	return f.Close()
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
// a DeleteObject call to s3 for the file. Returns any error returned by the API.
func (f *File) Delete() error {
	f.writeBuffer = nil
	if err := f.Close(); err != nil {
		return err
	}

	client, err := f.fileSystem.Client()
	if err != nil {
		return err
	}

	_, err = client.DeleteObject(&s3.DeleteObjectInput{
		Key:    &f.key,
		Bucket: &f.bucket,
	})
	return err
}

// Close cleans up underlying mechanisms for reading from and writing to the file. Closes and removes the
// local temp file, and triggers a write to s3 of anything in the f.writeBuffer if it has been created.
func (f *File) Close() error {

	if f.tempFile != nil {
		defer func() { _ = f.tempFile.Close() }()

		err := os.Remove(f.tempFile.Name())
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		f.tempFile = nil
	}

	if f.writeBuffer != nil {
		client, err := f.fileSystem.Client()
		if err != nil {
			return err
		}

		uploader := s3manager.NewUploaderWithClient(client)
		uploadInput := uploadInput(f)
		uploadInput.Body = f.writeBuffer

		_, err = uploader.Upload(uploadInput)
		if err != nil {
			return err
		}
	}

	f.writeBuffer = nil

	return waitUntilFileExists(f, 5)
}

// Read implements the standard for io.Reader. For this to work with an s3 file, a temporary local copy of
// the file is created, and reads work on that. This file is closed and removed upon calling f.Close()
func (f *File) Read(p []byte) (n int, err error) {
	if err := f.checkTempFile(); err != nil {
		return 0, err
	}
	return f.tempFile.Read(p)
}

// Seek implements the standard for io.Seeker. A temporary local copy of the s3 file is created (the same
// one used for Reads) which Seek() acts on. This file is closed and removed upon calling f.Close()
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if err := f.checkTempFile(); err != nil {
		return 0, err
	}
	return f.tempFile.Seek(offset, whence)
}

// Write implements the standard for io.Writer. A buffer is added to with each subsequent
// write. When f.Close() is called, the contents of the buffer are used to initiate the
// PutObject to s3. The underlying implementation uses s3manager which will determine whether
// it is appropriate to call PutObject, or initiate a multi-part upload.
func (f *File) Write(data []byte) (res int, err error) {
	if f.writeBuffer == nil {
		//note, initializing with 'data' and returning len(data), nil
		//causes issues with some Write usages, notably csv.Writer
		//so we simply initialize with no bytes and call the buffer Write after
		//
		//f.writeBuffer = bytes.NewBuffer(data)
		//return len(data), nil
		//
		//so now we do:

		f.writeBuffer = bytes.NewBuffer([]byte{})
	}
	return f.writeBuffer.Write(data)
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {

	//check if file exists
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
func (f *File) getHeadObject() (*s3.HeadObjectOutput, error) {
	headObjectInput := new(s3.HeadObjectInput).SetKey(f.key).SetBucket(f.bucket)
	client, err := f.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	return client.HeadObject(headObjectInput)
}

// Copy from S3-to-S3 when accounts are the same between source and target and fall back to a
// TouchCopy() call if they are different.
func (f *File) copyFile(targetFile *File) error {
	isSameAccount := false
	hasACLOption := false

	opts, hasOptions := f.fileSystem.options.(Options)
	if hasOptions {
		hasACLOption = opts.ACL != ""
	}

	if hasOptions && targetFile.fileSystem.options != nil {
		isSameAccount = opts.AccessKeyID == targetFile.fileSystem.options.(Options).AccessKeyID
	}

	// If both files use the same account, copy with native library. Otherwise, copy to disk
	// first before pushing out to the target file's location.
	copyInput := new(s3.CopyObjectInput).
		SetKey(targetFile.key).
		SetBucket(targetFile.bucket).
		SetCopySource(path.Join(f.bucket, f.key))

	if hasOptions && hasACLOption {
		copyInput.SetACL(opts.ACL)
	}

	if isSameAccount {
		client, err := f.fileSystem.Client()
		if err != nil {
			return err
		}

		_, err = client.CopyObject(copyInput)

		return err
	}

	if err := utils.TouchCopy(targetFile, f); err != nil {
		return err
	}

	return nil
}

func (f *File) checkTempFile() error {
	if f.tempFile == nil {
		localTempFile, err := f.copyToLocalTempReader()
		if err != nil {
			return err
		}
		f.tempFile = localTempFile
	}

	return nil
}

func (f *File) copyToLocalTempReader() (*os.File, error) {
	tmpFile, err := ioutil.TempFile("", fmt.Sprintf("%s.%d", f.Name(), time.Now().UnixNano()))
	if err != nil {
		return nil, err
	}

	outputReader, err := f.getObject()
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(tmpFile, outputReader); err != nil {
		return nil, err
	}

	// Return cursor to the beginning of the new temp file
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return nil, err
	}

	//initialize temp ReadCloser
	return tmpFile, nil
}

func (f *File) getObjectInput() *s3.GetObjectInput {
	return new(s3.GetObjectInput).SetBucket(f.bucket).SetKey(f.key)
}

func (f *File) getObject() (io.ReadCloser, error) {
	client, err := f.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	getOutput, err := client.GetObject(f.getObjectInput())
	if err != nil {
		return nil, err
	}

	return getOutput.Body, nil
}

//TODO: need to provide an implementation-agnostic container for providing config options such as SSE
func uploadInput(f *File) *s3manager.UploadInput {
	sseType := "AES256"
	input := &s3manager.UploadInput{
		Bucket:               &f.bucket,
		Key:                  &f.key,
		ServerSideEncryption: &sseType,
	}

	if f.fileSystem.options == nil {
		f.fileSystem.options = Options{}
	}

	if opts, ok := f.fileSystem.options.(Options); ok {
		if opts.ACL != "" {
			input.ACL = &opts.ACL
		}
	}

	return input
}

//WaitUntilFileExists attempts to ensure that a recently written file is available before moving on.  This is helpful for
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

		//check for existing file
		found, err := file.Exists()
		if err != nil {
			return errors.New("unable to check for file on S3")
		}

		if found {
			break
		}

		retryCount++
		time.Sleep(time.Second * 1)
	}

	return nil
}
