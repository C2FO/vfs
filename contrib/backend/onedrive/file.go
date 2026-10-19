package onedrive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
)

const (
	// maxSimpleUploadSize is Graph's limit for a single-request upload.
	maxSimpleUploadSize = 4 * 1024 * 1024

	// uploadChunkMultiple is the granularity Graph requires for every upload-session chunk except the last.
	uploadChunkMultiple = 320 * 1024
)

// File implements vfs.File for OneDrive.
//
// Reads and writes go through a local temp file. Reading or seeking first downloads the current
// content so that later writes modify it in place; writing first starts from empty, replacing the
// file. Close uploads the temp file if anything was written.
type File struct {
	location *Location
	path     string
	opts     []options.NewFileOption

	cursorPos   int64
	seekCalled  bool
	readCalled  bool
	writeCalled bool

	tempFile *os.File
}

// LastModified returns the last modified time of the file.
func (f *File) LastModified() (*time.Time, error) {
	item, err := f.getItem()
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}

	return &item.LastModified, nil
}

// Name returns the base name of the file.
func (f *File) Name() string {
	return path.Base(f.path)
}

// Path returns the full path of the file.
func (f *File) Path() string {
	return utils.EnsureLeadingSlash(f.path)
}

// Exists reports whether the file exists. A folder at the same path is not a file.
func (f *File) Exists() (bool, error) {
	item, err := f.getItem()
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, utils.WrapExistsError(err)
	}

	return !item.IsFolder, nil
}

// Size returns the size of the file in bytes.
func (f *File) Size() (uint64, error) {
	item, err := f.getItem()
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}

	return uint64(item.Size), nil
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

// Read implements io.Reader.
func (f *File) Read(p []byte) (int, error) {
	if err := f.ensureTempFile(true, false); err != nil {
		return 0, utils.WrapReadError(err)
	}

	n, err := f.tempFile.Read(p)
	f.cursorPos += int64(n)
	f.readCalled = true

	if err != nil && !errors.Is(err, io.EOF) {
		return n, utils.WrapReadError(err)
	}

	return n, err
}

// Write implements io.Writer.
func (f *File) Write(data []byte) (int, error) {
	if err := f.ensureTempFile(f.seekCalled || f.readCalled, true); err != nil {
		return 0, utils.WrapWriteError(err)
	}

	n, err := f.tempFile.Write(data)
	f.cursorPos += int64(n)
	f.writeCalled = true

	if err != nil {
		return n, utils.WrapWriteError(err)
	}

	return n, nil
}

// Seek implements io.Seeker.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	var size int64
	if f.tempFile != nil {
		stat, err := f.tempFile.Stat()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
		size = stat.Size()
	} else {
		item, err := f.getItem()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
		size = item.Size
	}

	newPos, err := utils.SeekTo(size, f.cursorPos, offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	if f.tempFile != nil {
		if _, err := f.tempFile.Seek(newPos, io.SeekStart); err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	f.cursorPos = newPos
	f.seekCalled = true

	return f.cursorPos, nil
}

// Close uploads any written content and releases local resources.
func (f *File) Close() error {
	var uploadErr error
	if f.writeCalled && f.tempFile != nil {
		uploadErr = f.upload(f.tempFile)
	}

	cleanupErr := f.reset()

	if uploadErr != nil {
		return utils.WrapCloseError(uploadErr)
	}
	if cleanupErr != nil {
		return utils.WrapCloseError(cleanupErr)
	}

	return nil
}

// Delete deletes the file, discarding any unwritten local changes.
func (f *File) Delete(_ ...options.DeleteOption) error {
	if err := f.reset(); err != nil {
		return utils.WrapDeleteError(err)
	}

	client, err := f.location.fileSystem.Client()
	if err != nil {
		return utils.WrapDeleteError(err)
	}

	if err := client.Delete(context.Background(), f.oneDrivePath()); err != nil {
		return utils.WrapDeleteError(err)
	}

	return nil
}

// Touch creates an empty file, or re-uploads an existing file's content so that OneDrive records a
// new modification time (Graph has no "touch" operation).
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
		_, err := client.Upload(context.Background(), f.oneDrivePath(), bytes.NewReader(nil))
		return utils.WrapTouchError(err)
	}

	tmp, err := f.downloadToTemp(client)
	if err != nil {
		return utils.WrapTouchError(err)
	}
	defer func() { _ = removeTemp(tmp) }()

	return utils.WrapTouchError(f.upload(tmp))
}

// CopyToFile copies the file to the target file.
func (f *File) CopyToFile(targetFile vfs.File) (err error) {
	if f.cursorPos != 0 {
		return vfs.ErrCopyToNotPossible
	}

	defer func() {
		if cerr := targetFile.Close(); cerr != nil && err == nil {
			err = utils.WrapCopyToFileError(cerr)
		}
		if cerr := f.Close(); cerr != nil && err == nil {
			err = utils.WrapCopyToFileError(cerr)
		}
	}()

	if tf, ok := f.nativeTarget(targetFile); ok {
		client, err := f.location.fileSystem.Client()
		if err != nil {
			return utils.WrapCopyToFileError(err)
		}

		err = client.Copy(context.Background(), f.oneDrivePath(), tf.location.oneDrivePath(), tf.Name())
		if err == nil {
			return nil
		}
		// Copy needs an existing destination folder; a streamed upload creates missing folders.
		if !errors.Is(err, ErrNotFound) {
			return utils.WrapCopyToFileError(err)
		}
	}

	if err := utils.TouchCopyBuffered(targetFile, f, 0); err != nil {
		return utils.WrapCopyToFileError(err)
	}

	return nil
}

