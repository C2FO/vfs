package list_test

import (
	"errors"
	"iter"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/mocks"
)

type HelpersTestSuite struct {
	suite.Suite
	loc *mocks.Location
}

func TestHelpersTestSuite(t *testing.T) {
	suite.Run(t, new(HelpersTestSuite))
}

func (s *HelpersTestSuite) SetupTest() {
	s.loc = mocks.NewLocation(s.T())
	s.loc.EXPECT().Path().Return("/root/").Maybe()
}

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func (s *HelpersTestSuite) file(rel string, size uint64, age time.Duration) vfs.Entry {
	s.T().Helper()
	e, err := vfs.NewFileEntry(s.loc, rel, vfs.WithEntryMetadata(size, epoch.Add(age)))
	s.Require().NoError(err)
	return e
}

func seqOf(entries []vfs.Entry, failAt int, err error) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		for i, e := range entries {
			if i == failAt {
				yield(vfs.Entry{}, err)
				return
			}
			if !yield(e, nil) {
				return
			}
		}
	}
}

func rels(entries []vfs.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.RelPath())
	}
	return out
}

func (s *HelpersTestSuite) TestCollect() {
	entries := []vfs.Entry{s.file("a", 1, 0), s.file("b", 2, 0)}
	boom := errors.New("boom")
	tests := []struct {
		name        string
		seq         iter.Seq2[vfs.Entry, error]
		expected    []string
		expectedErr error
	}{
		{name: "all entries in order", seq: seqOf(entries, -1, nil), expected: []string{"a", "b"}},
		{name: "empty listing", seq: seqOf(nil, -1, nil)},
		{name: "first error and no entries", seq: seqOf(entries, 1, boom), expectedErr: boom},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			got, err := list.Collect(tt.seq)
			if tt.expectedErr != nil {
				s.Require().ErrorIs(err, tt.expectedErr)
				s.Nil(got)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, append([]string(nil), rels(got)...))
		})
	}
}

func (s *HelpersTestSuite) TestComparators() {
	small := s.file("b/x", 1, 2*time.Hour)
	large := s.file("a/x", 9, time.Hour)
	other := s.file("a/y", 9, time.Hour)

	s.Negative(list.BySize(small, large))
	s.Positive(list.BySize(large, small))
	s.Zero(list.BySize(large, other))

	s.Positive(list.ByModTime(small, large))
	s.Negative(list.ByModTime(large, small))
	s.Zero(list.ByModTime(large, other))

	s.Equal(0, list.ByName(large, large))
	s.Negative(list.ByName(large, other))
	s.Negative(list.ByName(large, small), "same name, ordered by relative path")
	s.Positive(list.ByName(other, large))

	entries := []vfs.Entry{small, large, other}
	slices.SortFunc(entries, list.BySize)
	s.Equal([]string{"b/x", "a/x", "a/y"}, rels(entries))
}

func (s *HelpersTestSuite) TestTopNMatchesASortedPrefix() {
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // fixed seed keeps the test reproducible
	for trial := range 200 {
		count := rng.IntN(60)
		entries := make([]vfs.Entry, 0, count)
		for i := range count {
			// Few distinct sizes, so ties are common and their order is exercised.
			entries = append(entries, s.file(string(rune('a'+i%26))+string(rune('a'+i/26)), uint64(rng.IntN(8)), 0))
		}
		n := rng.IntN(count+3) + 1
		desc := func(a, b vfs.Entry) int { return list.BySize(b, a) }

		expected := slices.Clone(entries)
		slices.SortStableFunc(expected, desc)
		expected = expected[:min(n, len(expected))]

		got, err := list.TopN(seqOf(entries, -1, nil), n, desc)
		s.Require().NoError(err)
		s.Require().Equal(rels(expected), rels(got), "trial %d, n %d", trial, n)
	}
}

func (s *HelpersTestSuite) TestTopNLargestFiles() {
	entries := []vfs.Entry{s.file("a", 5, 0), s.file("b", 50, 0), s.file("c", 20, 0), s.file("d", 1, 0), s.file("e", 30, 0)}
	got, err := list.TopN(seqOf(entries, -1, nil), 3, func(a, b vfs.Entry) int { return list.BySize(b, a) })

	s.Require().NoError(err)
	s.Equal([]string{"b", "e", "c"}, rels(got))
}

func (s *HelpersTestSuite) TestTopNKeepsAtMostNEntries() {
	// A sequence far longer than n is consumed in full but only n entries are held: the result has length n.
	entries := make([]vfs.Entry, 0, 1000)
	for i := range 1000 {
		entries = append(entries, s.file("f"+string(rune('0'+i%10)), uint64(i), 0))
	}
	got, err := list.TopN(seqOf(entries, -1, nil), 4, list.BySize)

	s.Require().NoError(err)
	s.Len(got, 4)
	s.Equal([]uint64{0, 1, 2, 3}, []uint64{got[0].Size(), got[1].Size(), got[2].Size(), got[3].Size()})
}

func (s *HelpersTestSuite) TestTopNErrors() {
	entries := []vfs.Entry{s.file("a", 1, 0), s.file("b", 2, 0)}
	boom := errors.New("boom")
	consumed := false
	watching := func(yield func(vfs.Entry, error) bool) {
		consumed = true
		seqOf(entries, -1, nil)(yield)
	}

	tests := []struct {
		name        string
		seq         iter.Seq2[vfs.Entry, error]
		n           int
		cmp         func(a, b vfs.Entry) int
		expectedErr error
	}{
		{name: "listing error", seq: seqOf(entries, 1, boom), n: 2, cmp: list.BySize, expectedErr: boom},
		{name: "n below one", seq: watching, n: 0, cmp: list.BySize, expectedErr: list.ErrInvalidOption},
		{name: "negative n", seq: watching, n: -3, cmp: list.BySize, expectedErr: list.ErrInvalidOption},
		{name: "nil comparator", seq: watching, n: 1, cmp: nil, expectedErr: list.ErrInvalidOption},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			consumed = false
			got, err := list.TopN(tt.seq, tt.n, tt.cmp)
			s.Require().ErrorIs(err, tt.expectedErr)
			s.Nil(got)
			if errors.Is(tt.expectedErr, list.ErrInvalidOption) {
				s.False(consumed, "an invalid call does not start the listing")
			}
		})
	}
}
