package azure

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/delete"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/utils"
)

// File implements the vfs.File interface for Azure Blob Storage
type File struct {
	location *Location
	name     string
	opts     []options.NewFileOption
	tempFile *os.File
	isDirty  bool
}

// Close cleans up all the backing data structures used for reading/writing files.  This includes closing the
// temp file, uploading the contents of the temp file to Azure Blob Storage (if necessary), and calling Seek(0, 0).
func (f *File) Close() error {
	if f.tempFile != nil {
		defer func() {
			_ = f.tempFile.Close()
			f.tempFile = nil
			f.isDirty = false
		}()

		client, err := f.location.fileSystem.Client()
		if err != nil {
			return utils.WrapCloseError(err)
		}

		if _, err := f.Seek(0, 0); err != nil {
			return utils.WrapCloseError(err)
		}

		if f.isDirty {
			var contentType string
			for _, o := range f.opts {
				switch o := o.(type) {
				case *newfile.ContentType:
					contentType = *(*string)(o)
				default:
				}
			}

			if err := client.Upload(f, f.tempFile, contentType); err != nil {
				return utils.WrapCloseError(err)
			}
		}
	}
	return nil
}

// Read implements the io.Reader interface.  For this to work with Azure Blob Storage, a temporary local copy of
// the file is created and read operations are performed against that.  The temp file is closed and flushed to Azure
// when f.Close() is called.
func (f *File) Read(p []byte) (n int, err error) {
	if err := f.checkTempFile(false); err != nil {
		return 0, utils.WrapReadError(err)
	}
	read, err := f.tempFile.Read(p)
	if err != nil {
		// if we got io.EOF, we'll return the read and the EOF error
		// because io.Copy looks for EOF to determine if it's done
		// and doesn't support error wrapping
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, utils.WrapReadError(err)
	}

	return read, nil
}

// Seek implements the io.Seeker interface.  For this to work with Azure Blob Storage, a temporary local copy of
// the file is created and operations are performed against that.  The temp file is closed and flushed to Azure
// when f.Close() is called.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if err := f.checkTempFile(false); err != nil {
		return 0, utils.WrapSeekError(err)
	}
	pos, err := f.tempFile.Seek(offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}
	return pos, nil
}

// Write implements the io.Writer interface.  Writes are performed against a temporary local file.  The temp file is
// closed and flushed to Azure with f.Close() is called.
func (f *File) Write(p []byte) (int, error) {
	if err := f.checkTempFile(true); err != nil {
		return 0, utils.WrapWriteError(err)
	}

	n, err := f.tempFile.Write(p)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	f.isDirty = true

	return n, nil
}

// String returns the file URI
func (f *File) String() string {
	return f.URI()
}

// Exists returns true/false if the file exists/does not exist on Azure
func (f *File) Exists() (bool, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return false, utils.WrapExistsError(err)
	}
	_, err = client.Properties(f.Location().Authority().String(), f.Path())
	if err != nil {
		if !bloberror.HasCode(err, bloberror.BlobNotFound) {
			return false, utils.WrapExistsError(err)
		}
		return false, nil
	}
	return true, nil
}

// Location returns a Location instance for the files current location
func (f *File) Location() vfs.Location {
	return f.location
}

// CopyToLocation creates a copy of *File, using the file's current name as the new file's
// name at the given location. If the given location is also azure, the azure API for copying
// files will be utilized, otherwise, standard io.Copy will be done to the new file.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(utils.RemoveLeadingSlash(f.Name()), f.opts...)
	if err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}

	if err := f.CopyToFile(newFile); err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}

	return newFile, nil
}

// CopyToFile puts the contents of the receiver (f *File) into the passed vfs.File parameter.
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
				err = utils.WrapCopyToFileError(wErr)
			} else if rErr != nil {
				err = utils.WrapCopyToFileError(rErr)
			}
		}
	}()

	// validate seek is at 0,0 before doing copy
	if verr := backend.ValidateCopySeekPosition(f); verr != nil {
		return utils.WrapCopyToFileError(verr)
	}

	azFile, ok := file.(*File)
	if ok {
		if f.isSameAuth(azFile) {
			client, err := f.location.fileSystem.Client()
			if err != nil {
				return utils.WrapCopyToFileError(err)
			}
			return client.Copy(f, file)
		}
	}

	// Otherwise, use TouchCopyBuffered using io.CopyBuffer
	fileBufferSize := 0

	if f.location.fileSystem.options.FileBufferSize > 0 {
		fileBufferSize = f.location.fileSystem.options.FileBufferSize
	}

	if terr := utils.TouchCopyBuffered(file, f, fileBufferSize); terr != nil {
		return utils.WrapCopyToFileError(terr)
	}

	if cerr := file.Close(); cerr != nil {
		return utils.WrapCopyToFileError(cerr)
	}

	return nil
}

