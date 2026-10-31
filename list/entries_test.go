package list_test

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"path"
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/options"
)

// nativeLocation is a Location that also lists natively, through a generated EntryLister mock.
type nativeLocation struct {
	*mocks.Location
	lister *mocks.EntryLister
}

func (n nativeLocation) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return n.lister.ListEntries(ctx, opts...)
}

type EntriesTestSuite struct {
	suite.Suite
	loc    nativeLocation
	pulled int
}

func TestEntriesTestSuite(t *testing.T) {
	suite.Run(t, new(EntriesTestSuite))
}

func (s *EntriesTestSuite) SetupTest() {
	base := mocks.NewLocation(s.T())
	base.EXPECT().Path().Return("/root/").Maybe()
	s.loc = nativeLocation{Location: base, lister: mocks.NewEntryLister(s.T())}
	s.pulled = 0
}

// entry builds a listed entry; a trailing "/" makes it a directory.
func (s *EntriesTestSuite) entry(key string) vfs.Entry {
	s.T().Helper()
	build := vfs.NewFileEntry
	rel := key
	if key[len(key)-1] == '/' {
		build = vfs.NewDirEntry
		rel = key[:len(key)-1]
	}
	e, err := build(s.loc.Location, rel)
	s.Require().NoError(err)
	return e
}

func (s *EntriesTestSuite) results(keys ...string) []result {
	out := make([]result, 0, len(keys))
	for _, k := range keys {
		out = append(out, result{entry: s.entry(k)})
	}
	return out
}

// backend makes the location's native listing yield items, counting how many it was asked for and recording the
// options it received.
func (s *EntriesTestSuite) backend(items []result, received *[]options.ListOption) {
	serve := func(_ context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
		if received != nil {
			*received = opts
		}
		return func(yield func(vfs.Entry, error) bool) {
			for _, it := range items {
				s.pulled++
				if !yield(it.entry, it.err) {
					return
				}
			}
		}
	}
	// The mock records a call without options differently from one with them.
	s.loc.lister.EXPECT().ListEntries(mock.Anything, mock.Anything).RunAndReturn(serve).Maybe()
	s.loc.lister.EXPECT().ListEntries(mock.Anything).RunAndReturn(serve).Maybe()
}

// tree is what a backend that ignores every hint yields: the whole tree, in order.
func (s *EntriesTestSuite) tree() []result {
	return s.results(
		"a-b.txt", "a/", "a/b/", "a/b/c.txt", "a/x.csv", "b.csv", "skip/", "skip/deep/", "skip/deep/g.txt", "skip/f.txt",
		"to/", "to/report.csv", "to/x/", "to/x/y.csv", "top.txt",
	)
}

func (s *EntriesTestSuite) list(opts ...options.ListOption) []result {
	s.T().Helper()
	return drain(list.Entries(s.T().Context(), s.loc, opts...))
}

