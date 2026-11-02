package list_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// GlobTestSuite checks that a Glob matches segment by segment, so that no wildcard or character class matches a "/"
// and the depth the pattern implies is the depth of every match.
type GlobTestSuite struct {
	suite.Suite
	loc vfs.Location
}

func TestGlobTestSuite(t *testing.T) {
	suite.Run(t, new(GlobTestSuite))
}

func (s *GlobTestSuite) SetupTest() {
	loc, err := mem.NewFileSystem().NewLocation("glob", "/p/")
	s.Require().NoError(err)
	for _, name := range []string{"a/b", "ayb", "axb", "x/data.csv", "x/y/data.csv", "z.csv"} {
		f, err := loc.NewFile(name)
		s.Require().NoError(err)
		_, err = f.Write([]byte(name))
		s.Require().NoError(err)
		s.Require().NoError(f.Close())
	}
	s.loc = loc
}

func (s *GlobTestSuite) TestSegmentsAreMatchedSeparately() {
	tests := []struct {
		name     string
		pattern  string
		expected []string
	}{
		{name: "a class that could match a slash does not", pattern: "a[^x]b", expected: []string{"ayb"}},
		{name: "a question mark does not match a slash", pattern: "a?b", expected: []string{"axb", "ayb"}},
		{name: "a star does not match a slash", pattern: "*b", expected: []string{"axb", "ayb"}},
		{name: "one slash names depth two", pattern: "*/data.csv", expected: []string{"x/data.csv"}},
		{name: "two slashes name depth three", pattern: "*/*/data.csv", expected: []string{"x/y/data.csv"}},
		{name: "a literal nested path", pattern: "a/b", expected: []string{"a/b"}},
		{name: "no match", pattern: "nothing/*", expected: nil},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			entries, err := list.Collect(list.Entries(s.T().Context(), s.loc, []options.ListOption{list.Glob(tt.pattern), list.FilesOnly()}...))
			s.Require().NoError(err)
			var got []string
			for _, e := range entries {
				got = append(got, e.RelPath())
			}
			s.Equal(tt.expected, got)
		})
	}
}
