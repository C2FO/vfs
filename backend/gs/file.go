package gs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"cloud.google.com/go/storage"

	"github.com/c2fo/vfs/v3"
	"github.com/c2fo/vfs/v3/utils"
)

const (
	doesNotExistError = "storage: object doesn't exist"
)

//File implements vfs.File interface for GS fs.
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
		defer f.tempFile.Close()

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
		defer func() { cancel() }()
		w := handle.NewWriter(ctx)
		defer w.Close()
		if _, err := io.Copy(w, f.writeBuffer); err != nil {
			//cancel context (replaces CloseWithError)
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

//String returns the file URI string.
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
//
// TODO should this be including trailing slash?
func (f *File) Location() vfs.Location {
	return vfs.Location(&Location{
		fileSystem: f.fileSystem,
		prefix:     utils.EnsureTrailingSlash(utils.CleanPrefix(path.Dir(f.key))),
		bucket:     f.bucket,
	})
}

// CopyToLocation creates a copy of *File, using the file's current name as the new file's
// name at the given location. If the given location is also GCS, the GCS API for copying
// files will be utilized, otherwise, standard io.Copy will be done to the new file.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	// This is a copy to gcs, from gcs, we should attempt to utilize the Google Cloud Storage API for this.
	if location.FileSystem().Scheme() == Scheme {
		dest, err := location.NewFile(f.Name())
		if err != nil {
			return nil, err
		}
		cerr := f.copyWithinGCSToFile(dest.(*File))
		if cerr != nil {
			return nil, cerr
		}
		return dest, nil
	}

	newFile, err := location.FileSystem().NewFile(location.Volume(), path.Join(location.Path(), f.Name()))
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(newFile, f); err != nil {
		return nil, err
	}
	//Close target file to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := newFile.Close(); cerr != nil {
		return nil, cerr
	}
	//Close file (f) reader
	if cerr := f.Close(); cerr != nil {
		return nil, cerr
	}
	return newFile, nil
}

// CopyToFile puts the contents of File into the targetFile passed. Uses the GCS CopierFrom
// method if the target file is also on GCS, otherwise uses io.Copy.
func (f *File) CopyToFile(targetFile vfs.File) error {
	if tf, ok := targetFile.(*File); ok {
		return f.copyWithinGCSToFile(tf)
	}

	if err := utils.TouchCopy(targetFile, f); err != nil {
		return err
	}
	//Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := targetFile.Close(); cerr != nil {
		return cerr
	}
	//Close file (f) reader
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

// MoveToFile puts the contents of File into the targetFile passed using File.CopyToFile.
// If the copy succeeds, the source file is deleted. Any errors from the copy or delete are
// returned.
func (f *File) MoveToFile(targetFile vfs.File) error {
	if err := f.CopyToFile(targetFile); err != nil {
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
	return "/" + f.key
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

	if _, err := io.Copy(tmpFile, outputReader); err != nil {
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

	//initialize temp ReadCloser
	return tmpFile, nil
}

func (c *Copier) ContentType(val string) {
	c.copier.ContentType = val
}

// getObjectHandle returns cached Object struct for file
func (f *File) getObjectHandle() (ObjectHandleCopier, error) {
	client, err := f.fileSystem.Client()
	if err != nil {
		return nil, err
	}

	handler := client.Bucket(f.bucket).Object(f.key)
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
	_, cerr := copier.Run(f.fileSystem.ctx)
	if cerr != nil {
		return cerr
	}

	// Just copy content.
	_, err = tHandle.WrappedCopierFrom(fHandle.ObjectHandle()).Run(f.fileSystem.ctx)
	return nil
}

/* private helper functions */

func newFile(fs *FileSystem, bucket, key string) (*File, error) {
	if fs == nil {
		return nil, errors.New("non-nil gs.FileSystem pointer is required")
	}
	if bucket == "" || key == "" {
		return nil, errors.New("non-empty strings for Bucket and Key are required")
	}
	key = utils.CleanPrefix(key)
	return &File{
		fileSystem: fs,
		bucket:     bucket,
		key:        key,
	}, nil
}