func (s *EntriesTestSuite) TestEveryOptionIsAppliedToWhatTheBackendYields() {
	rejectSkip := func(e vfs.Entry) bool { return e.Name() != "skip" }
	tests := []struct {
		name     string
		opts     []options.ListOption
		expected []string
	}{
		{
			name:     "non-recursive keeps the first level",
			expected: []string{"a-b.txt", "a/", "b.csv", "skip/", "to/", "top.txt"},
		},
		{
			name: "recursive keeps everything",
			opts: []options.ListOption{list.Recursive()},
			expected: []string{
				"a-b.txt", "a/", "a/b/", "a/b/c.txt", "a/x.csv", "b.csv", "skip/", "skip/deep/", "skip/deep/g.txt", "skip/f.txt",
				"to/", "to/report.csv", "to/x/", "to/x/y.csv", "top.txt",
			},
		},
		{
			name: "files only",
			opts: []options.ListOption{list.Recursive(), list.FilesOnly()},
			expected: []string{
				"a-b.txt", "a/b/c.txt", "a/x.csv", "b.csv",
				"skip/deep/g.txt", "skip/f.txt", "to/report.csv", "to/x/y.csv", "top.txt",
			},
		},
		{
			name:     "directories only",
			opts:     []options.ListOption{list.Recursive(), list.DirsOnly()},
			expected: []string{"a/", "a/b/", "skip/", "skip/deep/", "to/", "to/x/"},
		},
		{
			name: "max depth",
			opts: []options.ListOption{list.Recursive(), list.MaxDepth(2)},
			expected: []string{
				"a-b.txt", "a/", "a/b/", "a/x.csv", "b.csv", "skip/",
				"skip/deep/", "skip/f.txt", "to/", "to/report.csv", "to/x/", "top.txt",
			},
		},
		{
			name:     "nested prefix without recursion",
			opts:     []options.ListOption{list.Prefix("to/rep")},
			expected: []string{"to/report.csv"},
		},
		{
			name:     "nested directory prefix with recursion",
			opts:     []options.ListOption{list.Recursive(), list.Prefix("to/")},
			expected: []string{"to/report.csv", "to/x/", "to/x/y.csv"},
		},
		{
			name:     "glob",
			opts:     []options.ListOption{list.Glob("*/*.csv")},
			expected: []string{"a/x.csv", "to/report.csv"},
		},
		{
			name:     "regexp on the relative path",
			opts:     []options.ListOption{list.Recursive(), list.Match(regexp.MustCompile(`\.csv$`))},
			expected: []string{"a/x.csv", "b.csv", "to/report.csv", "to/x/y.csv"},
		},
		{
			name: "custom filter does not prune",
			opts: []options.ListOption{list.Recursive(), list.Filter(func(e vfs.Entry) bool { return e.Name() != "a" })},
			expected: []string{
				"a-b.txt", "a/b/", "a/b/c.txt", "a/x.csv", "b.csv", "skip/", "skip/deep/", "skip/deep/g.txt", "skip/f.txt",
				"to/", "to/report.csv", "to/x/", "to/x/y.csv", "top.txt",
			},
		},
		{
			name: "directory filter prunes but still yields the directory",
			opts: []options.ListOption{list.Recursive(), list.DirFilter(rejectSkip)},
			expected: []string{
				"a-b.txt", "a/", "a/b/", "a/b/c.txt", "a/x.csv", "b.csv",
				"skip/", "to/", "to/report.csv", "to/x/", "to/x/y.csv", "top.txt",
			},
		},
		{
			name:     "directory filter is ignored without recursion",
			opts:     []options.ListOption{list.DirFilter(func(vfs.Entry) bool { return false })},
			expected: []string{"a-b.txt", "a/", "b.csv", "skip/", "to/", "top.txt"},
		},
		{
			name: "cursor inside a subtree",
			opts: []options.ListOption{list.Recursive(), list.StartAfter("a/b/c.txt")},
			expected: []string{
				"a/x.csv", "b.csv", "skip/", "skip/deep/", "skip/deep/g.txt", "skip/f.txt", "to/", "to/report.csv", "to/x/",
				"to/x/y.csv", "top.txt",
			},
		},
		{
			name:     "limit",
			opts:     []options.ListOption{list.Recursive(), list.Limit(3)},
			expected: []string{"a-b.txt", "a/", "a/b/"},
		},
		{
			name:     "backend-specific options are ignored",
			opts:     []options.ListOption{ignoredOption{}, list.Limit(1)},
			expected: []string{"a-b.txt"},
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.SetupTest()
			s.backend(s.tree(), nil)
			rs := s.list(tt.opts...)
			s.Empty(errsOf(rs))
			s.Equal(tt.expected, keysOf(rs))
		})
	}
}

type ignoredOption struct{}

func (ignoredOption) ListOptionName() string { return "backend-specific" }

func (s *EntriesTestSuite) TestOptionsReachTheBackendUnchanged() {
	var received []options.ListOption
	s.backend(nil, &received)
	opts := []options.ListOption{list.Recursive(), list.Limit(5), ignoredOption{}}

	s.list(opts...)
	s.Equal(opts, received)
}

func (s *EntriesTestSuite) TestInvalidOptions() {
	tests := []struct {
		name string
		opts []options.ListOption
	}{
		{name: "conflicting kinds", opts: []options.ListOption{list.FilesOnly(), list.DirsOnly()}},
		{name: "duplicate option", opts: []options.ListOption{list.Limit(1), list.Limit(2)}},
		{name: "escaping prefix", opts: []options.ListOption{list.Prefix("../other")}},
		{name: "nil option", opts: []options.ListOption{nil}},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.SetupTest() // no backend expectations: any request fails the test
			rs := s.list(tt.opts...)

			s.Require().Len(rs, 1)
			s.Require().ErrorIs(rs[0].err, list.ErrInvalidOption)
			var pe *fs.PathError
			s.Require().ErrorAs(rs[0].err, &pe)
			s.Equal("list", pe.Op)
			s.Equal("/root/", pe.Path)
			s.Equal(vfs.Entry{}.RelPath(), rs[0].entry.RelPath())
		})
	}
}

func (s *EntriesTestSuite) TestBadGlobMatchesBothErrors() {
	rs := s.list(list.Glob("["))
	s.Require().Len(rs, 1)
	s.Require().ErrorIs(rs[0].err, list.ErrInvalidOption)
	s.Require().ErrorIs(rs[0].err, path.ErrBadPattern)
}

func (s *EntriesTestSuite) TestBackendReturningNoSequenceYieldsNothing() {
	s.loc.lister.EXPECT().ListEntries(mock.Anything).Return(nil)

	s.Empty(s.list())
}

