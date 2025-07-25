package gs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/delete"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/utils"
)

// File implements vfs.File interface for GS fs.
type File struct {
	location *Location
	//	bucket     string
	key  string
	opts []options.NewFileOption

	// seek-related fields
	cursorPos  int64
	seekCalled bool

	// read-related fields
	reader     io.ReadCloser
	readCalled bool

	// write-related fields
	tempFileWriter *os.File
	gcsWriter      io.WriteCloser
	cancelFunc     context.CancelFunc
	writeCalled    bool
}

// Close commits any writes, either from the GCS writer stream or from a tempfile (in the case where Seek or Read are
// called after Write).  It then cleans up any open resources and resets the file's state.
func (f *File) Close() error {
	defer func() {
		// reset state
		f.reader = nil
		f.cancelFunc = nil
		f.gcsWriter = nil
		f.cursorPos = 0
		f.seekCalled = false
		f.readCalled = false
		f.writeCalled = false
	}()

	// cleanup reader (unless reader is also the writer tempfile)
	if f.reader != nil && !f.writeCalled {
		// close reader
		if err := f.reader.Close(); err != nil {
			return utils.WrapCloseError(err)
		}
	}

	// finalize writer
	if f.gcsWriter != nil {
		// close gcsWriter
		if err := f.gcsWriter.Close(); err != nil {
			return utils.WrapCloseError(err)
		}
	} else if f.tempFileWriter != nil { // gcsWriter is nil but tempFileWriter is not nil (seek after write, write after seek)
		// write tempFileWriter to gcs
		if err := f.tempToGCS(); err != nil {
			return utils.WrapCloseError(err)
		}
	}

	// cleanup tempFileWriter
	if f.tempFileWriter != nil {
		if err := f.cleanupTempFile(); err != nil {
			return utils.WrapCloseError(err)
		}
	}

	// close reader
	if f.reader != nil && !f.writeCalled {
		err := f.reader.Close()
		if err != nil {
			return utils.WrapCloseError(err)
		}
	}

	return nil
}

func (f *File) tempToGCS() error {
	handle, err := f.getObjectHandle()
	if err != nil {
		return err
	}

	w := handle.NewWriter(f.Location().FileSystem().(*FileSystem).ctx)
	defer func() { _ = w.Close() }()

	for _, o := range f.opts {
		switch o := o.(type) {
		case *newfile.ContentType:
			w.ContentType = *(*string)(o)
		default:
		}
	}

	_, err = f.tempFileWriter.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	if _, err := io.CopyBuffer(w, f.tempFileWriter, buffer); err != nil {
		return err
	}

	return nil
}

func (f *File) cleanupTempFile() error {
	if f.tempFileWriter != nil {
		err := f.tempFileWriter.Close()
		if err != nil {
			return err
		}

		err = os.Remove(f.tempFileWriter.Name())
		if err != nil {
			return err
		}

		f.tempFileWriter = nil
	}

	return nil
}

// Read implements the standard for io.Reader.
func (f *File) Read(p []byte) (n int, err error) {
	// check/initialize for reader
	r, err := f.getReader()
	if err != nil {
		return 0, utils.WrapReadError(err)
	}

	read, err := r.Read(p)
	if err != nil {
		// if we got io.EOF, we'll return the read and the EOF error
		// because io.Copy looks for EOF to determine if it's done
		// and doesn't support error wrapping
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, utils.WrapReadError(err)
	}

	f.cursorPos += int64(read)
	f.readCalled = true

	return read, nil
}

func (f *File) getReader() (io.ReadCloser, error) {
	if f.reader == nil {
		if f.writeCalled && f.tempFileWriter != nil {
			// we've edited or truncated the file, so we need to read from the temp file which should already be at the
			// current cursor position
			f.reader = f.tempFileWriter
		} else {
			// get object handle
			h, err := f.getObjectHandle()
			if err != nil {
				return nil, err
			}

			// get range reader (from current cursor position to end of file)
			reader, err := h.NewRangeReader(f.Location().FileSystem().(*FileSystem).ctx, f.cursorPos, -1)
			if err != nil {
				return nil, err
			}

			// Set the reader to the body of the object
			f.reader = reader
		}
	}
	return f.reader, nil
}

