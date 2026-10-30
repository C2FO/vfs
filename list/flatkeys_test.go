package list_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/options"
)

type FlatKeysTestSuite struct {
	suite.Suite
}

func TestFlatKeysTestSuite(t *testing.T) {
	suite.Run(t, new(FlatKeysTestSuite))
}

func (s *FlatKeysTestSuite) loc() *mocks.Location {
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().Path().Return("/root/").Maybe()
	return loc
}

func (s *FlatKeysTestSuite) query(opts ...options.ListOption) list.Query {
	s.T().Helper()
	q, err := list.ParseQuery(opts)
	s.Require().NoError(err)
	return q
}

func (s *FlatKeysTestSuite) flat(b *fakeBucket, opts ...options.ListOption) []result {
	s.T().Helper()
	return drain(list.FlatKeys(s.T().Context(), s.loc(), s.query(opts...), b.pages, nil))
}

func (s *FlatKeysTestSuite) TestSynthesizedDirectories() {
	tests := []struct {
		name     string
		keys     []string
		opts     []options.ListOption
		expected []string
	}{
		{
			name:     "implied directories",
			keys:     []string{"a/b/c.txt"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a/", "a/b/", "a/b/c.txt"},
		},
		{
			name:     "folder marker folds into its directory",
			keys:     []string{"a/", "a/x.txt"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a/", "a/x.txt"},
		},
		{
			name:     "folder marker alone is an empty directory",
			keys:     []string{"empty/", "f.txt"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"empty/", "f.txt"},
		},
		{
			name:     "directory shared by several files is yielded once",
			keys:     []string{"a/1", "a/2", "a/b/3", "a/b/4"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a/", "a/1", "a/2", "a/b/", "a/b/3", "a/b/4"},
		},
		{
			name:     "hyphen sorts before the directory slash",
			keys:     []string{"a-b.txt", "a/x.txt", "b.txt"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a-b.txt", "a/", "a/x.txt", "b.txt"},
		},
		{
			name:     "character after the slash sorts after the directory contents",
			keys:     []string{"a.b", "a/x", "a0"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a.b", "a/", "a/x", "a0"},
		},
		{
			name:     "the location's own marker is ignored",
			keys:     []string{"", "a.txt"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a.txt"},
		},
		{
			name:     "file and directory of the same name",
			keys:     []string{"a", "a/x"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a", "a/", "a/x"},
		},
		{
			name:     "non-recursive rolls keys up into directories",
			keys:     []string{"a.txt", "sub/b.txt", "sub/deep/c.txt", "z.txt"},
			expected: []string{"a.txt", "sub/", "z.txt"},
		},
	}
	for i := range tests {
		tt := &tests[i]
		for _, pageSize := range []int{0, 1, 2, 3} {
			s.Run(tt.name, func() {
				rs := s.flat(&fakeBucket{keys: sortedKeys(tt.keys...), pageSize: pageSize}, tt.opts...)
				s.Empty(errsOf(rs))
				s.Equal(tt.expected, keysOf(rs))
			})
		}
	}
}

func (s *FlatKeysTestSuite) TestPageBoundariesDoNotChangeTheResult() {
	keys := sortedKeys(
		"a-b.txt", "a/", "a/b/c.txt", "a/b/d.txt", "a/e.txt", "a0", "b/", "b/x/y/z", "c.txt", "d/e", "d/f/g", "d/f/h", "z",
	)
	cases := []struct {
		name string
		opts []options.ListOption
	}{
		{name: "recursive", opts: []options.ListOption{list.Recursive()}},
		{name: "non-recursive"},
		{name: "recursive with depth", opts: []options.ListOption{list.Recursive(), list.MaxDepth(2)}},
		{name: "nested prefix", opts: []options.ListOption{list.Prefix("d/")}},
		{name: "recursive prefix", opts: []options.ListOption{list.Recursive(), list.Prefix("a/b")}},
	}
	for i := range cases {
		tc := &cases[i]
		s.Run(tc.name, func() {
			expected := keysOf(s.flat(&fakeBucket{keys: keys}, tc.opts...))
			s.Require().NotEmpty(expected)

			for pageSize := 1; pageSize <= len(keys)+1; pageSize++ {
				rs := s.flat(&fakeBucket{keys: keys, pageSize: pageSize}, tc.opts...)
				s.Require().Empty(errsOf(rs), "page size %d", pageSize)
				s.Require().Equal(expected, keysOf(rs), "page size %d", pageSize)
			}
		})
	}
}

func (s *FlatKeysTestSuite) TestPagingConcatenates() {
	keys := sortedKeys("a-b.txt", "a/b/c.txt", "a/b/d.txt", "a/e.txt", "b.txt", "c/", "c0", "d/x/y/z")
	for _, recursive := range []bool{true, false} {
		var base []options.ListOption
		if recursive {
			base = append(base, list.Recursive())
		}
		all := keysOf(s.flat(&fakeBucket{keys: keys, pageSize: 2}, base...))
		s.Require().NotEmpty(all)

		for i, cursor := range all {
			s.Run(cursor, func() {
				opts := append([]options.ListOption{list.StartAfter(cursor)}, base...)
				rs := s.flat(&fakeBucket{keys: keys, pageSize: 2}, opts...)
				s.Empty(errsOf(rs))
				s.Equal(append([]string(nil), all[i+1:]...), keysOf(rs))
			})
		}
	}
}

func (s *FlatKeysTestSuite) TestCursorInsideASubtree() {
	b := &fakeBucket{keys: sortedKeys("a/b/c.txt", "a/b/d.txt", "b.txt")}
	rs := s.flat(b, list.Recursive(), list.StartAfter("a/b/c.txt"))
	s.Empty(errsOf(rs))
	s.Equal([]string{"a/b/d.txt", "b.txt"}, keysOf(rs))
}

func (s *FlatKeysTestSuite) TestRequestsCarryTheHints() {
	b := &fakeBucket{keys: sortedKeys("to/report.csv")}
	s.flat(b, list.Prefix("to/rep"), list.StartAfter("to/a"))
	s.Require().Len(b.requests, 1)
	s.Equal(list.PageRequest{StartAfter: "to/a", Prefix: "to/rep", Delimited: true}, b.requests[0])

	b = &fakeBucket{keys: sortedKeys("to/report.csv")}
	s.flat(b, list.Recursive())
	s.Require().Len(b.requests, 1)
	s.Equal(list.PageRequest{}, b.requests[0])
}

func (s *FlatKeysTestSuite) TestResumesPastARolledUpDirectory() {
	b := &fakeBucket{keys: sortedKeys("a/1", "a/2", "a/3", "b/1", "c"), pageSize: 1}
	rs := s.flat(b)
	s.Empty(errsOf(rs))
	s.Equal([]string{"a/", "b/", "c"}, keysOf(rs))

	// The request after the "a/" page must start past everything inside it. Starting after "a/" would roll "a/" up
	// again, on every page.
	s.Require().GreaterOrEqual(len(b.requests), 2)
	s.Equal("a/\U0010FFFF", b.requests[1].StartAfter)
	s.Len(b.requests, 3)
}

func (s *FlatKeysTestSuite) TestWindow() {
	keys := sortedKeys("a/b/c.txt", "a/b/d/e.txt", "to/report.csv", "to/reports/z.csv", "to/x/y.csv", "top.txt")
	tests := []struct {
		name     string
		opts     []options.ListOption
		expected []string
	}{
		{
			name:     "max depth stops below the limit",
			opts:     []options.ListOption{list.Recursive(), list.MaxDepth(2)},
			expected: []string{"a/", "a/b/", "to/", "to/report.csv", "to/reports/", "to/x/", "top.txt"},
		},
		{
			name:     "nested prefix without recursion",
			opts:     []options.ListOption{list.Prefix("to/rep")},
			expected: []string{"to/report.csv", "to/reports/"},
		},
		{
			name:     "nested prefix with recursion",
			opts:     []options.ListOption{list.Recursive(), list.Prefix("to/rep")},
			expected: []string{"to/report.csv", "to/reports/", "to/reports/z.csv"},
		},
		{
			name:     "flat prefix",
			opts:     []options.ListOption{list.Prefix("to")},
			expected: []string{"to/", "top.txt"},
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			rs := s.flat(&fakeBucket{keys: keys, pageSize: 2}, tt.opts...)
			s.Empty(errsOf(rs))
			s.Equal(tt.expected, keysOf(rs))
		})
	}
}

func (s *FlatKeysTestSuite) TestWindowIsEnforcedWhenTheBackendIgnoresHints() {
	keys := sortedKeys("a/b/c.txt", "a/z.txt", "b/x.txt", "to/report.csv", "top.txt")
	ignoresHints := func(b *fakeBucket) list.PageFunc {
		return func(ctx context.Context, req list.PageRequest) (list.Page, error) {
			return b.pages(ctx, list.PageRequest{StartAfter: req.StartAfter})
		}
	}
	tests := []struct {
		name     string
		opts     []options.ListOption
		expected []string
	}{
		{name: "prefix", opts: []options.ListOption{list.Recursive(), list.Prefix("to")}, expected: []string{"to/", "to/report.csv", "top.txt"}},
		{
			name:     "cursor",
			opts:     []options.ListOption{list.Recursive(), list.StartAfter("a/z.txt")},
			expected: []string{"b/", "b/x.txt", "to/", "to/report.csv", "top.txt"},
		},
		{name: "depth", opts: []options.ListOption{list.Recursive(), list.MaxDepth(1)}, expected: []string{"a/", "b/", "to/", "top.txt"}},
		{name: "no recursion", expected: []string{"a/", "b/", "to/", "top.txt"}},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			b := &fakeBucket{keys: keys, pageSize: 2}
			rs := drain(list.FlatKeys(s.T().Context(), s.loc(), s.query(tt.opts...), ignoresHints(b), nil))
			s.Empty(errsOf(rs))
			s.Equal(tt.expected, keysOf(rs))
		})
	}
}

func (s *FlatKeysTestSuite) TestDirFilterPrunes() {
	b := &fakeBucket{keys: sortedKeys("keep/a.txt", "skip/b.txt", "skip/deeper/c.txt", "zed.txt"), pageSize: 2}
	var asked []string
	rs := s.flat(b, list.Recursive(), list.DirFilter(func(e vfs.Entry) bool {
		asked = append(asked, e.RelPath())
		return e.Name() != "skip"
	}))

	s.Empty(errsOf(rs))
	s.Equal([]string{"keep/", "keep/a.txt", "skip/", "zed.txt"}, keysOf(rs))
	s.Equal([]string{"keep", "skip"}, asked)
}

func (s *FlatKeysTestSuite) TestDirFilterIsIgnoredWithoutRecursion() {
	b := &fakeBucket{keys: sortedKeys("a/b/c.txt", "a/d.txt")}
	asked := 0
	rs := s.flat(b, list.Prefix("a/"), list.DirFilter(func(vfs.Entry) bool {
		asked++
		return false
	}))

	s.Empty(errsOf(rs))
	s.Equal([]string{"a/b/", "a/d.txt"}, keysOf(rs), "the directory on the way to the prefix is read, not pruned")
	s.Zero(asked)
}

func (s *FlatKeysTestSuite) TestDirFilterErrorIsFatal() {
	boom := errors.New("boom")
	b := &fakeBucket{keys: sortedKeys("a/x.txt", "b/y.txt")}
	rs := s.flat(b, list.Recursive(), list.DirFilterContext(func(context.Context, vfs.Entry) (bool, error) {
		return true, boom
	}))

	s.Require().Len(rs, 2)
	s.Equal("a/", list.OrderKey(rs[0].entry))
	s.Require().ErrorIs(rs[1].err, boom)
	s.Equal(vfs.Entry{}.RelPath(), rs[1].entry.RelPath())
}

func (s *FlatKeysTestSuite) TestObjectMetadataIsPassedThrough() {
	b := &fakeBucket{keys: sortedKeys("a/f.txt")}
	var seen vfs.Entry
	pages := func(ctx context.Context, req list.PageRequest) (list.Page, error) {
		page, err := b.pages(ctx, req)
		for i := range page.Objects {
			page.Objects[i].Options = []vfs.EntryOption{vfs.WithEntryMetadata(42, s.modTime()), vfs.WithEntrySys("native")}
		}
		return page, err
	}
	for e, err := range list.FlatKeys(s.T().Context(), s.loc(), s.query(list.Recursive()), pages, nil) {
		s.Require().NoError(err)
		if !e.IsDir() {
			seen = e
		} else {
			s.False(e.HasMetadata(), "synthesized directories never carry metadata")
		}
	}
	s.True(seen.HasMetadata())
	s.Equal(uint64(42), seen.Size())
	s.Equal("native", seen.Sys())
}

func (s *FlatKeysTestSuite) TestInvalidKeys() {
	b := &fakeBucket{keys: sortedKeys("a.txt", "reports//2026.csv", "x/./y", "../z", "/lead", "ok/f.txt", "z.txt")}
	rs := s.flat(b, list.Recursive())

	s.Equal([]string{"a.txt", "ok/", "ok/f.txt", "z.txt"}, keysOf(rs))
	errs := errsOf(rs)
	paths := make([]string, 0, len(errs))
	for _, err := range errs {
		s.Require().ErrorIs(err, fs.ErrInvalid)
		var pe *fs.PathError
		s.Require().ErrorAs(err, &pe)
		s.Equal("list", pe.Op)
		paths = append(paths, pe.Path)
	}
	s.ElementsMatch([]string{"/lead", "../z", "reports//2026.csv", "x/./y"}, paths)
	for _, r := range rs {
		if r.err != nil {
			s.Equal(vfs.Entry{}.RelPath(), r.entry.RelPath())
		}
	}
}

func (s *FlatKeysTestSuite) TestInvalidKeyIsReportedInPlace() {
	b := &fakeBucket{keys: sortedKeys("a.txt", "m//n", "z.txt")}
	rs := s.flat(b, list.Recursive())

	s.Require().Len(rs, 3)
	s.Equal("a.txt", rs[0].entry.RelPath())
	s.Require().ErrorIs(rs[1].err, fs.ErrInvalid)
	s.Equal("z.txt", rs[2].entry.RelPath())
}

func (s *FlatKeysTestSuite) TestInvalidKeysRespectTheWindow() {
	keys := sortedKeys("a.txt", "m//n", "z.txt")
	s.Run("outside the prefix", func() {
		s.Empty(errsOf(s.flat(&fakeBucket{keys: keys}, list.Recursive(), list.Prefix("z"))))
	})
	s.Run("before the cursor", func() {
		s.Empty(errsOf(s.flat(&fakeBucket{keys: keys}, list.Recursive(), list.StartAfter("n"))))
	})
}

func (s *FlatKeysTestSuite) TestOutOfOrderKeysAreFatal() {
	b := &fakeBucket{keys: sortedKeys("a", "b", "c"), pageSize: 1, mutate: func(call int, p *list.Page) {
		if call == 0 {
			return
		}
		p.Objects = []list.Object{{Key: "0-before-everything"}}
		p.Done = true
	}}
	rs := s.flat(b, list.Recursive())

	errs := errsOf(rs)
	s.Require().Len(errs, 1)
	s.Contains(errs[0].Error(), "byte order")
	s.Equal([]string{"a"}, keysOf(rs))
	s.Len(b.requests, 2, "no request is made after the order violation")
}

func (s *FlatKeysTestSuite) TestDuplicateKeysAreIgnored() {
	b := &fakeBucket{keys: sortedKeys("a/", "a/x", "b"), mutate: func(_ int, p *list.Page) {
		p.Prefixes = append(p.Prefixes, "a/")
		p.Objects = append(p.Objects, list.Object{Key: "b"})
	}}
	rs := s.flat(b, list.Recursive())
	s.Empty(errsOf(rs))
	s.Equal([]string{"a/", "a/x", "b"}, keysOf(rs))
}

func (s *FlatKeysTestSuite) TestPageFailure() {
	tests := []struct {
		name         string
		failOnCall   int
		err          error
		expectedKeys []string
		expectedErrs int
	}{
		{name: "first page fails", failOnCall: 0, err: fs.ErrPermission, expectedErrs: 1},
		{name: "later page fails", failOnCall: 1, err: errors.New("connection reset"), expectedKeys: []string{"a"}, expectedErrs: 1},
		{name: "missing location yields nothing", failOnCall: 0, err: fs.ErrNotExist},
		{name: "later not-found is an error", failOnCall: 1, err: fs.ErrNotExist, expectedKeys: []string{"a"}, expectedErrs: 1},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			b := &fakeBucket{keys: sortedKeys("a", "b", "c"), pageSize: 1, pageErr: func(call int) error {
				if call == tt.failOnCall {
					return tt.err
				}
				return nil
			}}
			rs := s.flat(b, list.Recursive())
			s.Equal(tt.expectedKeys, keysOf(rs))
			errs := errsOf(rs)
			s.Require().Len(errs, tt.expectedErrs)
			if tt.expectedErrs == 0 {
				return
			}
			s.Require().ErrorIs(errs[0], tt.err)
			var pe *fs.PathError
			s.Require().ErrorAs(errs[0], &pe)
			s.Equal("list", pe.Op)
			s.Equal("/root/", pe.Path)
		})
	}
}

func (s *FlatKeysTestSuite) TestPagesThatMakeNoProgressEndTheListing() {
	b := &fakeBucket{keys: sortedKeys("a"), mutate: func(_ int, p *list.Page) {
		p.Objects = nil
		p.Prefixes = nil
		p.Done = false
	}}
	rs := s.flat(b, list.Recursive())

	errs := errsOf(rs)
	s.Require().Len(errs, 1)
	s.Contains(errs[0].Error(), "no progress")
	s.Greater(len(b.requests), 100)
}

func (s *FlatKeysTestSuite) TestEmptyPagesWithMoreToComeAreTolerated() {
	b := &fakeBucket{keys: sortedKeys("a", "b"), mutate: func(call int, p *list.Page) {
		if call < 3 {
			p.Objects = nil
			p.Prefixes = nil
			p.Done = false
		}
	}}
	rs := s.flat(b, list.Recursive())
	s.Empty(errsOf(rs))
	s.Equal([]string{"a", "b"}, keysOf(rs))
}

func (s *FlatKeysTestSuite) TestRetry() {
	s.Run("transient failure is retried", func() {
		b := &fakeBucket{keys: sortedKeys("a.txt")}
		attempts := 0
		pages := func(ctx context.Context, req list.PageRequest) (list.Page, error) {
			attempts++
			if attempts == 1 {
				return list.Page{}, errors.New("transient")
			}
			return b.pages(ctx, req)
		}
		retry := func(wrapped func() error) error {
			var err error
			for range 3 {
				if err = wrapped(); err == nil {
					return nil
				}
			}
			return err
		}

		rs := drain(list.FlatKeys(s.T().Context(), s.loc(), s.query(), pages, retry))
		s.Empty(errsOf(rs))
		s.Equal([]string{"a.txt"}, keysOf(rs))
		s.Equal(2, attempts)
	})

	s.Run("no further attempt once the context is done", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		defer cancel()
		attempts := 0
		pages := func(context.Context, list.PageRequest) (list.Page, error) {
			attempts++
			cancel()
			return list.Page{}, errors.New("transient")
		}
		retry := func(wrapped func() error) error {
			var err error
			for range 5 {
				err = wrapped()
			}
			return err
		}

		rs := drain(list.FlatKeys(ctx, s.loc(), s.query(), pages, retry))
		s.Equal(1, attempts)
		errs := errsOf(rs)
		s.Require().Len(errs, 1)
		s.Require().ErrorIs(errs[0], context.Canceled)
	})
}

func (s *FlatKeysTestSuite) TestCancellation() {
	s.Run("canceled before the listing starts makes no request", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		cancel()
		b := &fakeBucket{keys: sortedKeys("a")}

		rs := drain(list.FlatKeys(ctx, s.loc(), s.query(), b.pages, nil))
		s.Empty(b.requests)
		s.Require().Len(rs, 1)
		s.Require().ErrorIs(rs[0].err, context.Canceled)
	})

	s.Run("canceled after the first page makes no further request", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		defer cancel()
		b := &fakeBucket{keys: sortedKeys("a", "b", "c"), pageSize: 1}

		var got []result
		for e, err := range list.FlatKeys(ctx, s.loc(), s.query(list.Recursive()), b.pages, nil) {
			got = append(got, result{entry: e, err: err})
			if len(got) == 1 {
				cancel()
			}
		}

		s.Require().Len(got, 2)
		s.Require().NoError(got[0].err)
		s.Require().ErrorIs(got[1].err, context.Canceled)
		s.Len(b.requests, 1)
	})
}

func (s *FlatKeysTestSuite) TestLazyAndEarlyTermination() {
	s.Run("building the iterator makes no request", func() {
		b := &fakeBucket{keys: sortedKeys("a")}
		_ = list.FlatKeys(s.T().Context(), s.loc(), s.query(), b.pages, nil)
		s.Empty(b.requests)
	})

	s.Run("break makes no further request", func() {
		b := &fakeBucket{keys: sortedKeys("a", "b", "c", "d"), pageSize: 1}
		for _, err := range list.FlatKeys(s.T().Context(), s.loc(), s.query(list.Recursive()), b.pages, nil) {
			s.Require().NoError(err)
			break
		}
		s.Len(b.requests, 1)
	})

	s.Run("no request is in flight while the loop body runs", func() {
		b := &fakeBucket{keys: sortedKeys("a", "b", "c"), pageSize: 1}
		inFlight := 0
		pages := func(ctx context.Context, req list.PageRequest) (list.Page, error) {
			inFlight++
			defer func() { inFlight-- }()
			return b.pages(ctx, req)
		}
		for _, err := range list.FlatKeys(s.T().Context(), s.loc(), s.query(list.Recursive()), pages, nil) {
			s.Require().NoError(err)
			s.Zero(inFlight)
		}
	})
}
