package list_test

import (
	"context"
	"errors"
	"path"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/options"
)

type QueryTestSuite struct {
	suite.Suite
}

func TestQueryTestSuite(t *testing.T) {
	suite.Run(t, new(QueryTestSuite))
}

// foreignOption stands in for a backend-specific option that the list package knows nothing about.
type foreignOption struct{}

func (foreignOption) ListOptionName() string { return "foreign" }

func (s *QueryTestSuite) fileEntry(rel string) vfs.Entry {
	s.T().Helper()
	e, err := vfs.NewFileEntry(mocks.NewLocation(s.T()), rel)
	s.Require().NoError(err)
	return e
}

func (s *QueryTestSuite) dirEntry(rel string) vfs.Entry {
	s.T().Helper()
	e, err := vfs.NewDirEntry(mocks.NewLocation(s.T()), rel)
	s.Require().NoError(err)
	return e
}

func (s *QueryTestSuite) TestNoOptions() {
	q, err := list.ParseQuery(nil)
	s.Require().NoError(err)
	s.Equal(list.Query{}, q)
	s.Equal(list.KindAny, q.Kind)
}

func (s *QueryTestSuite) TestValidOptions() {
	re := regexp.MustCompile(`\.csv$`)

	tests := []struct {
		name     string
		opts     []options.ListOption
		expected func(q list.Query) bool
	}{
		{
			name:     "prefix",
			opts:     []options.ListOption{list.Prefix("to/rep")},
			expected: func(q list.Query) bool { return q.Prefix == "to/rep" },
		},
		{
			name:     "prefix with trailing slash",
			opts:     []options.ListOption{list.Prefix("to/")},
			expected: func(q list.Query) bool { return q.Prefix == "to/" },
		},
		{
			name:     "empty prefix is omitted",
			opts:     []options.ListOption{list.Prefix("")},
			expected: func(q list.Query) bool { return q.Prefix == "" },
		},
		{
			name:     "recursive",
			opts:     []options.ListOption{list.Recursive()},
			expected: func(q list.Query) bool { return q.Recursive && q.MaxDepth == 0 },
		},
		{
			name:     "max depth with recursive",
			opts:     []options.ListOption{list.Recursive(), list.MaxDepth(2)},
			expected: func(q list.Query) bool { return q.Recursive && q.MaxDepth == 2 },
		},
		{
			name:     "files only",
			opts:     []options.ListOption{list.FilesOnly()},
			expected: func(q list.Query) bool { return q.Kind == list.KindFiles },
		},
		{
			name:     "dirs only",
			opts:     []options.ListOption{list.DirsOnly()},
			expected: func(q list.Query) bool { return q.Kind == list.KindDirs },
		},
		{
			name:     "start after file",
			opts:     []options.ListOption{list.StartAfter("a/b.txt")},
			expected: func(q list.Query) bool { return q.StartAfter == "a/b.txt" },
		},
		{
			name:     "start after directory",
			opts:     []options.ListOption{list.StartAfter("a/b/")},
			expected: func(q list.Query) bool { return q.StartAfter == "a/b/" },
		},
		{
			name:     "empty start after is omitted",
			opts:     []options.ListOption{list.StartAfter("")},
			expected: func(q list.Query) bool { return q.StartAfter == "" },
		},
		{
			name:     "limit",
			opts:     []options.ListOption{list.Limit(10)},
			expected: func(q list.Query) bool { return q.Limit == 10 },
		},
		{
			name:     "match",
			opts:     []options.ListOption{list.Match(re)},
			expected: func(q list.Query) bool { return q.Match == re },
		},
		{
			name:     "unknown options are ignored",
			opts:     []options.ListOption{foreignOption{}, list.Limit(1)},
			expected: func(q list.Query) bool { return q.Limit == 1 },
		},
		{
			name:     "empty prefix may accompany a glob",
			opts:     []options.ListOption{list.Prefix(""), list.Glob("*.csv")},
			expected: func(q list.Query) bool { return q.Glob == "*.csv" },
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			q, err := list.ParseQuery(tt.opts)
			s.Require().NoError(err)
			s.True(tt.expected(q), "unexpected query: %+v", q)
		})
	}
}

