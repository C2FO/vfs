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

type WalkDirsTestSuite struct {
	suite.Suite
}

func TestWalkDirsTestSuite(t *testing.T) {
	suite.Run(t, new(WalkDirsTestSuite))
}

func (s *WalkDirsTestSuite) loc() *mocks.Location {
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().Path().Return("/root/").Maybe()
	return loc
}

func (s *WalkDirsTestSuite) query(opts ...options.ListOption) list.Query {
	s.T().Helper()
	q, err := list.ParseQuery(opts)
	s.Require().NoError(err)
	return q
}

func (s *WalkDirsTestSuite) walk(fd *fakeDirs, opts ...options.ListOption) []result {
	s.T().Helper()
	return drain(list.WalkDirs(s.T().Context(), s.loc(), s.query(opts...), fd.readDir, nil))
}

func (s *WalkDirsTestSuite) TestOrder() {
	tests := []struct {
		name     string
		paths    []string
		opts     []options.ListOption
		expected []string
	}{
		{
			name:     "recursive order matches full key order",
			paths:    []string{"a/x.txt", "a-b.txt", "b.txt"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a-b.txt", "a/", "a/x.txt", "b.txt"},
		},
		{
			name:     "file sorts before a directory of the same prefix",
			paths:    []string{"logs/x.log", "logs-2024.gz"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"logs-2024.gz", "logs/", "logs/x.log"},
		},
		{
			name:     "character after the slash sorts after the directory contents",
			paths:    []string{"a/x", "a0", "a.b"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a.b", "a/", "a/x", "a0"},
		},
		{
			name:     "non-recursive lists immediate children only",
			paths:    []string{"a.txt", "sub/b.txt"},
			expected: []string{"a.txt", "sub/"},
		},
		{
			name:     "empty directory",
			paths:    []string{"empty/", "f.txt"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"empty/", "f.txt"},
		},
		{
			name:     "file and directory of the same name",
			paths:    []string{"a", "a/x"},
			opts:     []options.ListOption{list.Recursive()},
			expected: []string{"a", "a/", "a/x"},
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			rs := s.walk(&fakeDirs{tree: treeFromPaths(tt.paths...)}, tt.opts...)
			s.Empty(errsOf(rs))
			s.Equal(tt.expected, keysOf(rs))
		})
	}
}

func (s *WalkDirsTestSuite) TestOnlyReadsWhatItMust() {
	tests := []struct {
		name          string
		paths         []string
		opts          []options.ListOption
		expectedReads []string
		expectedKeys  []string
	}{
		{
			name:          "non-recursive reads only the location",
			paths:         []string{"a/b.txt", "c.txt"},
			expectedReads: []string{""},
			expectedKeys:  []string{"a/", "c.txt"},
		},
		{
			name:          "max depth stops descent",
			paths:         []string{"a/b/c.txt"},
			opts:          []options.ListOption{list.Recursive(), list.MaxDepth(2)},
			expectedReads: []string{"", "a"},
			expectedKeys:  []string{"a/", "a/b/"},
		},
		{
			name:          "prefix skips unrelated directories",
			paths:         []string{"keep/to/report.csv", "other/x", "tom/y"},
			opts:          []options.ListOption{list.Recursive(), list.Prefix("keep/to/rep")},
			expectedReads: []string{"", "keep", "keep/to"},
			expectedKeys:  []string{"keep/to/report.csv"},
		},
		{
			name:          "cursor skips directories that sort before it",
			paths:         []string{"a/x", "b/y", "c/z"},
			opts:          []options.ListOption{list.Recursive(), list.StartAfter("b/y")},
			expectedReads: []string{"", "b", "c"},
			expectedKeys:  []string{"c/", "c/z"},
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			fd := &fakeDirs{tree: treeFromPaths(tt.paths...)}
			rs := s.walk(fd, tt.opts...)
			s.Empty(errsOf(rs))
			s.Equal(tt.expectedKeys, keysOf(rs))
			s.Equal(tt.expectedReads, fd.reads)
		})
	}
}