// Seek implements the standard for io.Seeker.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	// get length of file
	var length uint64
	if f.writeCalled {
		// if write has been called, then the length is the cursorPos
		length = uint64(f.cursorPos)
	} else {
		var err error
		length, err = f.Size()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	// invalidate reader (if any)
	if f.reader != nil {
		err := f.reader.Close()
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}

		f.reader = nil
	}

	// invalidate gcsWriter
	if f.gcsWriter != nil {
		// cancel gcsWriter
		f.cancelFunc()
		f.cancelFunc = nil

		f.gcsWriter = nil
	}

	// update seek position for tempFileWriter writer (if any)
	if f.tempFileWriter != nil {
		// seek tempFileWriter
		_, err := f.tempFileWriter.Seek(offset, whence)
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	// update cursorPos
	pos, err := utils.SeekTo(int64(length), f.cursorPos, offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}
	f.cursorPos = pos

	f.seekCalled = true
	return f.cursorPos, nil
}

// Write implements the standard for io.Writer.  Note that writes are not committed to GCS until CLose() is called.
func (f *File) Write(data []byte) (int, error) {
	// Here, we initialize both a tempFileWriter and a gcsWriter if they haven't been initialized yet.
	// Then write to both the local tempFileWriter and the gcsWriter stream.  We do this on the unlikely chance
	// that the file is being written to is later Seek()'d to or Read() from before Close() is called.
	// That would necessarily mean that the cursor for any later writes would change. Since we can't alter the current
	// GCS stream, we cancel it and would need to write to the tempFileWriter only.  Any later Close() would then write
	// the tempFileWriter to GCS.
	// This is a rare case, but is meant to emulate the behavior of a standard POSIX file system.
	// We might consider placing each write in a goroutine with a WaitGroup if this becomes a performance issue.

	// check/initialize for writer
	err := f.initWriters()
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	// write to tempfile
	written, err := f.tempFileWriter.Write(data)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	// write to gcs
	if f.gcsWriter != nil {
		// write to gcs
		gcsWritten, err := f.gcsWriter.Write(data)
		if err != nil {
			return 0, utils.WrapWriteError(err)
		}

		// ensure both writes are the same
		if written != gcsWritten {
			return 0, utils.WrapWriteError(errors.New("writers wrote different amounts of data"))
		}
	}

	// update cursorPos
	f.cursorPos += int64(written)
	f.writeCalled = true

	return written, nil
}

func (f *File) initWriters() error {
	if f.tempFileWriter == nil {
		// Create temp file
		tmpFile, err := os.CreateTemp("", fmt.Sprintf("vfs_gcs_%s.%d", f.Name(), time.Now().UnixNano()))
		if err != nil {
			return err
		}
		f.tempFileWriter = tmpFile
		if f.cursorPos != 0 {
			// if file exists(because cursor position is non-zero), we need to copy the existing gcsWriter file to temp
			err := f.copyToLocalTempReader(tmpFile)
			if err != nil {
				return err
			}

			// seek to cursorPos
			if _, err := f.tempFileWriter.Seek(f.cursorPos, 0); err != nil {
				return err
			}
		}
	}

	// if we haven't seeked yet, we need to get the gcsWriter
	if f.gcsWriter == nil {
		if !f.seekCalled && !f.readCalled {
			// setup cancelable context
			ctx, cancel := context.WithCancel(f.Location().FileSystem().(*FileSystem).ctx)
			f.cancelFunc = cancel

			// get object handle
			handle, err := f.getObjectHandle()
			if err != nil {
				return err
			}

			// get gcsWriter
			w := handle.NewWriter(ctx)
			if w == nil {
				return errors.New("failed to create GCS writer")
			}

			for _, o := range f.opts {
				switch o := o.(type) {
				case *newfile.ContentType:
					w.ContentType = *(*string)(o)
				default:
				}
			}

			// set gcsWriter
			f.gcsWriter = w
		}
	}

	return nil
}

// String returns the file URI string.
func (f *File) String() string {
	return f.URI()
}