func (s *QueryTestSuite) TestInvalidOptions() {
	tests := []struct {
		name string
		opts []options.ListOption
	}{
		{name: "prefix escapes the location", opts: []options.ListOption{list.Prefix("../other")}},
		{name: "prefix is absolute", opts: []options.ListOption{list.Prefix("/a")}},
		{name: "prefix has an empty segment", opts: []options.ListOption{list.Prefix("a//b")}},
		{name: "prefix has two trailing slashes", opts: []options.ListOption{list.Prefix("a//")}},
		{name: "prefix is dot", opts: []options.ListOption{list.Prefix(".")}},
		{name: "prefix is only a slash", opts: []options.ListOption{list.Prefix("/")}},
		{name: "start after escapes the location", opts: []options.ListOption{list.StartAfter("../x")}},
		{name: "start after is absolute", opts: []options.ListOption{list.StartAfter("/x")}},
		{name: "start after is dot", opts: []options.ListOption{list.StartAfter(".")}},
		{name: "start after has two trailing slashes", opts: []options.ListOption{list.StartAfter("a//")}},
		{name: "glob is malformed", opts: []options.ListOption{list.Glob("[")}},
		{name: "glob is empty", opts: []options.ListOption{list.Glob("")}},
		{name: "glob has an empty segment", opts: []options.ListOption{list.Glob("a//b")}},
		{name: "glob is absolute", opts: []options.ListOption{list.Glob("/a")}},
		{name: "glob has a trailing slash", opts: []options.ListOption{list.Glob("a/")}},
		{name: "glob escapes the location", opts: []options.ListOption{list.Glob("../*")}},
		{name: "glob has a dot segment", opts: []options.ListOption{list.Glob("a/./b")}},
		{name: "nil regexp", opts: []options.ListOption{list.Match(nil)}},
		{name: "nil filter", opts: []options.ListOption{list.Filter(nil)}},
		{name: "nil filter with context", opts: []options.ListOption{list.FilterContext(nil)}},
		{name: "nil dir filter", opts: []options.ListOption{list.DirFilter(nil)}},
		{name: "nil dir filter with context", opts: []options.ListOption{list.DirFilterContext(nil)}},
		{name: "nil option", opts: []options.ListOption{nil}},
		{name: "files only with dirs only", opts: []options.ListOption{list.FilesOnly(), list.DirsOnly()}},
		{name: "max depth without recursive", opts: []options.ListOption{list.MaxDepth(2)}},
		{name: "max depth of zero", opts: []options.ListOption{list.Recursive(), list.MaxDepth(0)}},
		{name: "negative max depth", opts: []options.ListOption{list.Recursive(), list.MaxDepth(-1)}},
		{name: "limit of zero", opts: []options.ListOption{list.Limit(0)}},
		{name: "negative limit", opts: []options.ListOption{list.Limit(-3)}},
		{name: "glob with prefix", opts: []options.ListOption{list.Glob("*.csv"), list.Prefix("a")}},
		{name: "glob with recursive", opts: []options.ListOption{list.Glob("*.csv"), list.Recursive()}},
		{name: "glob with max depth", opts: []options.ListOption{list.Glob("*.csv"), list.Recursive(), list.MaxDepth(2)}},
		{name: "after with the zero entry", opts: []options.ListOption{list.After(vfs.Entry{})}},
		{name: "prefix twice", opts: []options.ListOption{list.Prefix("a"), list.Prefix("b")}},
		{name: "limit twice", opts: []options.ListOption{list.Limit(1), list.Limit(2)}},
		{name: "recursive twice", opts: []options.ListOption{list.Recursive(), list.Recursive()}},
		{name: "files only twice", opts: []options.ListOption{list.FilesOnly(), list.FilesOnly()}},
		{name: "start after twice", opts: []options.ListOption{list.StartAfter("a"), list.StartAfter("b")}},
		{name: "start after with after", opts: []options.ListOption{list.StartAfter("a"), list.After(s.fileEntry("b"))}},
		{
			name: "filter with filter context",
			opts: []options.ListOption{
				list.Filter(func(vfs.Entry) bool { return true }),
				list.FilterContext(func(context.Context, vfs.Entry) (bool, error) { return true, nil }),
			},
		},
		{
			name: "dir filter with dir filter context",
			opts: []options.ListOption{
				list.DirFilter(func(vfs.Entry) bool { return true }),
				list.DirFilterContext(func(context.Context, vfs.Entry) (bool, error) { return true, nil }),
			},
		},
		{name: "empty prefix twice still counts as a duplicate", opts: []options.ListOption{list.Prefix(""), list.Prefix("")}},
		{name: "second option invalid after a valid one", opts: []options.ListOption{list.Limit(5), list.MaxDepth(1)}},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			q, err := list.ParseQuery(tt.opts)
			s.Require().ErrorIs(err, list.ErrInvalidOption)
			s.Equal(list.Query{}, q)
		})
	}
}

