package list_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/options"
)

// FallbackTestSuite lists locations that have no native listing, so only List, ListByPrefix and NewLocation exist.
// The mocks fail the test on any call that is not expected, which is how "no per-file request" is checked.
type FallbackTestSuite struct {
	suite.Suite
	loc *mocks.Location
}

func TestFallbackTestSuite(t *testing.T) {
	suite.Run(t, new(FallbackTestSuite))
}

func (s *FallbackTestSuite) SetupTest() {
	s.loc = mocks.NewLocation(s.T())
	s.loc.EXPECT().Path().Return("/root/").Maybe()
}

func (s *FallbackTestSuite) list(opts ...options.ListOption) []result {
	s.T().Helper()
	return drain(list.Entries(s.T().Context(), s.loc, opts...))
}

func (s *FallbackTestSuite) TestPlainListing() {
	s.loc.EXPECT().List().Return([]string{"b.txt", "a.txt", "a-b.txt", "A.txt"}, nil)

	rs := s.list()
	s.Empty(errsOf(rs))
	s.Equal([]string{"A.txt", "a-b.txt", "a.txt", "b.txt"}, keysOf(rs))
	for _, r := range rs {
		s.False(r.entry.IsDir())
		s.False(r.entry.HasMetadata(), "names carry no size or modification time")
	}
}

func (s *FallbackTestSuite) TestEmptyLocation() {
	s.loc.EXPECT().List().Return(nil, nil)
	s.Empty(s.list())
}

func (s *FallbackTestSuite) TestPrefix() {
	s.loc.EXPECT().ListByPrefix("rep").Return([]string{"report.csv", "reports.csv"}, nil)

	s.Equal([]string{"report.csv", "reports.csv"}, keysOf(s.list(list.Prefix("rep"))))
}

func (s *FallbackTestSuite) TestNestedPrefixListsTheSubLocation() {
	tests := []struct {
		name     string
		prefix   string
		expect   func(sub *mocks.Location)
		expected []string
	}{
		{
			name:   "prefix inside a sub-location",
			prefix: "to/rep",
			expect: func(sub *mocks.Location) {
				sub.EXPECT().ListByPrefix("rep").Return([]string{"report.csv"}, nil)
			},
			expected: []string{"to/report.csv"},
		},
		{
			name:   "prefix naming a sub-location",
			prefix: "to/",
			expect: func(sub *mocks.Location) {
				sub.EXPECT().List().Return([]string{"z.csv", "report.csv"}, nil)
			},
			expected: []string{"to/report.csv", "to/z.csv"},
		},
		{
			name:   "deeper prefix",
			prefix: "a/b/c",
			expect: func(sub *mocks.Location) {
				sub.EXPECT().ListByPrefix("c").Return([]string{"c1"}, nil)
			},
			expected: []string{"a/b/c1"},
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.SetupTest()
			sub := mocks.NewLocation(s.T())
			tt.expect(sub)
			dir := tt.prefix[:strings.LastIndex(tt.prefix, "/")+1]
			s.loc.EXPECT().NewLocation(dir).Return(sub, nil)

			rs := s.list(list.Prefix(tt.prefix))
			s.Empty(errsOf(rs))
			s.Equal(tt.expected, keysOf(rs))
		})
	}
}

func (s *FallbackTestSuite) TestOptionsAreAppliedToTheNames() {
	s.loc.EXPECT().List().Return([]string{"a.csv", "b.txt", "c.csv", "d.csv"}, nil)

	rs := s.list(list.Glob("*.csv"), list.StartAfter("a.csv"), list.Limit(2))
	s.Empty(errsOf(rs))
	s.Equal([]string{"c.csv", "d.csv"}, keysOf(rs))
}

func (s *FallbackTestSuite) TestPagesConcatenate() {
	s.loc.EXPECT().List().Return([]string{"e", "d", "c", "b", "a"}, nil)

	var got []string
	cursor := list.StartAfter("")
	for {
		rs := s.list(list.Limit(2), cursor)
		s.Require().Empty(errsOf(rs))
		if len(rs) == 0 {
			break
		}
		got = append(got, keysOf(rs)...)
		cursor = list.After(rs[len(rs)-1].entry)
	}
	s.Equal([]string{"a", "b", "c", "d", "e"}, got)
}

func (s *FallbackTestSuite) TestOptionsThatNeedDirectoriesAreUnsupported() {
	tests := []struct {
		name string
		opt  options.ListOption
	}{
		{name: "recursive", opt: list.Recursive()},
		{name: "max depth", opt: list.MaxDepth(2)},
		{name: "directories only", opt: list.DirsOnly()},
		{name: "glob with a slash", opt: list.Glob("*/data.csv")},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.SetupTest() // no List expectation: any backend request fails the test
			opts := []options.ListOption{tt.opt}
			if tt.name == "max depth" {
				opts = append(opts, list.Recursive())
			}

			rs := s.list(opts...)
			s.Require().Len(rs, 1)
			s.Require().ErrorIs(rs[0].err, errors.ErrUnsupported)
			var pe *fs.PathError
			s.Require().ErrorAs(rs[0].err, &pe)
			s.Equal("list", pe.Op)
			s.Equal("/root/", pe.Path)
		})
	}
}

