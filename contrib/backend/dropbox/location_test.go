package dropbox

import (
	"errors"
	"regexp"
	"testing"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/dropbox/mocks"
)

type LocationTestSuite struct {
	suite.Suite
	mockClient *mocks.Client
	fs         *FileSystem
	location   *Location
}

func (s *LocationTestSuite) SetupTest() {
	s.mockClient = mocks.NewClient(s.T())
	s.fs = &FileSystem{
		client:  s.mockClient,
		options: NewOptions(),
	}
	s.location = &Location{
		fileSystem: s.fs,
		path:       "/test/path/",
	}
}

func (s *LocationTestSuite) TestList() {
	s.Run("Success - empty directory", func() {
		s.mockClient.EXPECT().
			ListFolder(mock.MatchedBy(func(arg *files.ListFolderArg) bool {
				return arg.Path == "/test/path"
			})).
			Return(&files.ListFolderResult{
				Entries: []files.IsMetadata{},
				HasMore: false,
			}, nil).
			Once()

		fileList, err := s.location.List()
		s.Require().NoError(err)
		s.Empty(fileList)
	})

	s.Run("Success - with files", func() {
		s.mockClient.EXPECT().
			ListFolder(mock.Anything).
			Return(&files.ListFolderResult{
				Entries: []files.IsMetadata{
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "file1.txt",
							PathDisplay: "/test/path/file1.txt",
						},
					},
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "file2.txt",
							PathDisplay: "/test/path/file2.txt",
						},
					},
					&files.FolderMetadata{
						Metadata: files.Metadata{
							Name:        "subfolder",
							PathDisplay: "/test/path/subfolder",
						},
					},
				},
				HasMore: false,
			}, nil).
			Once()

		result, err := s.location.List()
		s.Require().NoError(err)
		s.Len(result, 2, "List should only return files, not subdirectories")
		s.Contains(result, "file1.txt")
		s.Contains(result, "file2.txt")
		s.NotContains(result, "subfolder/", "subdirectories should not be included")
	})

	s.Run("Success - with pagination", func() {
		s.mockClient.EXPECT().
			ListFolder(mock.Anything).
			Return(&files.ListFolderResult{
				Entries: []files.IsMetadata{
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "file1.txt",
							PathDisplay: "/test/path/file1.txt",
						},
					},
				},
				HasMore: true,
				Cursor:  "cursor1",
			}, nil).
			Once()

		s.mockClient.EXPECT().
			ListFolderContinue(mock.MatchedBy(func(arg *files.ListFolderContinueArg) bool {
				return arg.Cursor == "cursor1"
			})).
			Return(&files.ListFolderResult{
				Entries: []files.IsMetadata{
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "file2.txt",
							PathDisplay: "/test/path/file2.txt",
						},
					},
				},
				HasMore: false,
			}, nil).
			Once()

		result, err := s.location.List()
		s.Require().NoError(err)
		s.Len(result, 2)
	})

	s.Run("Path not found - returns empty list", func() {
		s.mockClient.EXPECT().
			ListFolder(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		result, err := s.location.List()
		s.Require().NoError(err)
		s.Empty(result)
	})
}

func (s *LocationTestSuite) TestListByPrefix() {
	s.Run("Success - filters by prefix", func() {
		s.mockClient.EXPECT().
			ListFolder(mock.Anything).
			Return(&files.ListFolderResult{
				Entries: []files.IsMetadata{
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "test_file1.txt",
							PathDisplay: "/test/path/test_file1.txt",
						},
					},
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "test_file2.txt",
							PathDisplay: "/test/path/test_file2.txt",
						},
					},
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "other.txt",
							PathDisplay: "/test/path/other.txt",
						},
					},
				},
				HasMore: false,
			}, nil).
			Once()

		result, err := s.location.ListByPrefix("test_")
		s.Require().NoError(err)
		s.Len(result, 2)
		s.Contains(result, "test_file1.txt")
		s.Contains(result, "test_file2.txt")
	})
}

