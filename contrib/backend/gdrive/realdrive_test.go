//go:build vfsintegration

package gdrive

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/options"
)

// The tests in this file run against a real Google Drive and skip unless credentials are set.
//
// Environment variables:
//
//   - VFS_GDRIVE_AUTH_TYPE: "token", "serviceaccount", "devicecode", or "adc" (default: auto-detect)
//   - VFS_GDRIVE_ACCESS_TOKEN: OAuth2 access token with the drive scope (token auth)
//   - VFS_GDRIVE_SERVICE_ACCOUNT_FILE: path to a service account key file (serviceaccount auth); the
//     service account needs access to the drive or folder under test
//   - VFS_GDRIVE_CLIENT_ID, VFS_GDRIVE_CLIENT_SECRET: OAuth client of the "TVs and Limited Input
//     devices" type (devicecode auth)
//   - VFS_GDRIVE_DRIVE_ID: shared drive ID to test in (default: My Drive)
//   - VFS_GDRIVE_TEST_PATH: base path for the conformance tests (default: /vfs-test/)
//
// Example (device code):
//
//	VFS_GDRIVE_CLIENT_ID=... VFS_GDRIVE_CLIENT_SECRET=... \
//	  go test -v -tags=vfsintegration -run 'TestConformance|TestRealDrive' ./...

// realDriveFS builds a FileSystem from the VFS_GDRIVE_* environment, skipping the test without credentials.
func realDriveFS(t *testing.T) *FileSystem {
	t.Helper()

	authType := os.Getenv("VFS_GDRIVE_AUTH_TYPE")
	if authType == "" {
		switch {
		case os.Getenv("VFS_GDRIVE_ACCESS_TOKEN") != "":
			authType = "token"
		case os.Getenv("VFS_GDRIVE_SERVICE_ACCOUNT_FILE") != "":
			authType = "serviceaccount"
		case os.Getenv("VFS_GDRIVE_CLIENT_ID") != "":
			authType = "devicecode"
		default:
			t.Skip("no Google Drive credentials configured; see realdrive_test.go for the VFS_GDRIVE_* variables")
		}
	}

	opts := []options.NewFileSystemOption[FileSystem]{WithTempDir(t.TempDir())}
	if id := os.Getenv("VFS_GDRIVE_DRIVE_ID"); id != "" {
		opts = append(opts, WithDriveID(id))
	}

	switch authType {
	case "token":
		opts = append(opts, WithAccessToken(os.Getenv("VFS_GDRIVE_ACCESS_TOKEN")))
	case "serviceaccount":
		key, err := os.ReadFile(os.Getenv("VFS_GDRIVE_SERVICE_ACCOUNT_FILE"))
		require.NoError(t, err)
		opts = append(opts, WithServiceAccountJSON(key))
	case "devicecode":
		opts = append(opts, WithDeviceCode(os.Getenv("VFS_GDRIVE_CLIENT_ID"), os.Getenv("VFS_GDRIVE_CLIENT_SECRET"),
			func(_ context.Context, verificationURL, userCode string) error {
				_, err := fmt.Fprintf(os.Stderr, "To sign in, visit %s and enter code %s\n", verificationURL, userCode)
				return err
			}))
	case "adc":
		opts = append(opts, WithDefaultCredentials())
	default:
		t.Fatalf("unknown VFS_GDRIVE_AUTH_TYPE %q (use token, serviceaccount, devicecode, or adc)", authType)
	}

	return NewFileSystem(opts...)
}

// ensureFolder makes sure the folder at p exists, as the conformance suite expects its base location to.
func ensureFolder(t *testing.T, fs *FileSystem, p string) {
	t.Helper()

	c, err := fs.Client("")
	require.NoError(t, err)
	dc := c.(*driveClient)
	_, _, err = dc.resolveFolder(context.Background(), path.Clean(p), true)
	require.NoError(t, err)
}

// TestConformance runs the shared VFS conformance and IO suites against a real Drive.
func TestConformance(t *testing.T) {
	fs := realDriveFS(t)
	base := cmp.Or(os.Getenv("VFS_GDRIVE_TEST_PATH"), "/vfs-test/")
	ensureFolder(t, fs, base)

	loc, err := fs.NewLocation("", base)
	require.NoError(t, err)

	t.Run("conformance", func(t *testing.T) { testsuite.RunConformanceTests(t, loc) })
	t.Run("io", func(t *testing.T) { testsuite.RunIOTests(t, loc) })
}