func (s *FallbackTestSuite) TestDirFilterWithoutRecursionIsIgnored() {
	s.loc.EXPECT().List().Return([]string{"a.txt"}, nil)

	rs := s.list(list.DirFilter(func(vfs.Entry) bool { return false }))
	s.Empty(errsOf(rs))
	s.Equal([]string{"a.txt"}, keysOf(rs))
}

func (s *FallbackTestSuite) TestFilesOnlyIsSupported() {
	s.loc.EXPECT().List().Return([]string{"a.txt"}, nil)
	s.Equal([]string{"a.txt"}, keysOf(s.list(list.FilesOnly())))
}

func (s *FallbackTestSuite) TestFailures() {
	boom := errors.New("connection reset")
	tests := []struct {
		name         string
		prepare      func()
		expectedErr  error
		expectsError bool
	}{
		{
			name:         "list fails",
			prepare:      func() { s.loc.EXPECT().List().Return(nil, boom) },
			expectedErr:  boom,
			expectsError: true,
		},
		{
			name:         "permission denied",
			prepare:      func() { s.loc.EXPECT().List().Return(nil, fs.ErrPermission) },
			expectedErr:  fs.ErrPermission,
			expectsError: true,
		},
		{
			name:    "missing location yields nothing",
			prepare: func() { s.loc.EXPECT().List().Return(nil, fs.ErrNotExist) },
		},
		{
			name: "missing sub-location yields nothing",
			prepare: func() {
				sub := mocks.NewLocation(s.T())
				sub.EXPECT().ListByPrefix("x").Return(nil, fs.ErrNotExist)
				s.loc.EXPECT().NewLocation("to/").Return(sub, nil)
			},
		},
		{
			name: "sub-location failure that is not a missing location",
			prepare: func() {
				sub := mocks.NewLocation(s.T())
				sub.EXPECT().ListByPrefix("x").Return(nil, boom)
				s.loc.EXPECT().NewLocation("to/").Return(sub, nil)
			},
			expectedErr:  boom,
			expectsError: true,
		},
		{
			name:         "sub-location cannot be resolved",
			prepare:      func() { s.loc.EXPECT().NewLocation("to/").Return(nil, boom) },
			expectedErr:  boom,
			expectsError: true,
		},
	}
	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.SetupTest()
			tt.prepare()
			opts := []options.ListOption{}
			if strings.HasPrefix(tt.name, "sub-location") || tt.name == "missing sub-location yields nothing" {
				opts = append(opts, list.Prefix("to/x"))
			}

			rs := s.list(opts...)
			if !tt.expectsError {
				s.Empty(rs)
				return
			}
			s.Require().Len(rs, 1)
			s.Require().ErrorIs(rs[0].err, tt.expectedErr)
			var pe *fs.PathError
			s.Require().ErrorAs(rs[0].err, &pe)
			s.Equal("list", pe.Op)
		})
	}
}

func (s *FallbackTestSuite) TestInvalidNamesAreReportedAndSkipped() {
	s.loc.EXPECT().List().Return([]string{"a.txt", "b//c", "z.txt"}, nil)

	rs := s.list()
	s.Equal([]string{"a.txt", "z.txt"}, keysOf(rs))
	errs := errsOf(rs)
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], fs.ErrInvalid)
}

func (s *FallbackTestSuite) TestCanceledBeforeTheRequest() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	rs := drain(list.Entries(ctx, s.loc))
	s.Require().Len(rs, 1)
	s.Require().ErrorIs(rs[0].err, context.Canceled)
}

func (s *FallbackTestSuite) TestCancellationBetweenYields() {
	ctx, cancel := context.WithCancel(s.T().Context())
	defer cancel()
	s.loc.EXPECT().List().Return([]string{"a", "b", "c"}, nil)

	var got []result
	for e, err := range list.Entries(ctx, s.loc) {
		got = append(got, result{entry: e, err: err})
		cancel()
	}
	s.Require().Len(got, 2)
	s.Require().NoError(got[0].err)
	s.Require().ErrorIs(got[1].err, context.Canceled)
}

func (s *FallbackTestSuite) TestBreakStopsTheListing() {
	s.loc.EXPECT().List().Return([]string{"a", "b"}, nil)

	n := 0
	for _, err := range list.Entries(s.T().Context(), s.loc) {
		s.Require().NoError(err)
		n++
		break
	}
	s.Equal(1, n)
}

func (s *FallbackTestSuite) TestCancellationIsReportedWhenEveryRemainingEntryIsFilteredOut() {
	s.loc.EXPECT().List().Return([]string{"a", "b", "c"}, nil).Once()
	ctx, cancel := context.WithCancel(s.T().Context())
	defer cancel()

	calls := 0
	var errs []error
	for _, err := range list.Entries(ctx, s.loc, list.Filter(func(vfs.Entry) bool {
		calls++
		cancel()
		return false
	})) {
		errs = append(errs, err)
	}

	s.Equal(1, calls, "no callback runs after the context is canceled")
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], context.Canceled)
}
