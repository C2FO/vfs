package sqlfs_test

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
)

const testChunk = sqlfs.MinChunkSize

type FileTestSuite struct {
	suite.Suite
	fs *sqlfs.FileSystem
	db *sql.DB
}

func TestFileTestSuite(t *testing.T) { suite.Run(t, new(FileTestSuite)) }

func (s *FileTestSuite) SetupTest() {
	s.fs, s.db = newSQLiteEnv(s.T(), sqlfs.WithChunkSize(testChunk))
}

func (s *FileTestSuite) file(p string) *sqlfs.File {
	f, err := s.fs.NewFile(testNS, p)
	s.Require().NoError(err)
	return f.(*sqlfs.File)
}

func (s *FileTestSuite) count(query string, args ...any) int {
	var n int
	s.Require().NoError(s.db.QueryRow(query, args...).Scan(&n))
	return n
}

func (s *FileTestSuite) TestReadWriteAcrossChunkBoundaries() {
	sizes := []int{0, 1, testChunk - 1, testChunk, testChunk + 1, 2 * testChunk, 2*testChunk + 17, 5*testChunk + 3}
	bufSizes := []int{1, 7, 1000, testChunk, testChunk + 1, 3 * testChunk}

	for _, size := range sizes {
		content := pattern(size)
		p := "/chunks/f.bin"
		put(s.T(), s.fs, p, content)

		s.Run("round trip", func() {
			s.True(bytes.Equal(content, get(s.T(), s.fs, p)), "size %d", size)
		})
		for _, bs := range bufSizes {
			f := s.file(p)
			var got []byte
			buf := make([]byte, bs)
			for {
				n, err := f.Read(buf)
				got = append(got, buf[:n]...)
				if errors.Is(err, io.EOF) {
					break
				}
				s.Require().NoError(err)
			}
			s.Require().NoError(f.Close())
			s.True(bytes.Equal(content, got), "size %d buffer %d", size, bs)
		}
		size64, err := s.file(p).Size()
		s.Require().NoError(err)
		s.Equal(uint64(size), size64)
	}
}

func (s *FileTestSuite) TestEmptyFile() {
	put(s.T(), s.fs, "/empty.txt", nil)
	f := s.file("/empty.txt")
	n, err := f.Read(make([]byte, 8))
	s.Zero(n)
	s.Require().ErrorIs(err, io.EOF)
	size, err := f.Size()
	s.Require().NoError(err)
	s.Zero(size)
	s.Zero(s.count("SELECT COUNT(*) FROM vfs_chunks"), "empty files have zero chunks")
}

func (s *FileTestSuite) TestBinarySafe() {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	put(s.T(), s.fs, "/bin", all)
	s.Equal(all, get(s.T(), s.fs, "/bin"))
}

func (s *FileTestSuite) TestSQLMetacharactersInNames() {
	const p = "/x'; DROP TABLE vfs_files; --.txt"
	put(s.T(), s.fs, p, []byte("safe"))
	s.Equal("safe", string(get(s.T(), s.fs, p)))
	s.Equal(1, s.count("SELECT COUNT(*) FROM vfs_files"))

	loc, err := s.fs.NewLocation(testNS, "/")
	s.Require().NoError(err)
	names, err := loc.List()
	s.Require().NoError(err)
	s.Equal([]string{"x'; DROP TABLE vfs_files; --.txt"}, names)
}

func (s *FileTestSuite) TestNotFoundErrors() {
	f := s.file("/nope.txt")
	_, err := f.Read(make([]byte, 1))
	s.Require().ErrorIs(err, fs.ErrNotExist)
	s.Require().ErrorIs(err, sqlfs.ErrNotFound)

	_, err = f.Size()
	s.Require().ErrorIs(err, fs.ErrNotExist)
	_, err = f.LastModified()
	s.Require().ErrorIs(err, fs.ErrNotExist)
	s.Require().ErrorIs(f.Delete(), fs.ErrNotExist)
	_, err = f.Seek(2, io.SeekStart)
	s.Require().Error(err)
}

