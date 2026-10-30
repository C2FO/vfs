package mem

import (
	"context"
	"io/fs"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

type ListEntriesTestSuite struct {
	suite.Suite
	fs  *FileSystem
	loc vfs.Location
}

func TestListEntriesTestSuite(t *testing.T) {
	suite.Run(t, new(ListEntriesTestSuite))
}

func (s *ListEntriesTestSuite) SetupTest() {
	s.fs = NewFileSystem()
	loc, err := s.fs.NewLocation("vol", "/root/")
	s.Require().NoError(err)
	s.loc = loc
}

func (s *ListEntriesTestSuite) write(loc vfs.Location, rel, body string) {
	s.T().Helper()
	f, err := loc.NewFile(rel)
	s.Require().NoError(err)
	_, err = f.Write([]byte(body))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
}

func (s *ListEntriesTestSuite) keys(loc vfs.Location, opts ...options.ListOption) []string {
	s.T().Helper()
	entries, err := list.Collect(list.Entries(s.T().Context(), loc, opts...))
	s.Require().NoError(err)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func (s *ListEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.loc)
}

func (s *ListEntriesTestSuite) TestListsFilesAndImpliedDirectories() {
	s.write(s.loc, "a.txt", "a")
	s.write(s.loc, "sub/b.txt", "bb")
	s.write(s.loc, "sub/deep/c.txt", "ccc")
	s.write(s.loc, "a-b.txt", "x")

	s.Equal([]string{"a-b.txt", "a.txt", "sub/", "sub/b.txt", "sub/deep/", "sub/deep/c.txt"}, s.keys(s.loc, list.Recursive()))
	s.Equal([]string{"a-b.txt", "a.txt", "sub/"}, s.keys(s.loc))
}

func (s *ListEntriesTestSuite) TestEntriesCarryTheStoredMetadata() {
	s.write(s.loc, "a.txt", "four")
	f, err := s.loc.NewFile("a.txt")
	s.Require().NoError(err)
	modTime, err := f.LastModified()
	s.Require().NoError(err)

	entries, err := list.Collect(list.Entries(s.T().Context(), s.loc))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	s.True(entries[0].HasMetadata())
	s.Equal(uint64(4), entries[0].Size())
	s.True(modTime.Equal(entries[0].ModTime()))
}

func (s *ListEntriesTestSuite) TestListsOnlyTheLocationsOwnSubtree() {
	s.write(s.loc, "in.txt", "x")
	other, err := s.fs.NewLocation("vol", "/rootbeer/")
	s.Require().NoError(err)
	s.write(other, "out.txt", "x")
	otherVolume, err := s.fs.NewLocation("elsewhere", "/root/")
	s.Require().NoError(err)
	s.write(otherVolume, "alien.txt", "x")

	s.Equal([]string{"in.txt"}, s.keys(s.loc, list.Recursive()))
	s.Equal([]string{"out.txt"}, s.keys(other, list.Recursive()))
}

func (s *ListEntriesTestSuite) TestNestedLocation() {
	s.write(s.loc, "a/b/c.txt", "x")
	s.write(s.loc, "a/d.txt", "x")
	sub, err := s.loc.NewLocation("a/")
	s.Require().NoError(err)

	s.Equal([]string{"b/", "b/c.txt", "d.txt"}, s.keys(sub, list.Recursive()))
}

func (s *ListEntriesTestSuite) TestDeletedFilesAreNotListed() {
	s.write(s.loc, "keep/a.txt", "x")
	s.write(s.loc, "gone/b.txt", "x")
	s.Require().NoError(s.loc.DeleteFile("gone/b.txt"))

	s.Equal([]string{"keep/", "keep/a.txt"}, s.keys(s.loc, list.Recursive()))
}

func (s *ListEntriesTestSuite) TestMissingLocationListsNothing() {
	missing, err := s.fs.NewLocation("nowhere", "/nothing/")
	s.Require().NoError(err)
	s.Empty(s.keys(missing, list.Recursive()))
	s.Empty(s.keys(s.loc, list.Recursive()))
}

func (s *ListEntriesTestSuite) TestSnapshotIgnoresLaterWrites() {
	s.write(s.loc, "a.txt", "x")
	s.write(s.loc, "b.txt", "x")

	var got []string
	for e, err := range s.loc.(vfs.EntryLister).ListEntries(s.T().Context()) {
		s.Require().NoError(err)
		if len(got) == 0 {
			s.write(s.loc, "c.txt", "x")
		}
		got = append(got, e.RelPath())
	}
	s.Equal([]string{"a.txt", "b.txt"}, got)
}

func (s *ListEntriesTestSuite) TestInvalidOptionsAreReportedAsListErrors() {
	var errs []error
	for _, err := range s.loc.(vfs.EntryLister).ListEntries(s.T().Context(), list.FilesOnly(), list.DirsOnly()) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], list.ErrInvalidOption)
	var pe *fs.PathError
	s.Require().ErrorAs(errs[0], &pe)
	s.Equal("list", pe.Op)
	s.Equal("/root/", pe.Path)
}

func (s *ListEntriesTestSuite) TestCanceledContext() {
	s.write(s.loc, "a.txt", "x")
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	var errs []error
	for _, err := range s.loc.(vfs.EntryLister).ListEntries(ctx) {
		errs = append(errs, err)
	}
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], context.Canceled)
}

// TestListingWhileWriting is for the race detector: several writers create and delete files while listings run, and
// listing takes the locks in the order File.Close does.
func (s *ListEntriesTestSuite) TestListingWhileWriting() {
	for i := range 20 {
		s.write(s.loc, "seed/"+string(rune('a'+i))+".txt", "x")
	}

	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 50 {
				f, err := s.loc.NewFile("w" + string(rune('a'+w)) + "/" + string(rune('a'+i%26)) + ".txt")
				if err != nil {
					return
				}
				_, _ = f.Write([]byte("data"))
				_ = f.Close()
				_ = s.loc.DeleteFile("seed/" + string(rune('a'+i%20)) + ".txt")
			}
		}()
	}
	for range 25 {
		for _, err := range s.loc.(vfs.EntryLister).ListEntries(s.T().Context(), list.Recursive()) {
			s.Require().NoError(err)
		}
	}
	wg.Wait()
}
