//go:build vfsintegration

package testcontainers

import (
	"io"
	"os"
	"strconv"
	"time"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/azure"
	"github.com/c2fo/vfs/v7/backend/gs"
	"github.com/c2fo/vfs/v7/backend/s3"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/vfssimple"
)

// objectTargets returns the targets whose backends stage writes in a local temp file (s3, gs, and az).
func (s *containerSuite) objectTargets() []backendTarget {
	var out []backendTarget
	for _, tgt := range s.targets {
		if tgt.scheme == "s3" || tgt.scheme == "gs" || tgt.scheme == "az" {
			out = append(out, tgt)
		}
	}
	return out
}

// unbufferedLocation returns a location on the same bucket/container as tgt whose filesystem has DisableWriteBuffer set.
func (s *containerSuite) unbufferedLocation(tgt backendTarget) vfs.Location {
	var fsys vfs.FileSystem
	switch fs := tgt.location.FileSystem().(type) {
	case *s3.FileSystem:
		cli, err := fs.Client()
		s.Require().NoError(err)
		fsys = s3.NewFileSystem(s3.WithClient(cli), s3.WithOptions(s3.Options{
			DisableWriteBuffer:          true,
			DisableServerSideEncryption: true,
		}))
	case *gs.FileSystem:
		cli, err := fs.Client()
		s.Require().NoError(err)
		fsys = gs.NewFileSystem(gs.WithClient(cli), gs.WithOptions(gs.Options{DisableWriteBuffer: true}))
	case *azure.FileSystem:
		cli, err := fs.Client()
		s.Require().NoError(err)
		fsys = azure.NewFileSystem(azure.WithClient(cli), azure.WithOptions(azure.Options{DisableWriteBuffer: true}))
	default:
		s.FailNow("unexpected filesystem", "%T", fs)
	}

	loc, err := fsys.NewLocation(tgt.location.Authority().String(), tgt.location.Path())
	s.Require().NoError(err)
	return loc
}

// isolateTempDir points the OS temp dir at an empty directory and returns it, so a test can prove no local temp
// file was created.
func (s *containerSuite) isolateTempDir() string {
	dir := s.T().TempDir()
	s.T().Setenv("TMPDIR", dir)
	s.T().Setenv("TMP", dir)
	s.T().Setenv("TEMP", dir)
	return dir
}

func (s *containerSuite) requireEmptyDir(dir string) {
	entries, err := os.ReadDir(dir)
	s.Require().NoError(err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	s.Require().Empty(names, "no local temp file should exist")
}

func streamName(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36) + ".txt"
}

func (s *containerSuite) readAll(loc vfs.Location, name string) string {
	f, err := loc.NewFile(name)
	s.Require().NoError(err)
	data, err := io.ReadAll(f)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
	return string(data)
}

func (s *containerSuite) seed(loc vfs.Location, name, content string) {
	f, err := loc.NewFile(name)
	s.Require().NoError(err)
	_, err = io.WriteString(f, content)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
	s.T().Cleanup(func() { _ = f.Delete() })
}

func (s *containerSuite) TestStreamingWrite() {
	for _, tgt := range s.objectTargets() {
		unbuffered := s.unbufferedLocation(tgt)
		modes := []struct {
			name string
			loc  vfs.Location
			opts []options.NewFileOption
		}{
			{name: "per-file option", loc: tgt.location, opts: []options.NewFileOption{newfile.WithStreamingWrite()}},
			{name: "filesystem option", loc: unbuffered},
		}
		for _, mode := range modes {
			s.Run(tgt.name+"/"+mode.name, func() {
				tmp := s.isolateTempDir()
				name := streamName("stream")

				f, err := mode.loc.NewFile(name, append(mode.opts, newfile.WithContentType("text/plain"))...)
				s.Require().NoError(err)
				s.T().Cleanup(func() { _ = f.Delete() })

				_, err = io.WriteString(f, "hello ")
				s.Require().NoError(err)
				_, err = io.WriteString(f, "world")
				s.Require().NoError(err)
				s.requireEmptyDir(tmp)
				s.Require().NoError(f.Close())

				s.Equal("hello world", s.readAll(tgt.location, name))

				// the same File can be read after Close
				_, err = f.Seek(6, io.SeekStart)
				s.Require().NoError(err)
				rest, err := io.ReadAll(f)
				s.Require().NoError(err)
				s.Equal("world", string(rest))
				s.Require().NoError(f.Close())
			})
		}
	}
}

func (s *containerSuite) TestStreamingZeroByteWrite() {
	for _, tgt := range s.objectTargets() {
		s.Run(tgt.name, func() {
			tmp := s.isolateTempDir()
			name := streamName("zero")

			f, err := tgt.location.NewFile(name, newfile.WithStreamingWrite())
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = f.Delete() })

			_, err = f.Write(nil)
			s.Require().NoError(err)
			s.Require().NoError(f.Close())
			s.requireEmptyDir(tmp)

			size, err := f.Size()
			s.Require().NoError(err)
			s.Zero(size)
		})
	}
}

