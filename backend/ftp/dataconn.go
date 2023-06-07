package ftp

import (
	"context"
	"io"

	"github.com/c2fo/vfs/v6/backend/ftp/types"
)

type dataConn struct {
	R      io.ReadCloser
	mode   types.OpenType
	client types.Client
	f      *File
}

func (dc *dataConn) Mode() types.OpenType {
	return dc.mode
}

func (dc *dataConn) Read(buf []byte) (int, error) {
	return dc.R.Read(buf)
}

func (dc *dataConn) Write(data []byte) (int, error) {
	// create a pipe writer for writes.
	pr, pw := io.Pipe()

	type writeResponse struct {
		bytesWritten int
		err          error
	}

	errChan := make(chan writeResponse, 1)
	client, err := dc.f.fileSystem.Client(context.TODO(), dc.f.authority)
	if err != nil {
		return 0, err
	}

	go func() {
		defer pw.Close()

		bytesWritten, err := pw.Write(data)

		errChan <- writeResponse{
			bytesWritten: bytesWritten,
			err:          err,
		}
	}()

	if err := client.StorFrom(dc.f.Path(), pr, uint64(dc.f.offset)); err != nil {
		return 0, err
	}

	resp := <-errChan
	return resp.bytesWritten, resp.err
}

func (dc *dataConn) Close() error {

	if dc.R != nil {
		return dc.R.Close()
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
				R:      resp,
				mode:   t,
				client: client,
				f:      f,
			}
		case types.OpenWrite:
			f.dataconn = &dataConn{
				mode:   t,
				client: client,
				f:      f,
			}
		}
	}

	return f.dataconn, nil
}
