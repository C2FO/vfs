package os

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
)

const osCrossDeviceLinkError = "invalid cross-device link"

type opener func(filePath string) (*os.File, error)

// File implements vfs.File interface for os fs.
type File struct {
	file        *os.File
	location    *Location
	name        string
	opts        []options.NewFileOption
	cursorPos   int64
	tempFile    *os.File
	useTempFile bool
	fileOpener  opener
	seekCalled  bool
	readCalled  bool
	tempDir     string // Custom temp dir path from FileSystem option
}

// Delete unlinks the file returning any error or nil.
func (f *File) Delete(_ ...options.DeleteOption) error {
	err := os.Remove(f.Path())
	if err == nil {
		f.file = nil
		return nil
	}
	return utils.WrapDeleteError(err)
}

// LastModified returns the timestamp of the file's mtime or error, if any.
func (f *File) LastModified() (*time.Time, error) {
	stats, err := os.Stat(f.Path())
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}

	statsTime := stats.ModTime()
	return &statsTime, nil
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

// Size returns the size (in bytes) of the File or any error.
func (f *File) Size() (uint64, error) {
	stats, err := os.Stat(f.Path())
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
		return 0, utils.WrapSeekError(err)
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
	_, err := os.Stat(f.Path())
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

// Location returns the underlying os.Location.
func (f *File) Location() vfs.Location {
	return f.location
}

// MoveToFile move a file. It accepts a target vfs.File and returns an error, if any.
func (f *File) MoveToFile(file vfs.File) error {
	if f.file != nil {
		// validate seek is at 0,0 before doing copy
		if err := backend.ValidateCopySeekPosition(f); err != nil {
			return utils.WrapMoveToFileError(err)
		}
	}
	// handle native os move/rename
	if file.Location().FileSystem().Scheme() == Scheme {
		return safeOsRename(f.Path(), file.Path())
	}

	// do copy/delete move for non-native os moves
	if _, err := f.copyWithName(file.Name(), file.Location()); err != nil {
		return utils.WrapMoveToFileError(err)
	}
	return f.Delete()
}

// safeOsRename will attempt to do an os.Rename. If error is "invalid cross-device link" (where one OS file is on a
// different device/volume than the other), then fall back to doing a copy-delete.
func safeOsRename(srcName, dstName string) error {
	err := os.Rename(srcName, dstName)
	if err != nil {
		e, ok := err.(*os.LinkError)
		if ok && (e.Err.Error() == osCrossDeviceLinkError || (runtime.GOOS == "windows" && e.Err.Error() == "Access is denied.")) {
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
			return nil, utils.WrapMoveToLocationError(err)
		}
	}

	// do a MoveToFile call (delegating native rename vs copy/delete logic)
	file, err := location.NewFile(f.Name())
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}
	err = f.MoveToFile(file)
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}

	// return vfs.File for newly moved file
	return location.NewFile(f.Name())
}

// CopyToFile copies the file to a new File.  It accepts a vfs.File and returns an error, if any.
func (f *File) CopyToFile(file vfs.File) error {
	if f.file != nil {
		// validate seek is at 0,0 before doing copy
		if err := backend.ValidateCopySeekPosition(f); err != nil {
			return utils.WrapCopyToFileError(err)
		}
	}
	_, err := f.copyWithName(file.Name(), file.Location())
	if err != nil {
		return utils.WrapCopyToFileError(err)
	}
	return nil
}

// CopyToLocation copies existing File to new Location with the same name.
// It accepts a vfs.Location and returns a vfs.File and error, if any.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	if f.file != nil {
		// validate seek is at 0,0 before doing copy
		if err := backend.ValidateCopySeekPosition(f); err != nil {
			return nil, utils.WrapCopyToLocationError(err)
		}
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
	return os.Chtimes(f.Path(), now, now)
}