func (s *FileTestSuite) TestSeek() {
	put(s.T(), s.fs, "/seek.txt", []byte("123456789"))
	f := s.file("/seek.txt")

	tests := []struct {
		name           string
		offset         int64
		whence         int
		expectedPos    int64
		expectedErrMsg string
	}{
		{name: "from start", offset: 3, whence: io.SeekStart, expectedPos: 3},
		{name: "from current", offset: 2, whence: io.SeekCurrent, expectedPos: 5},
		{name: "from end", offset: -4, whence: io.SeekEnd, expectedPos: 5},
		{name: "past end is allowed", offset: 100, whence: io.SeekStart, expectedPos: 100},
		{name: "negative offset rejected", offset: -1, whence: io.SeekStart, expectedErrMsg: "invalid"},
		{name: "invalid whence rejected", offset: 0, whence: 9, expectedErrMsg: "whence"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			before, err := f.Seek(0, io.SeekCurrent)
			s.Require().NoError(err)
			pos, err := f.Seek(tt.offset, tt.whence)
			if tt.expectedErrMsg != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tt.expectedErrMsg)
				after, _ := f.Seek(0, io.SeekCurrent)
				s.Equal(before, after, "cursor unchanged on error")
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expectedPos, pos)
		})
	}
}

func (s *FileTestSuite) TestSparseWrite() {
	put(s.T(), s.fs, "/sparse.bin", []byte("abc"))
	f := s.file("/sparse.bin")
	_, err := f.Seek(6, io.SeekStart)
	s.Require().NoError(err)
	_, err = f.Write([]byte("x"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
	s.Equal([]byte{'a', 'b', 'c', 0, 0, 0, 'x'}, get(s.T(), s.fs, "/sparse.bin"))

	// a gap spanning whole chunks reads back as zeros and is not stored
	g := s.file("/sparse2.bin")
	_, err = g.Seek(0, io.SeekStart)
	s.Require().Error(err, "cannot seek a missing file before writing")
	_, err = g.Write([]byte("a"))
	s.Require().NoError(err)
	_, err = g.Seek(3*testChunk+5, io.SeekStart)
	s.Require().NoError(err)
	_, err = g.Write([]byte("z"))
	s.Require().NoError(err)
	s.Require().NoError(g.Close())

	got := get(s.T(), s.fs, "/sparse2.bin")
	want := make([]byte, 3*testChunk+6)
	want[0], want[len(want)-1] = 'a', 'z'
	s.True(bytes.Equal(want, got))
	const chunksOf = "SELECT COUNT(*) FROM vfs_chunks c JOIN vfs_files f ON f.revision_id = c.revision_id WHERE f.name = ?"
	s.Equal(2, s.count(chunksOf, "sparse2.bin"), "only touched chunks are stored")
}

func (s *FileTestSuite) TestOverlayLargeFilePreservesUntouchedChunks() {
	content := pattern(4*testChunk + 100)
	put(s.T(), s.fs, "/big.bin", content)

	f := s.file("/big.bin")
	_, err := f.Seek(2*testChunk-2, io.SeekStart)
	s.Require().NoError(err)
	_, err = f.Write([]byte("XXXX"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	want := bytes.Clone(content)
	copy(want[2*testChunk-2:], "XXXX")
	s.True(bytes.Equal(want, get(s.T(), s.fs, "/big.bin")))
}

func (s *FileTestSuite) TestWritesInvisibleUntilClose() {
	f := s.file("/pending.txt")
	_, err := f.Write([]byte("new text"))
	s.Require().NoError(err)

	other := s.file("/pending.txt")
	ok, err := other.Exists()
	s.Require().NoError(err)
	s.False(ok)

	// the writer reads its own unpublished writes
	_, err = f.Seek(0, io.SeekStart)
	s.Require().NoError(err)
	buf := make([]byte, 5)
	_, err = io.ReadFull(f, buf)
	s.Require().NoError(err)
	s.Equal("new t", string(buf))

	s.Require().NoError(f.Close())
	ok, err = other.Exists()
	s.Require().NoError(err)
	s.True(ok)
}

func (s *FileTestSuite) TestAbort() {
	put(s.T(), s.fs, "/abort.txt", []byte("original"))

	f := s.file("/abort.txt")
	_, err := f.Write([]byte("replacement that is longer"))
	s.Require().NoError(err)
	s.Require().NoError(f.Abort())
	s.Equal("original", string(get(s.T(), s.fs, "/abort.txt")))
	s.Equal(1, s.count("SELECT COUNT(*) FROM vfs_revisions"), "staged revision removed")

	s.Require().NoError(s.file("/never-written.txt").Abort(), "nothing pending")

	g := s.file("/abort-new.txt")
	_, err = g.Write([]byte("x"))
	s.Require().NoError(err)
	s.Require().NoError(g.Abort())
	ok, err := g.Exists()
	s.Require().NoError(err)
	s.False(ok)
}

func (s *FileTestSuite) TestConflicts() {
	tests := []struct {
		name    string
		setup   func()
		second  func(path string) // runs between the two writers' Write and Close
		winner  string
		deleted bool
	}{
		{
			name:   "concurrent overwrite",
			setup:  func() { put(s.T(), s.fs, "/c.txt", []byte("original")) },
			winner: "B",
		},
		{
			name:   "concurrent create",
			setup:  func() {},
			winner: "B",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.SetupTest()
			tt.setup()
			a, b := s.file("/c.txt"), s.file("/c.txt")

			_, err := a.Write([]byte("A"))
			s.Require().NoError(err)
			_, err = b.Write([]byte("B"))
			s.Require().NoError(err)

			// the first to close wins; the other must not overwrite silently
			s.Require().NoError(b.Close())
			err = a.Close()
			s.Require().ErrorIs(err, sqlfs.ErrConflict)
			s.Equal(tt.winner, string(get(s.T(), s.fs, "/c.txt")))
			s.Require().NoError(a.Abort())
		})
	}

	s.Run("create conflict is also reported as already exists", func() {
		s.SetupTest()
		a, b := s.file("/n.txt"), s.file("/n.txt")
		_, _ = a.Write([]byte("A"))
		_, _ = b.Write([]byte("B"))
		s.Require().NoError(a.Close())
		err := b.Close()
		s.Require().ErrorIs(err, sqlfs.ErrAlreadyExists)
		s.Require().ErrorIs(err, sqlfs.ErrConflict)
		s.Equal("A", string(get(s.T(), s.fs, "/n.txt")))
	})

	s.Run("delete during write", func() {
		s.SetupTest()
		put(s.T(), s.fs, "/d.txt", []byte("original"))
		a := s.file("/d.txt")
		_, err := a.Write([]byte("A"))
		s.Require().NoError(err)
		s.Require().NoError(s.file("/d.txt").Delete())

		s.Require().ErrorIs(a.Close(), sqlfs.ErrConflict)
		ok, err := s.file("/d.txt").Exists()
		s.Require().NoError(err)
		s.False(ok)
	})

	s.Run("overlay of replaced file", func() {
		s.SetupTest()
		put(s.T(), s.fs, "/o.txt", []byte("original"))
		a := s.file("/o.txt")
		_, err := a.Seek(2, io.SeekStart)
		s.Require().NoError(err)
		put(s.T(), s.fs, "/o.txt", []byte("replaced by someone else"))
		_, err = a.Write([]byte("zz"))
		s.Require().NoError(err)
		s.Require().ErrorIs(a.Close(), sqlfs.ErrConflict)
		s.Equal("replaced by someone else", string(get(s.T(), s.fs, "/o.txt")))
	})
}

func (s *FileTestSuite) TestConcurrentWritersExactlyOneWins() {
	put(s.T(), s.fs, "/race.txt", []byte("start"))
	const writers = 6
	files := make([]*sqlfs.File, writers)
	for i := range files {
		files[i] = s.file("/race.txt")
		_, err := files[i].Write([]byte{byte('a' + i)})
		s.Require().NoError(err)
	}
	errs := make([]error, writers)
	var wg syncWaitGroup
	for i := range files {
		wg.Go(func() { errs[i] = files[i].Close() })
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		s.Require().ErrorIs(err, sqlfs.ErrConflict)
	}
	s.Equal(1, wins)
	s.Len(get(s.T(), s.fs, "/race.txt"), 1)
}

func (s *FileTestSuite) TestStableReadDuringReplacement() {
	original := pattern(3 * testChunk)
	put(s.T(), s.fs, "/stable.bin", original)

	reader := s.file("/stable.bin")
	first := make([]byte, testChunk+10) //nolint:prealloc // sized for the first read
	_, err := io.ReadFull(reader, first)
	s.Require().NoError(err)

	put(s.T(), s.fs, "/stable.bin", bytes.Repeat([]byte{0xFF}, 3*testChunk))
	s.Require().NoError(s.file("/other.bin").Touch())

	rest, err := io.ReadAll(reader)
	s.Require().NoError(err)
	s.True(bytes.Equal(original, append(first, rest...)), "reader must see only the original content")
	s.Require().NoError(reader.Close())

	// a fresh handle sees the replacement
	s.Equal(bytes.Repeat([]byte{0xFF}, 3*testChunk), get(s.T(), s.fs, "/stable.bin"))
}

func (s *FileTestSuite) TestMaxFileSize() {
	s.fs, s.db = newSQLiteEnv(s.T(), sqlfs.WithChunkSize(testChunk), sqlfs.WithMaxFileSize(10))

	f := s.file("/limit.txt")
	_, err := f.Write(make([]byte, 11))
	s.Require().Error(err)
	s.Require().NoError(f.Abort())
	ok, err := f.Exists()
	s.Require().NoError(err)
	s.False(ok)

	put(s.T(), s.fs, "/limit.txt", make([]byte, 10))
	g := s.file("/limit.txt")
	_, err = g.Seek(0, io.SeekEnd)
	s.Require().NoError(err)
	_, err = g.Write([]byte("x"))
	s.Require().Error(err, "extending past the limit must fail")
	s.Require().NoError(g.Abort())
	s.Len(get(s.T(), s.fs, "/limit.txt"), 10, "published content is unchanged")
}

func (s *FileTestSuite) TestTouch() {
	f := s.file("/touch.txt")
	s.Require().NoError(f.Touch())
	size, err := f.Size()
	s.Require().NoError(err)
	s.Zero(size)

	put(s.T(), s.fs, "/touch2.txt", []byte("content"))
	g := s.file("/touch2.txt")
	before, err := g.LastModified()
	s.Require().NoError(err)
	time.Sleep(5 * time.Millisecond)
	s.Require().NoError(g.Touch())
	after, err := g.LastModified()
	s.Require().NoError(err)
	s.True(after.After(*before))
	s.Equal("content", string(get(s.T(), s.fs, "/touch2.txt")), "touch must not change content")
}

func (s *FileTestSuite) TestDeleteSupersedesRevision() {
	put(s.T(), s.fs, "/gone.txt", []byte("x"))
	s.Require().NoError(s.file("/gone.txt").Delete())
	ok, err := s.file("/gone.txt").Exists()
	s.Require().NoError(err)
	s.False(ok)
	s.Equal(1, s.count("SELECT COUNT(*) FROM vfs_revisions WHERE state = 2"))
}

func (s *FileTestSuite) TestLastModifiedAfterWrite() {
	start := time.Now()
	put(s.T(), s.fs, "/lm.txt", []byte("x"))
	lm, err := s.file("/lm.txt").LastModified()
	s.Require().NoError(err)
	s.False(lm.Before(start.Add(-time.Millisecond)))
}

func (s *FileTestSuite) TestCopy() {
	content := pattern(2*testChunk + 5)
	put(s.T(), s.fs, "/src/a.bin", content)
	put(s.T(), s.fs, "/dst/a.bin", []byte("old target"))

	src := s.file("/src/a.bin")
	dstLoc, err := s.fs.NewLocation(testNS, "/dst/")
	s.Require().NoError(err)

	s.Run("overwrites target and keeps source", func() {
		got, err := src.CopyToLocation(dstLoc)
		s.Require().NoError(err)
		s.Equal("sqlfs://documents/dst/a.bin", got.URI())
		s.True(bytes.Equal(content, get(s.T(), s.fs, "/dst/a.bin")))
		s.True(bytes.Equal(content, get(s.T(), s.fs, "/src/a.bin")))
	})

	s.Run("non-zero cursor", func() {
		f := s.file("/src/a.bin")
		_, err := f.Seek(3, io.SeekStart)
		s.Require().NoError(err)
		s.Require().ErrorIs(f.CopyToFile(s.file("/dst/b.bin")), vfs.ErrCopyToNotPossible)
		ok, err := s.file("/dst/b.bin").Exists()
		s.Require().NoError(err)
		s.False(ok)
	})

	s.Run("missing source", func() {
		s.Require().ErrorIs(s.file("/missing.bin").CopyToFile(s.file("/dst/m.bin")), fs.ErrNotExist)
	})

	s.Run("unpublished writes are published first", func() {
		f := s.file("/src/w.txt")
		_, err := f.Write([]byte("fresh"))
		s.Require().NoError(err)
		s.Require().NoError(f.CopyToFile(s.file("/dst/w.txt")))
		s.Equal("fresh", string(get(s.T(), s.fs, "/dst/w.txt")))
	})

	s.Run("to the in-memory backend", func() {
		memLoc, err := mem.NewFileSystem().NewLocation("", "/m/")
		s.Require().NoError(err)
		got, err := s.file("/src/a.bin").CopyToLocation(memLoc)
		s.Require().NoError(err)
		b, err := io.ReadAll(got)
		s.Require().NoError(err)
		s.True(bytes.Equal(content, b))
	})

	s.Run("from the in-memory backend", func() {
		memFile, err := mem.NewFileSystem().NewFile("", "/m/in.txt")
		s.Require().NoError(err)
		_, err = memFile.Write([]byte("from memory"))
		s.Require().NoError(err)
		s.Require().NoError(memFile.Close())
		_, err = memFile.CopyToLocation(dstLoc)
		s.Require().NoError(err)
		s.Equal("from memory", string(get(s.T(), s.fs, "/dst/in.txt")))
	})

	s.Run("onto itself", func() {
		s.Require().NoError(s.file("/src/a.bin").CopyToFile(s.file("/src/a.bin")))
		s.True(bytes.Equal(content, get(s.T(), s.fs, "/src/a.bin")))
	})
}

func (s *FileTestSuite) TestMove() {
	content := pattern(testChunk + 9)
	put(s.T(), s.fs, "/m/src.bin", content)

	s.Run("within the same instance", func() {
		dstLoc, err := s.fs.NewLocation(testNS, "/m2/")
		s.Require().NoError(err)
		got, err := s.file("/m/src.bin").MoveToLocation(dstLoc)
		s.Require().NoError(err)
		s.Equal("sqlfs://documents/m2/src.bin", got.URI())
		s.True(bytes.Equal(content, get(s.T(), s.fs, "/m2/src.bin")))
		ok, err := s.file("/m/src.bin").Exists()
		s.Require().NoError(err)
		s.False(ok)
	})

	s.Run("onto an existing target", func() {
		put(s.T(), s.fs, "/m3/a.txt", []byte("source"))
		put(s.T(), s.fs, "/m3/b.txt", []byte("target"))
		s.Require().NoError(s.file("/m3/a.txt").MoveToFile(s.file("/m3/b.txt")))
		s.Equal("source", string(get(s.T(), s.fs, "/m3/b.txt")))
		ok, err := s.file("/m3/a.txt").Exists()
		s.Require().NoError(err)
		s.False(ok)
	})

	s.Run("across namespaces", func() {
		put(s.T(), s.fs, "/ns.txt", []byte("hop"))
		dst, err := s.fs.NewFile("other", "/ns.txt")
		s.Require().NoError(err)
		s.Require().NoError(s.file("/ns.txt").MoveToFile(dst))
		ok, err := s.file("/ns.txt").Exists()
		s.Require().NoError(err)
		s.False(ok)
		ok, err = dst.Exists()
		s.Require().NoError(err)
		s.True(ok)
	})

	s.Run("onto itself is a no-op", func() {
		put(s.T(), s.fs, "/self.txt", []byte("same"))
		s.Require().NoError(s.file("/self.txt").MoveToFile(s.file("/self.txt")))
		s.Equal("same", string(get(s.T(), s.fs, "/self.txt")))
	})

	s.Run("missing source", func() {
		s.Require().ErrorIs(s.file("/nothing.txt").MoveToFile(s.file("/anywhere.txt")), fs.ErrNotExist)
	})

	s.Run("to and from the in-memory backend", func() {
		put(s.T(), s.fs, "/tomem.txt", []byte("moving out"))
		memLoc, err := mem.NewFileSystem().NewLocation("", "/m/")
		s.Require().NoError(err)
		out, err := s.file("/tomem.txt").MoveToLocation(memLoc)
		s.Require().NoError(err)
		ok, err := s.file("/tomem.txt").Exists()
		s.Require().NoError(err)
		s.False(ok, "source removed after cross-backend move")

		back, err := s.fs.NewLocation(testNS, "/back/")
		s.Require().NoError(err)
		_, err = out.MoveToLocation(back)
		s.Require().NoError(err)
		s.Equal("moving out", string(get(s.T(), s.fs, "/back/tomem.txt")))
	})
}

func (s *FileTestSuite) TestSameInstanceCopyAndMoveDoNotStreamContent() {
	// Copies and moves within one instance happen in the database, so the content must remain correct even though
	// the application never reads it; see FaultTestSuite for the assertion that no chunk read is issued.
	content := pattern(8 * testChunk)
	put(s.T(), s.fs, "/big/a.bin", content)
	s.Require().NoError(s.file("/big/a.bin").CopyToFile(s.file("/big/b.bin")))
	s.Require().NoError(s.file("/big/b.bin").MoveToFile(s.file("/big/c.bin")))
	s.True(bytes.Equal(content, get(s.T(), s.fs, "/big/c.bin")))
	s.True(bytes.Equal(content, get(s.T(), s.fs, "/big/a.bin")))
}