func (s *LocationTestSuite) TestListByRegex() {
	s.Run("Success - filters by regex", func() {
		s.mockClient.EXPECT().
			ListFolder(mock.Anything).
			Return(&files.ListFolderResult{
				Entries: []files.IsMetadata{
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "file1.txt",
							PathDisplay: "/test/path/file1.txt",
						},
					},
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "file2.log",
							PathDisplay: "/test/path/file2.log",
						},
					},
					&files.FileMetadata{
						Metadata: files.Metadata{
							Name:        "file3.txt",
							PathDisplay: "/test/path/file3.txt",
						},
					},
				},
				HasMore: false,
			}, nil).
			Once()

		regex := regexp.MustCompile(`\.txt$`)
		result, err := s.location.ListByRegex(regex)
		s.Require().NoError(err)
		s.Len(result, 2)
		s.Contains(result, "file1.txt")
		s.Contains(result, "file3.txt")
	})
}

func (s *LocationTestSuite) TestExists() {
	s.Run("Success - location exists", func() {
		s.mockClient.EXPECT().
			GetMetadata(mock.MatchedBy(func(arg *files.GetMetadataArg) bool {
				return arg.Path == "/test/path"
			})).
			Return(&files.FolderMetadata{
				Metadata: files.Metadata{
					Name:        "path",
					PathDisplay: "/test/path",
				},
			}, nil).
			Once()

		exists, err := s.location.Exists()
		s.Require().NoError(err)
		s.True(exists)
	})

	s.Run("Success - location does not exist", func() {
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		exists, err := s.location.Exists()
		s.Require().NoError(err)
		s.False(exists)
	})

	s.Run("Success - root always exists", func() {
		rootLoc := &Location{
			fileSystem: s.fs,
			path:       "/",
		}

		exists, err := rootLoc.Exists()
		s.Require().NoError(err)
		s.True(exists)
	})
}

func (s *LocationTestSuite) TestNewLocation() {
	tests := []struct {
		name          string
		relativePath  string
		expectedPath  string
		expectedError string
	}{
		{
			name:         "Valid relative path",
			relativePath: "subdir/",
			expectedPath: "/test/path/subdir",
		},
		{
			name:         "Parent directory",
			relativePath: "../",
			expectedPath: "/test",
		},
		{
			name:          "Empty path",
			relativePath:  "",
			expectedError: "non-empty string for path is required",
		},
		{
			name:          "Absolute path",
			relativePath:  "/absolute/path/",
			expectedError: "relative",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc, err := s.location.NewLocation(tt.relativePath)

			if tt.expectedError != "" {
				s.Require().Error(err)
				s.Nil(loc)
				s.Contains(err.Error(), tt.expectedError)
			} else {
				s.Require().NoError(err)
				s.NotNil(loc)
				s.Contains(loc.Path(), tt.expectedPath)
			}
		})
	}
}

func (s *LocationTestSuite) TestNewFile() {
	s.Run("Success - creates file", func() {
		file, err := s.location.NewFile("test.txt")
		s.Require().NoError(err)
		s.NotNil(file)
		s.Equal("test.txt", file.Name())
	})

	s.Run("Error - empty filename", func() {
		file, err := s.location.NewFile("")
		s.Require().Error(err)
		s.Nil(file)
	})

	s.Run("Error - absolute path", func() {
		file, err := s.location.NewFile("/absolute/path.txt")
		s.Require().Error(err)
		s.Nil(file)
	})
}

func (s *LocationTestSuite) TestDeleteFile() {
	s.Run("Success - deletes file", func() {
		s.mockClient.EXPECT().
			DeleteV2(mock.MatchedBy(func(arg *files.DeleteArg) bool {
				return arg.Path == "/test/path/file.txt"
			})).
			Return(&files.DeleteResult{}, nil).
			Once()

		err := s.location.DeleteFile("file.txt")
		s.Require().NoError(err)
	})
}

func (s *LocationTestSuite) TestPath() {
	s.Equal("/test/path/", s.location.Path())
}

func (s *LocationTestSuite) TestURI() {
	uri := s.location.URI()
	s.Contains(uri, "dbx://")
	s.Contains(uri, "/test/path/")
}

func (s *LocationTestSuite) TestFileSystem() {
	fs := s.location.FileSystem()
	s.Equal(s.fs, fs)
}

func TestLocationTestSuite(t *testing.T) {
	suite.Run(t, new(LocationTestSuite))
}
