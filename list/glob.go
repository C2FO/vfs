package list

import (
	"fmt"
	"path"
	"strings"
)

// compiledGlob is what a glob pattern implies about the listing that will find its matches.
type compiledGlob struct {
	// prefix is the literal leading directories of the pattern, ending in "/", or "" when there are none. Every match
	// starts with it.
	prefix string
	// depth is the number of path segments in the pattern. A match is exactly this deep.
	depth int
}

// recursive reports whether finding matches requires descending below the listed location.
func (g compiledGlob) recursive() bool { return g.depth > 1 }

// compileGlob validates pattern and derives the literal prefix and depth a listing can use to avoid fetching entries
// that cannot match. The error wraps both ErrInvalidOption and, for a malformed pattern, path.ErrBadPattern.
func compileGlob(pattern string) (compiledGlob, error) {
	if _, err := path.Match(pattern, ""); err != nil {
		return compiledGlob{}, fmt.Errorf("%w: glob %q: %w", ErrInvalidOption, pattern, err)
	}
	segments := strings.Split(pattern, "/")
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			return compiledGlob{}, invalidf("glob %q must be a relative path with no empty, \".\" or \"..\" segments", pattern)
		}
	}

	literal := len(pattern)
	if i := strings.IndexAny(pattern, `*?[\`); i >= 0 {
		literal = i
	}
	prefix := ""
	if i := strings.LastIndex(pattern[:literal], "/"); i >= 0 {
		prefix = pattern[:i+1]
	}
	return compiledGlob{prefix: prefix, depth: len(segments)}, nil
}