func (s *QueryTestSuite) TestBadGlobMatchesBadPattern() {
	patterns := []string{"[", "a[", "*[", "?[", "x*[", "[a-", "[]a]", "[^]", "[a-]", "[-a]", "[a-b-c]", `\`, `a\`, "a/[b", "a/*/["}
	for _, pattern := range patterns {
		s.Run(pattern, func() {
			_, err := list.ParseQuery([]options.ListOption{list.Glob(pattern)})
			s.Require().ErrorIs(err, list.ErrInvalidOption)
			s.Require().ErrorIs(err, path.ErrBadPattern)
		})
	}
}

func (s *QueryTestSuite) TestGlobAcceptsPatternsPathMatchAccepts() {
	// These are not errors under path.Match, and Glob follows it exactly.
	for _, pattern := range []string{"[b-a]", "[[:alpha:]]", "**/x", `a\*b`} {
		s.Run(pattern, func() {
			_, err := list.ParseQuery([]options.ListOption{list.Glob(pattern)})
			s.Require().NoError(err)
		})
	}
}

func (s *QueryTestSuite) TestGlobCombinesWithPagingAndFilters() {
	q, err := list.ParseQuery([]options.ListOption{
		list.Glob("*/data.csv"),
		list.StartAfter("a/data.csv"),
		list.Limit(5),
		list.FilesOnly(),
		list.Match(regexp.MustCompile(`csv$`)),
		list.Filter(func(vfs.Entry) bool { return true }),
		list.DirFilter(func(vfs.Entry) bool { return true }),
	})
	s.Require().NoError(err)
	s.Equal("*/data.csv", q.Glob)
	s.Equal("a/data.csv", q.StartAfter)
	s.Equal(5, q.Limit)
	s.Equal(list.KindFiles, q.Kind)
	s.NotNil(q.Match)
	s.NotNil(q.Filter)
	s.NotNil(q.Descend)
	s.True(q.Recursive)
	s.Equal(2, q.MaxDepth)
}

func (s *QueryTestSuite) TestDepth() {
	tests := []struct {
		name     string
		opts     []options.ListOption
		expected int
	}{
		{name: "no options lists immediate children", expected: 1},
		{name: "recursive is unlimited", opts: []options.ListOption{list.Recursive()}, expected: 0},
		{name: "recursive with max depth", opts: []options.ListOption{list.Recursive(), list.MaxDepth(3)}, expected: 3},
		{name: "flat prefix without recursion", opts: []options.ListOption{list.Prefix("rep")}, expected: 1},
		{name: "nested prefix without recursion", opts: []options.ListOption{list.Prefix("to/rep")}, expected: 2},
		{name: "directory prefix without recursion", opts: []options.ListOption{list.Prefix("to/")}, expected: 2},
		{name: "deeply nested prefix without recursion", opts: []options.ListOption{list.Prefix("a/b/c")}, expected: 3},
		{name: "nested prefix with recursion is unlimited", opts: []options.ListOption{list.Prefix("to/rep"), list.Recursive()}, expected: 0},
		{
			name:     "nested prefix with recursion counts from the location",
			opts:     []options.ListOption{list.Prefix("a/"), list.Recursive(), list.MaxDepth(2)},
			expected: 2,
		},
		{name: "single segment glob", opts: []options.ListOption{list.Glob("*.csv")}, expected: 1},
		{name: "two segment glob", opts: []options.ListOption{list.Glob("*/data.csv")}, expected: 2},
		{name: "literal path glob", opts: []options.ListOption{list.Glob("a/b/c.txt")}, expected: 3},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			q, err := list.ParseQuery(tt.opts)
			s.Require().NoError(err)
			s.Equal(tt.expected, q.Depth())
		})
	}
}

func (s *QueryTestSuite) TestGlobCompilation() {
	tests := []struct {
		name              string
		pattern           string
		expectedPrefix    string
		expectedRecursive bool
		expectedMaxDepth  int
	}{
		{name: "one level of wildcard directory", pattern: "*/data.csv", expectedRecursive: true, expectedMaxDepth: 2},
		{name: "literal directory then wildcard segment", pattern: "a/b*/c", expectedPrefix: "a/", expectedRecursive: true, expectedMaxDepth: 3},
		{name: "fully literal path", pattern: "a/b/c.txt", expectedPrefix: "a/b/", expectedRecursive: true, expectedMaxDepth: 3},
		{name: "single segment is not recursive", pattern: "*.csv"},
		{name: "single literal segment", pattern: "data.csv"},
		{name: "literal directories then wildcard file", pattern: "x/y/*", expectedPrefix: "x/y/", expectedRecursive: true, expectedMaxDepth: 3},
		{
			name: "character class after a literal directory", pattern: "a/[bc]/x",
			expectedPrefix: "a/", expectedRecursive: true, expectedMaxDepth: 3,
		},
		{name: "question mark in the first segment", pattern: "?/a", expectedRecursive: true, expectedMaxDepth: 2},
		{name: "escape ends the literal prefix", pattern: `a/b\*/c`, expectedPrefix: "a/", expectedRecursive: true, expectedMaxDepth: 3},
		{
			name: "wildcard inside the last directory", pattern: "logs/2026-*/app.log",
			expectedPrefix: "logs/", expectedRecursive: true, expectedMaxDepth: 3,
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			q, err := list.ParseQuery([]options.ListOption{list.Glob(tt.pattern)})
			s.Require().NoError(err)
			s.Equal(tt.pattern, q.Glob)
			s.Equal(tt.expectedPrefix, q.Prefix)
			s.Equal(tt.expectedRecursive, q.Recursive)
			s.Equal(tt.expectedMaxDepth, q.MaxDepth)
		})
	}
}

func (s *QueryTestSuite) TestOrderKey() {
	tests := []struct {
		name     string
		entry    vfs.Entry
		expected string
	}{
		{name: "file", entry: s.fileEntry("a/b.txt"), expected: "a/b.txt"},
		{name: "directory", entry: s.dirEntry("a/b"), expected: "a/b/"},
		{name: "top-level directory", entry: s.dirEntry("logs"), expected: "logs/"},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.Equal(tt.expected, list.OrderKey(tt.entry))
		})
	}

	s.Run("zero entry", func() {
		s.Empty(list.OrderKey(vfs.Entry{}))
	})

	s.Run("file sorts before a directory of the same prefix", func() {
		s.Less(list.OrderKey(s.fileEntry("logs-2024.gz")), list.OrderKey(s.dirEntry("logs")))
	})
}

func (s *QueryTestSuite) TestAfterUsesTheOrderKey() {
	tests := []struct {
		name     string
		entry    vfs.Entry
		expected string
	}{
		{name: "file", entry: s.fileEntry("a/b.txt"), expected: "a/b.txt"},
		{name: "directory", entry: s.dirEntry("a/b"), expected: "a/b/"},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			q, err := list.ParseQuery([]options.ListOption{list.After(tt.entry)})
			s.Require().NoError(err)
			s.Equal(tt.expected, q.StartAfter)
		})
	}
}

func (s *QueryTestSuite) TestFilters() {
	entry := s.fileEntry("a.txt")
	type ctxKey struct{}
	ctx := context.WithValue(s.T().Context(), ctxKey{}, "listing")

	s.Run("plain filter ignores the context and cannot fail", func() {
		var got vfs.Entry
		q, err := list.ParseQuery([]options.ListOption{list.Filter(func(e vfs.Entry) bool {
			got = e
			return false
		})})
		s.Require().NoError(err)

		keep, err := q.Filter(ctx, entry)
		s.Require().NoError(err)
		s.False(keep)
		s.Equal(entry.RelPath(), got.RelPath())
	})
	s.Run("context filter receives the listing context", func() {
		var got context.Context
		q, err := list.ParseQuery([]options.ListOption{list.FilterContext(func(ctx context.Context, _ vfs.Entry) (bool, error) {
			got = ctx
			return true, nil
		})})
		s.Require().NoError(err)

		keep, err := q.Filter(ctx, entry)
		s.Require().NoError(err)
		s.True(keep)
		s.Equal("listing", got.Value(ctxKey{}))
	})
	s.Run("context filter error is returned", func() {
		boom := errors.New("boom")
		q, err := list.ParseQuery([]options.ListOption{list.FilterContext(func(context.Context, vfs.Entry) (bool, error) {
			return true, boom
		})})
		s.Require().NoError(err)

		_, err = q.Filter(ctx, entry)
		s.Require().ErrorIs(err, boom)
	})
	s.Run("plain dir filter becomes Descend", func() {
		q, err := list.ParseQuery([]options.ListOption{list.DirFilter(func(e vfs.Entry) bool { return e.Name() != "skip" })})
		s.Require().NoError(err)

		descend, err := q.Descend(ctx, s.dirEntry("skip"))
		s.Require().NoError(err)
		s.False(descend)
		descend, err = q.Descend(ctx, s.dirEntry("keep"))
		s.Require().NoError(err)
		s.True(descend)
	})
	s.Run("context dir filter receives the listing context and can fail", func() {
		boom := errors.New("boom")
		var got context.Context
		q, err := list.ParseQuery([]options.ListOption{list.DirFilterContext(func(ctx context.Context, _ vfs.Entry) (bool, error) {
			got = ctx
			return false, boom
		})})
		s.Require().NoError(err)

		_, err = q.Descend(ctx, s.dirEntry("d"))
		s.Require().ErrorIs(err, boom)
		s.Equal("listing", got.Value(ctxKey{}))
	})
	s.Run("no filters leaves them nil", func() {
		q, err := list.ParseQuery(nil)
		s.Require().NoError(err)
		s.Nil(q.Filter)
		s.Nil(q.Descend)
	})
}

func (s *QueryTestSuite) TestOptionNames() {
	tests := []struct {
		opt  options.ListOption
		name string
	}{
		{opt: list.Prefix("a"), name: "prefix"},
		{opt: list.Recursive(), name: "recursive"},
		{opt: list.MaxDepth(1), name: "maxDepth"},
		{opt: list.FilesOnly(), name: "filesOnly"},
		{opt: list.DirsOnly(), name: "dirsOnly"},
		{opt: list.StartAfter("a"), name: "startAfter"},
		{opt: list.After(s.fileEntry("a")), name: "startAfter"},
		{opt: list.Limit(1), name: "limit"},
		{opt: list.Glob("*"), name: "glob"},
		{opt: list.Match(regexp.MustCompile("a")), name: "match"},
		{opt: list.Filter(func(vfs.Entry) bool { return true }), name: "filter"},
		{opt: list.FilterContext(func(context.Context, vfs.Entry) (bool, error) { return true, nil }), name: "filter"},
		{opt: list.DirFilter(func(vfs.Entry) bool { return true }), name: "dirFilter"},
		{opt: list.DirFilterContext(func(context.Context, vfs.Entry) (bool, error) { return true, nil }), name: "dirFilter"},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.Equal(tt.name, tt.opt.ListOptionName())
		})
	}
}
