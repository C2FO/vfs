package dropbox

import (
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
)

const (
	// maxSimpleUploadSize is Dropbox's limit for simple uploads (150MB)
	maxSimpleUploadSize = 150 * 1024 * 1024
)

// File implements vfs.File for Dropbox.
type File struct {
	location *Location
	path     string
	opts     []options.NewFileOption

	// State management
	cursorPos   int64
	seekCalled  bool
	readCalled  bool
	writeCalled bool
	readEOFSeen bool

	// Read buffering (for Seek support)
	reader       io.ReadCloser
	tempFileRead *os.File

	// Write buffering
	tempFileWrite *os.File
	uploadSession *string
}

// Info Functions

// LastModified returns the last modified time of the file.
func (f *File) LastModified() (*time.Time, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}

	metadata, err := client.GetMetadata(&files.GetMetadataArg{
		Path: f.path,
	})
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}

	if fileMetadata, ok := metadata.(*files.FileMetadata); ok {
		return &fileMetadata.ServerModified, nil
	}

	return nil, utils.WrapLastModifiedError(errors.New("not a file"))
}

// Name returns the base name of the file.
func (f *File) Name() string {
	return path.Base(f.path)
}

// Path returns the full path of the file.
func (f *File) Path() string {
	return utils.EnsureLeadingSlash(f.path)
}

// Exists checks if the file exists.
func (f *File) Exists() (bool, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return false, utils.WrapExistsError(err)
	}

	_, err = client.GetMetadata(&files.GetMetadataArg{
		Path: f.path,
	})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, utils.WrapExistsError(err)
	}

	return true, nil
}

// Size returns the size of the file in bytes.
func (f *File) Size() (uint64, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}

	metadata, err := client.GetMetadata(&files.GetMetadataArg{
		Path: f.path,
	})
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}

	if fileMetadata, ok := metadata.(*files.FileMetadata); ok {
		return fileMetadata.Size, nil
	}

	return 0, utils.WrapSizeError(errors.New("not a file"))
}

// Location returns the file's location.
func (f *File) Location() vfs.Location {
	return f.location
}

// URI returns the file's URI.
func (f *File) URI() string {
	return utils.GetFileURI(f)
}

// String returns the file's URI as a string.
func (f *File) String() string {
	return f.URI()
}

// Read implements io.Reader for the file.
func (f *File) Read(p []byte) (n int, err error) {
	// Handle EOF semantics
	if f.readEOFSeen {
		return 0, io.EOF
	}

	// If we have a write buffer, read from that instead of downloading
	// This handles the case where we've written to a file that doesn't exist yet in Dropbox
	if f.tempFileWrite != nil {
		// Seek to current position in write buffer
		if _, err := f.tempFileWrite.Seek(f.cursorPos, io.SeekStart); err != nil {
			return 0, utils.WrapReadError(err)
		}

		n, err := f.tempFileWrite.Read(p)
		f.cursorPos += int64(n)
		f.readCalled = true

		if err != nil && !errors.Is(err, io.EOF) {
			return n, utils.WrapReadError(err)
		}
		if errors.Is(err, io.EOF) {
			f.readEOFSeen = true
		}
		return n, err
	}

	// Ensure we have a reader (download file if needed)
	if err := f.ensureTempFileRead(); err != nil {
		return 0, utils.WrapReadError(err)
	}

	// Read from temp file
	n, err = f.tempFileRead.Read(p)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return 0, utils.WrapReadError(err)
		}
		f.readEOFSeen = true
	}

	f.cursorPos += int64(n)
	f.readCalled = true

	return n, err
}

// ensureTempFileRead downloads the file to a temp file if not already done.
// This is necessary because Dropbox doesn't support range reads.
func (f *File) ensureTempFileRead() error {
	if f.tempFileRead != nil {
		return nil
	}

	client, err := f.location.fileSystem.Client()
	if err != nil {
		return err
	}

	// Download file
	_, reader, err := client.Download(&files.DownloadArg{
		Path: f.path,
	})
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	// Create temp file
	tempDir := f.location.fileSystem.options.TempDir
	f.tempFileRead, err = os.CreateTemp(tempDir, "vfs-dropbox-read-*")
	if err != nil {
		return err
	}

	// Copy content to temp file
	if _, err := io.Copy(f.tempFileRead, reader); err != nil {
		_ = f.tempFileRead.Close()
		_ = os.Remove(f.tempFileRead.Name())
		f.tempFileRead = nil
		return err
	}

	// Seek to cursor position
	if _, err := f.tempFileRead.Seek(f.cursorPos, io.SeekStart); err != nil {
		_ = f.tempFileRead.Close()
		_ = os.Remove(f.tempFileRead.Name())
		f.tempFileRead = nil
		return err
	}

	return nil
}

