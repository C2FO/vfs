package list_test

import (
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// MigrationTestSuite checks that each replacement the README gives for a deprecated Location method returns the same
// file names, so a caller can follow the migration table without a change in behavior.
type MigrationTestSuite struct {
	suite.Suite
	loc vfs.Location
}

func TestMigrationTestSuite(t *testing.T) {
	suite.Run(t, new(MigrationTestSuite))
}

func (s *MigrationTestSuite) SetupTest() {
	fs := mem.NewFileSystem()
	loc, err := fs.NewLocation("migration", "/some/path/")
	s.Require().NoError(err)
	names := []string{
		"file1.txt", "file2.csv", "Zebra.txt", "other.txt", "to/somepattern1.txt", "to/somepattern2.txt", "to/else.txt",
	}
	for _, name := range names {
		f, err := loc.NewFile(name)
		s.Require().NoError(err)
		_, err = f.Write([]byte(name))
		s.Require().NoError(err)
		s.Require().NoError(f.Close())
	}
	s.loc = loc
}

// names lists the replacement and returns its file names sorted, with the directory part of the path dropped as the
// legacy methods return base names.
func (s *MigrationTestSuite) names(opts ...options.ListOption) []string {
	s.T().Helper()
	entries, err := list.Collect(list.Entries(s.T().Context(), s.loc, append(opts, list.FilesOnly())...))
	s.Require().NoError(err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

func sorted(names []string, err error) ([]string, error) {
	slices.Sort(names)
	return names, err
}

func (s *MigrationTestSuite) TestReplacementsMatchTheDeprecatedMethods() {
	tests := []struct {
		name        string
		legacy      func() ([]string, error)
		replacement []options.ListOption
	}{
		{
			name:   "List",
			legacy: func() ([]string, error) { return sorted(s.loc.List()) },
		},
		{
			name:        "ListByPrefix",
			legacy:      func() ([]string, error) { return sorted(s.loc.ListByPrefix("file")) },
			replacement: []options.ListOption{list.Prefix("file")},
		},
		{
			name:        "ListByPrefix with a relative prefix",
			legacy:      func() ([]string, error) { return sorted(s.loc.ListByPrefix("to/some")) },
			replacement: []options.ListOption{list.Prefix("to/some")},
		},
		{
			name: "ListByRegex",
			legacy: func() ([]string, error) {
				return sorted(s.loc.ListByRegex(regexp.MustCompile(`\.txt$`)))
			},
			replacement: []options.ListOption{list.Match(regexp.MustCompile(`\.txt$`))},
		},
		{
			name: "ListByRegex anchored at the start",
			legacy: func() ([]string, error) {
				return sorted(s.loc.ListByRegex(regexp.MustCompile(`^f`)))
			},
			replacement: []options.ListOption{list.Match(regexp.MustCompile(`^f`))},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			want, err := tt.legacy()
			s.Require().NoError(err)
			s.Require().NotEmpty(want, "the case should match something")
			s.Equal(want, s.names(tt.replacement...))
		})
	}
}
