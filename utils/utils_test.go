package utils_test

import (
	"fmt"
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	_os "github.com/c2fo/vfs/v6/backend/os"
	"github.com/c2fo/vfs/v6/mocks"
	"github.com/c2fo/vfs/v6/utils"
)

/**********************************
 ************TESTS*****************
 **********************************/

type utilsSuite struct {
	suite.Suite
}

type slashTest struct {
	path     string
	expected string
	message  string
}

func (s *utilsSuite) TestEnsureTrailingSlash() {
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
			message:  "no slash but looks like a file - add one anyway",
		},
	}

	for _, slashtest := range tests {
		s.Run(slashtest.message, func() {
			s.Equal(slashtest.expected, utils.EnsureTrailingSlash(slashtest.path), slashtest.message)
		})
	}
}

func (s *utilsSuite) TestEnsureLeadingSlash() {
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
		s.Run(slashtest.message, func() {
			s.Equal(slashtest.expected, utils.EnsureLeadingSlash(slashtest.path), slashtest.message)
		})
	}
}

func (s *utilsSuite) TestRemoveTrailingSlash() {
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
		s.Run(slashtest.message, func() {
			s.Equal(slashtest.expected, utils.RemoveTrailingSlash(slashtest.path), slashtest.message)
		})
	}
}

func (s *utilsSuite) TestRemoveLeadingSlash() {
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
		s.Run(slashtest.message, func() {
			s.Equal(slashtest.expected, utils.RemoveLeadingSlash(slashtest.path), slashtest.message)
		})
	}
}

type pathValidationTest struct {
	path         string
	passExpected bool
	message      string
}

func (s *utilsSuite) TestValidateAbsFilePath() {
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
		s.Run(validationTest.message, func() {
			err := utils.ValidateAbsoluteFilePath(validationTest.path)
			if !validationTest.passExpected {
				s.EqualError(err, utils.ErrBadAbsFilePath, validationTest.message)
			} else {
				s.NoError(err, validationTest.message)
			}
		})
	}
}

func (s *utilsSuite) TestValidateAbsLocationPath() {
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
		s.Run(validationTest.message, func() {
			err := utils.ValidateAbsoluteLocationPath(validationTest.path)
			if !validationTest.passExpected {
				s.EqualError(err, utils.ErrBadAbsLocationPath, validationTest.message)
			} else {
				s.NoError(err, validationTest.message)
			}
		})
	}
}

func (s *utilsSuite) TestValidateRelFilePath() {
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
		s.Run(validationTest.message, func() {
			err := utils.ValidateRelativeFilePath(validationTest.path)
			if !validationTest.passExpected {
				s.EqualError(err, utils.ErrBadRelFilePath, validationTest.message)
			} else {
				s.NoError(err, validationTest.message)
			}
		})
	}
}

func (s *utilsSuite) TestValidateRelLocationPath() {
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
		s.Run(validationTest.message, func() {
			err := utils.ValidateRelativeLocationPath(validationTest.path)
			if !validationTest.passExpected {
				s.EqualError(err, utils.ErrBadRelLocationPath, validationTest.message)
			} else {
				s.NoError(err, validationTest.message)
			}
		})
	}
}

func (s *utilsSuite) TestValidatePrefix() {
	tests := []struct {
		prefix       string
		passExpected bool
		message      string
	}{
		{
			prefix:       "/some/path/",
			passExpected: false,
			message:      "abs location path",
		},
		{
			prefix:       "/some/./path/../",
			passExpected: false,
			message:      "abs location path with dot dirs",
		},
		{
			prefix:       "/some/path/file.txt",
			passExpected: false,
			message:      "abs file path",
		},
		{
			prefix:       "/some/path/../file.txt",
			passExpected: false,
			message:      "abs file path with dot dirs",
		},
		{
			prefix:       "/",
			passExpected: false,
			message:      "slash only",
		},
		{
			prefix:       "",
			passExpected: false,
			message:      "empty string",
		},
		{
			prefix:       "foo",
			passExpected: true,
			message:      "prefix only",
		},
		{
			prefix:       "some/path/",
			passExpected: false,
			message:      "rel location path",
		},
		{
			prefix:       "some/./path/../",
			passExpected: false,
			message:      "rel location path with dot dirs",
		},
		{
			prefix:       "some/path/foo",
			passExpected: true,
			message:      "rel prefix",
		},
		{
			prefix:       "some/path/file.txt",
			passExpected: true,
			message:      "rel file-like prefix",
		},
		{
			prefix:       "some/path/../file.txt",
			passExpected: true,
			message:      "rel prefix path with dot dirs",
		},
		{
			prefix:       ".",
			passExpected: true,
			message:      "dot only",
		},
		{
			prefix:       ".foo",
			passExpected: true,
			message:      "dot prefix",
		},
	}

	for _, validationTest := range tests {
		s.Run(validationTest.message, func() {
			err := utils.ValidatePrefix(validationTest.prefix)
			if !validationTest.passExpected {
				s.EqualError(err, utils.ErrBadPrefix, validationTest.message)
			} else {
				s.NoError(err, validationTest.message)
			}
		})
	}
}

