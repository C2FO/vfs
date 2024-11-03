package ftp

import (
	"context"
	"errors"
	"io"
	"net/textproto"
	"time"

	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v6/backend/ftp/types"
	"github.com/c2fo/vfs/v6/utils"
)

type dataConn struct {
	R       io.ReadCloser
	W       io.WriteCloser
	mode    types.OpenType
	c       types.Client
	errChan chan error
}

// Delete deletes the file at the given path. Only allowed
// in a single op connection.
func (dc *dataConn) Delete(path string) error {
	if dc.mode != types.SingleOp {
		return singleOpInvalidDataconnType
	}
	return dc.c.Delete(path)
}

// GetEntry attempts to retrieve the ftp entry for
// a file at the given path. Only allowed
// in a single op connection.
func (dc *dataConn) GetEntry(p string) (*_ftp.Entry, error) {
	if dc.mode != types.SingleOp {
		return nil, singleOpInvalidDataconnType
	}
	return dc.c.GetEntry(p)
}

// List conducts an FTP list for the given path. Only allowed in a single op connection.
func (dc *dataConn) List(p string) ([]*_ftp.Entry, error) {
	if dc.mode != types.SingleOp {
		return nil, singleOpInvalidDataconnType
	}
	return dc.c.List(p)
}

// MakeDir creates a directory for the given path.
// Only allowed in a single op connection.
func (dc *dataConn) MakeDir(path string) error {
	if dc.mode != types.SingleOp {
		return singleOpInvalidDataconnType
	}
	return dc.c.MakeDir(path)
}

// Rename attempts to change the name of the file at from
// to the name specified at to. Only allowed in a single op connection.
func (dc *dataConn) Rename(from, to string) error {
	if dc.mode != types.SingleOp {
		return singleOpInvalidDataconnType
	}
	return dc.c.Rename(from, to)
}

// IsSetTimeSupported will specify whether the ftp
// set time function is available for the connection.
func (dc *dataConn) IsSetTimeSupported() bool {
	return dc.c.IsSetTimeSupported()
}

// SetTime will attempt to set the last modified time of
// a file at the given path to the given time. Only allowed
// in single op mode.
func (dc *dataConn) SetTime(path string, t time.Time) error {
	if dc.mode != types.SingleOp {
		return singleOpInvalidDataconnType
	}
	return dc.c.SetTime(path, t)
}

// IsTimePreciseInList will return true if MLST is an available FTP action in the given dataconn.
func (dc *dataConn) IsTimePreciseInList() bool {
	return dc.c.IsTimePreciseInList()
}

// Mode will return the mode of the DataConn.
func (dc *dataConn) Mode() types.OpenType {
	return dc.mode
}

// Read will read bytes from the DataConn open file to the given buffer. Only allowed in an open read DataConn
func (dc *dataConn) Read(buf []byte) (int, error) {
	if dc.mode != types.OpenRead {
		return 0, readInvalidDataconnType
	}
	return dc.R.Read(buf)
}

// Write will write bytes to the DataConn open file.
func (dc *dataConn) Write(data []byte) (int, error) {
	if dc.mode != types.OpenWrite {
		return 0, writeInvalidDataconnType
	}
	return dc.W.Write(data)
}

// Close will close the DataConnection for reading
// and writing.
func (dc *dataConn) Close() error {
	switch dc.Mode() {
	case types.OpenRead:
		if dc.R != nil {
			err := dc.R.Close()
			dc.W = nil
			dc.R = nil
			return err
		}
	case types.OpenWrite:
		if dc.W != nil {
			if err := dc.W.Close(); err != nil {
				return err
			}
			// after writer is closed STOR should commit - check for error
			err := <-dc.errChan
			dc.W = nil
			dc.R = nil
			return err
		}
	}

	return nil
}

func getDataConn(ctx context.Context, authority utils.Authority, fs *FileSystem, f *File, t types.OpenType) (types.DataConn, error) {
	if fs == nil {
		return nil, errors.New("can not get a dataconn for a nil fileset")
	}
	if fs.dataconn != nil && fs.dataconn.Mode() != t {
		// wrong session type ... close current session and unset it (ps so we can set a new one after)
		if err := fs.dataconn.Close(); err != nil {
			return nil, err
		}
		fs.dataconn = nil
	}

	if fs.dataconn == nil || fs.resetConn {
		client, err := fs.Client(ctx, authority)
		if err != nil {
			return nil, err
		}

		switch t {
		case types.OpenRead:
			resp, err := client.RetrFrom(f.Path(), uint64(f.offset))
			// check errors
			if err != nil {
				return nil, err
			}
			fs.dataconn = &dataConn{
				R:    resp,
				mode: t,
			}
		case types.OpenWrite:
			dc, err := openWriteConnection(client, f)
			if err != nil {
				return nil, err
			}
			fs.dataconn = dc
		case types.SingleOp:
			fs.dataconn = &dataConn{
				mode: t,
				c:    client,
			}
		}
		// ensure resetConn is false since we've opened/reopened the file
		if f != nil {
			fs.resetConn = false
		}
	}

	return fs.dataconn, nil
}

func openWriteConnection(client types.Client, f *File) (types.DataConn, error) {
	found, err := f.Location().Exists()
	if err != nil {
		return nil, err
	}
	if !found {
		err := client.MakeDir(f.Location().Path())
		if err != nil {
			var e *textproto.Error
			if !(errors.As(err, &e) && e.Code == _ftp.StatusFileUnavailable) {
				// Return if the error is not because the directory already exists
				return nil, err
			}
		}
	}
	pr, pw := io.Pipe()
	errChan := make(chan error, 1)
	go func(errChan chan error) {
		err := client.StorFrom(f.Path(), pr, uint64(f.offset))
		errChan <- err
		// close the pipe reader so that writes to the dataconn aren't blocking.
		// error will occur when pipereader is already closed - nothing to do in that case.
		_ = pr.Close()
	}(errChan)

	return &dataConn{
		mode:    types.OpenWrite,
		R:       pr,
		W:       pw,
		errChan: errChan,
	}, nil
}