func (s *WalkDirsTestSuite) TestPrefix() {
	paths := []string{"to/report.csv", "to/reports/z.csv", "to/x/y.csv", "top.txt", "other.txt"}
	tests := []struct {
		name     string
		opts     []options.ListOption
		expected []string
	}{
		{
			name:     "nested prefix without recursion lists one level inside the directory",
			opts:     []options.ListOption{list.Prefix("to/rep")},
			expected: []string{"to/report.csv", "to/reports/"},
		},
		{
			name:     "nested prefix with recursion",
			opts:     []options.ListOption{list.Recursive(), list.Prefix("to/rep")},
			expected: []string{"to/report.csv", "to/reports/", "to/reports/z.csv"},
		},
		{
			name:     "directory prefix",
			opts:     []options.ListOption{list.Prefix("to/")},
			expected: []string{"to/report.csv", "to/reports/", "to/x/"},
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
			rs := s.walk(&fakeDirs{tree: treeFromPaths(paths...)}, tt.opts...)
			s.Empty(errsOf(rs))
			s.Equal(tt.expected, keysOf(rs))
		})
	}
}

func (s *WalkDirsTestSuite) TestPagingConcatenates() {
	tree := treeFromPaths("a-b.txt", "a/b/c.txt", "a/b/d.txt", "a/e.txt", "b.txt", "c/", "c0", "d/x/y/z")
	all := keysOf(s.walk(&fakeDirs{tree: tree}, list.Recursive()))
	s.Require().Greater(len(all), 5)

	for i, cursor := range all {
		s.Run(cursor, func() {
			rs := s.walk(&fakeDirs{tree: tree}, list.Recursive(), list.StartAfter(cursor))
			s.Empty(errsOf(rs))
			s.Equal(append([]string(nil), all[i+1:]...), keysOf(rs))
		})
	}
}

func (s *WalkDirsTestSuite) TestCursorInsideASubtree() {
	rs := s.walk(&fakeDirs{tree: treeFromPaths("a/b/c.txt", "a/b/d.txt", "b.txt")}, list.Recursive(), list.StartAfter("a/b/c.txt"))
	s.Empty(errsOf(rs))
	s.Equal([]string{"a/b/d.txt", "b.txt"}, keysOf(rs))
}

func (s *WalkDirsTestSuite) TestSymlinksAreNotFollowed() {
	tree := treeFromPaths("real/x.txt")
	tree[""] = append(tree[""], list.Child{
		Name:    "link",
		Options: []vfs.EntryOption{vfs.WithEntryMode(fs.ModeSymlink | 0o777)},
	})
	fd := &fakeDirs{tree: tree}

	rs := s.walk(fd, list.Recursive())
	s.Empty(errsOf(rs))
	s.Equal([]string{"link", "real/", "real/x.txt"}, keysOf(rs))
	s.Equal(fs.ModeSymlink, rs[0].entry.Type()&fs.ModeSymlink)
	s.NotContains(fd.reads, "link")
}

func (s *WalkDirsTestSuite) TestDirFilterPrunes() {
	fd := &fakeDirs{tree: treeFromPaths("keep/a.txt", "skip/b.txt", "skip/deeper/c.txt")}
	var asked []string
	rs := s.walk(fd, list.Recursive(), list.DirFilter(func(e vfs.Entry) bool {
		asked = append(asked, e.RelPath())
		return e.Name() != "skip"
	}))

	s.Empty(errsOf(rs))
	s.Equal([]string{"keep/", "keep/a.txt", "skip/"}, keysOf(rs))
	s.Equal([]string{"keep", "skip"}, asked)
	s.NotContains(fd.reads, "skip")
}

func (s *WalkDirsTestSuite) TestDirFilterOnlyAskedAboutDirectoriesItWouldRead() {
	fd := &fakeDirs{tree: treeFromPaths("a/b/c.txt", "z/y.txt")}
	var asked []string
	s.walk(fd, list.Recursive(), list.MaxDepth(2), list.DirFilter(func(e vfs.Entry) bool {
		asked = append(asked, e.RelPath())
		return true
	}))
	// a/b is at the depth limit and is never read, so it is never offered for pruning.
	s.Equal([]string{"a", "z"}, asked)
}

func (s *WalkDirsTestSuite) TestDirFilterIsIgnoredWithoutRecursion() {
	fd := &fakeDirs{tree: treeFromPaths("a/b/c.txt", "a/d.txt")}
	asked := 0
	rs := s.walk(fd, list.Prefix("a/"), list.DirFilter(func(vfs.Entry) bool {
		asked++
		return false
	}))

	s.Empty(errsOf(rs))
	s.Equal([]string{"a/b/", "a/d.txt"}, keysOf(rs), "the directory on the way to the prefix is read, not pruned")
	s.Zero(asked)
}