// Exists returns a boolean of whether the object exists in GCS.
func (f *File) Exists() (bool, error) {
	_, err := f.getObjectAttrs()
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrObjectNotExist), errors.Is(err, storage.ErrBucketNotExist):
			// return false if object doesn't exist
			return false, nil
		}

		// return error if not a known error
		return false, utils.WrapExistsError(err)
	}

	// return true if object exists
	return true, nil
}

// Location returns a Location instance for the file's current location.
func (f *File) Location() vfs.Location {
	return f.location
}

// CopyToLocation creates a copy of *File, using the file's current name as the new file's
// name at the given location. If the given location is also GCS, the GCS API for copying
// files will be utilized, otherwise, standard io.Copy will be done to the new file.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	dest, err := location.NewFile(f.Name())
	if err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}
	err = f.CopyToFile(dest)
	if err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}
	return dest, nil
}

// CopyToFile puts the contents of File into the target vfs.File passed in. Uses the GCS CopierFrom
// method if the target file is also on GCS, otherwise uses io.CopyBuffer.
// This method should be called on a closed file or a file with 0 cursor position to avoid errors.
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
	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return utils.WrapCopyToFileError(err)
	}

	// do native copy if same location/auth
	if tf, ok := file.(*File); ok {
		// If the target file has no newfile options, use the source file's options (used if not same auth)
		if len(f.opts) == 0 {
			tf.opts = f.opts
		}

		if f.isSameAuth(&f.Location().FileSystem().(*FileSystem).options) {
			return f.copyWithinGCSToFile(tf)
		}
	}

	// Otherwise, use TouchCopyBuffered using io.CopyBuffer
	fileBufferSize := 0

	if f.Location().FileSystem().(*FileSystem).options.FileBufferSize > 0 {
		fileBufferSize = f.Location().FileSystem().(*FileSystem).options.FileBufferSize
	}

	if err := utils.TouchCopyBuffered(file, f, fileBufferSize); err != nil {
		return utils.WrapCopyToFileError(err)
	}
	// Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := file.Close(); cerr != nil {
		return utils.WrapCopyToFileError(cerr)
	}
	// Close file (f) reader
	return nil
}

// MoveToLocation works by first calling File.CopyToLocation(vfs.Location) then, if that
// succeeds, it deletes the original file, returning the new file. If the copy process fails
// the error is returned, and the Delete isn't called. If the call to Delete fails, the error
// and the file generated by the copy are both returned.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := f.CopyToLocation(location)
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}
	if delErr := f.Delete(); delErr != nil {
		return newFile, utils.WrapMoveToLocationError(delErr)
	}
	return newFile, nil
}

// MoveToFile puts the contents of File into the target vfs.File passed in using File.CopyToFile.
// If the copy succeeds, the source file is deleted. Any errors from the copy or delete are
// returned.
func (f *File) MoveToFile(file vfs.File) error {
	if err := f.CopyToFile(file); err != nil {
		return utils.WrapMoveToFileError(err)
	}

	return f.Delete()
}

