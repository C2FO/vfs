package gs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"cloud.google.com/go/storage"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

const (
	doesNotExistError = "storage: object doesn't exist"
)

// File implements vfs.File interface for GS fs.
type File struct {
	fileSystem  *FileSystem
	bucket      string
	key         string
	tempFile    *os.File
	writeBuffer *bytes.Buffer
}

// Close cleans up underlying mechanisms for reading from and writing to the file. Closes and removes the
// local temp file, and triggers a write to GCS of anything in the f.writeBuffer if it has been created.
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

		handle, err := f.getObjectHandle()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(f.fileSystem.ctx)
		defer cancel()
		w := handle.NewWriter(ctx)
		defer func() { _ = w.Close() }()
		buffer := make([]byte, utils.TouchCopyMinBufferSize)
		if _, err := io.CopyBuffer(w, f.writeBuffer, buffer); err != nil {
			// cancel context (replaces CloseWithError)
			return err
		}
	}

	f.writeBuffer = nil
	return nil
}

// Read implements the standard for io.Reader. For this to work with an GCS file, a temporary local copy of
// the file is created, and reads work on that. This file is closed and removed upon calling f.Close()
func (f *File) Read(p []byte) (n int, err error) {
	if err := f.checkTempFile(); err != nil {
		return 0, err
	}
	return f.tempFile.Read(p)
}

// Seek implements the standard for io.Seeker. A temporary local copy of the GCS file is created (the same
// one used for Reads) which Seek() acts on. This file is closed and removed upon calling f.Close()
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if err := f.checkTempFile(); err != nil {
		return 0, err
	}
	return f.tempFile.Seek(offset, whence)
}

// Write implements the standard for io.Writer. A buffer is added to with each subsequent
// write. Calling Close() will write the contents back to GCS.
func (f *File) Write(data []byte) (n int, err error) {
	if f.writeBuffer == nil {
		// note, initializing with 'data' and returning len(data), nil
		// causes issues with some Write usages, notably csv.Writer
		// so we simply initialize with no bytes and call the buffer Write after
		//
		// f.writeBuffer = bytes.NewBuffer(data)
		// return len(data), nil
		//
		// so now we do:

		f.writeBuffer = bytes.NewBuffer([]byte{})

	}
	return f.writeBuffer.Write(data)
}

// String returns the file URI string.
func (f *File) String() string {
	return f.URI()
}

