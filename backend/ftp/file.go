package ftp

import (
	"io"
	"os"
	"path"
	"strconv"
	"time"

	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/utils"
)

type OpenType int

const (
	openRead OpenType = iota
	openWrite
)

// File implements vfs.File interface for FTP fs.
type File struct {
	fileSystem *FileSystem
	Authority  utils.Authority
	path       string
	dataconn   *dataConn
	offset     int64
}

// Info Functions

// LastModified returns the LastModified property of ftp file.
func (f *File) LastModified() (*time.Time, error) {

	entry, err := f.stat()
	if err != nil {
		return nil, err
	}
	t := entry.Time
	return &t, nil
}

func (f *File) stat() (*_ftp.Entry, error) {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return nil, err
	}
	entries, err := client.List(f.Path())
	if err != nil {
		return nil, err
	}
	return entries[0], nil
}

// Name returns the path portion of the file's path property. IE: "file.txt" of "ftp://someuser@host.com/some/path/to/file.txt
func (f *File) Name() string {
	return path.Base(f.path)
}

// Path return the directory portion of the file's path. IE: "path/to" of "ftp://someuser@host.com/some/path/to/file.txt
func (f *File) Path() string {
	return utils.EnsureLeadingSlash(f.path)
}

// Exists returns a boolean of whether or not the file exists on the ftp server
func (f *File) Exists() (bool, error) {

	_, err := f.stat()
	if err != nil && err == os.ErrNotExist {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {
	exists, err := f.Exists()
	if err != nil {
		return err
	}

	if !exists {
		_, err := f.Write([]byte{})
		if err != nil {
			return err
		}
		return f.Close()
	}

	// doing move and move back (rename)...TODO might need to do copy-delete - copy-delete

	now := time.Now()
	newFile, err := f.Location().NewFile(f.Name() + strconv.FormatInt(now.UnixNano(), 10))
	if err != nil {
		return err
	}

	err = f.MoveToFile(newFile)
	if err != nil {
		return err
	}

	return newFile.MoveToFile(f)
}

// Size returns the size of the remote file.
func (f *File) Size() (uint64, error) {

	entry, err := f.stat()
	if err != nil {
		return 0, err
	}
	return entry.Size, nil
}

// Location returns a vfs.Location at the location of the file. IE: if file is at
// ftp://someuser@host.com/here/is/the/file.txt the location points to ftp://someuser@host.com/here/is/the/
func (f *File) Location() vfs.Location {
	return &Location{
		fileSystem: f.fileSystem,
		path:       path.Dir(f.path),
		Authority:  f.Authority,
	}
}

// Move/Copy Operations

// MoveToFile puts the contents of File into the targetFile passed using File.CopyToFile.
// If the copy succeeds, the source file is deleted. Any errors from the copy or delete are
// returned.
// If the given location is also ftp AND for the same user and host, the ftp Rename method is used, otherwise
// we'll do a an io.Copy to the destination file then delete source file.
func (f *File) MoveToFile(t vfs.File) error {
	// ftp rename if vfs is ftp and for the same user/host
	if f.fileSystem.Scheme() == t.Location().FileSystem().Scheme() &&
		f.Authority.User == t.(*File).Authority.User &&
		f.Authority.Host == t.(*File).Authority.Host {

		client, err := f.fileSystem.Client(f.Authority)
		if err != nil {
			return err
		}

		// TODO ensure we have test for if renaming to new path that doesn't exist.
		// If so, we don't need to check exists and do mkdir, just rename

		// ensure destination exists before moving
		exists, err := t.Location().Exists()
		if err != nil {
			return err
		}

		if !exists {
			// it doesn't matter which client we use since they are effectively the same
			err = client.MakeDir(t.Location().Path())
			if err != nil {
				return err
			}
		}
		return client.Rename(t.Path(), f.Path())
	}

	// otherwise do copy-delete
	if err := f.CopyToFile(t); err != nil {
		return err
	}
	return f.Delete()
}

// MoveToLocation works by creating a new file on the target location then calling MoveToFile() on it.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {

	newFile, err := location.FileSystem().NewFile(location.Volume(), path.Join(location.Path(), f.Name()))
	if err != nil {
		return nil, err
	}

	err = f.MoveToFile(newFile)
	if err != nil {
		return nil, err
	}
	return newFile, nil
}

// CopyToFile puts the contents of File into the targetFile passed.
func (f *File) CopyToFile(file vfs.File) error {

	if err := utils.TouchCopyBuffered(file, f, 0); err != nil {
		return err
	}
	// Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := file.Close(); cerr != nil {
		return cerr
	}
	// Close file (f) reader
	return f.Close()
}

