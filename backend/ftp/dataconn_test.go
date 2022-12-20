package ftp

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/backend/ftp/mocks"
	"github.com/c2fo/vfs/v6/backend/ftp/types"
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
		},
		path: fp,
		dataconn: &dataConn{
			mode: types.OpenRead,
		},
	}

	// dataconn already exists
	dc, err := getDataConn(context.Background(), ftpfile, types.OpenRead)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// dataconn is nil - open for read
	ftpfile.dataconn = nil
	client.EXPECT().
		RetrFrom(ftpfile.Path(), uint64(0)).
		Return(&ftp.Response{}, nil).
		Once()
	dc, err = getDataConn(context.Background(), ftpfile, types.OpenRead)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// dataconn is nil - error getting client
	ftpfile.dataconn = nil
	defaultClientGetter = clientGetterReturnsError
	ftpfile.fileSystem.ftpclient = nil
	dc, err = getDataConn(context.Background(), ftpfile, types.OpenRead)
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
	dc, err = getDataConn(context.Background(), ftpfile, types.OpenRead)
	s.Error(err, "error is expected")
	s.ErrorIs(err, someErr, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")

	// dataconn is nil - open for read - error calling client.StorFrom
	ftpfile.dataconn = nil
	client.EXPECT().
		StorFrom(ftpfile.Path(), mock.Anything, uint64(0)).
		Return(someErr).
		Once()
	dc, err = getDataConn(context.Background(), ftpfile, types.OpenWrite)
	s.Error(err, "error is expected")
	s.ErrorIs(err, someErr, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")

	// dataconn is nil - open for read - success
	ftpfile.dataconn = nil
	client.EXPECT().
		StorFrom(ftpfile.Path(), mock.Anything, uint64(0)).
		Return(nil).
		Once()
	dc, err = getDataConn(context.Background(), ftpfile, types.OpenWrite)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// open dataconn for read after dataconn for write exists - error on dataconn.Close
	fakedconn := NewFakeDataConn(types.OpenWrite)
	closeErr := errors.New("some close err")
	fakedconn.AssertCloseErr(closeErr)
	ftpfile.dataconn = fakedconn
	dc, err = getDataConn(context.Background(), ftpfile, types.OpenRead)
	s.Error(err, "error is expected")
	s.ErrorIs(err, closeErr, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")

	// open dataconn for read after dataconn for write exists
	ftpfile.dataconn = &dataConn{
		mode: types.OpenRead,
		R:    io.NopCloser(strings.NewReader("")),
	}
	client.EXPECT().
		StorFrom(ftpfile.Path(), mock.Anything, uint64(0)).
		Return(nil).
		Once()
	dc, err = getDataConn(context.Background(), ftpfile, types.OpenWrite)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

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

func (s *dataConnSuite) TestWrite() {
	contents := "some data"
	w := &strings.Builder{}
	dc := &dataConn{
		W:    WriteNopCloser(w),
		mode: types.OpenRead,
	}
	r := strings.NewReader(contents)
	written, err := io.Copy(dc, r)
	s.NoError(err, "error not expected")
	s.EqualValues(len(contents), written, "byte count should equal contents of reader")
	s.Equal(contents, w.String(), "read contents equals original contents")
}

func (s *dataConnSuite) TestClose() {
	dc := &dataConn{
		mode: types.OpenRead,
	}

	// Close when neither read no write is set (this may actually not be possible)
	s.NoError(dc.Close(), "expect no error")

	// Close when mode is read
	dc.R = io.NopCloser(strings.NewReader(""))
	s.NoError(dc.Close(), "expect no error")

	// Close when mode is read
	dc.mode = types.OpenWrite
	dc.W = WriteNopCloser(&strings.Builder{})
	s.NoError(dc.Close(), "expect no error")
}

type writeNopCloser struct {
	io.Writer
}

func (writeNopCloser) Close() error { return nil }

func WriteNopCloser(w io.Writer) io.WriteCloser {
	return writeNopCloser{w}
}
