//go:build vfsintegration

package onedrive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"path"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// RealDriveTestSuite checks behavior the shared conformance suite doesn't reach, against a real
// drive: upload sessions, overwrite semantics for copy and move, the folder guard, and file and
// folder names containing characters that Graph may treat specially. It uses the same credentials
// as TestConformance and skips without them. Everything is created under a unique folder that is
// removed afterwards.
type RealDriveTestSuite struct {
	suite.Suite
	fs   *FileSystem
	base string
}

func TestRealDriveTestSuite(t *testing.T) {
	suite.Run(t, new(RealDriveTestSuite))
}

func (s *RealDriveTestSuite) SetupSuite() {
	s.fs = NewFileSystem(realDriveAuth(s.T()), WithTempDir(s.T().TempDir()))
	s.base = fmt.Sprintf("vfs-realtest-%d", time.Now().UnixNano())
}

func (s *RealDriveTestSuite) TearDownSuite() {
	if s.fs == nil {
		return
	}

	c, err := s.fs.Client()
	if err != nil {
		return
	}
	gc := c.(*graphClient)

	// Client.Delete refuses folders, so remove the test folder with a raw request.
	u, err := gc.itemURL(context.Background(), s.base, "")
	s.Require().NoError(err)
	_, _, err = gc.call(context.Background(), request{method: http.MethodDelete, url: u}, nil)
	s.T().Logf("cleanup of %s: %v", s.base, err)
}

func (s *RealDriveTestSuite) path(parts ...string) string {
	return "/" + path.Join(append([]string{s.base}, parts...)...)
}

func (s *RealDriveTestSuite) write(p string, data []byte) {
	f, err := s.fs.NewFile("", p)
	s.Require().NoError(err)
	_, err = f.Write(data)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
}

func (s *RealDriveTestSuite) read(p string) []byte {
	f, err := s.fs.NewFile("", p)
	s.Require().NoError(err)
	data, err := io.ReadAll(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
	return data
}

func (s *RealDriveTestSuite) exists(p string) bool {
	f, err := s.fs.NewFile("", p)
	s.Require().NoError(err)
	ok, err := f.Exists()
	s.Require().NoError(err)
	return ok
}

func (s *RealDriveTestSuite) location(p string) *Location {
	l, err := s.fs.NewLocation("", p)
	s.Require().NoError(err)
	return l.(*Location)
}

func (s *RealDriveTestSuite) TestLargeUploadRoundTrip() {
	tests := []struct {
		name      string
		size      int
		chunkSize int64
	}{
		{name: "two default-size chunks", size: 9 << 20, chunkSize: 5 << 20},
		{name: "many small chunks just over the simple limit", size: maxSimpleUploadSize + 1000, chunkSize: 1 << 20},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			original := s.fs.options.ChunkSize
			s.fs.options.ChunkSize = tt.chunkSize
			defer func() { s.fs.options.ChunkSize = original }()

			data := make([]byte, tt.size)
			_, _ = rand.New(rand.NewSource(int64(tt.size))).Read(data) //nolint:gosec // deterministic test data

			p := s.path(fmt.Sprintf("large-%d.bin", tt.size))
			s.write(p, data)

			f, err := s.fs.NewFile("", p)
			s.Require().NoError(err)
			size, err := f.Size()
			s.Require().NoError(err)
			s.Equal(uint64(tt.size), size)

			s.True(bytes.Equal(data, s.read(p)), "content read back differs from content written")
		})
	}
}

func (s *RealDriveTestSuite) TestCopyAndMoveOverwriteExistingTarget() {
	src, dst := s.path("ow", "src.txt"), s.path("ow", "dst.txt")
	s.write(src, []byte("source"))
	s.write(dst, []byte("old destination content"))

	srcFile, err := s.fs.NewFile("", src)
	s.Require().NoError(err)
	dstFile, err := s.fs.NewFile("", dst)
	s.Require().NoError(err)

	s.Require().NoError(srcFile.CopyToFile(dstFile))
	s.Equal("source", string(s.read(dst)), "copy replaces the existing file")

	moved := s.path("ow", "moved.txt")
	s.write(moved, []byte("moved over it"))
	movedFile, err := s.fs.NewFile("", moved)
	s.Require().NoError(err)
	dstFile, err = s.fs.NewFile("", dst)
	s.Require().NoError(err)

	s.Require().NoError(movedFile.MoveToFile(dstFile))
	s.Equal("moved over it", string(s.read(dst)), "move replaces the existing file")
	s.False(s.exists(moved), "move removes the source")
}

func (s *RealDriveTestSuite) TestFoldersAreNeverDeletedMovedOrCopied() {
	s.write(s.path("guard", "keep.txt"), []byte("keep"))

	folder, err := s.fs.NewFile("", s.path("guard"))
	s.Require().NoError(err)

	s.Require().ErrorIs(folder.Delete(), errIsFolder)
	_, err = folder.MoveToLocation(s.location(s.path("guard-moved") + "/"))
	s.Require().ErrorIs(err, errIsFolder)
	_, err = folder.CopyToLocation(s.location(s.path("guard-copied") + "/"))
	s.Require().ErrorIs(err, errIsFolder)

	s.True(s.exists(s.path("guard", "keep.txt")), "folder contents survive")
}

func (s *RealDriveTestSuite) TestSpecialCharacters() {
	names := []string{
		"plus+sign", "hash#tag", "amp&amp", "apos'trophe", "pct%20enc", "pct%", "100%25",
		"space name", "unicode-ñé", "semi;colon", "comma,comma", "equals=sign", "paren(x)", "brk[x]",
		"at@sign", "tilde~", "dollar$", "brace{x}", "caret^", "back`tick",
	}

	for _, name := range names {
		s.Run(name, func() {
			content := []byte("content of " + name)
			file := "f-" + name + ".txt"
			p := s.path("chars", name, file)
			s.write(p, content)

			s.True(s.exists(p), "file exists under its own path")
			s.Equal(content, s.read(p))

			listing, err := s.location(s.path("chars", name) + "/").List()
			s.Require().NoError(err)
			s.True(slices.Contains(listing, file), "listing %v contains %q", listing, file)

			src, err := s.fs.NewFile("", p)
			s.Require().NoError(err)
			copied, err := src.CopyToLocation(s.location(s.path("chars", name+"-copy") + "/"))
			s.Require().NoError(err)
			s.Equal(content, s.read(copied.Path()))
			s.True(s.exists(p), "copy leaves the source")

			moved, err := copied.MoveToLocation(s.location(s.path("chars", name+"-moved") + "/"))
			s.Require().NoError(err)
			s.Equal(content, s.read(moved.Path()))
			s.False(s.exists(copied.Path()), "move removes the source")
		})
	}
}
