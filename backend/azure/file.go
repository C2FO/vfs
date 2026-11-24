package azure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/azure/types"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/delete"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/utils"
)

// File implements the vfs.File interface for Azure Blob Storage.
type File struct {
	location *Location
	name     string
	opts     []options.NewFileOption

	// seek-related fields
	cursorPos  int64
	seekCalled bool

	// read-related fields
	reader     io.ReadCloser
	readCalled bool

	// write-related fields
	tempFileWriter     *os.File
	azWriter           *io.PipeWriter
	cancelFunc         context.CancelFunc
	azWriterCompleteCh chan error
	writeCalled        bool

	// streaming is true once a write began without a local temp file; Seek and Read are rejected until Close.
	streaming bool
	// ephemeralStream is set by internal copy helpers to stream a single sequential write without a temp file.
	ephemeralStream bool
	// writeErr latches a local write buffer failure; the upload was aborted and Close reports it without committing.
	writeErr error
}

// Close commits any writes, either from the active Azure upload stream or from the local temp file (when Seek or Read
// occurred before/after Write, or when using a custom Client that does not implement types.BlobStreamer). It then
// closes and removes any local temporary file and resets the file's session state.
func (f *File) Close() error {
	defer func() {
		if f.cancelFunc != nil {
			f.cancelFunc()
			f.cancelFunc = nil
		}
		f.reader = nil
		f.azWriter = nil
		f.azWriterCompleteCh = nil
		f.cursorPos = 0
		f.seekCalled = false
		f.readCalled = false
		f.writeCalled = false
		f.streaming = false
		f.ephemeralStream = false
		f.writeErr = nil
	}()

	// A failed local write already aborted the upload and removed the temp file; never commit a partial blob.
	if f.writeErr != nil {
		return utils.WrapCloseError(f.writeErr)
	}

	// Clean up reader unless it is the active tempFileWriter handle.
	if f.reader != nil && f.reader != f.tempFileWriter {
		if err := f.reader.Close(); err != nil {
			f.reader = nil
			f.cancelAzWriter()
			return utils.WrapCloseError(f.cleanupTempFileOnErr(err))
		}
		f.reader = nil
	}

	// Finalize writer.
	if f.azWriter != nil {
		if err := f.azWriter.Close(); err != nil {
			f.cancelAzWriter()
			return utils.WrapCloseError(f.cleanupTempFileOnErr(err))
		}
		if f.azWriterCompleteCh != nil {
			if err := <-f.azWriterCompleteCh; err != nil {
				return utils.WrapCloseError(f.cleanupTempFileOnErr(err))
			}
		}
	} else if f.tempFileWriter != nil && f.writeCalled {
		if err := f.tempToAzure(); err != nil {
			return utils.WrapCloseError(f.cleanupTempFileOnErr(err))
		}
	}

	if f.tempFileWriter != nil {
		if err := f.cleanupTempFile(); err != nil {
			return utils.WrapCloseError(err)
		}
	}

	return nil
}

func (f *File) tempToAzure() error {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return err
	}

	if _, err := f.tempFileWriter.Seek(0, io.SeekStart); err != nil {
		return err
	}

	contentType := f.resolveContentType()
	if streamer, ok := client.(types.BlobStreamer); ok {
		return streamer.UploadStream(context.Background(), f, f.tempFileWriter, contentType)
	}

	return client.Upload(f, f.tempFileWriter, contentType)
}

func (f *File) cleanupTempFile() error {
	if f.tempFileWriter != nil {
		name := f.tempFileWriter.Name()
		closeErr := f.tempFileWriter.Close()
		removeErr := os.Remove(name)
		f.tempFileWriter = nil

		return errors.Join(closeErr, removeErr)
	}

	return nil
}

func (f *File) cleanupTempFileOnErr(origErr error) error {
	if cleanupErr := f.cleanupTempFile(); cleanupErr != nil {
		return errors.Join(origErr, cleanupErr)
	}
	return origErr
}

// Read implements the io.Reader interface by streaming from Azure Blob Storage at the current cursor offset (or from
// the local temp file when reading back uncommitted buffered writes).
func (f *File) Read(p []byte) (n int, err error) {
	r, err := f.getReader()
	if err != nil {
		return 0, utils.WrapReadError(err)
	}

	read, err := r.Read(p)
	f.cursorPos += int64(read)
	f.readCalled = true

	if err != nil {
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, utils.WrapReadError(err)
	}

	return read, nil
}