// MoveToLocation copies the receiver to the passed location.  After the copy succeeds, the original is deleted.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := f.CopyToLocation(location)
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}

	return newFile, f.Delete()
}

// MoveToFile copies the receiver to the specified file and deletes the original file.
func (f *File) MoveToFile(file vfs.File) error {
	if err := f.CopyToFile(file); err != nil {
		return utils.WrapMoveToFileError(err)
	}

	return f.Delete()
}

// Delete deletes the file.
// If delete.AllVersions option is provided, each version of the file is deleted. NOTE: if soft deletion is enabled,
// it will mark all versions as soft deleted, and they will be removed by Azure as per soft deletion policy.
// Returns any error returned by the API.
func (f *File) Delete(opts ...options.DeleteOption) error {
	if err := f.Close(); err != nil {
		return utils.WrapDeleteError(err)
	}

	client, err := f.location.fileSystem.Client()
	if err != nil {
		return utils.WrapDeleteError(err)
	}

	var allVersions bool
	for _, o := range opts {
		switch o.(type) {
		case delete.AllVersions, delete.DeleteAllVersions: //nolint:staticcheck // TODO: remove when delete.DeleteAllVersions is removed
			allVersions = true
		default:
		}
	}

	if err := client.Delete(f); err != nil {
		return utils.WrapDeleteError(err)
	}

	if allVersions {
		return client.DeleteAllVersions(f)
	}

	return nil
}

// LastModified returns the last modified time as a time.Time
func (f *File) LastModified() (*time.Time, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}
	props, err := client.Properties(f.Location().Authority().String(), f.Path())
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}
	return props.LastModified, nil
}

// Size returns the size of the blob
func (f *File) Size() (uint64, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}
	props, err := client.Properties(f.Location().Authority().String(), f.Path())
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}
	return uint64(*props.Size), nil
}

// Path returns full path with leading slash.
func (f *File) Path() string {
	return f.name
}

// Name returns the name of the file
func (f *File) Name() string {
	return path.Base(f.name)
}

// Touch creates a zero-length file on the vfs.File if no File exists.  If the file exists, Touch updates the file's
// last modified parameter.
func (f *File) Touch() error {
	exists, err := f.Exists()
	if err != nil {
		return utils.WrapTouchError(err)
	}

	client, err := f.location.fileSystem.Client()
	if err != nil {
		return utils.WrapTouchError(err)
	}

	if !exists {
		var contentType string
		for _, o := range f.opts {
			switch o := o.(type) {
			case *newfile.ContentType:
				contentType = *(*string)(o)
			default:
			}
		}

		return client.Upload(f, strings.NewReader(""), contentType)
	}

	props, err := client.Properties(f.Location().Authority().String(), f.Path())
	if err != nil {
		return utils.WrapTouchError(err)
	}

	newMetadata := make(map[string]*string)
	newMetadata["updated"] = to.Ptr("true")
	if err := client.SetMetadata(f, newMetadata); err != nil {
		return utils.WrapTouchError(err)
	}

	if err := client.SetMetadata(f, props.Metadata); err != nil {
		return utils.WrapTouchError(err)
	}

	return nil
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	return utils.GetFileURI(f)
}

func (f *File) checkTempFile(isWrite bool) error {
	if f.tempFile == nil {
		client, err := f.location.fileSystem.Client()
		if err != nil {
			return err
		}

		exists, err := f.Exists()
		if err != nil {
			return err
		}

		tf, tfErr := os.CreateTemp("", fmt.Sprintf("%s.%d", path.Base(f.Name()), time.Now().UnixNano()))
		if tfErr != nil {
			return tfErr
		}
		f.tempFile = tf

		if !isWrite {
			if !exists {
				return os.ErrNotExist
			}

			reader, dlErr := client.Download(f)
			if dlErr != nil {
				return dlErr
			}

			buffer := make([]byte, utils.TouchCopyMinBufferSize)
			if _, err := io.CopyBuffer(tf, reader, buffer); err != nil {
				return err
			}

			if _, err := tf.Seek(0, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *File) isSameAuth(target *File) bool {
	sourceOptions := f.location.fileSystem.options
	targetOptions := target.location.fileSystem.options
	return sourceOptions.AccountKey == targetOptions.AccountKey
}