func (s *containerSuite) TestWriteBufferOverridesFilesystemOption() {
	for _, tgt := range s.objectTargets() {
		unbuffered := s.unbufferedLocation(tgt)
		s.Run(tgt.name, func() {
			name := streamName("override")
			f, err := unbuffered.NewFile(name, newfile.WithWriteBuffer())
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = f.Delete() })

			_, err = io.WriteString(f, "hello world")
			s.Require().NoError(err)
			_, err = f.Seek(6, io.SeekStart)
			s.Require().NoError(err)
			_, err = io.WriteString(f, "Bob!")
			s.Require().NoError(err)
			s.Require().NoError(f.Close())

			s.Equal("hello Bob!d", s.readAll(tgt.location, name))
		})
	}
}

func (s *containerSuite) TestStreamingRejectsMixedIO() {
	for _, tgt := range s.objectTargets() {
		unbuffered := s.unbufferedLocation(tgt)

		s.Run(tgt.name+"/write then seek", func() {
			name := streamName("ws")
			f, err := unbuffered.NewFile(name)
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = f.Delete() })

			_, err = io.WriteString(f, "abc")
			s.Require().NoError(err)
			_, err = f.Seek(0, io.SeekStart)
			s.Require().ErrorIs(err, vfs.ErrWriteBufferDisabled)
			// the rejected Seek must not abort the upload
			_, err = io.WriteString(f, "def")
			s.Require().NoError(err)
			s.Require().NoError(f.Close())
			s.Equal("abcdef", s.readAll(tgt.location, name))
		})

		s.Run(tgt.name+"/write then read", func() {
			name := streamName("wr")
			f, err := unbuffered.NewFile(name)
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = f.Delete() })

			_, err = io.WriteString(f, "abc")
			s.Require().NoError(err)
			_, err = f.Read(make([]byte, 1))
			s.Require().ErrorIs(err, vfs.ErrWriteBufferDisabled)
			_, err = io.WriteString(f, "def")
			s.Require().NoError(err)
			s.Require().NoError(f.Close())
			s.Equal("abcdef", s.readAll(tgt.location, name))
		})

		s.Run(tgt.name+"/seek then write", func() {
			name := streamName("sw")
			s.seed(tgt.location, name, "seed")
			f, err := unbuffered.NewFile(name)
			s.Require().NoError(err)

			_, err = f.Seek(0, io.SeekStart)
			s.Require().NoError(err)
			_, err = io.WriteString(f, "abc")
			s.Require().ErrorIs(err, vfs.ErrWriteBufferDisabled)
			s.Require().NoError(f.Close())
			s.Equal("seed", s.readAll(tgt.location, name), "rejected write must leave the object untouched")
		})

		s.Run(tgt.name+"/read then write", func() {
			name := streamName("rw")
			s.seed(tgt.location, name, "seed")
			f, err := unbuffered.NewFile(name)
			s.Require().NoError(err)

			_, err = f.Read(make([]byte, 1))
			s.Require().NoError(err)
			_, err = io.WriteString(f, "abc")
			s.Require().ErrorIs(err, vfs.ErrWriteBufferDisabled)
			s.Require().NoError(f.Close())
			s.Equal("seed", s.readAll(tgt.location, name), "rejected write must leave the object untouched")
		})
	}
}

func (s *containerSuite) TestTouchCreatesObjectWithoutTempFile() {
	for _, tgt := range s.objectTargets() {
		s.Run(tgt.name, func() {
			tmp := s.isolateTempDir()
			f, err := tgt.location.NewFile(streamName("touch"))
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = f.Delete() })

			s.Require().NoError(f.Touch())
			s.requireEmptyDir(tmp)

			exists, err := f.Exists()
			s.Require().NoError(err)
			s.True(exists)
			size, err := f.Size()
			s.Require().NoError(err)
			s.Zero(size)
		})
	}
}

// TestCopyToLocationWithoutTempFile checks that non-native copies into s3, gs, and az stream without a local temp file.
func (s *containerSuite) TestCopyToLocationWithoutTempFile() {
	memLoc, err := vfssimple.NewLocation("mem://test/")
	s.Require().NoError(err)
	sources := map[string]vfs.Location{"mem": memLoc}
	for _, tgt := range s.targets {
		if tgt.scheme == "file" {
			sources["os"] = tgt.location
		}
	}

	for _, tgt := range s.objectTargets() {
		for srcName, srcLoc := range sources {
			s.Run(tgt.name+"/from "+srcName, func() {
				tmp := s.isolateTempDir()
				name := streamName("copy")
				s.seed(srcLoc, name, "copied content")

				src, err := srcLoc.NewFile(name)
				s.Require().NoError(err)
				// a distinct sub-location keeps mem from treating the copy as a same-path copy onto itself
				destLoc, err := tgt.location.NewLocation("copydest-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "/")
				s.Require().NoError(err)
				dst, err := src.CopyToLocation(destLoc)
				s.Require().NoError(err)
				s.T().Cleanup(func() { _ = dst.Delete() })

				s.requireEmptyDir(tmp)
				s.Equal("copied content", s.readAll(destLoc, name))
			})
		}
	}
}