// RealDriveTestSuite checks behavior the shared conformance suite doesn't reach: resumable uploads,
// overwrite semantics for copy and move, the folder guard, and names with characters Drive's query
// language or URLs could mishandle. Everything lives under a unique folder removed afterwards.
type RealDriveTestSuite struct {
	suite.Suite
	fs   *FileSystem
	base string
}

func TestRealDriveTestSuite(t *testing.T) {
	suite.Run(t, new(RealDriveTestSuite))
}

func (s *RealDriveTestSuite) SetupSuite() {
	s.fs = realDriveFS(s.T())
	s.base = fmt.Sprintf("vfs-realtest-%d", time.Now().UnixNano())
	ensureFolder(s.T(), s.fs, s.base)
}

func (s *RealDriveTestSuite) TearDownSuite() {
	if s.fs == nil {
		return
	}

	c, err := s.fs.Client("")
	if err != nil {
		return
	}
	dc := c.(*driveClient)

	// Client.Delete refuses folders, so remove the test folder by ID.
	id, _, err := dc.resolveFolder(context.Background(), s.base, false)
	if err == nil {
		err = dc.remove(context.Background(), id)
	}
	s.T().Logf("cleanup of %s: %v", s.base, err)
}

func (s *RealDriveTestSuite) file(parts ...string) vfs.File {
	f, err := s.fs.NewFile("", "/"+path.Join(append([]string{s.base}, parts...)...))
	s.Require().NoError(err)
	return f
}

func (s *RealDriveTestSuite) write(f vfs.File, data []byte) {
	_, err := f.Write(data)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
}

func (s *RealDriveTestSuite) read(f vfs.File) []byte {
	data, err := io.ReadAll(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
	return data
}

func (s *RealDriveTestSuite) TestLargeFilesUseResumableUpload() {
	for _, size := range []int{maxSimpleUploadSize + 1000, 2*DefaultChunkSize + 3} {
		s.Run(strconv.Itoa(size), func() {
			data := bytes.Repeat([]byte("0123456789abcdef"), size/16+1)[:size]
			f := s.file("large", fmt.Sprintf("%d.bin", size))

			s.write(f, data)

			got, err := f.Size()
			s.Require().NoError(err)
			s.EqualValues(size, got)
			s.True(bytes.Equal(data, s.read(f)))
		})
	}
}

func (s *RealDriveTestSuite) TestCopyAndMoveOverwrite() {
	src, dst := s.file("cm", "src.txt"), s.file("cm", "dst.txt")
	s.write(src, []byte("new"))
	s.write(dst, []byte("old"))

	s.Require().NoError(src.CopyToFile(dst))
	s.Equal("new", string(s.read(dst)))

	moved := s.file("cm", "moved.txt")
	s.write(moved, []byte("moved"))
	s.Require().NoError(moved.MoveToFile(dst))
	s.Equal("moved", string(s.read(dst)))

	exists, err := moved.Exists()
	s.Require().NoError(err)
	s.False(exists)

	names, err := dst.Location().List()
	s.Require().NoError(err)
	s.ElementsMatch([]string{"src.txt", "dst.txt"}, names, "overwrite left no duplicates")
}

func (s *RealDriveTestSuite) TestFolderGuard() {
	s.write(s.file("guard", "keep.txt"), []byte("keep"))

	folder, err := s.fs.NewFile("", "/"+path.Join(s.base, "guard"))
	s.Require().NoError(err)

	exists, err := folder.Exists()
	s.Require().NoError(err)
	s.False(exists, "a folder is not a file")
	s.Require().Error(folder.Delete())

	s.Equal("keep", string(s.read(s.file("guard", "keep.txt"))))
}

func (s *RealDriveTestSuite) TestSpecialCharacterNames() {
	names := []string{
		"plus+sign.txt", "hash#tag.txt", "amp&ersand.txt", "it's.txt", "100%.txt", "pct%20enc.txt",
		"with space.txt", "ünïcödé.txt", `back\slash.txt`, "semi;colon.txt", "eq=ual.txt", "at@sign.txt",
	}

	for _, n := range names {
		s.Run(n, func() {
			f := s.file("special dir %20", n)
			s.write(f, []byte(n))
			s.Equal(n, string(s.read(f)))

			names, err := f.Location().List()
			s.Require().NoError(err)
			s.Contains(names, n)
		})
	}
}