func (s *EntriesTestSuite) TestBuildingTheIteratorMakesNoRequest() {
	// The mocks fail the test on any unexpected call.
	_ = list.Entries(s.T().Context(), s.loc, list.Recursive())
}

func (s *EntriesTestSuite) TestErrorsBypassEveryFilter() {
	denied := s.entry("b/")
	items := []result{
		{entry: s.entry("a.txt")},
		{entry: denied},
		{entry: denied, err: &fs.PathError{Op: "list", Path: "b", Err: fs.ErrPermission}},
		{entry: s.entry("c.txt")},
	}
	s.backend(items, nil)

	rs := s.list(list.Recursive(), list.FilesOnly(), list.Match(regexp.MustCompile(`^never$`)))
	s.Empty(keysOf(rs))
	errs := errsOf(rs)
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], fs.ErrPermission)
	s.Equal("b/", list.OrderKey(rs[0].entry), "the unreadable directory's entry comes with its error")
}

func (s *EntriesTestSuite) TestPartialErrorDoesNotEndTheListing() {
	denied := s.entry("b/")
	s.backend([]result{
		{entry: s.entry("a/")},
		{entry: denied},
		{entry: denied, err: fs.ErrPermission},
		{entry: s.entry("c/")},
	}, nil)

	rs := s.list(list.Recursive(), list.DirsOnly())
	s.Equal([]string{"a/", "b/", "b/", "c/"}, []string{
		list.OrderKey(rs[0].entry), list.OrderKey(rs[1].entry), list.OrderKey(rs[2].entry), list.OrderKey(rs[3].entry),
	})
	s.Require().ErrorIs(rs[2].err, fs.ErrPermission)
}

func (s *EntriesTestSuite) TestFatalErrorEndsTheListing() {
	s.backend([]result{
		{entry: s.entry("a.txt")},
		{err: fs.ErrPermission},
		{entry: s.entry("b.txt")},
	}, nil)

	rs := s.list(list.Recursive())
	s.Require().Len(rs, 2)
	s.Equal([]string{"a.txt"}, keysOf(rs))
	s.Require().ErrorIs(rs[1].err, fs.ErrPermission)
}

func (s *EntriesTestSuite) TestUnrepresentableKeyIsNotFatalAndNotCounted() {
	s.backend([]result{
		{entry: s.entry("a.txt")},
		{err: &fs.PathError{Op: "list", Path: "m//n", Err: fs.ErrInvalid}},
		{entry: s.entry("b.txt")},
		{entry: s.entry("c.txt")},
	}, nil)

	rs := s.list(list.Recursive(), list.Limit(2))
	s.Equal([]string{"a.txt", "b.txt"}, keysOf(rs))
	s.Require().Len(errsOf(rs), 1)
	s.Require().ErrorIs(errsOf(rs)[0], fs.ErrInvalid)
}

func (s *EntriesTestSuite) TestLimitStopsRequestingEntries() {
	s.backend(s.tree(), nil)

	rs := s.list(list.Recursive(), list.Limit(2))
	s.Len(rs, 2)
	s.Equal(2, s.pulled, "nothing is read from the backend after the limit")
}

func (s *EntriesTestSuite) TestBreakStopsRequestingEntries() {
	s.backend(s.tree(), nil)

	for _, err := range list.Entries(s.T().Context(), s.loc, list.Recursive()) {
		s.Require().NoError(err)
		break
	}
	s.Equal(1, s.pulled)
}

func (s *EntriesTestSuite) TestPagesConcatenateToTheFullListing() {
	s.backend(s.tree(), nil)
	full := keysOf(s.list(list.Recursive()))

	for _, size := range []int{1, 2, 4, 7, 20} {
		var got []string
		cursor := list.StartAfter("")
		for {
			rs := s.list(list.Recursive(), list.Limit(size), cursor)
			s.Require().Empty(errsOf(rs))
			if len(rs) == 0 {
				break
			}
			got = append(got, keysOf(rs)...)
			cursor = list.After(rs[len(rs)-1].entry)
		}
		s.Equal(full, got, "page size %d", size)
	}
}

type ctxKey struct{}

func (s *EntriesTestSuite) TestContextCallbacksReceiveTheListingContext() {
	s.backend(s.tree(), nil)
	ctx := context.WithValue(s.T().Context(), ctxKey{}, "listing")

	var filterSaw, dirSaw []any
	opts := []options.ListOption{
		list.Recursive(),
		list.FilterContext(func(ctx context.Context, _ vfs.Entry) (bool, error) {
			filterSaw = append(filterSaw, ctx.Value(ctxKey{}))
			return true, nil
		}),
		list.DirFilterContext(func(ctx context.Context, _ vfs.Entry) (bool, error) {
			dirSaw = append(dirSaw, ctx.Value(ctxKey{}))
			return true, nil
		}),
	}
	rs := drain(list.Entries(ctx, s.loc, opts...))

	s.Empty(errsOf(rs))
	s.Require().NotEmpty(filterSaw)
	s.Require().NotEmpty(dirSaw)
	for _, v := range append(filterSaw, dirSaw...) {
		s.Equal("listing", v)
	}
}

