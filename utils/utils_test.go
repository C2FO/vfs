package utils_test

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	_os "github.com/c2fo/vfs/v6/backend/os"
	"github.com/c2fo/vfs/v6/mocks"
	"github.com/c2fo/vfs/v6/utils"
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

func (s *utilsTest) TestEnsureTrailingSlash() {
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
			path:     "/",
			expected: "/",
			message:  "just a slash - don't add one",
		},
		{
			path:     "",
			expected: "/",
			message:  "empty string - add slash",
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

func (s *utilsTest) TestEnsureLeadingSlash() {
	tests := []slashTest{
		{
			path:     "some/path/",
			expected: "/some/path/",
			message:  "no slash - adding one",
		},
		{
			path:     "/some/path/",
			expected: "/some/path/",
			message:  "slash found - don't add one",
		},
		{
			path:     "/",
			expected: "/",
			message:  "just a slash - don't add one",
		},
		{
			path:     "",
			expected: "/",
			message:  "empty string - add slash",
		},
	}

	for _, slashtest := range tests {
		s.Equal(slashtest.expected, utils.EnsureLeadingSlash(slashtest.path), slashtest.message)
	}
}

func (s *utilsTest) TestRemoveTrailingSlash() {
	tests := []slashTest{
		{
			path:     "/some/path",
			expected: "/some/path",
			message:  "no slash - do nothing",
		},
		{
			path:     "/some/path/",
			expected: "/some/path",
			message:  "slash found - remove it",
		},
		{
			path:     "/",
			expected: "",
			message:  "just a slash - remove it",
		},
		{
			path:     "",
			expected: "",
			message:  "empty string - do nothing",
		},
		{
			path:     "/some/path/file.txt",
			expected: "/some/path/file.txt",
			message:  "no slash and looks like a file - do nothing",
		},
	}

	for _, slashtest := range tests {
		s.Equal(slashtest.expected, utils.RemoveTrailingSlash(slashtest.path), slashtest.message)
	}
}

func (s *utilsTest) TestRemoveLeadingSlash() {
	tests := []slashTest{
		{
			path:     "some/path/",
			expected: "some/path/",
			message:  "no slash - do nothing",
		},
		{
			path:     "/some/path/",
			expected: "some/path/",
			message:  "slash found - remove it",
		},
		{
			path:     "/",
			expected: "",
			message:  "just a slash - remove it",
		},
		{
			path:     "",
			expected: "",
			message:  "empty string - do nothing",
		},
		{
			path:     "/some/path/file.txt",
			expected: "some/path/file.txt",
			message:  "slash found and looks like a file - remove it",
		},
	}

	for _, slashtest := range tests {
		s.Equal(slashtest.expected, utils.RemoveLeadingSlash(slashtest.path), slashtest.message)
	}
}

type pathValidationTest struct {
	path         string
	passExpected bool
	message      string
}

func (s *utilsTest) TestValidateAbsFilePath() {
	tests := []pathValidationTest{
		{
			path:         "/some/path/",
			passExpected: false,
			message:      "abs location path",
		},
		{
			path:         "/some/./path/../",
			passExpected: false,
			message:      "abs location path with dot dirs",
		},
		{
			path:         "/some/path/file.txt",
			passExpected: true,
			message:      "abs file path",
		},
		{
			path:         "/some/path/../file.txt",
			passExpected: true,
			message:      "abs file path with dot dirs",
		},
		{
			path:         "/",
			passExpected: false,
			message:      "slash only",
		},
		{
			path:         "",
			passExpected: false,
			message:      "empty string",
		},
		{
			path:         "some/path/",
			passExpected: false,
			message:      "rel location path",
		},
		{
			path:         "some/./path/../",
			passExpected: false,
			message:      "rel location path with dot dirs",
		},
		{
			path:         "some/path/file.txt",
			passExpected: false,
			message:      "rel file path",
		},
		{
			path:         "some/path/../file.txt",
			passExpected: false,
			message:      "rel file path with dot dirs",
		},
	}

	for _, validationTest := range tests {
		err := utils.ValidateAbsoluteFilePath(validationTest.path)
		if !validationTest.passExpected {
			s.EqualError(err, utils.ErrBadAbsFilePath, validationTest.message)
		} else {
			s.NoError(err, validationTest.message)
		}
	}
}

func (s *utilsTest) TestValidateAbsLocationPath() {
	tests := []pathValidationTest{
		{
			path:         "/some/path/",
			passExpected: true,
			message:      "abs location path",
		},
		{
			path:         "/some/./path/../",
			passExpected: true,
			message:      "abs location path with dot dirs",
		},
		{
			path:         "/some/path/file.txt",
			passExpected: false,
			message:      "abs file path",
		},
		{
			path:         "/some/path/../file.txt",
			passExpected: false,
			message:      "abs file path with dot dirs",
		},
		{
			path:         "/",
			passExpected: true,
			message:      "slash only",
		},
		{
			path:         "",
			passExpected: false,
			message:      "empty string",
		},
		{
			path:         "some/path/",
			passExpected: false,
			message:      "rel location path",
		},
		{
			path:         "some/./path/../",
			passExpected: false,
			message:      "rel location path with dot dirs",
		},
		{
			path:         "some/path/file.txt",
			passExpected: false,
			message:      "rel file path",
		},
		{
			path:         "some/path/../file.txt",
			passExpected: false,
			message:      "rel file path with dot dirs",
		},
	}

	for _, validationTest := range tests {
		err := utils.ValidateAbsoluteLocationPath(validationTest.path)
		if !validationTest.passExpected {
			s.EqualError(err, utils.ErrBadAbsLocationPath, validationTest.message)
		} else {
			s.NoError(err, validationTest.message)
		}
	}
}

func (s *utilsTest) TestValidateRelFilePath() {
	tests := []pathValidationTest{
		{
			path:         "/some/path/",
			passExpected: false,
			message:      "abs location path",
		},
		{
			path:         "/some/./path/../",
			passExpected: false,
			message:      "abs location path with dot dirs",
		},
		{
			path:         "/some/path/file.txt",
			passExpected: false,
			message:      "abs file path",
		},
		{
			path:         "/some/path/../file.txt",
			passExpected: false,
			message:      "abs file path with dot dirs",
		},
		{
			path:         "/",
			passExpected: false,
			message:      "slash only",
		},
		{
			path:         "",
			passExpected: false,
			message:      "empty string",
		},
		{
			path:         "some/path/",
			passExpected: false,
			message:      "rel location path",
		},
		{
			path:         "some/./path/../",
			passExpected: false,
			message:      "rel location path with dot dirs",
		},
		{
			path:         "some/path/file.txt",
			passExpected: true,
			message:      "rel file path",
		},
		{
			path:         "some/path/../file.txt",
			passExpected: true,
			message:      "rel file path with dot dirs",
		},
	}

	for _, validationTest := range tests {
		err := utils.ValidateRelativeFilePath(validationTest.path)
		if !validationTest.passExpected {
			s.EqualError(err, utils.ErrBadRelFilePath, validationTest.message)
		} else {
			s.NoError(err, validationTest.message)
		}
	}
}

func (s *utilsTest) TestValidateRelLocationPath() {
	tests := []pathValidationTest{
		{
			path:         "/some/path/",
			passExpected: false,
			message:      "abs location path",
		},
		{
			path:         "/some/./path/../",
			passExpected: false,
			message:      "abs location path with dot dirs",
		},
		{
			path:         "/some/path/file.txt",
			passExpected: false,
			message:      "abs file path",
		},
		{
			path:         "/some/path/../file.txt",
			passExpected: false,
			message:      "abs file path with dot dirs",
		},
		{
			path:         "/",
			passExpected: false,
			message:      "slash only",
		},
		{
			path:         "",
			passExpected: false,
			message:      "empty string",
		},
		{
			path:         "some/path/",
			passExpected: true,
			message:      "rel location path",
		},
		{
			path:         "some/./path/../",
			passExpected: true,
			message:      "rel location path with dot dirs",
		},
		{
			path:         "some/path/file.txt",
			passExpected: false,
			message:      "rel file path",
		},
		{
			path:         "some/path/../file.txt",
			passExpected: false,
			message:      "rel file path with dot dirs",
		},
	}

	for _, validationTest := range tests {
		err := utils.ValidateRelativeLocationPath(validationTest.path)
		if !validationTest.passExpected {
			s.EqualError(err, utils.ErrBadRelLocationPath, validationTest.message)
		} else {
			s.NoError(err, validationTest.message)
		}
	}
}

func (s *utilsTest) TestGetURI() {

	// set up mocks
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

	// GetFileURI
	s.Equal("file:///some/path/to/file.txt", utils.GetFileURI(mockFile1), "os file uri matches ")
	s.Equal("s3://mybucket/this/path/to/file.txt", utils.GetFileURI(mockFile2), "s3 file uri matches ")

	// GetLocationURI
	s.Equal("file:///some/path/to/", utils.GetLocationURI(mockLoc1), "os location uri matches ")
	s.Equal("s3://mybucket/this/path/to/", utils.GetLocationURI(mockLoc2), "s3 location uri matches ")
}

func (s *utilsTest) TestTouchCopy() {

	// write out blank file
	tmpfile, err := ioutil.TempFile("", "utils_test")
	if err != nil {
		s.NoError(err, "unexpected temp file setup error")
	}
	defer func() {
		err := os.Remove(tmpfile.Name())
		if err != nil {
			panic(err)
		}
	}()

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

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	byteCount, err := io.CopyBuffer(writer, reader, buffer)
	s.NoError(err, "unexpected doing io.Copy")
	s.Equal(int64(0), byteCount, "should be no content")

	// writer file should not exist
	_, err = os.Stat(writer.Path())
	s.Error(err, "should have failed stat")
	s.True(os.IsNotExist(err), "should be true: not exists")

	// now with TouchCopy
	_, err = reader.Seek(0, 0) // reset reader
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

	// TouchCopy on file that actually has data
	_, err = reader.Write([]byte("blah"))
	s.NoError(err)
	_ = reader.Close()
	_, _ = reader.Seek(0, 0)

	err = utils.TouchCopy(writer, reader)
	s.NoError(err, "unexpected error running TouchCopy()")
	fi, err = os.Stat(writer.Path())
	s.NoError(err, "file should exist, so no error")
	s.NotEqual(fi, 0, "file should have a non-zero byte size")

	// TouchCopy should fail on a reader.Size() error
	nonexistantFile := path.Join(writer.Path(), "nonexistent.file")
	noFile, err := osfs.NewFile("", nonexistantFile)
	s.NoError(err, "unexpected error creating vfs.File reader for non-existent file")
	err = utils.TouchCopy(writer, noFile)
	s.Error(err, "expected error running TouchCopy() using non-existent reader")

}

func (s *utilsTest) TestTouchCopyBufferedDefaultBufferSize() {

	// write out blank file
	tmpfile, err := ioutil.TempFile("", "utils_test")
	if err != nil {
		s.NoError(err, "unexpected temp file setup error")
	}
	defer func() {
		err := os.Remove(tmpfile.Name())
		if err != nil {
			panic(err)
		}
	}()

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

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	byteCount, err := io.CopyBuffer(writer, reader, buffer)
	s.NoError(err, "unexpected doing io.Copy")
	s.Equal(int64(0), byteCount, "should be no content")

	// writer file should not exist
	_, err = os.Stat(writer.Path())
	s.Error(err, "should have failed stat")
	s.True(os.IsNotExist(err), "should be true: not exists")

	// now with TouchCopyBuffered
	_, err = reader.Seek(0, 0) // reset reader
	s.NoError(err, "unexpected error resetting vfs.File reader")
	err = utils.TouchCopyBuffered(writer, reader, 0)
	s.NoError(err, "unexpected error running TouchCopyBuffered()")
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

	// TouchCopyBuffered on file that actually has data
	_, err = reader.Write([]byte("blah"))
	s.NoError(err)
	_ = reader.Close()
	_, _ = reader.Seek(0, 0)

	err = utils.TouchCopyBuffered(writer, reader, 0)
	s.NoError(err, "unexpected error running TouchCopyBuffered()")
	fi, err = os.Stat(writer.Path())
	s.NoError(err, "file should exist, so no error")
	s.NotEqual(fi, 0, "file should have a non-zero byte size")

	// TouchCopyBuffered should fail on a reader.Size() error
	nonexistantFile := path.Join(writer.Path(), "nonexistent.file")
	noFile, err := osfs.NewFile("", nonexistantFile)
	s.NoError(err, "unexpected error creating vfs.File reader for non-existent file")
	err = utils.TouchCopyBuffered(writer, noFile, 0)
	s.Error(err, "expected error running TouchCopyBuffered() using non-existent reader")

}

func (s *utilsTest) TestTouchCopyBufferedNonDefaultBufferSize() {

	// write out blank file
	tmpfile, err := ioutil.TempFile("", "utils_test")
	if err != nil {
		s.NoError(err, "unexpected temp file setup error")
	}
	defer func() {
		err := os.Remove(tmpfile.Name())
		if err != nil {
			panic(err)
		}
	}()

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

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	byteCount, err := io.CopyBuffer(writer, reader, buffer)
	s.NoError(err, "unexpected doing io.Copy")
	s.Equal(int64(0), byteCount, "should be no content")

	// writer file should not exist
	_, err = os.Stat(writer.Path())
	s.Error(err, "should have failed stat")
	s.True(os.IsNotExist(err), "should be true: not exists")

	// now with TouchCopyBuffered
	_, err = reader.Seek(0, 0) // reset reader
	s.NoError(err, "unexpected error resetting vfs.File reader")
	err = utils.TouchCopyBuffered(writer, reader, utils.TouchCopyMinBufferSize*2)
	s.NoError(err, "unexpected error running TouchCopyBuffered()")
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

	// TouchCopyBuffered on file that actually has data
	_, err = reader.Write([]byte("blah"))
	s.NoError(err)
	_ = reader.Close()
	_, _ = reader.Seek(0, 0)

	err = utils.TouchCopyBuffered(writer, reader, 1048576)
	s.NoError(err, "unexpected error running TouchCopyBuffered()")
	fi, err = os.Stat(writer.Path())
	s.NoError(err, "file should exist, so no error")
	s.NotEqual(fi, 0, "file should have a non-zero byte size")

	// TouchCopyBuffered should fail on a reader.Size() error
	nonexistantFile := path.Join(writer.Path(), "nonexistent.file")
	noFile, err := osfs.NewFile("", nonexistantFile)
	s.NoError(err, "unexpected error creating vfs.File reader for non-existent file")
	err = utils.TouchCopyBuffered(writer, noFile, 1048576)
	s.Error(err, "expected error running TouchCopyBuffered() using non-existent reader")

}

func TestUtils(t *testing.T) {
	suite.Run(t, new(utilsTest))
}
