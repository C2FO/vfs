package vfs_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	. "github.com/c2fo/vfs" // mocks also imports vfs resulting in circular dependency. See: https://github.com/golang/go/wiki/CodeReviewComments#import-dot
	"github.com/c2fo/vfs/mocks"
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
			message:  "slash founn - don't add one",
		},
		{
			path:     "/some/path\\",
			expected: "/some/path\\",
			message:  "backslash found - don't add one",
		},
		{
			path:     "/some/path/file.txt",
			expected: "/some/path/file.txt/",
			message:  "no slash but looks like a file - add one anyaway",
		},
	}

	for _, slashtest := range tests {
		s.Equal(slashtest.expected, AddTrailingSlash(slashtest.path), slashtest.message)
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
	s.Equal("file:///some/path/to/file.txt", GetFileURI(mockFile1), "os file uri matches ")
	s.Equal("s3://mybucket/this/path/to/file.txt", GetFileURI(mockFile2), "s3 file uri matches ")

	//GetLocationURI
	s.Equal("file:///some/path/to/", GetLocationURI(mockLoc1), "os location uri matches ")
	s.Equal("s3://mybucket/this/path/to/", GetLocationURI(mockLoc2), "s3 location uri matches ")
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(utilsTest))
}