// CopyToLocation creates a copy of *File, using the file's current path as the new file's
// path at the given location.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}

	return newFile, f.CopyToFile(newFile)
}

// CRUD Operations

// Delete removes the remote file.  Error is returned, if any.
func (f *File) Delete(_ ...options.DeleteOption) error {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return err
	}
	return client.Delete(f.Path())
}

// Close calls the underlying ftp.Response Close, if opened, and clears the internal pointer
func (f *File) Close() error {
	if f.dataconn != nil {
		err := f.dataconn.Close()
		if err != nil {
			return err
		}
		f.dataconn = nil
	}
	// no op for unopened file
	f.offset = 0
	return nil
}

// Read calls the underlying ftp.File Read.
func (f *File) Read(p []byte) (n int, err error) {
	dc, err := f.getDataConn(openRead)
	if err != nil {
		return 0, err
	}

	read, err := dc.R.Read(p)
	if err != nil {
		return read, err
	}

	f.offset += int64(read)

	return read, nil
}

// Seek calls the underlying ftp.File Seek.
func (f *File) Seek(offset int64, whence int) (int64, error) {

	mode := openRead
	// no file open yet - assume read (will get reset to write on a subsequent write)
	if f.dataconn == nil {
		f.offset = offset
	} else {
		mode = f.dataconn.Mode()

		switch whence {
		case 0: // offset from beginning of the file (position 0)
			f.offset = offset
		case 1: // offset relative to current position
			if f.offset < 0 {
				f.offset = 0
			}

			err := f.dataconn.Close()
			if err != nil {
				return 0, err
			}
			f.dataconn = nil
			break
		case 2: // offset from end of the file
			sz, err := f.Size()
			if err != nil {
				// TODO: what if files doesn't exist yet... shouldn't we just use 0 as the offset
				return 0, err
			}
			f.offset = int64(sz) - offset
			if f.offset < 0 {
				f.offset = 0
			}

			err = f.dataconn.Close()
			if err != nil {
				return 0, err
			}
			f.dataconn = nil
			break
		}
	}

	// now that f.offset has been adjusted and mode was captured, reinitialize file
	_, err := f.getDataConn(mode)
	if err != nil {
		return 0, err
	}

	// return new offset from beginning of file
	return f.offset, nil
}

// Write calls the underlying ftp.File Write.
func (f *File) Write(data []byte) (res int, err error) {

	dc, err := f.getDataConn(openWrite)
	if err != nil {
		return 0, err
	}

	b, err := dc.W.Write(data)
	if err != nil {
		return 0, err
	}

	offset := int64(b)
	f.offset += offset

	return b, nil
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	return utils.GetFileURI(f)
}

// String implement fmt.Stringer, returning the file's URI as the default string.
func (f *File) String() string {
	return f.URI()
}

/*
	Private helper functions
*/

type dataConn struct {
	R    *_ftp.Response
	W    *io.PipeWriter
	mode OpenType
}

func (dc *dataConn) Mode() OpenType {
	return dc.mode
}

func (dc *dataConn) Close() error {
	if dc == nil {
		return nil
	}
	switch dc.Mode() {
	case openRead:
		return dc.R.Close()
	case openWrite:
		return dc.W.Close()
	}
	return nil
}

func (f *File) getDataConn(t OpenType) (*dataConn, error) {
	if f.dataconn != nil {
		if f.dataconn.Mode() != t {
			// wrong session type ... close current session and unset it (so we can set a new one after)
			err := f.dataconn.Close()
			if err != nil {
				return f.dataconn, err
			}
			f.dataconn = nil
		}
	}

	if f.dataconn == nil {
		client, err := f.fileSystem.Client(f.Authority)
		if err != nil {
			return nil, err
		}

		switch t {
		case openRead:
			resp, err := client.RetrFrom(f.Path(), uint64(f.offset))
			if err != nil {
				return nil, err
			}
			f.dataconn = &dataConn{
				R:    resp,
				mode: t,
			}
			break
		case openWrite:
			// create a pipewriter for writes.
			pr, pw := io.Pipe()
			err = client.StorFrom(f.Path(), pr, uint64(f.offset))
			if err != nil {
				return nil, err
			}
			f.dataconn = &dataConn{
				W:    pw,
				mode: t,
			}
			break
		}
	}

	return f.dataconn, nil
}
