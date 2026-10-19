package gdrive

import (
	"io"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
)

// FileFakeDriveTestSuite drives the vfs.File and vfs.Location API against the fake Drive, for
// behavior that depends on options or on Drive's data model rather than on Client calls alone.
type FileFakeDriveTestSuite struct {
	suite.Suite
	drive *fakeDrive
}

func TestFileFakeDriveTestSuite(t *testing.T) {
	suite.Run(t, new(FileFakeDriveTestSuite))
}

func (s *FileFakeDriveTestSuite) SetupTest() {
	s.drive = newFakeDrive(s.T())
}

func (s *FileFakeDriveTestSuite) file(fs *FileSystem, p string) vfs.File {
	f, err := fs.NewFile("", p)
	s.Require().NoError(err)
	return f
}

func (s *FileFakeDriveTestSuite) write(f vfs.File, content string) {
	_, err := f.Write([]byte(content))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
}

func (s *FileFakeDriveTestSuite) TestTrashOnDelete() {
	fs := newTestFS(s.T(), s.drive, func(fs *FileSystem) { WithTrashOnDelete().Apply(fs) })

	f := s.file(fs, "/dir/f.txt")
	s.write(f, "x")
	s.Require().NoError(f.Delete())

	exists, err := f.Exists()
	s.Require().NoError(err)
	s.False(exists)
	s.Equal(0, s.drive.liveFiles())

	s.drive.mu.Lock()
	var trashed int
	for _, df := range s.drive.files {
		if df.name == "f.txt" && df.trashed {
			trashed++
		}
	}
	s.drive.mu.Unlock()
	s.Equal(1, trashed, "the file is trashed, not deleted")

	for _, r := range s.drive.requestLog() {
		s.NotContains(r, "DELETE", "trash mode never deletes permanently")
	}
}

func (s *FileFakeDriveTestSuite) TestExportMimeType() {
	s.drive.addMime("report", "application/vnd.google-apps.document")

	plain := newTestFS(s.T(), s.drive)
	_, err := io.ReadAll(s.file(plain, "/report"))
	s.Require().ErrorIs(err, ErrNotDownloadable)

	exporting := newTestFS(s.T(), s.drive, func(fs *FileSystem) { WithExportMimeType("application/pdf").Apply(fs) })
	f := s.file(exporting, "/report")
	data, err := io.ReadAll(f)
	s.Require().NoError(err)
	s.Equal("exported:report:application/pdf", string(data))
	s.Require().NoError(f.Close())
}

func (s *FileFakeDriveTestSuite) TestWriteOverNativeFileFails() {
	s.drive.addMime("sheet", "application/vnd.google-apps.spreadsheet")
	fs := newTestFS(s.T(), s.drive)

	f := s.file(fs, "/sheet")
	_, err := f.Write([]byte("bytes"))
	s.Require().NoError(err, "writes are buffered locally")
	s.Require().ErrorIs(f.Close(), ErrNotDownloadable)
}

func (s *FileFakeDriveTestSuite) TestPathThroughFile() {
	fs := newTestFS(s.T(), s.drive)
	s.write(s.file(fs, "/plain.txt"), "x")

	exists, err := s.file(fs, "/plain.txt/child.txt").Exists()
	s.Require().NoError(err)
	s.False(exists, "a path under a file does not exist")

	loc, err := fs.NewLocation("", "/plain.txt/")
	s.Require().NoError(err)
	exists, err = loc.Exists()
	s.Require().NoError(err)
	s.False(exists)

	names, err := loc.List()
	s.Require().NoError(err)
	s.Empty(names)

	child := s.file(fs, "/plain.txt/child.txt")
	_, err = child.Write([]byte("y"))
	s.Require().NoError(err)
	s.Require().ErrorIs(child.Close(), ErrNotFolder)
}