type URITest struct {
	path     string
	expected string
	message  string
	isRegex  bool
}

func (s *utilsSuite) TestPathToURI() {
	tests := []URITest{
		{
			path:     "/absolute/path/",
			expected: "file:///absolute/path/",
			message:  "absolute location path",
		},
		{
			path:     "/absolute/path",
			expected: "file:///absolute/path",
			message:  "absolute file path - looks like location",
		},
		{
			path:     "/absolute/path/to/file.txt",
			expected: "file:///absolute/path/to/file.txt",
			message:  "absolute file path",
		},
		{
			path:     "relative/path/",
			expected: "file:///.+?/relative/path/",
			message:  "relative location path",
			isRegex:  true,
		},
		{
			path:     "relative/path",
			expected: "file:///.+?/relative/path",
			message:  "relative file path - looks like location",
			isRegex:  true,
		},
		{
			path:     "relative/path/to/file.txt",
			expected: "file:///.+?/relative/path/to/file.txt",
			message:  "relative file path",
			isRegex:  true,
		},
		{
			path:     "/",
			expected: "file:///",
			message:  "just a slash - root location",
		},
		{
			path:     "",
			expected: "file:///",
			message:  "empty string - root location",
		},
		{
			path:     "file:///some/path/file.txt",
			expected: "file:///some/path/file.txt",
			message:  "already a URI - OS Scheme",
		},
		{
			path:     "s3://bucket/some/path/file.txt",
			expected: "s3://bucket/some/path/file.txt",
			message:  "already a URI - other Scheme",
		},
	}

	for _, slashtest := range tests {
		s.Run(slashtest.message, func() {
			uri, err := utils.PathToURI(slashtest.path)
			s.Require().NoError(err, "no error expected")
			if slashtest.isRegex {
				s.Regexp(slashtest.expected, uri, slashtest.message)
			} else {
				s.Equal(slashtest.expected, uri, slashtest.message)
			}
		})
	}

	// test error return from bad uri.parse
	const nullChar = '\u0000'

	// parse path with null character
	_, err := utils.PathToURI(fmt.Sprintf("/some%s/path/", string(nullChar)))
	s.EqualError(err, "parse \"/some\\x00/path/\": net/url: invalid control character in URL", "expected error on ctrl char in path")
}

func (s *utilsSuite) TestGetURI() {
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
	s.Equal("file:///some/path/to/file.txt", utils.GetFileURI(mockFile1), "os file uri matches")
	s.Equal("s3://mybucket/this/path/to/file.txt", utils.GetFileURI(mockFile2), "s3 file uri matches")

	// GetLocationURI
	s.Equal("file:///some/path/to/", utils.GetLocationURI(mockLoc1), "os location uri matches")
	s.Equal("s3://mybucket/this/path/to/", utils.GetLocationURI(mockLoc2), "s3 location uri matches")
}

