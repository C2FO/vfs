package azure

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

// File implements the vfs.File interface for Azure Blob Storage
type File struct {
	fileSystem *FileSystem
	container  string
	name       string
	tempFile   *os.File
	isDirty    bool
}

// Close cleans up all of the backing data structures used for reading/writing files.  This includes, closing the
// temp file, uploading the contents of the temp file to Azure Blob Storage (if necessary), and calling Seek(0, 0).
func (f *File) Close() error {
	if f.tempFile != nil {
		defer func() {
			_ = f.tempFile.Close()
			f.tempFile = nil
			f.isDirty = false
		}()

		client, err := f.fileSystem.Client()
		if err != nil {
			return err
		}

		if _, err := f.Seek(0, 0); err != nil {
			return err
		}

		if f.isDirty {
			if err := client.Upload(f, f.tempFile); err != nil {
				return err
			}
		}
	}
	return nil
}

// Read implements the io.Reader interface.  For this to work with Azure Blob Storage, a temporary local copy of
// the file is created and read operations are performed against that.  The temp file is closed and flushed to Azure
// when f.Close() is called.
func (f *File) Read(p []byte) (n int, err error) {
	if err := f.checkTempFile(); err != nil {
		return 0, err
	}
	return f.tempFile.Read(p)
}

// Seek implements the io.Seeker interface.  For this to work with Azure Blob Storage, a temporary local copy of
// the file is created and operations are performed against that.  The temp file is closed and flushed to Azure
// when f.Close() is called.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if err := f.checkTempFile(); err != nil {
		return 0, nil
	}
	return f.tempFile.Seek(offset, whence)
}

// Write implements the io.Writer interface.  Writes are performed against a temporary local file.  The temp file is
// closed and flushed to Azure with f.Close() is called.
func (f *File) Write(p []byte) (int, error) {
	if err := f.checkTempFile(); err != nil {
		return 0, err
	}

	n, err := f.tempFile.Write(p)
	if err != nil {
		return 0, err
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
	client, err := f.fileSystem.Client()
	if err != nil {
		return false, err
	}
	_, err = client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	if err != nil {
		if err.(azblob.StorageError).ServiceCode() != "BlobNotFound" {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// Location returns a Location instance for the files current location
func (f *File) Location() vfs.Location {
	return vfs.Location(&Location{
		fileSystem: f.fileSystem,
		container:  f.container,
		path:       path.Dir(f.name),
	})
}

// CopyToLocation creates a copy of *File, using the file's current name as the new file's
// name at the given location. If the given location is also azure, the azure API for copying
// files will be utilized, otherwise, standard io.Copy will be done to the new file.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(utils.RemoveLeadingSlash(f.Name()))
	if err != nil {
		return nil, err
	}

	if err := f.CopyToFile(newFile); err != nil {
		return nil, err
	}

	return newFile, nil
}

// CopyToFile puts the contents of the receiver (f *File) into the passed vfs.File parameter.
func (f *File) CopyToFile(file vfs.File) error {
	// validate seek is at 0,0 before doing copy
	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return err
	}

	azFile, ok := file.(*File)
	if ok {
		if f.isSameAuth(azFile) {
			client, err := f.fileSystem.Client()
			if err != nil {
				return err
			}
			return client.Copy(f, file)
		}
	}

	// Otherwise, use TouchCopyBuffered using io.CopyBuffer
	fileBufferSize := 0

	if fs, ok := f.Location().FileSystem().(*FileSystem); ok {
		fileBufferSize = fs.options.FileBufferSize
	}

	if err := utils.TouchCopyBuffered(file, f, fileBufferSize); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return f.Close()
}

// MoveToLocation copies the receiver to the passed location.  After the copy succeeds, the original is deleted.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := f.CopyToLocation(location)
	if err != nil {
		return nil, err
	}

	return newFile, f.Delete()
}

// MoveToFile copies the receiver to the specified file and deletes the original file.
func (f *File) MoveToFile(file vfs.File) error {
	if err := f.CopyToFile(file); err != nil {
		return err
	}

	return f.Delete()
}

// Delete deletes the file.
func (f *File) Delete() error {
	if err := f.Close(); err != nil {
		return err
	}

	client, err := f.fileSystem.Client()
	if err != nil {
		return err
	}
	return client.Delete(f)
}

// LastModified returns the last modified time as a time.Time
func (f *File) LastModified() (*time.Time, error) {
	client, err := f.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	props, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	if err != nil {
		return nil, err
	}
	return props.LastModified, nil
}

// Size returns the size of the blob
func (f *File) Size() (uint64, error) {
	client, err := f.fileSystem.Client()
	if err != nil {
		return 0, err
	}
	props, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	if err != nil {
		return 0, err
	}
	return props.Size, nil
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
		return err
	}

	client, err := f.fileSystem.Client()
	if err != nil {
		return err
	}

	if !exists {
		return client.Upload(f, strings.NewReader(""))
	}

	props, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	if err != nil {
		return err
	}

	newMetadata := make(map[string]string)
	newMetadata["updated"] = "true"
	if err := client.SetMetadata(f, newMetadata); err != nil {
		return err
	}

	if err := client.SetMetadata(f, props.Metadata); err != nil {
		return err
	}

	return nil
}

// URI returns a full Azure URI for the file
func (f *File) URI() string {
	return fmt.Sprintf("%s://%s%s", f.fileSystem.Scheme(), utils.EnsureTrailingSlash(f.fileSystem.Host()), path.Join(f.container, f.name))
}

func (f *File) checkTempFile() error {
	if f.tempFile == nil {
		client, err := f.fileSystem.Client()
		if err != nil {
			return err
		}

		exists, err := f.Exists()
		if err != nil {
			return err
		}
		if !exists {
			tf, tfErr := ioutil.TempFile("", fmt.Sprintf("%s.%d", path.Base(f.Name()), time.Now().UnixNano()))
			if tfErr != nil {
				return tfErr
			}
			f.tempFile = tf
		} else {
			reader, dlErr := client.Download(f)
			if dlErr != nil {
				return dlErr
			}

			tf, tfErr := ioutil.TempFile("", fmt.Sprintf("%s.%d", path.Base(f.Name()), time.Now().UnixNano()))
			if tfErr != nil {
				return tfErr
			}

			buffer := make([]byte, utils.TouchCopyMinBufferSize)
			if _, err := io.CopyBuffer(tf, reader, buffer); err != nil {
				return err
			}

			if _, err := tf.Seek(0, 0); err != nil {
				return err
			}

			f.tempFile = tf
		}
	}
	return nil
}

func (f *File) isSameAuth(target *File) bool {
	sourceOptions := f.fileSystem.options
	targetOptions := target.fileSystem.options
	return sourceOptions.AccountKey == targetOptions.AccountKey
}