func (f *File) getReader() (io.ReadCloser, error) {
	if f.writeErr != nil {
		return nil, f.writeErr
	}
	if f.streaming {
		return nil, vfs.ErrWriteBufferDisabled
	}
	if f.writeCalled && f.tempFileWriter != nil {
		if err := f.closeRemoteReader(); err != nil {
			return nil, err
		}
		f.reader = f.tempFileWriter
		return f.reader, nil
	}
	if f.reader != nil {
		return f.reader, nil
	}

	r, err := f.openRemoteReader()
	if err != nil {
		return nil, err
	}
	f.reader = r

	return f.reader, nil
}

func (f *File) openRemoteReader() (io.ReadCloser, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return nil, err
	}

	props, err := client.Properties(f.Location().Authority().String(), f.Path())
	if err != nil {
		return nil, mapBlobError(err)
	}

	if props != nil && props.Size != nil && (*props.Size == 0 || f.cursorPos >= *props.Size) {
		return io.NopCloser(strings.NewReader("")), nil
	}

	if streamer, ok := client.(types.BlobStreamer); ok {
		r, err := streamer.DownloadRange(context.Background(), f, f.cursorPos)
		if err != nil {
			return nil, mapBlobError(err)
		}
		return r, nil
	}

	return f.downloadWithOffset(client)
}

func (f *File) downloadWithOffset(client types.Client) (io.ReadCloser, error) {
	r, err := client.Download(f)
	if err != nil {
		return nil, mapBlobError(err)
	}
	if f.cursorPos > 0 {
		if _, err := io.CopyN(io.Discard, r, f.cursorPos); err != nil {
			_ = r.Close()
			if errors.Is(err, io.EOF) {
				return io.NopCloser(strings.NewReader("")), nil
			}
			return nil, err
		}
	}
	return r, nil
}

func mapBlobError(err error) error {
	if bloberror.HasCode(err, bloberror.BlobNotFound) {
		return os.ErrNotExist
	}
	return err
}

// Seek implements the io.Seeker interface without downloading the blob to a local temporary file.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.writeErr != nil {
		return 0, utils.WrapSeekError(f.writeErr)
	}
	// Without a local buffer the written bytes cannot be revisited; reject before touching the active upload.
	if f.streaming {
		return 0, utils.WrapSeekError(vfs.ErrWriteBufferDisabled)
	}

	length, err := f.seekLength()
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	pos, err := utils.SeekTo(int64(length), f.cursorPos, offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	if err := f.closeRemoteReader(); err != nil {
		return 0, utils.WrapSeekError(err)
	}

	f.cancelAzWriter()

	if f.tempFileWriter != nil {
		if _, err := f.tempFileWriter.Seek(pos, io.SeekStart); err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	f.cursorPos = pos
	f.seekCalled = true

	return f.cursorPos, nil
}

func (f *File) seekLength() (uint64, error) {
	switch {
	case f.writeCalled && f.tempFileWriter != nil:
		fi, err := f.tempFileWriter.Stat()
		if err != nil {
			return 0, err
		}
		return uint64(fi.Size()), nil
	case f.writeCalled:
		return uint64(f.cursorPos), nil
	default:
		return f.blobSize()
	}
}

func (f *File) closeRemoteReader() error {
	if f.reader == nil {
		return nil
	}
	if f.reader != f.tempFileWriter {
		if err := f.reader.Close(); err != nil {
			return err
		}
	}
	f.reader = nil
	return nil
}

func (f *File) cancelAzWriter() {
	if f.azWriter == nil {
		return
	}
	if f.cancelFunc != nil {
		f.cancelFunc()
		f.cancelFunc = nil
	}
	_ = f.azWriter.CloseWithError(context.Canceled)
	f.azWriter = nil
	if f.azWriterCompleteCh != nil {
		<-f.azWriterCompleteCh //nolint:errcheck // upload was intentionally canceled on Seek
		f.azWriterCompleteCh = nil
	}
}

// Write implements the io.Writer interface. Writes are not committed to Azure Blob Storage until Close() is called.
func (f *File) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, utils.WrapWriteError(f.writeErr)
	}

	if err := f.initWriters(); err != nil {
		return 0, utils.WrapWriteError(err)
	}

	// Streaming mode: write straight to the Azure upload pipe without a local temp file.
	if f.tempFileWriter == nil {
		written, err := f.azWriter.Write(p)
		f.cursorPos += int64(written)
		if err != nil {
			return written, utils.WrapWriteError(err)
		}
		f.writeCalled = true

		return written, nil
	}

	written, err := f.tempFileWriter.Write(p)
	if err != nil {
		return 0, utils.WrapWriteError(f.abortWrite(errLocalWriteBuffer(err)))
	}

	if f.azWriter != nil {
		azWritten, err := f.azWriter.Write(p)
		if err != nil {
			return 0, utils.WrapWriteError(err)
		}
		if written != azWritten {
			return 0, utils.WrapWriteError(
				fmt.Errorf("local write and azure write are different sizes: local=%d, azure=%d", written, azWritten),
			)
		}
	}

	f.cursorPos += int64(written)
	f.writeCalled = true

	return written, nil
}