func (s *WalkDirsTestSuite) TestDirFilterErrorIsFatal() {
	boom := errors.New("boom")
	fd := &fakeDirs{tree: treeFromPaths("a/x.txt", "b/y.txt")}
	rs := s.walk(fd, list.Recursive(), list.DirFilterContext(func(context.Context, vfs.Entry) (bool, error) {
		return true, boom
	}))

	s.Require().Len(rs, 2)
	s.Require().NoError(rs[0].err)
	s.Equal("a/", list.OrderKey(rs[0].entry))
	s.Require().ErrorIs(rs[1].err, boom)
	s.Equal(vfs.Entry{}.RelPath(), rs[1].entry.RelPath())
	var pe *fs.PathError
	s.Require().ErrorAs(rs[1].err, &pe)
	s.Equal("list", pe.Op)
	s.Equal([]string{""}, fd.reads)
}

func (s *WalkDirsTestSuite) TestRootFailure() {
	tests := []struct {
		name         string
		err          error
		expectedErrs int
	}{
		{name: "permission denied is fatal", err: fs.ErrPermission, expectedErrs: 1},
		{name: "other failure is fatal", err: errors.New("connection reset"), expectedErrs: 1},
		{name: "missing location yields nothing", err: fs.ErrNotExist, expectedErrs: 0},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			fd := &fakeDirs{tree: treeFromPaths("a.txt"), errs: map[string]error{"": tt.err}}
			rs := s.walk(fd, list.Recursive())
			s.Empty(keysOf(rs))
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
			s.Equal(vfs.Entry{}.RelPath(), rs[0].entry.RelPath())
		})
	}
}

func (s *WalkDirsTestSuite) TestPartialFailureCarriesTheDirectory() {
	fd := &fakeDirs{
		tree: treeFromPaths("a/1.txt", "b/2.txt", "c/3.txt", "d.txt"),
		errs: map[string]error{"b": fs.ErrPermission},
	}
	rs := s.walk(fd, list.Recursive())

	got := make([]string, 0, len(rs))
	for _, r := range rs {
		label := list.OrderKey(r.entry)
		if r.err != nil {
			label += " !"
		}
		got = append(got, label)
	}
	s.Equal([]string{"a/", "a/1.txt", "b/", "b/ !", "c/", "c/3.txt", "d.txt"}, got)

	failed := rs[3]
	s.Require().ErrorIs(failed.err, fs.ErrPermission)
	var pe *fs.PathError
	s.Require().ErrorAs(failed.err, &pe)
	s.Equal("list", pe.Op)
	s.Equal("/root/b/", pe.Path)
	s.True(failed.entry.IsDir())
	s.Equal("b", failed.entry.RelPath())
}

func (s *WalkDirsTestSuite) TestVanishedSubdirectoryIsReported() {
	fd := &fakeDirs{
		tree: treeFromPaths("a/1.txt", "b/2.txt"),
		errs: map[string]error{"a": fs.ErrNotExist},
	}
	rs := s.walk(fd, list.Recursive())
	errs := errsOf(rs)
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], fs.ErrNotExist)
	s.Equal([]string{"a/", "b/", "b/2.txt"}, keysOf(rs))
}

func (s *WalkDirsTestSuite) TestInvalidNames() {
	tree := treeFromPaths("ok.txt", "z.txt")
	tree[""] = append(tree[""],
		list.Child{Name: "has/slash"},
		list.Child{Name: ".."},
		list.Child{Name: ""},
		list.Child{Name: "weird/dir", IsDir: true},
	)
	rs := s.walk(&fakeDirs{tree: tree})

	s.Equal([]string{"ok.txt", "z.txt"}, keysOf(rs))
	errs := errsOf(rs)
	s.Require().Len(errs, 4)
	for _, err := range errs {
		s.Require().ErrorIs(err, fs.ErrInvalid)
	}
	for _, r := range rs[:4] {
		s.Require().Error(r.err, "invalid names are reported before the directory's entries")
		s.Equal(vfs.Entry{}.RelPath(), r.entry.RelPath())
	}
	var pe *fs.PathError
	s.Require().ErrorAs(errs[0], &pe)
	s.Equal("list", pe.Op)
}

