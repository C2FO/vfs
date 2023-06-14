package ftp

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/c2fo/vfs/v6/backend/ftp/types"
	_ftp "github.com/jlaffaye/ftp"
)

type dataConn struct {
	R       io.ReadCloser
	W       io.WriteCloser
	mode    types.OpenType
	c       types.Client
	errChan chan error
}

func (dc *dataConn) Delete(path string) error {
	if dc.mode != types.SingleOp {
		return errors.New("dataconn must be open for single op mode to single op")
	}
	return dc.c.Delete(path)

}

func (dc *dataConn) GetEntry(p string) (*_ftp.Entry, error) {
	if dc.mode != types.SingleOp {
		return nil, errors.New("dataconn must be open for single op mode to single op")
	}
	return dc.c.GetEntry(p)
}

func (dc *dataConn) List(p string) ([]*_ftp.Entry, error) {
	if dc.mode != types.SingleOp {
		return nil, errors.New("dataconn must be open for single op mode to single op")
	}
	return dc.c.List(p)
}

func (dc *dataConn) MakeDir(path string) error {
	if dc.mode != types.SingleOp {
		return errors.New("dataconn must be open for single op mode to single op")
	}
	return dc.c.MakeDir(path)
}

func (dc *dataConn) Rename(from, to string) error {
	if dc.mode != types.SingleOp {
		return errors.New("dataconn must be open for single op mode to single op")
	}
	return dc.c.Rename(from, to)
}

func (dc *dataConn) IsSetTimeSupported() bool {
	return dc.c.IsSetTimeSupported()
}

func (dc *dataConn) SetTime(path string, t time.Time) error {
	if dc.mode != types.SingleOp {
		return errors.New("dataconn must be open for single op mode to single op")
	}
	return dc.c.SetTime(path, t)
}

func (dc *dataConn) IsTimePreciseInList() bool {
	return dc.c.IsTimePreciseInList()
}

func (dc *dataConn) Mode() types.OpenType {
	return dc.mode
}

func (dc *dataConn) Read(buf []byte) (int, error) {
	if dc.mode != types.OpenRead {
		return 0, errors.New("dataconn must be open for read mode to read")
	}
	return dc.R.Read(buf)
}

func (dc *dataConn) Write(data []byte) (int, error) {
	if dc.mode != types.OpenWrite {
		return 0, errors.New("dataconn must be open for write mode to write")
	}
	return dc.W.Write(data)
}

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
			// after writer is closed STOR shoud commit - check for error
			err := <-dc.errChan
			dc.W = nil
			dc.R = nil
			return err
		}
	}

	return nil
}

func getDataConn(ctx context.Context, f *File, t types.OpenType) (types.DataConn, error) {
	if f.dataconn != nil {
		if f.dataconn.Mode() != t {
			// wrong session type ... close current session and unset it (ps so we can set a new one after)
			err := f.dataconn.Close()
			if err != nil {
				return nil, err
			}
			f.dataconn = nil
		}
	}

	if f.dataconn == nil || f.resetConn {
		client, err := f.fileSystem.Client(ctx, f.authority)
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
			f.dataconn = &dataConn{
				R:    resp,
				mode: t,
			}
		case types.OpenWrite:
			found, err := f.Location().Exists()
			if err != nil {
				return nil, err
			}
			if !found {
				err := client.MakeDir(f.Location().Path())
				if err != nil {
					return nil, err
				}
			}
			pr, pw := io.Pipe()
			errChan := make(chan error, 1)
			go func(errChan chan error) {
				err := client.StorFrom(f.Path(), pr, uint64(f.offset))
				errChan <- err
				// close the pipe reader so that writes to the dataconn aren't blocking.
				// error will occur when pipereader is already closed - nothing to do in that case.
				//nolint:errcheck
				pr.Close() // #nosec G104 if reader can't be closed there is no action to be taken
			}(errChan)

			f.dataconn = &dataConn{
				mode:    t,
				R:       pr,
				W:       pw,
				errChan: errChan,
			}
		case types.SingleOp:
			f.dataconn = &dataConn{
				mode: t,
				c:    client,
			}
		}
		// ensure resetConn is false since we've opened/reopened the file
		f.resetConn = false
	}

	return f.dataconn, nil
}
