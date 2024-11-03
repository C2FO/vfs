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
	ftpFile *File
	client  *mocks.Client
}

func TestDataConn(t *testing.T) {
	suite.Run(t, new(dataConnSuite))
}

// test setup
func (s *dataConnSuite) SetupTest() {
	// set up ftpfile
	filepath := "/some/path.txt"
	s.client = mocks.NewClient(s.T())
	s.ftpFile = &File{
		fileSystem: &FileSystem{
			ftpclient: s.client,
			options:   Options{},
		},
		path: filepath,
	}

}

func (s *dataConnSuite) TestGetDataConn_alreadyExists() {

	// dataconn already exists
	s.ftpFile.fileSystem.dataconn = &dataConn{
		mode: types.OpenRead,
	}
	dc, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenRead)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")
}

func (s *dataConnSuite) TestGetDataConn_openForRead() {

	// dataconn is nil - open for read
	s.client.EXPECT().
		RetrFrom(s.ftpFile.Path(), uint64(0)).
		Return(&_ftp.Response{}, nil).
		Once()
	dc, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenRead)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")
}

func (s *dataConnSuite) TestGetDataConn_errorClientSetup() {
	// dataconn is nil - error getting client
	defaultClientGetter = clientGetterReturnsError
	s.ftpFile.fileSystem.ftpclient = nil
	dc, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenRead)
	s.Error(err, "error is expected")
	s.ErrorIs(err, errClientGetter, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")
	defaultClientGetter = getClient
}

func (s *dataConnSuite) TestGetDataConn_ReadError() {

	// dataconn is nil - error calling client.RetrFrom
	someErr := errors.New("some error")

	s.client.EXPECT().
		RetrFrom(s.ftpFile.Path(), uint64(0)).
		Return(nil, someErr).
		Once()
	dc, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenRead)
	s.Error(err, "error is expected")
	s.ErrorIs(err, someErr, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")
}

func (s *dataConnSuite) TestGetDataConn_WriteLocationNotExists() {

	// dataconn is nil - open for write - location doesn't exist - success
	s.client.EXPECT().
		List("/").
		Return(nil, errors.New("550")).
		Once()
	s.client.EXPECT().
		MakeDir(s.ftpFile.Location().Path()).
		Return(nil).
		Once()
	s.client.EXPECT().
		StorFrom(s.ftpFile.Path(), mock.Anything, uint64(0)).
		Return(nil).
		Once()
	_, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenWrite)
	s.NoError(err, "no error expected")

	// brief sleep to ensure goroutines running StorFrom can all complete
	time.Sleep(50 * time.Millisecond)
}

func (s *dataConnSuite) TestGetDataConn_WriteLocationNotExistsFails() {
	someerr := errors.New("some error")
	// dataconn is nil - open for write - location doesn't exist - success
	s.client.EXPECT().
		List("/").
		Return(nil, errors.New("550")).
		Once()
	s.client.EXPECT().
		MakeDir(s.ftpFile.Location().Path()).
		Return(someerr).
		Once()
	_, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenWrite)
	s.ErrorIs(err, someerr, "error expected")

	// brief sleep to ensure goroutines running StorFrom can all complete
	time.Sleep(50 * time.Millisecond)
}

func (s *dataConnSuite) TestGetDataConn_errorWriting() {
	entries := []*_ftp.Entry{{
		Name: "some",
		Type: _ftp.EntryTypeFolder,
	}}
	someErr := errors.New("some error")

	// dataconn is nil - open for write - error calling client.StorFrom
	s.client.EXPECT().
		List("/").
		Return(entries, nil).
		Once()
	s.client.EXPECT().
		StorFrom(s.ftpFile.Path(), mock.Anything, uint64(0)).
		Return(someErr).
		Once()
	dc, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenWrite)
	s.NoError(err, "no error expected")
	// error in getDataConn should close the PipeReader meaning Write errors
	_, err = dc.Write([]byte{})
	s.Error(err, "error is expected")
}

func (s *dataConnSuite) TestGetDataConn_writeSuccess() {
	entries := []*_ftp.Entry{{
		Name: "some",
		Type: _ftp.EntryTypeFolder,
	}}

	// dataconn is nil - open for write - success
	s.client.EXPECT().
		List("/").
		Return(entries, nil).
		Once()
	s.client.EXPECT().
		StorFrom(s.ftpFile.Path(), mock.Anything, uint64(0)).
		Return(nil).
		Once()
	dc, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenWrite)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// brief sleep to ensure goroutines running StorFrom can all complete
	time.Sleep(50 * time.Millisecond)
}

func (s *dataConnSuite) TestGetDataConn_readAfterWriteError() {

	// open dataconn for read after dataconn for write exists - error on dataconn.Close
	fakedconn := NewFakeDataConn(types.OpenWrite)
	closeErr := errors.New("some close err")
	fakedconn.AssertCloseErr(closeErr)
	s.ftpFile.fileSystem.dataconn = fakedconn
	dc, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenRead)
	s.Error(err, "error is expected")
	s.ErrorIs(err, closeErr, "error is right kind of error")
	s.Nil(dc, "dataconn should be nil on error")
}

func (s *dataConnSuite) TestGetDataConn_writeAfterReadSuccess() {
	// open dataconn for write after dataconn for read exists
	entries := []*_ftp.Entry{{
		Name: "some",
		Type: _ftp.EntryTypeFolder,
	}}
	s.ftpFile.fileSystem.dataconn = &dataConn{
		mode: types.OpenRead,
		R:    io.NopCloser(strings.NewReader("")),
	}
	s.client.EXPECT().
		List("/").
		Return(entries, nil).
		Once()
	s.client.EXPECT().
		StorFrom(s.ftpFile.Path(), mock.Anything, uint64(0)).
		Return(nil).
		Once()
	dc, err := getDataConn(context.Background(), utils.Authority{}, s.ftpFile.fileSystem, s.ftpFile, types.OpenWrite)
	s.NoError(err, "no error expected")
	s.IsTypef(&dataConn{}, dc, "dataconn returned")

	// brief sleep to ensure goroutines running StorFrom can all complete
	time.Sleep(50 * time.Millisecond)
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
	s.Len(contents, int(written), "byte count should equal contents of reader")
	s.Equal(contents, w.String(), "read contents equals original contents")
}

type writeNopCloser struct {
	io.Writer
}

func (writeNopCloser) Close() error { return nil }

func WriteNopCloser(w io.Writer) io.WriteCloser {
	return writeNopCloser{w}
}