func (s *EntriesTestSuite) TestCallbackErrorIsFatalAndNamesTheEntry() {
	boom := errors.New("stat failed")
	tests := []struct {
		name         string
		opt          options.ListOption
		expectedKeys []string
		expectedPath string
	}{
		{
			name: "filter",
			opt: list.FilterContext(func(_ context.Context, e vfs.Entry) (bool, error) {
				if e.Name() == "b.csv" {
					return false, boom
				}
				return true, nil
			}),
			expectedKeys: []string{"a-b.txt", "a/", "a/b/", "a/b/c.txt", "a/x.csv"},
			expectedPath: "/root/b.csv",
		},
		{
			name: "directory filter",
			opt: list.DirFilterContext(func(_ context.Context, e vfs.Entry) (bool, error) {
				if e.Name() == "skip" {
					return false, boom
				}
				return true, nil
			}),
			expectedKeys: []string{"a-b.txt", "a/", "a/b/", "a/b/c.txt", "a/x.csv", "b.csv"},
			expectedPath: "/root/skip/",
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.SetupTest()
			s.backend(s.tree(), nil)

			rs := s.list(list.Recursive(), tt.opt)
			s.Equal(tt.expectedKeys, keysOf(rs))
			errs := errsOf(rs)
			s.Require().Len(errs, 1, "the listing ends at the failed callback")
			s.Require().ErrorIs(errs[0], boom)
			var pe *fs.PathError
			s.Require().ErrorAs(errs[0], &pe)
			s.Equal("list", pe.Op)
			s.Equal(tt.expectedPath, pe.Path)
			s.Equal(vfs.Entry{}.RelPath(), rs[len(rs)-1].entry.RelPath())
		})
	}
}

func (s *EntriesTestSuite) TestDirFilterIsNotAskedAboutDirectoriesAtTheDepthLimit() {
	s.backend(s.tree(), nil)
	var asked []string
	s.list(list.Recursive(), list.MaxDepth(2), list.DirFilter(func(e vfs.Entry) bool {
		asked = append(asked, e.RelPath())
		return true
	}))
	s.Equal([]string{"a", "skip", "to"}, asked)
}

func (s *EntriesTestSuite) TestPrunedDirectoryBeforeTheCursorStillPrunes() {
	s.backend(s.tree(), nil)
	rejectA := list.DirFilter(func(e vfs.Entry) bool { return e.Name() != "a" })

	rs := s.list(list.Recursive(), rejectA, list.StartAfter("a/b/c.txt"))
	s.Empty(errsOf(rs))
	s.Equal(
		[]string{"b.csv", "skip/", "skip/deep/", "skip/deep/g.txt", "skip/f.txt", "to/", "to/report.csv", "to/x/", "to/x/y.csv", "top.txt"},
		keysOf(rs),
		"nothing beneath the pruned directory a is yielded, whichever page it falls on",
	)
}

func (s *EntriesTestSuite) TestDirFilterIsNotAskedAboutDirectoriesOutsideThePrefix() {
	s.backend(s.tree(), nil)
	var asked []string
	s.list(list.Recursive(), list.Prefix("to/"), list.DirFilter(func(e vfs.Entry) bool {
		asked = append(asked, e.RelPath())
		return true
	}))
	s.Equal([]string{"to", "to/x"}, asked)
}

func (s *EntriesTestSuite) TestCancellation() {
	s.Run("canceled before the listing starts", func() {
		s.SetupTest()
		s.backend(s.tree(), nil)
		ctx, cancel := context.WithCancel(s.T().Context())
		cancel()

		rs := drain(list.Entries(ctx, s.loc, list.Recursive()))
		s.Require().Len(rs, 1)
		s.Require().ErrorIs(rs[0].err, context.Canceled)
	})

	s.Run("canceled while iterating", func() {
		s.SetupTest()
		s.backend(s.tree(), nil)
		ctx, cancel := context.WithCancel(s.T().Context())
		defer cancel()

		var got []result
		for e, err := range list.Entries(ctx, s.loc, list.Recursive()) {
			got = append(got, result{entry: e, err: err})
			if len(got) == 2 {
				cancel()
			}
		}
		s.Require().Len(got, 3)
		s.Require().NoError(got[1].err)
		s.Require().ErrorIs(got[2].err, context.Canceled)
	})
}