func (s *utilsSuite) TestTouchCopy() {
	// write out blank file
	tmpfile, err := os.CreateTemp("", "utils_test")
	s.NoError(err, "unexpected temp file setup error")
	defer func() {
		err := os.Remove(tmpfile.Name())
		if err != nil {
			panic(err)
		}
	}()

	_, err = tmpfile.Write([]byte{})
	s.NoError(err, "unexpected temp file writing error")
	err = tmpfile.Close()
	s.NoError(err, "unexpected temp file close error")

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
	s.Zero(byteCount, "should be no content")

	// writer file should not exist
	_, err = os.Stat(writer.Path())
	s.Error(err, "should have failed stat")
	s.ErrorIs(err, os.ErrNotExist, "should be not exists")

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
	s.NoError(writer.Close())

	// writer file should exist
	fi, err := os.Stat(writer.Path())
	s.NoError(err, "file should exist, so no error")
	if fi != nil {
		s.Zero(fi.Size(), "file should be zero length")
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
	s.NotZero(fi, "file should have a non-zero byte size")

	s.NoError(reader.Close())

	// TouchCopy should fail on a reader.Size() error
	nonexistentFile := path.Join(writer.Path(), "nonexistent.file")
	noFile, err := osfs.NewFile("", nonexistentFile)
	s.NoError(err, "unexpected error creating vfs.File reader for non-existent file")
	err = utils.TouchCopy(writer, noFile)
	s.Error(err, "expected error running TouchCopy() using non-existent reader")
}

func (s *utilsSuite) TestTouchCopyBufferedDefaultBufferSize() {
	// write out blank file
	tmpfile, err := os.CreateTemp("", "utils_test")
	s.NoError(err, "unexpected temp file setup error")
	defer func() {
		err := os.Remove(tmpfile.Name())
		if err != nil {
			panic(err)
		}
	}()

	_, err = tmpfile.Write([]byte{})
	s.NoError(err, "unexpected temp file writing error")
	err = tmpfile.Close()
	s.NoError(err, "unexpected temp file close error")

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
	s.Zero(byteCount, "should be no content")

	// writer file should not exist
	_, err = os.Stat(writer.Path())
	s.Error(err, "should have failed stat")
	s.ErrorIs(err, os.ErrNotExist, "should be not exists")

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
	s.NoError(writer.Close())

	// writer file should exist
	fi, err := os.Stat(writer.Path())
	s.NoError(err, "file should exist, so no error")
	if fi != nil {
		s.Zero(fi.Size(), "file should be zero length")
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
	s.NotZero(fi, "file should have a non-zero byte size")

	s.NoError(reader.Close())

	// TouchCopyBuffered should fail on a reader.Size() error
	nonexistentFile := path.Join(writer.Path(), "nonexistent.file")
	noFile, err := osfs.NewFile("", nonexistentFile)
	s.NoError(err, "unexpected error creating vfs.File reader for non-existent file")
	err = utils.TouchCopyBuffered(writer, noFile, 0)
	s.Error(err, "expected error running TouchCopyBuffered() using non-existent reader")
}

func (s *utilsSuite) TestTouchCopyBufferedNonDefaultBufferSize() {
	// write out blank file
	tmpfile, err := os.CreateTemp("", "utils_test")
	s.NoError(err, "unexpected temp file setup error")
	defer func() {
		err := os.Remove(tmpfile.Name())
		if err != nil {
			panic(err)
		}
	}()

	_, err = tmpfile.Write([]byte{})
	s.NoError(err, "unexpected temp file writing error")
	err = tmpfile.Close()
	s.NoError(err, "unexpected temp file close error")

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
	s.Zero(byteCount, "should be no content")

	// writer file should not exist
	_, err = os.Stat(writer.Path())
	s.Error(err, "should have failed stat")
	s.ErrorIs(err, os.ErrNotExist, "should be not exists")

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
	s.NoError(writer.Close())

	// writer file should exist
	fi, err := os.Stat(writer.Path())
	s.NoError(err, "file should exist, so no error")
	if fi != nil {
		s.Zero(fi.Size(), "file should be zero length")
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
	s.NotZero(fi, "file should have a non-zero byte size")

	s.NoError(reader.Close())

	// TouchCopyBuffered should fail on a reader.Size() error
	nonexistentFile := path.Join(writer.Path(), "nonexistent.file")
	noFile, err := osfs.NewFile("", nonexistentFile)
	s.NoError(err, "unexpected error creating vfs.File reader for non-existent file")
	err = utils.TouchCopyBuffered(writer, noFile, 1048576)
	s.Error(err, "expected error running TouchCopyBuffered() using non-existent reader")
}

// TestSeekTo tests the seekTo function with various cases
func (s *utilsSuite) TestSeekTo() {
	testCases := []struct {
		position         int64
		offset           int64
		whence           int
		length           int64
		expectedPosition int64
		expectError      error
	}{
		// Test seeking from start
		{0, 10, io.SeekStart, 100, 10, nil},
		{0, -10, io.SeekStart, 100, 0, vfs.ErrSeekInvalidOffset}, // Negative offset from start
		{0, 110, io.SeekStart, 100, 110, nil},                    // Offset beyond length

		// Test seeking from current position
		{50, 10, io.SeekCurrent, 100, 60, nil},
		{50, -60, io.SeekCurrent, 100, 0, vfs.ErrSeekInvalidOffset}, // Moving before start
		{50, 60, io.SeekCurrent, 100, 110, nil},                     // Moving beyond length

		// Test seeking from end
		{0, -10, io.SeekEnd, 100, 90, nil},
		{0, -110, io.SeekEnd, 100, 0, vfs.ErrSeekInvalidOffset}, // Moving before start
		{0, 10, io.SeekEnd, 100, 110, nil},                      // Moving beyond length

		// Additional edge cases
		{0, 0, io.SeekStart, 100, 0, nil},       // No movement from start
		{100, 0, io.SeekCurrent, 100, 100, nil}, // No movement from current
		{0, 0, io.SeekEnd, 100, 100, nil},       // No movement from end

		// invalid whence case
		{0, 0, 3, 100, 0, vfs.ErrSeekInvalidWhence},
	}

	for _, tc := range testCases {
		result, err := utils.SeekTo(tc.length, tc.position, tc.offset, tc.whence)

		if tc.expectError != nil {
			s.Error(err, "error expected")
			s.ErrorIs(err, tc.expectError)
		} else {
			s.NoError(err, "no error expected")
			s.Equal(tc.expectedPosition, result)
		}
	}
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(utilsSuite))
}