func (s *WalkDirsTestSuite) TestInvalidNamesRespectTheWindow() {
	tree := treeFromPaths("ok.txt")
	tree[""] = append(tree[""], list.Child{Name: "bad/name"})

	s.Run("outside the prefix", func() {
		rs := s.walk(&fakeDirs{tree: tree}, list.Prefix("ok"))
		s.Empty(errsOf(rs))
	})
	s.Run("before the cursor", func() {
		rs := s.walk(&fakeDirs{tree: tree}, list.StartAfter("zzz"))
		s.Empty(errsOf(rs))
	})
	s.Run("inside the window", func() {
		rs := s.walk(&fakeDirs{tree: tree})
		s.Require().Len(errsOf(rs), 1)
	})
}

func (s *WalkDirsTestSuite) TestRetry() {
	s.Run("transient failure is retried", func() {
		attempts := 0
		fd := &fakeDirs{tree: treeFromPaths("a.txt")}
		readDir := func(ctx context.Context, relDir string) ([]list.Child, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("transient")
			}
			return fd.readDir(ctx, relDir)
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

		rs := drain(list.WalkDirs(s.T().Context(), s.loc(), s.query(), readDir, retry))
		s.Empty(errsOf(rs))
		s.Equal([]string{"a.txt"}, keysOf(rs))
		s.Equal(2, attempts)
	})

	s.Run("no further attempt once the context is done", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		defer cancel()
		attempts := 0
		readDir := func(context.Context, string) ([]list.Child, error) {
			attempts++
			cancel()
			return nil, errors.New("transient")
		}
		retry := func(wrapped func() error) error {
			var err error
			for range 5 {
				err = wrapped()
			}
			return err
		}

		rs := drain(list.WalkDirs(ctx, s.loc(), s.query(), readDir, retry))
		s.Equal(1, attempts)
		errs := errsOf(rs)
		s.Require().Len(errs, 1)
		s.Require().ErrorIs(errs[0], context.Canceled)
	})
}

func (s *WalkDirsTestSuite) TestCancellation() {
	s.Run("canceled before the listing starts makes no request", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		cancel()
		fd := &fakeDirs{tree: treeFromPaths("a.txt")}

		rs := drain(list.WalkDirs(ctx, s.loc(), s.query(), fd.readDir, nil))
		s.Empty(fd.reads)
		s.Require().Len(rs, 1)
		s.Require().ErrorIs(rs[0].err, context.Canceled)
	})

	s.Run("canceled mid-listing ends with one error and no further read", func() {
		ctx, cancel := context.WithCancel(s.T().Context())
		defer cancel()
		fd := &fakeDirs{tree: treeFromPaths("a/1.txt", "b/2.txt", "c/3.txt")}

		var got []result
		for e, err := range list.WalkDirs(ctx, s.loc(), s.query(list.Recursive()), fd.readDir, nil) {
			got = append(got, result{entry: e, err: err})
			if len(got) == 1 {
				cancel()
			}
		}

		s.Require().Len(got, 2)
		s.Require().NoError(got[0].err)
		s.Require().ErrorIs(got[1].err, context.Canceled)
		s.Equal([]string{""}, fd.reads)
	})
}

func (s *WalkDirsTestSuite) TestLazyAndEarlyTermination() {
	s.Run("building the iterator makes no request", func() {
		fd := &fakeDirs{tree: treeFromPaths("a.txt")}
		_ = list.WalkDirs(s.T().Context(), s.loc(), s.query(), fd.readDir, nil)
		s.Empty(fd.reads)
	})

	s.Run("break makes no further request", func() {
		fd := &fakeDirs{tree: treeFromPaths("a/1.txt", "b/2.txt")}
		for _, err := range list.WalkDirs(s.T().Context(), s.loc(), s.query(list.Recursive()), fd.readDir, nil) {
			s.Require().NoError(err)
			break
		}
		// "a/" is yielded before it is read, so stopping there reads only the location.
		s.Equal([]string{""}, fd.reads)
	})

	s.Run("no read is in flight while the loop body runs", func() {
		fd := &fakeDirs{tree: treeFromPaths("a/1.txt", "b/2.txt")}
		inFlight := 0
		fd.onRead = func(string) { inFlight++ }
		readDir := func(ctx context.Context, relDir string) ([]list.Child, error) {
			defer func() { inFlight-- }()
			return fd.readDir(ctx, relDir)
		}

		for _, err := range list.WalkDirs(s.T().Context(), s.loc(), s.query(list.Recursive()), readDir, nil) {
			s.Require().NoError(err)
			s.Zero(inFlight)
		}
	})
}
