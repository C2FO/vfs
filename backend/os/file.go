package os

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

const osCrossDeviceLinkError = "invalid cross-device link"

type opener func(filePath string) (*os.File, error)

// File implements vfs.File interface for os fs.
type File struct {
	file        *os.File
	name        string
	filesystem  *FileSystem
	cursorPos   int64
	tempFile    *os.File
	useTempFile bool
	fileOpener  opener
}

// Delete unlinks the file returning any error or nil.
func (f *File) Delete() error {
	err := os.Remove(f.Path())
	if err == nil {
		f.file = nil
	}
	return err
}

// LastModified returns the timestamp of the file's mtime or error, if any.
func (f *File) LastModified() (*time.Time, error) {
	stats, err := os.Stat(f.Path())
	if err != nil {
		return nil, err
	}

	statsTime := stats.ModTime()
	return &statsTime, err
}

// Name returns the full name of the File relative to Location.Name().
func (f *File) Name() string {
	return path.Base(f.name)
}

// Path returns the the path of the File relative to Location.Name().
func (f *File) Path() string {
	return filepath.Join(f.Location().Path(), f.Name())
}

// Size returns the size (in bytes) of the File or any error.
func (f *File) Size() (uint64, error) {
	stats, err := os.Stat(f.Path())
	if err != nil {
		return 0, err
	}

	return uint64(stats.Size()), err
}

// Close implements the io.Closer interface, closing the underlying *os.File. its an error, if any.
func (f *File) Close() error {
	f.useTempFile = false
	f.cursorPos = 0

	// check if temp file
	if f.tempFile != nil {
		// close temp (os) file
		err := f.tempFile.Close()
		if err != nil {
			return err
		}

		// get original (os) file, open it if it has not been opened
		finalFile, err := f.getInternalFile()
		if err != nil {
			return err
		}
		// rename temp file to actual file
		err = safeOsRename(f.tempFile.Name(), finalFile.Name())
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		f.tempFile = nil
	}
	if f.file == nil {
		// Do nothing on files that were never referenced
		return nil
	}

	err := f.file.Close()
	if err == nil {
		f.file = nil
	}
	return err
}

// Read implements the io.Reader interface.  It returns the bytes read and an error, if any.
func (f *File) Read(p []byte) (int, error) {

	// if we have not written to this file, ensure the original file exists
	if !f.useTempFile {
		if exists, err := f.Exists(); err != nil {
			return 0, err
		} else if !exists {
			return 0, fmt.Errorf("failed to read. File does not exist at %s", f)
		}
	}
	// get the file we need, either tempFile or original file
	useFile, err := f.getInternalFile()
	if err != nil {
		return 0, err
	}

	read, err := useFile.Read(p)
	if err != nil {
		return read, err
	}

	f.cursorPos += int64(read)

	return read, nil
}

// Seek implements the io.Seeker interface.  It accepts an offset and "whence" where 0 means relative to the origin of
// the file, 1 means relative to the current offset, and 2 means relative to the end.  It returns the new offset and
// an error, if any.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	useFile, err := f.getInternalFile()
	if err != nil {
		return 0, err
	}

	f.cursorPos, err = useFile.Seek(offset, whence)
	if err != nil {
		return 0, err
	}

	return f.cursorPos, err
}

// Exists true if the file exists on the file system, otherwise false, and an error, if any.
func (f *File) Exists() (bool, error) {
	_, err := os.Stat(f.Path())
	if err != nil {
		// file does not exist
		if os.IsNotExist(err) {
			return false, nil
		}
		// some other error
		return false, err
	}
	// file exists
	return true, nil
}

// Write implements the io.Writer interface.  It accepts a slice of bytes and returns the number of bytes written and an error, if any.
func (f *File) Write(p []byte) (n int, err error) {
	f.useTempFile = true

	useFile, err := f.getInternalFile()
	if err != nil {
		return 0, err
	}
	write, err := useFile.Write(p)
	if err != nil {
		return 0, err
	}
	offset := int64(write)
	f.cursorPos += offset

	return write, err
}

// Location returns the underlying os.Location.
func (f *File) Location() vfs.Location {
	return &Location{
		fileSystem: f.filesystem,
		name:       utils.EnsureTrailingSlash(path.Dir(f.name)),
	}
}

// MoveToFile move a file. It accepts a target vfs.File and returns an error, if any.
func (f *File) MoveToFile(file vfs.File) error {
	// validate seek is at 0,0 before doing copy
	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return err
	}
	// handle native os move/rename
	if file.Location().FileSystem().Scheme() == Scheme {
		return safeOsRename(f.Path(), file.Path())
	}

	// do copy/delete move for non-native os moves
	if _, err := f.copyWithName(file.Name(), file.Location()); err != nil {
		return err
	}
	return f.Delete()
}

// safeOsRename will attempt to do an os.Rename. If error is "invalid cross-device link" (where one OS file is on a
// different device/volume than the other), then fall back to doing a copy-delete.
func safeOsRename(srcName, dstName string) error {
	err := os.Rename(srcName, dstName)
	if err != nil {
		e, ok := err.(*os.LinkError)
		if ok && e.Err.Error() == osCrossDeviceLinkError {
			// do cross-device renaming
			if err := osCopy(srcName, dstName); err != nil {
				return err
			}
			// delete original file
			return os.Remove(srcName)
		}
		// return non-CrossDeviceLink error
		return err
	}
	return nil
}

