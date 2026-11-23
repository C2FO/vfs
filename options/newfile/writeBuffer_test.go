package newfile_test

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/backend/os"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/newfile"
)

type WriteBufferTestSuite struct {
	suite.Suite
}

func (s *WriteBufferTestSuite) TestOptionNames() {
	s.Equal("newFileStreamingWrite", newfile.WithStreamingWrite().NewFileOptionName())
	s.Equal("newFileWriteBuffer", newfile.WithWriteBuffer().NewFileOptionName())
	s.NotEqual(newfile.WithStreamingWrite().NewFileOptionName(), newfile.WithWriteBuffer().NewFileOptionName())
}

func (s *WriteBufferTestSuite) TestOptionTypes() {
	_, ok := newfile.WithStreamingWrite().(newfile.StreamingWrite)
	s.True(ok, "expected newfile.StreamingWrite")
	_, ok = newfile.WithWriteBuffer().(newfile.WriteBuffer)
	s.True(ok, "expected newfile.WriteBuffer")
}

func (s *WriteBufferTestSuite) TestUseWriteBuffer() {
	streaming := newfile.WithStreamingWrite()
	buffer := newfile.WithWriteBuffer()
	streamingPtr := new(newfile.StreamingWrite)
	bufferPtr := new(newfile.WriteBuffer)

	tests := []struct {
		name     string
		def      bool
		opts     []options.NewFileOption
		expected bool
	}{
		{name: "default buffered, no options", def: true, expected: true},
		{name: "default streaming, no options", def: false, expected: false},
		{name: "streaming option overrides buffered default", def: true, opts: []options.NewFileOption{streaming}, expected: false},
		{name: "buffer option overrides streaming default", def: false, opts: []options.NewFileOption{buffer}, expected: true},
		{name: "last option wins: streaming then buffer", def: true, opts: []options.NewFileOption{streaming, buffer}, expected: true},
		{name: "last option wins: buffer then streaming", def: false, opts: []options.NewFileOption{buffer, streaming}, expected: false},
		{name: "pointer forms are honored", def: true, opts: []options.NewFileOption{streamingPtr}, expected: false},
		{name: "pointer buffer form is honored", def: false, opts: []options.NewFileOption{bufferPtr}, expected: true},
		{name: "unrelated options are ignored", def: true, opts: []options.NewFileOption{newfile.WithContentType("text/plain")}, expected: true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expected, newfile.UseWriteBuffer(tt.def, tt.opts))
		})
	}
}

// TestPortableNoOp verifies backends that do not stage writes accept and ignore the options.
func (s *WriteBufferTestSuite) TestPortableNoOp() {
	tmp := s.T().TempDir()

	osLoc, err := os.NewFileSystem().NewLocation("", filepath.ToSlash(tmp)+"/")
	s.Require().NoError(err)
	memLoc, err := mem.NewFileSystem().NewLocation("", "/")
	s.Require().NoError(err)

	locations := map[string]vfs.Location{"os": osLoc, "mem": memLoc}
	opts := []options.NewFileOption{newfile.WithStreamingWrite(), newfile.WithWriteBuffer()}

	for name, loc := range locations {
		for _, opt := range opts {
			s.Run(name+" "+opt.NewFileOptionName(), func() {
				f, err := loc.NewFile("portable.txt", opt)
				s.Require().NoError(err)

				_, err = f.Write([]byte("hello"))
				s.Require().NoError(err)
				_, err = f.Seek(0, io.SeekStart)
				s.Require().NoError(err)
				b := make([]byte, 5)
				_, err = io.ReadFull(f, b)
				s.Require().NoError(err)
				s.Equal("hello", string(b))
				s.Require().NoError(f.Close())
				s.Require().NoError(f.Delete())
			})
		}
	}
}

func TestWriteBufferTestSuite(t *testing.T) {
	suite.Run(t, new(WriteBufferTestSuite))
}
