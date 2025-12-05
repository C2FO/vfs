package vfssimple

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/s3"
	"github.com/c2fo/vfs/v7/backend/s3/mocks"
)

func TestVFSSimple(t *testing.T) {
	suite.Run(t, new(vfsSimpleSuite))
}

type vfsSimpleSuite struct {
	suite.Suite
}

func (s *vfsSimpleSuite) TestParseURI() {
	tests := []struct {
		uri, message, scheme, authority, path string
		err                                   error
	}{
		{
			uri:     "",
			err:     ErrBlankURI,
			message: "cannot use an empty uri",
		},
		{
			uri:     "asdf@asdf.com",
			err:     ErrMissingScheme,
			message: "email address is not a uri",
		},
		{
			uri:     "1",
			err:     ErrMissingScheme,
			message: "integer is not a uri",
		},
		{
			uri:     "host.com/path",
			err:     ErrMissingScheme,
			message: "missing scheme",
		},
		{
			uri:     "s3.test.com",
			err:     ErrMissingScheme,
			message: "resembles, but is not, a uri",
		},
		{
			uri:     "/some/path/to/file.txt",
			err:     ErrMissingScheme,
			message: "path-only is not a uri",
		},
		{
			uri:     "s3://",
			err:     ErrMissingAuthority,
			message: "scheme only is not a uri without authority",
		},
		{
			uri:     "\u007f",
			err:     errors.New("net/url: invalid control character in URL"),
			message: "invalid char causes parse error",
		},
		{
			uri:       "fake://host.com/path/to/file.txt",
			err:       nil,
			message:   "valid uri for fake scheme",
			scheme:    "fake",
			authority: "host.com",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "file:///path/to/file.txt",
			err:       nil,
			message:   "valid file uri, no authority required",
			scheme:    "file",
			authority: "",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "file://c:/path/to/file.txt",
			err:       nil,
			message:   "valid file uri with authority",
			scheme:    "file",
			authority: "c:",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "file://c/path/to/file.txt",
			err:       nil,
			message:   "valid file uri with authority, no colon",
			scheme:    "file",
			authority: "c",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "mem:///path/to/file.txt",
			err:       nil,
			message:   "valid mem uri, no authority (namespace) required",
			scheme:    "mem",
			authority: "",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "mem://namespace/path/to/file.txt",
			err:       nil,
			message:   "valid mem uri with namespace(authority)",
			scheme:    "mem",
			authority: "namespace",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "s3://mybucket/path/to/file.txt",
			err:       nil,
			message:   "valid s3 uri",
			scheme:    "s3",
			authority: "mybucket",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "gs://mybucket/path/to/file.txt",
			err:       nil,
			message:   "valid gs uri",
			scheme:    "gs",
			authority: "mybucket",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "az://mycontainer/path/to/file.txt",
			err:       nil,
			message:   "valid azure uri",
			scheme:    "az",
			authority: "mycontainer",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "sftp://user@host.com/path/to/file.txt",
			err:       nil,
			message:   "valid sftp uri",
			scheme:    "sftp",
			authority: "user@host.com",
			path:      "/path/to/file.txt",
		},
		{
			uri:       "sftp://user@host.com:22/path/to/file.txt",
			err:       nil,
			message:   "valid sftp uri, with port",
			scheme:    "sftp",
			authority: "user@host.com:22",
			path:      "/path/to/file.txt",
		},
		{
			uri:       `sftp://domain.com%5Cuser@host.com:22/path/to/file.txt`,
			err:       nil,
			message:   "valid sftp uri, with percent-encoded char",
			scheme:    "sftp",
			authority: `domain.com%5Cuser@host.com:22`,
			path:      "/path/to/file.txt",
		},
		{
			uri:     `sftp://domain.com\user@host.com:22/path/to/file.txt`,
			err:     errors.New("net/url: invalid userinfo"),
			message: `invalid sftp uri, with raw reserved char \`,
		},
	}

	for _, test := range tests {
		s.Run(test.message, func() {
			scheme, authority, path, err := parseURI(test.uri)
			if test.err != nil {
				s.Require().Error(err, test.message)
				if !errors.Is(err, test.err) {
					// this is necessary since we can't recreate sentinel errors from url.Parse() to do errors.Is() comparison
					s.Require().ErrorContains(err, test.err.Error(), test.message)
				}
			} else {
				s.Require().NoError(err, test.message)
				s.Equal(test.scheme, scheme, test.message)
				s.Equal(test.authority, authority, test.message)
				s.Equal(test.path, path, test.message)
			}
		})
	}
}

func (s *vfsSimpleSuite) TestParseSupportedURI() {
	// register backend fs's that have a mock client injected that we can introspect in tests to ensure we right the right fs back
	backend.Register("s3://mybucket/", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "bucket1"))))
	backend.Register("s3://otherbucket/", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "bucket2"))))
	backend.Register("s3://mybucket/path/", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "path"))))
	backend.Register("s3://mybucket/path/file.txt", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "file1"))))
	backend.Register("s3://mybucket/path/file.txt.pgp", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "file2"))))

	tests := []struct {
		uri, message, scheme, authority, path, regFS string
		err                                          error
	}{
		{
			uri:       "s3://mybucket/",
			err:       nil,
			message:   "registered bucket1",
			scheme:    "s3",
			authority: "mybucket",
			path:      "/",
			regFS:     "bucket1",
		},
		{
			uri:       "s3://otherbucket/",
			err:       nil,
			message:   "registered bucket2",
			scheme:    "s3",
			authority: "otherbucket",
			path:      "/",
			regFS:     "bucket2",
		},
		{
			uri:       "s3://mybucket/unregistered/path/",
			err:       nil,
			message:   "registered bucket, unregistered path",
			scheme:    "s3",
			authority: "mybucket",
			path:      "/unregistered/path/",
			regFS:     "bucket1",
		},
		{
			uri:       "s3://mybucket/path/",
			err:       nil,
			message:   "registered bucket, registered path",
			scheme:    "s3",
			authority: "mybucket",
			path:      "/path/",
			regFS:     "path",
		},
		{
			uri:       "s3://mybucket/path/and/more/path/",
			err:       nil,
			message:   "registered bucket, registered path with more unregistered path",
			scheme:    "s3",
			authority: "mybucket",
			path:      "/path/and/more/path/",
			regFS:     "path",
		},
		{
			uri:       "s3://mybucket/path/file.txt",
			err:       nil,
			message:   "registered bucket, registered path with file1",
			scheme:    "s3",
			authority: "mybucket",
			path:      "/path/file.txt",
			regFS:     "file1",
		},
		{
			uri:       "s3://mybucket/path/file.txt.tar.gz",
			err:       nil,
			message:   "registered bucket, registered path with file1 prefix", // *********** TODO: not totally sure about this test
			scheme:    "s3",
			authority: "mybucket",
			path:      "/path/file.txt.tar.gz",
			regFS:     "file1",
		},
		{
			uri:       "s3://mybucket/path/file.txt.pgp",
			err:       nil,
			message:   "registered bucket, registered path with file2",
			scheme:    "s3",
			authority: "mybucket",
			path:      "/path/file.txt.pgp",
			regFS:     "file2",
		},
	}

	for _, test := range tests {
		s.Run(test.message, func() {
			fs, authority, path, err := parseSupportedURI(test.uri)
			if test.err != nil {
				s.Require().Error(err, test.message)
				if !errors.Is(err, test.err) {
					// this is necessary since we can't recreate sentinel errors from url.Parse() to do errors.Is() comparison
					s.Require().ErrorContains(err, test.err.Error(), test.message)
				}
			} else {
				s.Require().NoError(err, test.message)
				s.Equal(test.scheme, fs.Scheme(), test.message)
				s.Equal(test.authority, authority, test.message)
				s.Equal(test.path, path, test.message)
				// check client for named registered mock
				switch fs.Scheme() {
				case "s3":
					s3cli, err := fs.(*s3.FileSystem).Client()
					s.Require().NoError(err, test.message)
					c, ok := s3cli.(*namedS3ClientMock)
					s.Require().True(ok, "should have returned mock", test.message)
					s.Equal(test.regFS, c.RegName, test.message)
				default:
					s.Fail("we should have a case for returned fs type", test.message)
				}
			}
		})
	}
}

func (s *vfsSimpleSuite) TestNewFile() {
	backend.Register("s3://filetest/path/", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "filetest-path"))))
	backend.Register("s3://filetest/", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "filetest-bucket"))))

	// success
	goodURI := "s3://filetest/path/file.txt"
	file, err := NewFile(goodURI)
	s.Require().NoError(err, "no error expected for NewFile")
	s.IsType((*s3.File)(nil), file, "should be an s3.File")
	s.Equal(file.URI(), goodURI)
	s3cli, err := file.Location().FileSystem().(*s3.FileSystem).Client()
	s.Require().NoError(err, "no error expected")
	c, ok := s3cli.(*namedS3ClientMock)
	s.Require().True(ok, "should have returned mock")
	s.Equal("filetest-path", c.RegName, "should be 'filetest-path', not 'filetest-bucket' or 's3'")

	// failure
	badURI := "unknown://filetest/path/file.txt"
	file, err = NewFile(badURI)
	s.Require().Error(err, "error expected for NewFile")
	s.Nil(file, "file should be nil")
	s.Require().ErrorIs(err, ErrRegFsNotFound)

	badURI = ""
	file, err = NewFile(badURI)
	s.Require().Error(err, "error expected for NewFile")
	s.Nil(file, "file should be nil")
	s.Require().ErrorIs(err, ErrBlankURI)
}

func (s *vfsSimpleSuite) TestNewLocation() {
	backend.Register("s3://loctest/path/", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "loctest-path"))))
	backend.Register("s3://loctest/", s3.NewFileSystem(s3.WithClient(getS3NamedClientMock(s.T(), "loctest-bucket"))))

	// success
	goodURI := "s3://loctest/path/to/here/"
	loc, err := NewLocation(goodURI)
	s.Require().NoError(err, "no error expected for NewLocation")
	s.IsType((*s3.Location)(nil), loc, "should be an s3.Location")
	s.Equal(loc.URI(), goodURI)
	s3cli, err := loc.FileSystem().(*s3.FileSystem).Client()
	s.Require().NoError(err, "no error expected")
	c, ok := s3cli.(*namedS3ClientMock)
	s.Require().True(ok, "should have returned mock")
	s.Equal("loctest-path", c.RegName, "should be 'loctest-path', not 'loctest-bucket' or 's3'")

	// failure
	badURI := "unknown://filetest/path/to/here/"
	loc, err = NewLocation(badURI)
	s.Require().Error(err, "error expected for NewLocation")
	s.Nil(loc, "location should be nil")
	s.Require().ErrorIs(err, ErrRegFsNotFound)

	badURI = ""
	loc, err = NewLocation(badURI)
	s.Require().Error(err, "error expected for NewLocation")
	s.Nil(loc, "location should be nil")
	s.Require().ErrorIs(err, ErrBlankURI)
}

type namedS3ClientMock struct {
	*mocks.Client
	RegName string
}

// getS3NamedClientMock returns an s3 client that satisfies the interface but we only really care about the name,
// to introspect in the test return
func getS3NamedClientMock(t *testing.T, name string) *namedS3ClientMock {
	t.Helper()
	return &namedS3ClientMock{
		Client:  mocks.NewClient(t),
		RegName: name,
	}
}