// Delete clears any local temp file, or write buffer from read/writes to the file, then makes
// a DeleteObject call to GCS for the file. If delete.AllVersions option is provided,
// DeleteObject call is made to GCS for each version of the file. Returns any error returned by the API.
func (f *File) Delete(opts ...options.DeleteOption) error {
	if err := f.Close(); err != nil {
		return utils.WrapDeleteError(err)
	}

	var allVersions bool
	for _, o := range opts {
		switch o.(type) {
		case delete.AllVersions, delete.DeleteAllVersions:
			allVersions = true
		default:
		}
	}

	handle, err := f.getObjectHandle()
	if err != nil {
		return utils.WrapDeleteError(err)
	}
	err = handle.Delete(f.Location().FileSystem().(*FileSystem).ctx)
	if err != nil {
		return utils.WrapDeleteError(err)
	}

	if allVersions {
		handles, err := f.getObjectGenerationHandles()
		if err != nil {
			return utils.WrapDeleteError(err)
		}
		for _, handle := range handles {
			err := handle.Delete(f.Location().FileSystem().(*FileSystem).ctx)
			if err != nil {
				return utils.WrapDeleteError(err)
			}
		}
	}
	return nil
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {
	// check if file exists
	exists, err := f.Exists()
	if err != nil {
		return utils.WrapTouchError(err)
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
		return utils.WrapTouchError(err)
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

	cctx, cancel := context.WithCancel(f.Location().FileSystem().(*FileSystem).ctx)
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
	client, err := f.Location().FileSystem().(*FileSystem).Client()
	if err != nil {
		return false, err
	}
	cctx, cancel := context.WithCancel(f.Location().FileSystem().(*FileSystem).ctx)
	defer cancel()
	attrs, err := client.Bucket(f.Location().Authority().String()).Attrs(cctx)
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
	ctx, cancel := context.WithCancel(f.Location().FileSystem().(*FileSystem).ctx)
	defer cancel()

	w := handle.NewWriter(ctx)

	for _, o := range f.opts {
		switch o := o.(type) {
		case *newfile.ContentType:
			w.ContentType = *(*string)(o)
		default:
		}
	}

	defer func() { _ = w.Close() }()
	if _, err := w.Write(make([]byte, 0)); err != nil {
		return err
	}

	// return early
	return nil
}

func (f *File) isSameAuth(opts *Options) bool {
	fOptions := f.Location().FileSystem().(*FileSystem).options

	// If options are nil on both sides, assume Google's default context is used in both cases.
	if opts == nil && reflect.DeepEqual(fOptions, Options{}) {
		return true
	}

	if opts != nil && opts.CredentialFile != "" && opts.CredentialFile == fOptions.CredentialFile {
		return true
	}

	if opts != nil && opts.APIKey != "" && opts.APIKey == fOptions.APIKey {
		return true
	}

	return false
}

// LastModified returns the 'Updated' property from the GCS attributes.
func (f *File) LastModified() (*time.Time, error) {
	attr, err := f.getObjectAttrs()
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}
	return &attr.Updated, nil
}

// Size returns the 'Size' property from the GCS attributes.
func (f *File) Size() (uint64, error) {
	attr, err := f.getObjectAttrs()
	if err != nil {
		return 0, utils.WrapSizeError(err)
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

func (f *File) copyToLocalTempReader(tmpFile *os.File) error {
	handle, err := f.getObjectHandle()
	if err != nil {
		return err
	}

	outputReader, err := handle.NewReader(f.Location().FileSystem().(*FileSystem).ctx)
	if err != nil {
		return err
	}

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	if _, err := io.CopyBuffer(tmpFile, outputReader, buffer); err != nil {
		return err
	}

	if err := outputReader.Close(); err != nil {
		if cerr := tmpFile.Close(); cerr != nil {
			return cerr
		}
		return err
	}

	// Return cursor to the beginning of the new temp file
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return err
	}

	return nil
}

// getObjectHandle returns cached Object struct for file
func (f *File) getObjectHandle() (ObjectHandleCopier, error) {
	client, err := f.Location().FileSystem().(*FileSystem).Client()
	if err != nil {
		return nil, err
	}

	handler := client.Bucket(f.Location().Authority().String()).Object(utils.RemoveLeadingSlash(f.key))
	return &RetryObjectHandler{Retry: f.Location().FileSystem().(*FileSystem).retryer, handler: handler}, nil
}

// getObjectGenerationHandles returns Object generation structs for file
func (f *File) getObjectGenerationHandles() ([]*storage.ObjectHandle, error) {
	client, err := f.Location().FileSystem().(*FileSystem).Client()
	var handles []*storage.ObjectHandle
	if err != nil {
		return nil, err
	}
	it := client.Bucket(f.Location().Authority().String()).
		Objects(f.Location().FileSystem().(*FileSystem).ctx, &storage.Query{Versions: true, Prefix: utils.RemoveLeadingSlash(f.key)})

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		handle := client.Bucket(attrs.Bucket).Object(attrs.Name).Generation(attrs.Generation)
		handles = append(handles, handle)
	}
	return handles, err
}

// getObjectAttrs returns the file's attributes
func (f *File) getObjectAttrs() (*storage.ObjectAttrs, error) {
	handle, err := f.getObjectHandle()
	if err != nil {
		return nil, err
	}
	return handle.Attrs(f.Location().FileSystem().(*FileSystem).ctx)
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
	_, cerr := copier.Run(f.Location().FileSystem().(*FileSystem).ctx)
	return cerr
}
