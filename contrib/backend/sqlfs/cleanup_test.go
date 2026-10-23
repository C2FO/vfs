package sqlfs_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
)

// settle lets the clock move past timestamps recorded by earlier operations.
func settle() { time.Sleep(3 * time.Millisecond) }

type CleanupTestSuite struct {
	suite.Suite
	db *sql.DB
}

func TestCleanupTestSuite(t *testing.T) { suite.Run(t, new(CleanupTestSuite)) }

func (s *CleanupTestSuite) count(query string) int {
	var n int
	s.Require().NoError(s.db.QueryRow(query).Scan(&n))
	return n
}

func (s *CleanupTestSuite) TestCrashedWriterIsRemoved() {
	var fsys *sqlfs.FileSystem
	fsys, s.db = newSQLiteEnv(s.T(), sqlfs.WithChunkSize(testChunk), sqlfs.WithStagingRetention(time.Nanosecond))
	put(s.T(), fsys, "/w.txt", []byte("published"))

	crashed, err := fsys.NewFile(testNS, "/w.txt")
	s.Require().NoError(err)
	_, err = crashed.Write([]byte("never closed"))
	s.Require().NoError(err)
	// flush the staged chunk, then abandon the handle as a crashed process would
	_, err = crashed.Seek(0, io.SeekStart)
	s.Require().NoError(err)
	_, err = crashed.Read(make([]byte, 1))
	s.Require().NoError(err)
	settle()

	stats, err := fsys.Cleanup(s.T().Context())
	s.Require().NoError(err)
	s.Equal(1, stats.StagedRemoved)
	s.Equal("published", string(get(s.T(), fsys, "/w.txt")), "published content is untouched")
	s.Zero(s.count("SELECT COUNT(*) FROM vfs_revisions WHERE state = 0"))

	err = crashed.Close()
	s.Require().ErrorIs(err, sqlfs.ErrConflict, "a reclaimed writer must fail instead of publishing partial data")
}

func (s *CleanupTestSuite) TestActiveWriterIsKept() {
	var fsys *sqlfs.FileSystem
	fsys, s.db = newSQLiteEnv(s.T(), sqlfs.WithChunkSize(testChunk))

	w, err := fsys.NewFile(testNS, "/active.txt")
	s.Require().NoError(err)
	_, err = w.Write([]byte("in progress"))
	s.Require().NoError(err)

	stats, err := fsys.Cleanup(s.T().Context())
	s.Require().NoError(err)
	s.Equal(sqlfs.CleanupStats{}, stats)
	s.Require().NoError(w.Close())
	s.Equal("in progress", string(get(s.T(), fsys, "/active.txt")))
}

func (s *CleanupTestSuite) TestSupersededRevisionsPastRetention() {
	var fsys *sqlfs.FileSystem
	fsys, s.db = newSQLiteEnv(s.T(), sqlfs.WithChunkSize(testChunk), sqlfs.WithRevisionRetention(time.Nanosecond))
	put(s.T(), fsys, "/r.txt", []byte("v1"))
	put(s.T(), fsys, "/r.txt", []byte("v2"))
	gone, err := fsys.NewFile(testNS, "/gone.txt")
	s.Require().NoError(err)
	s.Require().NoError(gone.Touch())
	s.Require().NoError(gone.Delete())
	settle()

	stats, err := fsys.Cleanup(s.T().Context())
	s.Require().NoError(err)
	s.Equal(2, stats.SupersededRemoved)
	s.Equal("v2", string(get(s.T(), fsys, "/r.txt")))
	s.Equal(1, s.count("SELECT COUNT(*) FROM vfs_revisions"), "only the current revision remains")
	s.Equal(1, s.count("SELECT COUNT(*) FROM vfs_chunks"))
}

func (s *CleanupTestSuite) TestRecentlySupersededContentKeptForReaders() {
	var fsys *sqlfs.FileSystem
	fsys, s.db = newSQLiteEnv(s.T(), sqlfs.WithChunkSize(testChunk))
	original := pattern(3 * testChunk)
	put(s.T(), fsys, "/keep.bin", original)

	reader, err := fsys.NewFile(testNS, "/keep.bin")
	s.Require().NoError(err)
	first := make([]byte, 10) //nolint:prealloc // sized for the first read
	_, err = io.ReadFull(reader, first)
	s.Require().NoError(err)

	put(s.T(), fsys, "/keep.bin", []byte("replacement"))
	stats, err := fsys.Cleanup(s.T().Context())
	s.Require().NoError(err)
	s.Zero(stats.SupersededRemoved)

	rest, err := io.ReadAll(reader)
	s.Require().NoError(err)
	s.Equal(original, append(first, rest...))
}

func (s *CleanupTestSuite) TestReclaimedRevisionReportsConflict() {
	var fsys *sqlfs.FileSystem
	fsys, s.db = newSQLiteEnv(s.T(), sqlfs.WithChunkSize(testChunk), sqlfs.WithRevisionRetention(time.Nanosecond))
	put(s.T(), fsys, "/slow.bin", pattern(3*testChunk))

	reader, err := fsys.NewFile(testNS, "/slow.bin")
	s.Require().NoError(err)
	_, err = io.ReadFull(reader, make([]byte, 10))
	s.Require().NoError(err)

	put(s.T(), fsys, "/slow.bin", []byte("replacement"))
	settle()
	_, err = fsys.Cleanup(s.T().Context())
	s.Require().NoError(err)

	_, err = io.ReadAll(reader)
	s.Require().ErrorIs(err, sqlfs.ErrConflict, "must not return empty or mixed content")
}

func (s *CleanupTestSuite) TestCancelledContext() {
	var fsys *sqlfs.FileSystem
	fsys, s.db = newSQLiteEnv(s.T(), sqlfs.WithRevisionRetention(time.Nanosecond))
	put(s.T(), fsys, "/c.txt", []byte("v1"))
	put(s.T(), fsys, "/c.txt", []byte("v2"))
	settle()

	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()
	_, err := fsys.Cleanup(ctx)
	s.Require().ErrorIs(err, context.Canceled)
	s.Equal("v2", string(get(s.T(), fsys, "/c.txt")))
}

func (s *CleanupTestSuite) TestRemovesInBatches() {
	var fsys *sqlfs.FileSystem
	fsys, s.db = newSQLiteEnv(s.T(), sqlfs.WithRevisionRetention(time.Nanosecond))
	const files = 250
	for i := range files {
		f, err := fsys.NewFile(testNS, fmt.Sprintf("/b/%03d.txt", i))
		s.Require().NoError(err)
		s.Require().NoError(f.Touch())
		s.Require().NoError(f.Delete())
	}
	settle()

	stats, err := fsys.Cleanup(s.T().Context())
	s.Require().NoError(err)
	s.Equal(files, stats.SupersededRemoved)
	s.Zero(s.count("SELECT COUNT(*) FROM vfs_revisions"))
}