// abortWrite tears down an in-flight write after a local buffer or copy failure: it cancels the upload so no partial
// blob is committed, removes the temp file, and latches err so later operations and Close report it.
func (f *File) abortWrite(err error) error {
	hadWriter := f.azWriter != nil
	if f.cancelFunc != nil {
		f.cancelFunc()
		f.cancelFunc = nil
	}
	if f.azWriter != nil {
		_ = f.azWriter.CloseWithError(err)
		f.azWriter = nil
	}
	if hadWriter && f.azWriterCompleteCh != nil {
		<-f.azWriterCompleteCh //nolint:errcheck // upload was just canceled, so its error is expected
		f.azWriterCompleteCh = nil
	}
	if r, ok := f.reader.(*os.File); ok && r == f.tempFileWriter {
		f.reader = nil
	}
	if f.reader != nil {
		if closeErr := f.reader.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		f.reader = nil
	}
	if cleanupErr := f.cleanupTempFile(); cleanupErr != nil {
		err = errors.Join(err, cleanupErr)
	}
	f.writeErr = err

	return err
}

func (f *File) selectWriteMode() error {
	if f.tempFileWriter != nil || f.streaming {
		return nil
	}
	explicitlyUnbuffered := !newfile.UseWriteBuffer(!f.location.fileSystem.options.DisableWriteBuffer, f.opts)
	if !explicitlyUnbuffered && !f.ephemeralStream {
		return nil
	}
	if f.seekCalled || f.readCalled {
		return vfs.ErrWriteBufferDisabled
	}

	client, err := f.location.fileSystem.Client()
	if err != nil {
		return err
	}
	if _, ok := client.(types.BlobStreamer); !ok {
		if explicitlyUnbuffered {
			return errors.New("streaming writes require an azure Client that implements types.BlobStreamer")
		}
		f.ephemeralStream = false
		return nil
	}

	f.streaming = true
	return nil
}

func (f *File) initWriters() error {
	if err := f.selectWriteMode(); err != nil {
		return err
	}

	if f.tempFileWriter == nil && !f.streaming {
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("vfs_azure_%s.%d", path.Base(f.Name()), time.Now().UnixNano()))
		if err != nil {
			return f.abortWrite(errLocalWriteBuffer(err))
		}
		f.tempFileWriter = tmpFile

		// If Seek or Read preceded this first Write, close any open remote reader, download the existing blob
		// into the temp file, and reposition to cursorPos so the write edits rather than replaces the blob.
		if f.seekCalled || f.readCalled {
			if err := f.closeRemoteReader(); err != nil {
				return f.cleanupTempFileOnErr(err)
			}
			if err := f.copyToLocalTempReader(tmpFile); err != nil {
				return f.cleanupTempFileOnErr(err)
			}
			if _, err := f.tempFileWriter.Seek(f.cursorPos, io.SeekStart); err != nil {
				return f.cleanupTempFileOnErr(err)
			}
		}
	}

	if f.azWriter == nil && !f.seekCalled && !f.readCalled {
		client, err := f.location.fileSystem.Client()
		if err != nil {
			f.streaming = false
			return f.cleanupTempFileOnErr(err)
		}
		if streamer, ok := client.(types.BlobStreamer); ok {
			f.azWriter = f.startAzWriter(streamer)
		}
	}

	return nil
}

func (f *File) startAzWriter(streamer types.BlobStreamer) *io.PipeWriter {
	ctx, cancel := context.WithCancel(context.Background())
	f.cancelFunc = cancel
	completeCh := make(chan error, 1)
	f.azWriterCompleteCh = completeCh
	pr, pw := io.Pipe()
	contentType := f.resolveContentType()

	go func() {
		err := streamer.UploadStream(ctx, f, pr, contentType)
		if err != nil {
			_ = pr.CloseWithError(err)
		}
		completeCh <- err
	}()

	return pw
}

