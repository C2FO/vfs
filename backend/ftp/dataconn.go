package ftp

import (
	"context"
	"io"

	"github.com/c2fo/vfs/v6/backend/ftp/types"
)

type dataConn struct {
	R    io.ReadCloser
	W    io.WriteCloser
	mode types.OpenType
}

func (dc *dataConn) Mode() types.OpenType {
	return dc.mode
}

func (dc *dataConn) Read(buf []byte) (int, error) {
	return dc.R.Read(buf)
}

func (dc *dataConn) Write(data []byte) (int, error) {
	return dc.W.Write(data)
}

func (dc *dataConn) Close() error {
	switch dc.Mode() {
	case types.OpenRead:
		if dc.R != nil {
			return dc.R.Close()
		}
	case types.OpenWrite:
		if dc.W != nil {
			return dc.R.Close()
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
			if err != nil {
				return nil, err
			}
			f.dataconn = &dataConn{
				R:    resp,
				mode: t,
			}
		case types.OpenWrite:
			// create a pipe writer for writes.
			pr, pw := io.Pipe()
			err = client.StorFrom(f.Path(), pr, uint64(f.offset))
			if err != nil {
				return nil, err
			}
			f.dataconn = &dataConn{
				W:    pw,
				mode: t,
			}
		}
	}

	return f.dataconn, nil
}