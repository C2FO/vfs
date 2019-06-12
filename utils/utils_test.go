package utils_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"

	_os "github.com/c2fo/vfs/v4/backend/os"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v4/mocks"
	"github.com/c2fo/vfs/v4/utils"
)

/**********************************
 ************TESTS*****************
 **********************************/

type utilsTest struct {
	suite.Suite
}

type slashTest struct {
	path     string
	expected string
	message  string
}

func (s *utilsTest) TestAddTrailingSlash() {
	tests := []slashTest{
		{
			path:     "/some/path",
			expected: "/some/path/",
			message:  "no slash - adding one",
		},
		{
			path:     "/some/path/",
			expected: "/some/path/",
			message:  "slash found - don't add one",
		},
		{
			path:     "/some/path/file.txt",
			expected: "/some/path/file.txt/",
			message:  "no slash but looks like a file - add one anyaway",
		},
	}

	for _, slashtest := range tests {
		s.Equal(slashtest.expected, utils.EnsureTrailingSlash(slashtest.path), slashtest.message)
	}
}

func (s *utilsTest) TestGetURI() {

	//set up mocks
	mockFs1 := new(mocks.FileSystem)
	mockFs1.On("Scheme", mock.Anything).Return("file")

	mockLoc1 := new(mocks.Location)
	mockLoc1.On("Path").Return("/some/path/to/")
	mockLoc1.On("Volume", mock.Anything).Return("")
	mockLoc1.On("FileSystem", mock.Anything).Return(mockFs1)

	mockFile1 := new(mocks.File)
	mockFile1.On("Path").Return("/some/path/to/file.txt")
	mockFile1.On("Location").Return(mockLoc1)

	mockFs2 := new(mocks.FileSystem)
	mockFs2.On("Scheme", mock.Anything).Return("s3")

	mockLoc2 := new(mocks.Location)
	mockLoc2.On("Path").Return("/this/path/to/")
	mockLoc2.On("Volume", mock.Anything).Return("mybucket")
	mockLoc2.On("FileSystem", mock.Anything).Return(mockFs2)

	mockFile2 := new(mocks.File)
	mockFile2.On("Path").Return("/this/path/to/file.txt")
	mockFile2.On("Location").Return(mockLoc2)

	//GetFileURI
	s.Equal("file:///some/path/to/file.txt", utils.GetFileURI(mockFile1), "os file uri matches ")
	s.Equal("s3://mybucket/this/path/to/file.txt", utils.GetFileURI(mockFile2), "s3 file uri matches ")

	//GetLocationURI
	s.Equal("file:///some/path/to/", utils.GetLocationURI(mockLoc1), "os location uri matches ")
	s.Equal("s3://mybucket/this/path/to/", utils.GetLocationURI(mockLoc2), "s3 location uri matches ")
}

func (s *utilsTest) TestEnsureTrailingSlash() {
	tests := map[string]string{
		"/some/path/": "/some/path/",
		"some/path":   "some/path/",
		"":            "/",
		"/":           "/",
	}

	for path, expected := range tests {
		s.Equal(expected, utils.EnsureTrailingSlash(path))
	}
}

func (s *utilsTest) TestTouchCopy() {

	// write out blank file
	tmpfile, err := ioutil.TempFile("", "utils_test")
	defer os.Remove(tmpfile.Name()) // clean up
	if err != nil {
		s.NoError(err, "unexpected temp file setup error")
	}

	if _, err := tmpfile.Write([]byte{}); err != nil {
		s.NoError(err, "unexpected temp file writing error")
	}
	if err := tmpfile.Close(); err != nil {
		s.NoError(err, "unexpected temp file close error")
	}

	// setup reader vfs.File
	osfs := _os.FileSystem{}
	reader, err := osfs.NewFile("", tmpfile.Name())
	s.NoError(err, "unexpected error creating vfs.File reader")

	// setup writer vfs.File
	writer, err := osfs.NewFile("", tmpfile.Name()+".new")
	s.NoError(err, "unexpected error creating vfs.File writer")

	byteCount, err := io.Copy(writer, reader)
	s.NoError(err, "unexpected doing io.Copy")
	s.Equal(int64(0), byteCount, "should be no content")

	// writer file should not exist
	fmt.Printf("%s\n", writer.Path())
	_, err = os.Stat(writer.Path())
	s.Error(err, "should have failed stat")
	s.True(os.IsNotExist(err), "should be true: not exists")

	//now with TouchCopy
	_, err = reader.Seek(0, 0) //reset reader
	s.NoError(err, "unexpected error resetting vfs.File reader")
	err = utils.TouchCopy(writer, reader)
	s.NoError(err, "unexpected error running TouchCopy()")
	defer func() {
		err := writer.Delete()
		if err != nil {
			panic(err)
		}
	}()

	// writer file should exist
	fi, err := os.Stat(writer.Path())
	s.NoError(err, "file should exist, so no error")
	if fi != nil {
		s.Equal(int64(0), fi.Size(), "file should be zero length")
	}

	//TouchCopy on file that actually has data
	_, err = reader.Write([]byte("blah"))
	s.NoError(err)
	_ = reader.Close()
	_, _ = reader.Seek(0, 0)

	err = utils.TouchCopy(writer, reader)
	s.NoError(err, "unexpected error running TouchCopy()")
	fi, err = os.Stat(writer.Path())
	s.NoError(err, "file should exist, so no error")
	s.NotEqual(fi, 0, "file should have a non-zero byte size")

	//TouchCopy should fail on a reader.Size() error
	nonexistantFile := path.Join(writer.Path(), "nonexistent.file")
	noFile, err := osfs.NewFile("", nonexistantFile)
	s.NoError(err, "unexpected error creating vfs.File reader for non-existent file")
	err = utils.TouchCopy(writer, noFile)
	s.Error(err, "expected error running TouchCopy() using non-existent reader")

}

func TestUtils(t *testing.T) {
	suite.Run(t, new(utilsTest))
}