// Write implements io.Writer for the file.
func (f *File) Write(data []byte) (int, error) {
	// Initialize write temp file if needed
	if err := f.ensureTempFileWrite(); err != nil {
		return 0, utils.WrapWriteError(err)
	}

	// Write to temp file
	n, err := f.tempFileWrite.Write(data)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	f.cursorPos += int64(n)
	f.writeCalled = true

	return n, nil
}

// ensureTempFileWrite creates a temp file for writing if not already done.
func (f *File) ensureTempFileWrite() error {
	if f.tempFileWrite != nil {
		return nil
	}

	tempDir := f.location.fileSystem.options.TempDir
	var err error

	// If seek was called before write, or read was called, need to download existing file first
	if f.seekCalled || f.readCalled {
		// Download existing file to temp
		if err := f.ensureTempFileRead(); err != nil {
			// File might not exist, create empty temp file
			if !isNotFoundError(err) {
				return err
			}
		}

		if f.tempFileRead != nil {
			// Use the read temp file for writing
			f.tempFileWrite = f.tempFileRead
			f.tempFileRead = nil
			return nil
		}
	}

	// Create new temp file for writing
	f.tempFileWrite, err = os.CreateTemp(tempDir, "vfs-dropbox-write-*")
	if err != nil {
		return err
	}

	// If cursor is not at 0, seek to position
	if f.cursorPos > 0 {
		if _, err := f.tempFileWrite.Seek(f.cursorPos, io.SeekStart); err != nil {
			_ = f.tempFileWrite.Close()
			_ = os.Remove(f.tempFileWrite.Name())
			f.tempFileWrite = nil
			return err
		}
	}

	return nil
}