// Exists returns a boolean of whether or not the object exists in GCS.
func (f *File) Exists() (bool, error) {
	_, err := f.getObjectAttrs()
	if err != nil {
		if err.Error() == doesNotExistError {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Location returns a Location instance for the file's current location.
func (f *File) Location() vfs.Location {
	return vfs.Location(&Location{
		fileSystem: f.fileSystem,
		prefix:     utils.EnsureTrailingSlash(utils.EnsureLeadingSlash(path.Clean(path.Dir(f.key)))),
		bucket:     f.bucket,
	})
}

// CopyToLocation creates a copy of *File, using the file's current name as the new file's
// name at the given location. If the given location is also GCS, the GCS API for copying
// files will be utilized, otherwise, standard io.Copy will be done to the new file.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	dest, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}
	err = f.CopyToFile(dest)
	if err != nil {
		return nil, err
	}
	return dest, nil
}

// CopyToFile puts the contents of File into the target vfs.File passed in. Uses the GCS CopierFrom
// method if the target file is also on GCS, otherwise uses io.CopyBuffer.
// This method should be called on a closed file or a file with 0 cursor position to avoid errors.
func (f *File) CopyToFile(file vfs.File) error {
	// validate seek is at 0,0 before doing copy
	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return err
	}

	// do native copy if same location/auth
	if tf, ok := file.(*File); ok {
		options, ok := tf.Location().FileSystem().(*FileSystem).options.(Options)
		if ok {
			if f.isSameAuth(&options) {
				return f.copyWithinGCSToFile(tf)
			}
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
	if cerr := file.Close(); cerr != nil {
		return cerr
	}
	// Close file (f) reader
	return f.Close()
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

// MoveToFile puts the contents of File into the target vfs.File passed in using File.CopyToFile.
// If the copy succeeds, the source file is deleted. Any errors from the copy or delete are
// returned.
func (f *File) MoveToFile(file vfs.File) error {
	if err := f.CopyToFile(file); err != nil {
		return err
	}

	return f.Delete()
}

// Delete clears any local temp file, or write buffer from read/writes to the file, then makes
// a DeleteObject call to GCS for the file. Returns any error returned by the API.
func (f *File) Delete() error {
	f.writeBuffer = nil
	if err := f.Close(); err != nil {
		return err
	}
	handle, err := f.getObjectHandle()
	if err != nil {
		return err
	}
	return handle.Delete(f.fileSystem.ctx)
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {

	// check if file exists
	exists, err := f.Exists()
	if err != nil {
		return err
	}

	// if file doesn't already exist, create it
	if !exists {
		return f.createEmptyFile()
	}

	// already exists so update it so Last-Modified is updated

	//  With versioning enabled, updates to custom meta won't update Updated date:
	//  from https://godoc.org/cloud.google.com/go/storage#ObjectAttrs:
	//        // Updated is the creation or modification time of the object.
	//        // For buckets with versioning enabled, changing an object's
	//        // metadata does not change this property. This field is read-only.
	//        Updated time.Time

	enabled, err := f.isBucketVersioningEnabled()
	if err != nil {
		return err
	}

	if enabled {
		return utils.UpdateLastModifiedByMoving(f)
	}

	return f.updateLastModifiedByAttrUpdate()
}

func (f *File) updateLastModifiedByAttrUpdate() error {

	// save original metadata (in case it was set already)
	objAttrs, err := f.getObjectAttrs()
	if err != nil {
		return err
	}
	oldMetaData := objAttrs.Metadata

	// setup dummy metadata tag and update object with it
	var updateAttrs storage.ObjectAttrsToUpdate
	updateAttrs.Metadata = map[string]string{"updateMe": "true"}

	obj, err := f.getObjectHandle()
	if err != nil {
		return err
	}

	cctx, cancel := context.WithCancel(f.fileSystem.ctx)
	defer cancel()

	_, err = obj.Update(cctx, updateAttrs)
	if err != nil {
		return err
	}

	// now switch metadata back to original values
	updateAttrs.Metadata = oldMetaData
	_, err = obj.Update(cctx, updateAttrs)
	if err != nil {
		return err
	}

	return nil
}

func (f *File) isBucketVersioningEnabled() (bool, error) {
	client, err := f.fileSystem.Client()
	if err != nil {
		return false, err
	}
	cctx, cancel := context.WithCancel(f.fileSystem.ctx)
	defer cancel()
	attrs, err := client.Bucket(f.bucket).Attrs(cctx)
	if err != nil {
		return false, err
	}
	return attrs.VersioningEnabled, nil
}

func (f *File) createEmptyFile() error {

	handle, err := f.getObjectHandle()
	if err != nil {
		return err
	}

	// write zero length file.
	ctx, cancel := context.WithCancel(f.fileSystem.ctx)
	defer cancel()

	w := handle.NewWriter(ctx)
	defer func() { _ = w.Close() }()
	if _, err := w.Write(make([]byte, 0)); err != nil {
		return err
	}

	// return early
	return nil
}

func (f *File) isSameAuth(options *Options) bool {
	// If options are nil on both sides, assume Google's default context is used in both cases.
	if options == nil && f.fileSystem.options == nil {
		return true
	}

	if options == nil || f.fileSystem.options == nil {
		return false
	}

	fOptions := f.fileSystem.options.(Options)

	if options.CredentialFile != "" && options.CredentialFile == fOptions.CredentialFile {
		return true
	}

	if options.APIKey != "" && options.APIKey == fOptions.APIKey {
		return true
	}

	return false
}

// LastModified returns the 'Updated' property from the GCS attributes.
func (f *File) LastModified() (*time.Time, error) {
	attr, err := f.getObjectAttrs()
	if err != nil {
		return nil, err
	}
	return &attr.Updated, nil
}

// Size returns the 'Size' property from the GCS attributes.
func (f *File) Size() (uint64, error) {
	attr, err := f.getObjectAttrs()
	if err != nil {
		return 0, err
	}
	return uint64(attr.Size), nil
}

// Path returns full path with leading slash of the GCS file key.
func (f *File) Path() string {
	return f.key
}

// Name returns the file name.
func (f *File) Name() string {
	return path.Base(f.key)
}

// URI returns a full GCS URI string of the file.
func (f *File) URI() string {
	return utils.GetFileURI(vfs.File(f))
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

	handle, err := f.getObjectHandle()
	if err != nil {
		return nil, err
	}

	outputReader, err := handle.NewReader(f.fileSystem.ctx)
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	if _, err := io.CopyBuffer(tmpFile, outputReader, buffer); err != nil {
		return nil, err
	}

	if err := outputReader.Close(); err != nil {
		if cerr := tmpFile.Close(); cerr != nil {
			return nil, cerr
		}
		return nil, err
	}

	// Return cursor to the beginning of the new temp file
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return nil, err
	}

	// initialize temp ReadCloser
	return tmpFile, nil
}

// getObjectHandle returns cached Object struct for file
func (f *File) getObjectHandle() (ObjectHandleCopier, error) {
	client, err := f.fileSystem.Client()
	if err != nil {
		return nil, err
	}

	handler := client.Bucket(f.bucket).Object(utils.RemoveLeadingSlash(f.key))
	return &RetryObjectHandler{Retry: f.fileSystem.Retry(), handler: handler}, nil
}

// getObjectAttrs returns the file's attributes
func (f *File) getObjectAttrs() (*storage.ObjectAttrs, error) {
	handle, err := f.getObjectHandle()
	if err != nil {
		return nil, err
	}
	return handle.Attrs(f.fileSystem.ctx)
}

func (f *File) copyWithinGCSToFile(targetFile *File) error {
	tHandle, err := targetFile.getObjectHandle()
	if err != nil {
		return err
	}
	fHandle, err := f.getObjectHandle()
	if err != nil {
		return err
	}
	// Copy content and modify metadata.
	copier := tHandle.WrappedCopierFrom(fHandle.ObjectHandle())
	attrs, gerr := f.getObjectAttrs()
	if gerr != nil {
		return gerr
	}
	copier.ContentType(attrs.ContentType)

	// Just copy content.
	_, cerr := copier.Run(f.fileSystem.ctx)
	return cerr
}