func (f *File) copyToLocalTempReader(tmpFile *os.File) error {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return err
	}

	var reader io.ReadCloser
	if streamer, ok := client.(types.BlobStreamer); ok {
		reader, err = streamer.DownloadRange(context.Background(), f, 0)
	} else {
		reader, err = client.Download(f)
	}
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return os.ErrNotExist
		}
		return err
	}

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	if _, err := io.CopyBuffer(tmpFile, reader, buffer); err != nil {
		_ = reader.Close()
		return err
	}

	if err := reader.Close(); err != nil {
		return err
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return err
	}

	return nil
}

// String returns the file URI.
func (f *File) String() string {
	return f.URI()
}

// Exists returns true/false if the file exists/does not exist on Azure.
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

// Location returns a Location instance for the file's current location.
func (f *File) Location() vfs.Location {
	return f.location
}

// CopyToLocation creates a copy of *File, using the file's current name as the new file's
// name at the given location. If the given location is also azure, the azure API for copying
// files will be utilized, otherwise, standard io.Copy will be done to the new file.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	var newFileOpts []options.NewFileOption
	if ct := contentTypeOption(f.opts); ct != nil {
		newFileOpts = append(newFileOpts, ct)
	}

	newFile, err := location.NewFile(utils.RemoveLeadingSlash(f.Name()), newFileOpts...)
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
	defer func() {
		wErr := file.Close()
		rErr := f.Close()
		if err == nil {
			if wErr != nil {
				err = utils.WrapCopyToFileError(wErr)
			} else if rErr != nil {
				err = utils.WrapCopyToFileError(rErr)
			}
		}
	}()

	if verr := backend.ValidateCopySeekPosition(f); verr != nil {
		return utils.WrapCopyToFileError(verr)
	}

	if azFile, ok := file.(*File); ok {
		if f.isSameAuth(azFile) {
			client, err := f.location.fileSystem.Client()
			if err != nil {
				return utils.WrapCopyToFileError(err)
			}
			return client.Copy(f, file)
		}

		// Cross-auth copy streams through TouchCopyBuffered: if the target has no ContentType option of its own,
		// inherit any ContentType from the source for the duration of this copy without mutating azFile.opts
		// permanently or copying non-metadata options such as WithWriteBuffer/WithStreamingWrite.
		if contentTypeOption(azFile.opts) == nil {
			if ct := contentTypeOption(f.opts); ct != nil {
				origOpts := azFile.opts
				azFile.opts = append(slices.Clone(azFile.opts), ct)
				defer func() { azFile.opts = origOpts }()
			}
		}
	}

	fileBufferSize := max(f.location.fileSystem.options.FileBufferSize, 0)

	if terr := utils.TouchCopyBuffered(file, f, fileBufferSize); terr != nil {
		return utils.WrapCopyToFileError(terr)
	}

	if cerr := file.Close(); cerr != nil {
		return utils.WrapCopyToFileError(cerr)
	}

	return nil
}

// MoveToLocation copies the receiver to the passed location. After the copy succeeds, the original is deleted.
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

// LastModified returns the last modified time as a time.Time.
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

// Size returns the size of the blob.
func (f *File) Size() (uint64, error) {
	size, err := f.blobSize()
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}
	return size, nil
}

func (f *File) blobSize() (uint64, error) {
	client, err := f.location.fileSystem.Client()
	if err != nil {
		return 0, err
	}
	props, err := client.Properties(f.Location().Authority().String(), f.Path())
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return 0, os.ErrNotExist
		}
		return 0, err
	}
	if props == nil || props.Size == nil {
		return 0, nil
	}
	return uint64(*props.Size), nil
}

// Path returns full path with leading slash.
func (f *File) Path() string {
	return f.name
}

// Name returns the name of the file.
func (f *File) Name() string {
	return path.Base(f.name)
}

// Touch creates a zero-length file on the vfs.File if no File exists. If the file exists, Touch updates the file's
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
		return client.Upload(f, strings.NewReader(""), f.resolveContentType())
	}

	props, err := client.Properties(f.Location().Authority().String(), f.Path())
	if err != nil {
		return utils.WrapTouchError(err)
	}

	newMetadata := make(map[string]*string)
	newMetadata["updated"] = new("true")
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

func (f *File) resolveContentType() string {
	if ct := contentTypeOption(f.opts); ct != nil {
		return string(*ct)
	}
	return ""
}

func contentTypeOption(opts []options.NewFileOption) *newfile.ContentType {
	var ct *newfile.ContentType
	for _, o := range opts {
		if c, ok := o.(*newfile.ContentType); ok {
			ct = c
		}
	}
	return ct
}

func (f *File) isSameAuth(target *File) bool {
	sourceOptions := f.location.fileSystem.options
	targetOptions := target.location.fileSystem.options
	return sourceOptions.AccountKey == targetOptions.AccountKey
}
