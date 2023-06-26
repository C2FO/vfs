package ftp

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	_ftp "github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/backend/ftp/mocks"
	"github.com/c2fo/vfs/v6/backend/ftp/types"
	"github.com/c2fo/vfs/v6/utils"
)

type dataConnSuite struct {
	suite.Suite
}

func TestDataConn(t *testing.T) {
	suite.Run(t, new(dataConnSuite))
}

func (s *dataConnSuite) TestGetDataConn() {
	// set up ftpfile
	fp := "/some/path.txt"
	client := mocks.NewClient(s.T())
	ftpfile := &File{
		fileSystem: &FileSystem{
			ftpclient: client,
			options:   Options{},
			dataconn: &dataConn{
				mode: types.OpenRead,
			},
		},
		path: fp,
	}

	// dataconn already exists
	dc, err := getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenRead)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// dataconn is nil - open for read
	ftpfile.fileSystem.dataconn = nil
	client.EXPECT().
		RetrFrom(ftpfile.Path(), uint64(0)).
		Return(&_ftp.Response{}, nil).
		Once()
	dc, err = getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenRead)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// dataconn is nil - error getting client
	ftpfile.fileSystem.dataconn = nil
	defaultClientGetter = clientGetterReturnsError
	ftpfile.fileSystem.ftpclient = nil
	dc, err = getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenRead)
	s.Error(err, "error is expected")
	s.ErrorIs(err, errClientGetter, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")
	defaultClientGetter = getClient
	ftpfile.fileSystem.ftpclient = client

	// dataconn is nil - error calling client.RetrFrom
	someErr := errors.New("some error")
	client.EXPECT().
		RetrFrom(ftpfile.Path(), uint64(0)).
		Return(nil, someErr).
		Once()
	dc, err = getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenRead)
	s.Error(err, "error is expected")
	s.ErrorIs(err, someErr, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")

	// dataconn is nil - open for write - location doesnt exist - success
	entries := []*_ftp.Entry{{
		Type: _ftp.EntryTypeFolder,
	}}
	ftpfile.fileSystem.dataconn = nil
	client.EXPECT().
		List(ftpfile.Location().Path()).
		Return(nil, errors.New("550")).
		Once()
	client.EXPECT().
		MakeDir(ftpfile.Location().Path()).
		Return(nil).
		Once()
	client.EXPECT().
		StorFrom(ftpfile.Path(), mock.Anything, uint64(0)).
		Return(nil).
		Once()
	_, err = getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenWrite)
	s.NoError(err, "no error expected")

	// dataconn is nil - open for write - error calling client.StorFrom
	ftpfile.fileSystem.dataconn = nil
	client.EXPECT().
		List(ftpfile.Location().Path()).
		Return(entries, nil).
		Once()
	client.EXPECT().
		StorFrom(ftpfile.Path(), mock.Anything, uint64(0)).
		Return(someErr).
		Once()
	dc, err = getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenWrite)
	s.NoError(err, "no error expected")
	// error in getDataConn should close the PipeReader meaning Write errors
	_, err = dc.Write([]byte{})
	s.Error(err, "error is expected")

	// dataconn is nil - open for write - success
	ftpfile.fileSystem.dataconn = nil
	client.EXPECT().
		List(ftpfile.Location().Path()).
		Return(entries, nil).
		Once()
	client.EXPECT().
		StorFrom(ftpfile.Path(), mock.Anything, uint64(0)).
		Return(nil).
		Once()
	dc, err = getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenWrite)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// open dataconn for read after dataconn for write exists - error on dataconn.Close
	fakedconn := NewFakeDataConn(types.OpenWrite)
	closeErr := errors.New("some close err")
	fakedconn.AssertCloseErr(closeErr)
	ftpfile.fileSystem.dataconn = fakedconn
	dc, err = getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenRead)
	s.Error(err, "error is expected")
	s.ErrorIs(err, closeErr, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")

	// open dataconn for write after dataconn for read exists
	ftpfile.fileSystem.dataconn = &dataConn{
		mode: types.OpenRead,
		R:    io.NopCloser(strings.NewReader("")),
	}
	client.EXPECT().
		List(ftpfile.Location().Path()).
		Return(entries, nil).
		Once()
	client.EXPECT().
		StorFrom(ftpfile.Path(), mock.Anything, uint64(0)).
		Return(nil).
		Once()
	dc, err = getDataConn(context.Background(), utils.Authority{}, ftpfile.fileSystem, ftpfile, types.OpenWrite)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// brief sleep to ensure goroutines running StorFrom can all complete
	time.Sleep(50 * time.Millisecond)

	client.AssertExpectations(s.T())
}

func (s *dataConnSuite) TestMode() {
	dc := &dataConn{
		mode: types.OpenRead,
	}
	s.Equal(types.OpenRead, dc.Mode())
}

func (s *dataConnSuite) TestRead() {
	contents := "some data"
	dc := &dataConn{
		R:    io.NopCloser(strings.NewReader(contents)),
		mode: types.OpenRead,
	}
	w := &strings.Builder{}
	written, err := io.Copy(w, dc)
	s.NoError(err, "error not expected")
	s.EqualValues(len(contents), written, "byte count should equal contents of reader")
	s.Equal(contents, w.String(), "read contents equals original contents")
}

type writeNopCloser struct {
	io.Writer
}

func (writeNopCloser) Close() error { return nil }

func WriteNopCloser(w io.Writer) io.WriteCloser {
	return writeNopCloser{w}
}