// Seek implements io.Seeker for the file.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	// Get file size if needed
	var size uint64
	if f.writeCalled && f.tempFileWrite != nil {
		// Get size from temp file
		stat, err := f.tempFileWrite.Stat()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
		size = uint64(stat.Size())
	} else if f.readCalled && f.tempFileRead != nil {
		// Get size from temp file
		stat, err := f.tempFileRead.Stat()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
		size = uint64(stat.Size())
	} else {
		// If we haven't written or read, check if file exists
		// Seeking on a non-existent file should fail
		exists, err := f.Exists()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
		if !exists {
			return 0, utils.WrapSeekError(errors.New("cannot seek on non-existent file"))
		}

		// Get size from Dropbox
		size, err = f.Size()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	// Calculate new position
	newPos, err := utils.SeekTo(int64(size), f.cursorPos, offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	// If we have temp files, seek them
	if f.tempFileRead != nil {
		if _, err := f.tempFileRead.Seek(newPos, io.SeekStart); err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	if f.tempFileWrite != nil {
		if _, err := f.tempFileWrite.Seek(newPos, io.SeekStart); err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	f.cursorPos = newPos
	f.seekCalled = true
	f.readEOFSeen = f.cursorPos >= int64(size)

	return f.cursorPos, nil
}

// Close closes the file and uploads any buffered writes.
func (f *File) Close() error {
	var uploadErr error

	// Upload temp file if we wrote anything
	if f.writeCalled && f.tempFileWrite != nil {
		uploadErr = f.uploadToDropbox()
	}

	// Clean up temp files
	if f.tempFileRead != nil {
		_ = f.tempFileRead.Close()
		_ = os.Remove(f.tempFileRead.Name())
		f.tempFileRead = nil
	}

	if f.tempFileWrite != nil {
		_ = f.tempFileWrite.Close()
		_ = os.Remove(f.tempFileWrite.Name())
		f.tempFileWrite = nil
	}

	// Reset state
	f.cursorPos = 0
	f.seekCalled = false
	f.readCalled = false
	f.writeCalled = false
	f.readEOFSeen = false
	f.reader = nil
	f.uploadSession = nil

	if uploadErr != nil {
		return utils.WrapCloseError(uploadErr)
	}

	return nil
}

// uploadToDropbox uploads the temp file to Dropbox.
func (f *File) uploadToDropbox() error {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return err
	}

	// Seek to beginning of temp file
	if _, err := f.tempFileWrite.Seek(0, io.SeekStart); err != nil {
		return err
	}

	// Get file size
	stat, err := f.tempFileWrite.Stat()
	if err != nil {
		return err
	}

	// Use simple upload for files <= 150MB
	if stat.Size() <= maxSimpleUploadSize {
		uploadArg := files.NewUploadArg(f.path)
		uploadArg.Mode = &files.WriteMode{Tagged: dropbox.Tagged{Tag: "overwrite"}}
		_, err := client.Upload(uploadArg, f.tempFileWrite)
		return err
	}

	// Use chunked upload for larger files
	commitInfo := files.NewCommitInfo(f.path)
	commitInfo.Mode = &files.WriteMode{Tagged: dropbox.Tagged{Tag: "overwrite"}}
	return f.chunkedUpload(client, commitInfo, stat.Size())
}

// chunkedUpload uploads a large file using chunked upload sessions.
func (f *File) chunkedUpload(client Client, commitInfo *files.CommitInfo, fileSize int64) error {
	chunkSize := f.location.fileSystem.options.ChunkSize

	// Start upload session
	result, err := client.UploadSessionStart(&files.UploadSessionStartArg{}, io.LimitReader(f.tempFileWrite, chunkSize))
	if err != nil {
		return err
	}

	sessionID := result.SessionId
	offset := chunkSize

	// Upload chunks
	for offset < fileSize {
		remaining := fileSize - offset
		chunkLen := chunkSize
		if remaining < chunkSize {
			chunkLen = remaining
		}

		err := client.UploadSessionAppendV2(&files.UploadSessionAppendArg{
			Cursor: &files.UploadSessionCursor{
				SessionId: sessionID,
				Offset:    uint64(offset),
			},
		}, io.LimitReader(f.tempFileWrite, chunkLen))

		if err != nil {
			return err
		}

		offset += chunkLen
	}

	// Finish upload session
	_, err = client.UploadSessionFinish(&files.UploadSessionFinishArg{
		Cursor: &files.UploadSessionCursor{
			SessionId: sessionID,
			Offset:    uint64(offset),
		},
		Commit: commitInfo,
	}, nil)

	return err
}

// Delete deletes the file from Dropbox.
func (f *File) Delete(opts ...options.DeleteOption) error {
	if err := f.Close(); err != nil {
		return utils.WrapDeleteError(err)
	}

	client, err := f.location.fileSystem.Client()
	if err != nil {
		return utils.WrapDeleteError(err)
	}

	_, err = client.DeleteV2(&files.DeleteArg{
		Path: f.path,
	})
	if err != nil {
		return utils.WrapDeleteError(err)
	}

	return nil
}

// Touch creates an empty file or updates the timestamp of an existing file.
// Note: For existing files, this requires downloading and re-uploading, which can be expensive.
func (f *File) Touch() error {
	exists, err := f.Exists()
	if err != nil {
		return utils.WrapTouchError(err)
	}

	if !exists {
		// Create empty file
		client, err := f.location.fileSystem.Client()
		if err != nil {
			return utils.WrapTouchError(err)
		}

		uploadArg := files.NewUploadArg(f.path)
		uploadArg.Mode = &files.WriteMode{Tagged: dropbox.Tagged{Tag: "add"}}
		_, err = client.Upload(uploadArg, strings.NewReader(""))

		return utils.WrapTouchError(err)
	}

	// For existing files, download and re-upload to update timestamp
	// This is expensive but necessary as Dropbox has no touch API
	if err := f.ensureTempFileRead(); err != nil {
		return utils.WrapTouchError(err)
	}

	// Get current content and size
	stat, err := f.tempFileRead.Stat()
	if err != nil {
		return utils.WrapTouchError(err)
	}

	// Seek to beginning
	if _, err := f.tempFileRead.Seek(0, io.SeekStart); err != nil {
		return utils.WrapTouchError(err)
	}

	// Upload with explicit client_modified to force timestamp update
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return utils.WrapTouchError(err)
	}

	now := time.Now().UTC().Truncate(time.Second) // Dropbox requires UTC time with second precision (no microseconds)
	uploadArg := files.NewUploadArg(f.path)
	uploadArg.Mode = &files.WriteMode{Tagged: dropbox.Tagged{Tag: "overwrite"}}
	uploadArg.ClientModified = &now // Explicitly set client_modified to current time

	if stat.Size() <= maxSimpleUploadSize {
		_, err = client.Upload(uploadArg, f.tempFileRead)
		return utils.WrapTouchError(err)
	}

	// For large files, use session upload with client_modified
	commitInfo := files.NewCommitInfo(f.path)
	commitInfo.Mode = &files.WriteMode{Tagged: dropbox.Tagged{Tag: "overwrite"}}
	commitInfo.ClientModified = &now // Already converted to UTC above

	// Start upload session
	sessionResult, err := client.UploadSessionStart(&files.UploadSessionStartArg{}, nil)
	if err != nil {
		return utils.WrapTouchError(err)
	}
	sessionID := sessionResult.SessionId

	// Upload chunks
	chunkSize := f.location.fileSystem.options.ChunkSize
	offset := int64(0)
	for offset < stat.Size() {
		remaining := stat.Size() - offset
		chunkLen := chunkSize
		if chunkLen > remaining {
			chunkLen = remaining
		}

		err := client.UploadSessionAppendV2(&files.UploadSessionAppendArg{
			Cursor: &files.UploadSessionCursor{
				SessionId: sessionID,
				Offset:    uint64(offset),
			},
		}, io.LimitReader(f.tempFileRead, chunkLen))

		if err != nil {
			return utils.WrapTouchError(err)
		}

		offset += chunkLen
	}

	// Finish upload session with commitInfo that has ClientModified set
	_, err = client.UploadSessionFinish(&files.UploadSessionFinishArg{
		Cursor: &files.UploadSessionCursor{
			SessionId: sessionID,
			Offset:    uint64(offset),
		},
		Commit: commitInfo,
	}, nil)

	return utils.WrapTouchError(err)
}

// CopyToFile copies the file to the target file.
func (f *File) CopyToFile(targetFile vfs.File) error {
	// Validate cursor position
	if f.cursorPos != 0 {
		return vfs.ErrCopyToNotPossible
	}

	defer func() {
		_ = targetFile.Close()
		_ = f.Close()
	}()

	// If target is also Dropbox, use native copy
	if tf, ok := targetFile.(*File); ok && f.location.fileSystem == tf.location.fileSystem {
		// Check if target exists and delete it first (Dropbox copy doesn't overwrite by default)
		exists, err := tf.Exists()
		if err != nil {
			return utils.WrapCopyToFileError(err)
		}
		if exists {
			if err := tf.Delete(); err != nil {
				return utils.WrapCopyToFileError(err)
			}
		}

		client, err := f.location.fileSystem.Client()
		if err != nil {
			return utils.WrapCopyToFileError(err)
		}

		_, err = client.CopyV2(&files.RelocationArg{
			RelocationPath: files.RelocationPath{
				FromPath: f.path,
				ToPath:   tf.path,
			},
		})
		return utils.WrapCopyToFileError(err)
	}

	// Otherwise use standard copy
	if err := utils.TouchCopyBuffered(targetFile, f, 0); err != nil {
		return utils.WrapCopyToFileError(err)
	}

	return nil
}

// CopyToLocation copies the file to the target location.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}

	if err := f.CopyToFile(newFile); err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}

	return newFile, nil
}

// MoveToFile moves the file to the target file.
func (f *File) MoveToFile(targetFile vfs.File) error {
	// Validate cursor position
	if f.cursorPos != 0 {
		return vfs.ErrCopyToNotPossible
	}

	// If target is also Dropbox, use native move
	if tf, ok := targetFile.(*File); ok && f.location.fileSystem == tf.location.fileSystem {
		client, err := f.location.fileSystem.Client()
		if err != nil {
			return utils.WrapMoveToFileError(err)
		}

		_, err = client.MoveV2(&files.RelocationArg{
			RelocationPath: files.RelocationPath{
				FromPath: f.path,
				ToPath:   tf.path,
			},
		})
		return utils.WrapMoveToFileError(err)
	}

	// Otherwise copy then delete
	if err := f.CopyToFile(targetFile); err != nil {
		return utils.WrapMoveToFileError(err)
	}

	return utils.WrapMoveToFileError(f.Delete())
}

// MoveToLocation moves the file to the target location.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := f.CopyToLocation(location)
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}

	if err := f.Delete(); err != nil {
		return newFile, utils.WrapMoveToLocationError(err)
	}

	return newFile, nil
}