func (f *File) copyWithName(name string, location vfs.Location) (vfs.File, error) {
	newFile, err := location.FileSystem().NewFile(location.Authority().String(), path.Join(location.Path(), name))
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
	if err := os.MkdirAll(filepath.Dir(filePath), os.ModeDir|0750); err != nil {
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

// copyToLocalTempReader creates a temporary file for writing operations.
// It handles finding an appropriate temp directory and copying existing file content if needed.
func (f *File) copyToLocalTempReader() (*os.File, error) {
	filePath := f.Path() // Get target path early for errors

	// Step 1: Find appropriate temp directory
	tempDir, err := f.findAppropriateTemporaryDirectory(filePath)
	if err != nil {
		return nil, err
	}

	// Step 2: Create the intermediate file in the chosen directory
	tmpFile, err := os.CreateTemp(tempDir, fmt.Sprintf("%s.%d.*.tmp", f.Name(), time.Now().UnixNano()))
	if err != nil {
		return nil, fmt.Errorf("os write error: cannot create intermediate file for %s: %w", filePath, err)
	}

	// Setup cleanup of the temp file in case of errors
	tmpFileCleanup := tmpFile // Create a copy for the defer to use
	defer func() {
		if tmpFileCleanup != nil {
			if _, statErr := os.Stat(tmpFileCleanup.Name()); statErr == nil {
				_ = os.Remove(tmpFileCleanup.Name())
			}
		}
	}()

	// Step 3: Copy existing content if needed
	if err := f.copyExistingContentIfNeeded(filePath, tmpFile); err != nil {
		return nil, err
	}

	// Success - prevent deferred cleanup from running
	tmpFileCleanup = nil
	return tmpFile, nil
}

// findAppropriateTemporaryDirectory determines the best location for a temporary file.
// It tries several locations in order of preference to find one that's suitable.
func (f *File) findAppropriateTemporaryDirectory(filePath string) (string, error) {
	// If user specified a temp directory, use that
	if f.tempDir != "" {
		return f.ensureCustomTempDir(f.tempDir)
	}

	// Try to find the best system location
	return f.findSystemTempDirectory(filePath)
}

// ensureCustomTempDir verifies a user-specified temp directory exists and is writable
func (f *File) ensureCustomTempDir(tempDir string) (string, error) {
	// Ensure the custom temp directory exists
	if mkdirErr := os.MkdirAll(tempDir, 0750); mkdirErr != nil {
		return "", fmt.Errorf("os option error: cannot create temp dir %s: %w", tempDir, mkdirErr)
	}

	// Basic check if it's writable
	testFile, testErr := os.CreateTemp(tempDir, ".vfstestwritable.*.tmp")
	if testErr != nil {
		return "", fmt.Errorf("os option error: temp dir %s not writable: %w", tempDir, testErr)
	}
	_ = testFile.Close()
	_ = os.Remove(testFile.Name())

	return tempDir, nil
}

// findSystemTempDirectory tries to find an appropriate system temp directory
func (f *File) findSystemTempDirectory(filePath string) (string, error) {
	// Try system temp dir first, but only if it's on the same device as the target
	osTempDir := os.TempDir()

	// First try: system temp if on same device
	sameAsOsTemp, checkErr := areSameVolumeOrDevice(filePath, osTempDir)
	if checkErr == nil && sameAsOsTemp {
		if isUsableDirectory(osTempDir) {
			return osTempDir, nil
		}
	}

	// Second try: parent directory of the target file
	targetDir := filepath.Dir(filePath)
	if isWritableDirectory(targetDir) {
		return targetDir, nil
	}

	// Final fallback: system temp dir, even if on a different device
	_ = os.MkdirAll(osTempDir, 0750) // Ensure it exists
	return osTempDir, nil
}

// isUsableDirectory checks if a directory exists and is usable
func isUsableDirectory(dir string) bool {
	if mkdirErr := os.MkdirAll(dir, 0750); mkdirErr == nil {
		if _, statErr := os.Stat(dir); statErr == nil {
			return true
		}
	}
	return false
}

// isWritableDirectory checks if a directory exists and is writable
func isWritableDirectory(dir string) bool {
	if _, statErr := os.Stat(dir); statErr == nil {
		// Check if dir is writable
		testFile, testErr := os.CreateTemp(dir, ".vfstestwritable.*.tmp")
		if testErr == nil {
			_ = testFile.Close()
			_ = os.Remove(testFile.Name())
			return true
		}
	}
	return false
}

// copyExistingContentIfNeeded copies content from the original file to the temp file if needed
func (f *File) copyExistingContentIfNeeded(filePath string, tmpFile *os.File) error {
	exists, err := f.Exists()
	if err != nil {
		return fmt.Errorf("os error: cannot check existence of %s: %w", filePath, err)
	}

	// Only copy if file exists AND we've called Seek or Read first
	if !exists || (!f.seekCalled && !f.readCalled) {
		return nil
	}

	// Open the original file
	openFunc := openOSFile
	if f.fileOpener != nil {
		openFunc = f.fileOpener
	}

	actualFile, err := openFunc(filePath)
	if err != nil {
		return fmt.Errorf("os write error: cannot open %s for update: %w", filePath, err)
	}
	defer func() { _ = actualFile.Close() }()

	// Copy content and set cursor position
	if err := f.copyAndSetCursorPosition(tmpFile, actualFile); err != nil {
		return err
	}

	return nil
}

// copyAndSetCursorPosition copies content from source to destination and sets the cursor position
func (f *File) copyAndSetCursorPosition(dstFile, srcFile *os.File) error {
	filePath := f.Path()

	// Rewind temp file before copying
	if _, seekErr := dstFile.Seek(0, io.SeekStart); seekErr != nil {
		return fmt.Errorf("os write error: cannot prepare %s for update (seek failed): %w", filePath, seekErr)
	}

	// Copy content
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close() // Close temp file before returning error
		return fmt.Errorf("os write error: failed copying content for %s: %w", filePath, err)
	}

	// Set cursor position
	return f.setCursorPosition(dstFile)
}

// setCursorPosition sets the cursor position in the file
func (f *File) setCursorPosition(file *os.File) error {
	filePath := f.Path()

	if f.cursorPos > 0 {
		// Match cursor position in temp file
		if _, err := file.Seek(f.cursorPos, io.SeekStart); err != nil {
			_ = file.Close() // Close file before returning error
			return fmt.Errorf("os write error: failed setting write cursor for %s: %w", filePath, err)
		}
	} else {
		// Ensure we are at the start if cursorPos is 0
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			_ = file.Close() // Close file before returning error
			return fmt.Errorf("os write error: failed setting write cursor for %s: %w", filePath, err)
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

// areSameVolumeOrDevice checks if two paths reside on the same volume (Windows)
// or device (Unix).
func areSameVolumeOrDevice(path1, path2 string) (bool, error) {
	vol1, err1 := getVolumeOrDevice(path1)
	if err1 != nil {
		return false, fmt.Errorf("could not get volume/device for %s: %w", path1, err1)
	}
	vol2, err2 := getVolumeOrDevice(path2)
	if err2 != nil {
		return false, fmt.Errorf("could not get volume/device for %s: %w", path2, err2)
	}

	return vol1 == vol2, nil
}