// osCopy just io.Copy's the os files
func osCopy(srcName, dstName string) error {
	// setup os reader
	srcReader, err := os.Open(srcName) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = srcReader.Close() }()

	// setup os writer
	dstWriter, err := os.Create(dstName)
	if err != nil {
		return err
	}
	defer func() { _ = dstWriter.Close() }()

	// copy os files. Note that os.Create always does a "touch" (creates an empty file before writing data) so no need to
	// do a TouchCopy like we do with other filesystems.
	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	_, err = io.CopyBuffer(dstWriter, srcReader, buffer)
	return err
}

// MoveToLocation moves a file to a new Location. It accepts a target vfs.Location and returns a vfs.File and an error, if any.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	if location.FileSystem().Scheme() == Scheme {
		if err := ensureDir(location); err != nil {
			return nil, err
		}
	}

	// do a MoveToFile call (delegating native rename vs copy/delete logic)
	file, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}
	err = f.MoveToFile(file)
	if err != nil {
		return nil, err
	}

	// return vfs.File for newly moved file
	return location.NewFile(f.Name())
}

// CopyToFile copies the file to a new File.  It accepts a vfs.File and returns an error, if any.
func (f *File) CopyToFile(file vfs.File) error {
	// validate seek is at 0,0 before doing copy
	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return err
	}
	_, err := f.copyWithName(file.Name(), file.Location())
	return err
}

// CopyToLocation copies existing File to new Location with the same name.
// It accepts a vfs.Location and returns a vfs.File and error, if any.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	// validate seek is at 0,0 before doing copy
	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return nil, err
	}
	return f.copyWithName(f.Name(), location)
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	return utils.GetFileURI(f)
}

// String implement fmt.Stringer, returning the file's URI as the default string.
func (f *File) String() string {
	return f.URI()
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {
	exists, err := f.Exists()
	if err != nil {
		return err
	}

	if !exists {
		file, err := f.openFile()
		if err != nil {
			return err
		}
		f.file = file
		return f.Close()
	}
	now := time.Now()
	return os.Chtimes(f.Path(), now, now)
}

func (f *File) copyWithName(name string, location vfs.Location) (vfs.File, error) {
	newFile, err := location.FileSystem().NewFile(location.Volume(), path.Join(location.Path(), name))
	if err != nil {
		return nil, err
	}

	if err := utils.TouchCopyBuffered(newFile, f, utils.TouchCopyMinBufferSize); err != nil {
		return nil, err
	}
	err = f.Close()
	if err != nil {
		return nil, err
	}

	err = newFile.Close()
	if err != nil {
		return nil, err
	}
	return newFile, nil
}

func (f *File) openFile() (*os.File, error) {
	if f.file != nil {
		return f.file, nil
	}

	// replace default file opener, is set in struct
	openFunc := openOSFile
	if f.fileOpener != nil {
		openFunc = f.fileOpener
	}

	file, err := openFunc(f.Path())
	if err != nil {
		return nil, err
	}
	f.file = file

	return file, nil
}

func openOSFile(filePath string) (*os.File, error) {

	// Ensure the path exists before opening the file, NoOp if dir already exists.
	var fileMode os.FileMode = 0666
	if err := os.MkdirAll(path.Dir(filePath), os.ModeDir|0777); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, fileMode) //nolint:gosec
	return file, err
}

func ensureDir(location vfs.Location) error {
	if exists, err := location.Exists(); err != nil {
		return err
	} else if !exists {
		if err := os.MkdirAll(location.Path(), os.ModeDir|0777); err != nil {
			return err
		}
	}
	return nil
}

// If cursor is not (0,0) will copy original file to a temp file,
// opening its file descriptor to the current cursor position.
// If cursor is (0,0), just begin writing to new temp file.
// No need to copy original first.
func (f *File) getInternalFile() (*os.File, error) {
	// this is the use case of vfs.file
	if !f.useTempFile {
		if f.file == nil {
			// replace default file opener, is set in struct
			openFunc := openOSFile
			if f.fileOpener != nil {
				openFunc = f.fileOpener
			}

			finalFile, err := openFunc(f.Path())
			if err != nil {
				return nil, err
			}
			f.file = finalFile
		}
		return f.file, nil
	}
	// this is the use case of vfs.tempFile
	if f.tempFile == nil {
		localTempFile, err := f.copyToLocalTempReader()
		if err != nil {
			return nil, err
		}
		f.tempFile = localTempFile
	}

	return f.tempFile, nil
}

func (f *File) copyToLocalTempReader() (*os.File, error) {
	tmpFile, err := ioutil.TempFile("", fmt.Sprintf("%s.%d", f.Name(), time.Now().UnixNano()))
	if err != nil {
		return nil, err
	}

	openFunc := openOSFile
	if f.fileOpener != nil {
		openFunc = f.fileOpener
	}

	if _, err = openFunc(f.Path()); err != nil {
		return nil, err
	}
	// todo: editing in place logic/appending logic (see issue #42)
	// if _, err := io.Copy(tmpFile, f.file); err != nil {
	//	return nil, err
	// }
	//
	// // Return cursor to the beginning of the new temp file
	// if _, err := tmpFile.Seek(f.cursorPos, 0); err != nil {
	//	return nil, err
	// }

	return tmpFile, nil
}
