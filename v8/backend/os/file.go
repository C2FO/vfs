package os

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
)

const osCrossDeviceLinkError = "invalid cross-device link"

type opener func(filePath string) (*os.File, error)

// File implements [vfs.File] for the local OS file system.
type File struct {
	file        *os.File
	location    *Location
	name        string
	cursorPos   int64
	tempFile    *os.File
	useTempFile bool
	fileOpener  opener
	seekCalled  bool
	readCalled  bool
	tempDir     string
}

// LastModified returns the timestamp of the file's mtime or error, if any.
func (f *File) LastModified() (*time.Time, error) {
	stats, err := os.Stat(osFilePath(f))
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}

	return utils.Ptr(stats.ModTime()), nil
}

// Stat implements [vfs.FileMeta.Stat].
func (f *File) Stat() (fs.FileInfo, error) {
	return os.Stat(osFilePath(f))
}

// Name returns the base name of the file path.
//
// For `file:///some/path/to/file.txt`, it would return `file.txt`
func (f *File) Name() string {
	return path.Base(f.name)
}

// Path returns absolute path, including filename,
// For `file:///some/path/to/file.txt`, it would return `/some/path/to/file.txt`
//
// If the directory portion of a file is desired, call
//
//	someFile.Location().Path()
func (f *File) Path() string {
	return path.Join(f.Location().Path(), f.Name())
}

// Authority returns the URI authority for this file (often empty for file://).
func (f *File) Authority() authority.Authority {
	return f.location.Authority()
}

// Size returns the size (in bytes) of the File or any error.
func (f *File) Size() (uint64, error) {
	stats, err := os.Stat(osFilePath(f))
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}

	return uint64(stats.Size()), nil
}

// Close implements the io.Closer interface, closing the underlying *os.File. its an error, if any.
func (f *File) Close() error {
	f.useTempFile = false
	f.cursorPos = 0
	f.seekCalled = false
	f.readCalled = false

	// check if temp file
	if f.tempFile != nil {
		// close temp (os) file
		err := f.tempFile.Close()
		if err != nil {
			return utils.WrapCloseError(err)
		}

		// get original (os) file, open it if it has not been opened
		finalFile, err := f.getInternalFile()
		if err != nil {
			return utils.WrapCloseError(err)
		}
		// rename temp file to actual file
		err = safeOsRename(f.tempFile.Name(), finalFile.Name())
		if err != nil && !os.IsNotExist(err) {
			return utils.WrapCloseError(err)
		}
		f.tempFile = nil
	}
	if f.file == nil {
		// Do nothing on files that were never referenced
		return nil
	}

	err := f.file.Close()
	if err != nil {
		return utils.WrapCloseError(err)
	}
	f.file = nil
	return nil
}

// Read implements the io.Reader interface.  It returns the bytes read and an error, if any.
func (f *File) Read(p []byte) (int, error) {
	// if we have not written to this file, ensure the original file exists
	if !f.useTempFile {
		if exists, err := f.Exists(); err != nil {
			return 0, utils.WrapReadError(err)
		} else if !exists {
			return 0, utils.WrapReadError(fmt.Errorf("failed to read. File does not exist at %s", f))
		}
	}
	// get the file we need, either tempFile or original file
	useFile, err := f.getInternalFile()
	if err != nil {
		return 0, utils.WrapReadError(err)
	}

	read, err := useFile.Read(p)
	if err != nil {
		// if we got io.EOF, we'll return the read and the EOF error
		// because io.Copy looks for EOF to determine if it's done
		// and doesn't support error wrapping
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, utils.WrapReadError(err)
	}

	f.readCalled = true
	f.cursorPos += int64(read)

	return read, nil
}

// Seek implements the io.Seeker interface.  It accepts an offset and "whence" where 0 means relative to the origin of
// the file, 1 means relative to the current offset, and 2 means relative to the end.  It returns the new offset and
// an error, if any.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	// when writing, we first write to a temp file which ensures a file isn't created before we call close.
	// However, if we've never written AND the original file doesn't exist, we can't seek.
	exists, err := f.Exists()
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}
	if !exists && !f.useTempFile {
		return 0, utils.WrapSeekError(errors.New("cannot seek on non-existent file"))
	}
	useFile, err := f.getInternalFile()
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	f.cursorPos, err = useFile.Seek(offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	f.seekCalled = true
	return f.cursorPos, err
}

// Exists true if the file exists on the file system, otherwise false, and an error, if any.
func (f *File) Exists() (bool, error) {
	_, err := os.Stat(osFilePath(f))
	if err != nil {
		// file does not exist
		if os.IsNotExist(err) {
			return false, nil
		}
		// some other error
		return false, utils.WrapExistsError(err)
	}
	// file exists
	return true, nil
}

// Write implements the io.Writer interface.  It accepts a slice of bytes and returns the number of bytes written and an error, if any.
func (f *File) Write(p []byte) (n int, err error) {
	// useTempFile prevents the immediate update of the file until we Close()
	f.useTempFile = true

	useFile, err := f.getInternalFile()
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}
	write, err := useFile.Write(p)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}
	offset := int64(write)
	f.cursorPos += offset

	return write, err
}

