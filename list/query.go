package list

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
)

// ErrInvalidOption is matched by every error [ParseQuery] returns for an invalid option or combination of options.
var ErrInvalidOption = errors.New("invalid list option")

// Kind restricts which kind of entry a listing yields.
type Kind int

const (
	// KindAny yields files and directories.
	KindAny Kind = iota
	// KindFiles yields files only.
	KindFiles
	// KindDirs yields directories only.
	KindDirs
)

// Query is the validated form of a set of listing options. It is what a backend's native listing starts from.
//
// Prefix, Recursive, MaxDepth, Kind, StartAfter and Limit are hints: a backend may use them to avoid fetching entries
// that would be discarded, and may ignore any of them, because the listing applies every option to the entries a
// backend yields. Glob, Match, Filter and Descend are always applied by the listing itself; a backend never needs to
// evaluate them. When Glob is set, Prefix, Recursive and MaxDepth hold the values the pattern implies.
type Query struct {
	// Prefix is the relative path every yielded entry must start with, or "".
	Prefix string
	// Recursive reports whether entries deeper than Depth's non-recursive limit are listed.
	Recursive bool
	// MaxDepth is the deepest path segment count to list, measured from the listed location, or 0 for no limit. It is
	// only non-zero when Recursive is true.
	MaxDepth int
	// Kind restricts the kind of entry yielded.
	Kind Kind
	// StartAfter is the order key (see OrderKey) after which entries are yielded, or "".
	StartAfter string
	// Limit is the number of entries after which the listing ends, or 0 for no limit.
	Limit int
	// Glob is the path.Match pattern entries' relative paths must match, or "".
	Glob string
	// Match is the expression entries' relative paths must match, or nil.
	Match *regexp.Regexp
	// Filter is the custom filter entries must pass, or nil.
	Filter EntryFunc
	// Descend decides whether to descend into a directory in a recursive listing, or is nil to descend into all.
	Descend EntryFunc
}

// Depth returns the largest depth of entry the listing can yield, or 0 when there is no limit. Depth is the number of
// segments in an entry's relative path and is always measured from the listed location, so "a/b/c.txt" has depth 3.
//
// A recursive listing is limited only by MaxDepth. A non-recursive listing is limited to one level inside the directory
// Prefix names: with no Prefix that is 1 (immediate children), and with Prefix "to/rep" it is 2, because the
// entries of "to" are listed. Backends and engines use Depth rather than re-deriving this, so that every backend
// applies the same limit.
func (q Query) Depth() int {
	if q.Recursive {
		return q.MaxDepth
	}
	return strings.Count(q.Prefix, "/") + 1
}

// OrderKey returns the key entries are ordered by: the entry's relative path, with "/" appended for directories.
// Because "-" sorts before "/", the file "logs-2024.gz" is listed before the directory "logs". Byte order of the key
// equals the order object stores return full keys in, which is what lets them stream a listing without buffering.
func OrderKey(e vfs.Entry) string {
	if e.IsDir() {
		return e.RelPath() + "/"
	}
	return e.RelPath()
}

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidOption, fmt.Sprintf(format, args...))
}

// queryOption is implemented by every option this package defines. It validates the option and records it.
type queryOption interface {
	options.ListOption
	apply(b *builder) error
}

// builder accumulates a Query and the facts needed to validate options against one another.
type builder struct {
	q         Query
	recursive bool
	maxDepth  bool
	prefixed  bool
	files     bool
	dirs      bool
	glob      *compiledGlob
}

// ParseQuery validates opts and returns the Query they describe. Any error matches [ErrInvalidOption].
//
// Each option may be given at most once; [After] and [StartAfter] count as one option, as do [Filter] and
// [FilterContext], and [DirFilter] and [DirFilterContext]. Options this package does not define are ignored, so
// backend-specific options can be passed alongside.
func ParseQuery(opts []options.ListOption) (Query, error) {
	var b builder
	seen := make(map[string]struct{}, len(opts))
	for _, opt := range opts {
		if opt == nil {
			return Query{}, invalidf("nil option")
		}
		qo, ok := opt.(queryOption)
		if !ok {
			continue
		}
		name := qo.ListOptionName()
		if _, dup := seen[name]; dup {
			return Query{}, invalidf("option %q given more than once", name)
		}
		seen[name] = struct{}{}
		if err := qo.apply(&b); err != nil {
			return Query{}, err
		}
	}
	if err := b.finish(); err != nil {
		return Query{}, err
	}
	return b.q, nil
}

// finish checks the options against one another and applies what a glob implies.
func (b *builder) finish() error {
	if b.files && b.dirs {
		return invalidf("FilesOnly and DirsOnly cannot be combined")
	}
	if b.maxDepth && !b.recursive {
		return invalidf("MaxDepth requires Recursive")
	}
	if b.glob == nil {
		return nil
	}
	if b.prefixed || b.recursive || b.maxDepth {
		return invalidf("Glob cannot be combined with Prefix, Recursive or MaxDepth")
	}
	b.q.Prefix = b.glob.prefix
	b.q.Recursive = b.glob.recursive()
	if b.q.Recursive {
		b.q.MaxDepth = b.glob.depth
	}
	return nil
}

func afterKey(e vfs.Entry) (string, error) {
	if e.RelPath() == "" {
		return "", invalidf("After requires a listed entry, got the zero Entry")
	}
	return OrderKey(e), nil
}

// validRelPath reports whether p, ignoring one trailing "/", is a valid io/fs name other than ".".
func validRelPath(p string) bool {
	p = strings.TrimSuffix(p, "/")
	return p != "." && fs.ValidPath(p)
}