// CopyToLocation copies the file to the target location, keeping its name.
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
	if f.cursorPos != 0 {
		return vfs.ErrCopyToNotPossible
	}

	if tf, ok := f.nativeTarget(targetFile); ok {
		client, err := f.location.fileSystem.Client()
		if err != nil {
			return utils.WrapMoveToFileError(err)
		}

		_, err = client.Move(context.Background(), f.oneDrivePath(), tf.location.oneDrivePath(), tf.Name())
		if err == nil {
			return nil
		}
		// Move needs an existing destination folder; copy-then-delete creates missing folders.
		if !errors.Is(err, ErrNotFound) {
			return utils.WrapMoveToFileError(err)
		}
	}

	if err := f.CopyToFile(targetFile); err != nil {
		return utils.WrapMoveToFileError(err)
	}

	if err := f.Delete(); err != nil {
		return utils.WrapMoveToFileError(err)
	}

	return nil
}

// MoveToLocation moves the file to the target location, keeping its name.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}

	if err := f.MoveToFile(newFile); err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}

	return newFile, nil
}

// nativeTarget returns the target as a *File when a server-side copy or move is possible: same
// FileSystem (so same drive and credentials) and no unsaved local state on either file.
func (f *File) nativeTarget(target vfs.File) (*File, bool) {
	tf, ok := target.(*File)
	if !ok || tf.location.fileSystem != f.location.fileSystem {
		return nil, false
	}
	if f.tempFile != nil || tf.tempFile != nil {
		return nil, false
	}
	return tf, true
}

// ensureTempFile creates the local temp file, seeded with the remote content when download is true.
// When allowMissing is true, a remote file that does not exist seeds an empty temp file.
func (f *File) ensureTempFile(download, allowMissing bool) error {
	if f.tempFile != nil {
		return nil
	}

	var tmp *os.File
	if download {
		client, err := f.location.fileSystem.Client()
		if err != nil {
			return err
		}

		tmp, err = f.downloadToTemp(client)
		if err != nil && (!allowMissing || !errors.Is(err, ErrNotFound)) {
			return err
		}
	}

	if tmp == nil {
		var err error
		tmp, err = os.CreateTemp(f.location.fileSystem.options.TempDir, "vfs-onedrive-*")
		if err != nil {
			return fmt.Errorf("onedrive: creating temp file: %w", err)
		}
	}

	if _, err := tmp.Seek(f.cursorPos, io.SeekStart); err != nil {
		_ = removeTemp(tmp)
		return fmt.Errorf("onedrive: positioning temp file: %w", err)
	}

	f.tempFile = tmp
	return nil
}

func (f *File) downloadToTemp(client Client) (*os.File, error) {
	rc, err := client.Download(context.Background(), f.oneDrivePath())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	tmp, err := os.CreateTemp(f.location.fileSystem.options.TempDir, "vfs-onedrive-*")
	if err != nil {
		return nil, fmt.Errorf("onedrive: creating temp file: %w", err)
	}

	if _, err := io.Copy(tmp, rc); err != nil {
		_ = removeTemp(tmp)
		return nil, fmt.Errorf("onedrive: downloading %s: %w", f.Path(), err)
	}

	return tmp, nil
}

// upload replaces the remote file with the full content of src, using an upload session when the
// content exceeds Graph's single-request limit.
func (f *File) upload(src *os.File) error {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return err
	}

	stat, err := src.Stat()
	if err != nil {
		return fmt.Errorf("onedrive: reading temp file size: %w", err)
	}

	if stat.Size() <= maxSimpleUploadSize {
		_, err := client.Upload(context.Background(), f.oneDrivePath(), src)
		return err
	}

	return f.chunkedUpload(client, src, stat.Size())
}

func (f *File) chunkedUpload(client Client, src *os.File, size int64) error {
	ctx := context.Background()

	session, err := client.CreateUploadSession(ctx, f.oneDrivePath())
	if err != nil {
		return err
	}

	chunk := make([]byte, chunkSize(f.location.fileSystem.options.ChunkSize))
	for offset := int64(0); offset < size; {
		n, err := src.ReadAt(chunk, offset)
		if err != nil && !errors.Is(err, io.EOF) {
			_ = session.Cancel(ctx)
			return fmt.Errorf("onedrive: reading temp file: %w", err)
		}
		if n == 0 {
			_ = session.Cancel(ctx)
			return fmt.Errorf("onedrive: temp file shorter than expected %d bytes", size)
		}

		if _, err := session.UploadChunk(ctx, chunk[:n], offset, size); err != nil {
			_ = session.Cancel(ctx)
			return err
		}
		offset += int64(n)
	}

	return nil
}

// chunkSize rounds the configured chunk size down to a multiple of 320 KiB, with a floor of 320 KiB.
func chunkSize(configured int64) int64 {
	return max(configured/uploadChunkMultiple, 1) * uploadChunkMultiple
}

// reset discards local state without uploading.
func (f *File) reset() error {
	var err error
	if f.tempFile != nil {
		err = removeTemp(f.tempFile)
		f.tempFile = nil
	}

	f.cursorPos = 0
	f.seekCalled = false
	f.readCalled = false
	f.writeCalled = false

	return err
}

func removeTemp(tmp *os.File) error {
	closeErr := tmp.Close()
	if err := os.Remove(tmp.Name()); err != nil {
		return fmt.Errorf("onedrive: removing temp file: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("onedrive: closing temp file: %w", closeErr)
	}
	return nil
}

func (f *File) getItem() (*DriveItem, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return nil, err
	}

	return client.GetItem(context.Background(), f.oneDrivePath())
}

// oneDrivePath returns the file path in Client form, without a leading slash.
func (f *File) oneDrivePath() string {
	return strings.TrimPrefix(f.path, "/")
}