// Location returns the underlying [Location].
func (f *File) Location() vfs.Location {
	return f.location
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	return fmt.Sprintf("%s://%s%s", f.Location().FileSystem().Scheme(), f.Location().Authority().String(), f.Path())
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
		return utils.WrapTouchError(err)
	}

	if !exists {
		file, err := f.openFile()
		if err != nil {
			return utils.WrapTouchError(err)
		}
		f.file = file
		return f.Close()
	}
	now := time.Now()
	return os.Chtimes(osFilePath(f), now, now)
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

	file, err := openFunc(osFilePath(f))
	if err != nil {
		return nil, err
	}
	f.file = file

	return file, nil
}

func openOSFile(filePath string) (*os.File, error) {
	// Ensure the path exists before opening the file, NoOp if dir already exists.
	var fileMode os.FileMode = 0666
	if err := os.MkdirAll(filepath.Dir(filePath), os.ModeDir|0750); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, fileMode) //nolint:gosec
	return file, err
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

			finalFile, err := openFunc(osFilePath(f))
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
	filePath := osFilePath(f)

	tempDir, err := f.selectTempDir(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to select temp directory for %s: %w", filePath, err)
	}

	tmpFile, err := os.CreateTemp(tempDir, ".vfs-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file for %s: %w", filePath, err)
	}

	// Clean up temp file on any error after this point
	ok := false
	defer func() {
		if !ok {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}
	}()

	// If file exists AND we've called Seek or Read first, any subsequent writes should edit the file (temp),
	// so we copy the original file to the temp file then set the cursor position on the temp file to the current position.
	// If we're opening because Write is called first, we always overwrite the file, so no need to copy the original contents.
	//
	// So imagine we have a file with content "hello world" and we call Seek(6, 0) and then Write([]byte("there")), the
	// temp file should have "hello there" and not "there".  Then finally when Close is called, the temp file is renamed
	// to the original file.  This code ensures that scenario works as expected.
	exists, err := f.Exists()
	if err != nil {
		return nil, err
	}
	if exists && (f.seekCalled || f.readCalled) {
		openFunc := openOSFile
		if f.fileOpener != nil {
			openFunc = f.fileOpener
		}

		actualFile, err := openFunc(filePath)
		if err != nil {
			return nil, err
		}
		defer func() { _ = actualFile.Close() }()
		if _, err := io.Copy(tmpFile, actualFile); err != nil {
			return nil, err
		}

		if f.cursorPos > 0 {
			if _, err := tmpFile.Seek(f.cursorPos, 0); err != nil {
				return nil, err
			}
		}
	}

	ok = true
	return tmpFile, nil
}

// selectTempDir determines the best directory for creating a temporary file.
// Priority:
//  1. User-specified tempDir (via WithTempDir option)
//  2. System temp dir, if on the same device/volume as the target file
//  3. Target file's parent directory (last resort; may be visible to file watchers)
func (f *File) selectTempDir(targetPath string) (string, error) {
	if f.tempDir != "" {
		if err := os.MkdirAll(f.tempDir, 0750); err != nil {
			return "", fmt.Errorf("cannot create specified temp dir %s: %w", f.tempDir, err)
		}
		return f.tempDir, nil
	}

	osTempDir := os.TempDir()
	same, err := areSameVolumeOrDevice(targetPath, osTempDir)
	if err == nil && same {
		return osTempDir, nil
	}

	// Fall back to target's parent directory — guaranteed same device
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0750); err != nil {
		return "", fmt.Errorf("cannot create target directory %s: %w", targetDir, err)
	}
	return targetDir, nil
}

// areSameVolumeOrDevice checks if two paths reside on the same volume (Windows)
// or device (Unix).
func areSameVolumeOrDevice(path1, path2 string) (bool, error) {
	vol1, err := getVolumeOrDevice(path1)
	if err != nil {
		return false, err
	}
	vol2, err := getVolumeOrDevice(path2)
	if err != nil {
		return false, err
	}
	return vol1 == vol2, nil
}

// safeOsRename will attempt to do an os.Rename. If error is "invalid cross-device link" (where one OS file is on a
// different device/volume than the other), then fall back to doing a copy-delete.
func safeOsRename(srcName, dstName string) error {
	err := os.Rename(srcName, dstName)
	if err != nil {
		var e *os.LinkError
		if errors.As(err, &e) && (e.Err.Error() == osCrossDeviceLinkError ||
			(runtime.GOOS == "windows" && e.Err.Error() == "Access is denied.")) {
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
	dstWriter, err := os.Create(dstName) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = dstWriter.Close() }()

	// copy os files. Note: os.Create always does a "touch" (creates an empty file before writing data) so no need to
	// do a TouchCopy like we do with other filesystems.
	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	_, err = io.CopyBuffer(dstWriter, srcReader, buffer)
	return err
}

// osFilePath converts a VFS file path back to a native OS path.
// On Windows, the internal path /C:/foo/bar.txt becomes C:\foo\bar.txt.
func osFilePath(f *File) string {
	return toNativeOSPath(f.Path())
}

// toNativeOSPath converts an internal forward-slash path to a native OS path.
// On Windows, strips the leading "/" before a drive letter and converts slashes.
func toNativeOSPath(p string) string {
	if p == "" {
		return ""
	}
	if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

var _ vfs.File = (*File)(nil)
