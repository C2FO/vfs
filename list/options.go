package list

import (
	"context"
	"regexp"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNamePrefix     = "prefix"
	optionNameRecursive  = "recursive"
	optionNameMaxDepth   = "maxDepth"
	optionNameFilesOnly  = "filesOnly"
	optionNameDirsOnly   = "dirsOnly"
	optionNameStartAfter = "startAfter"
	optionNameLimit      = "limit"
	optionNameGlob       = "glob"
	optionNameMatch      = "match"
	optionNameFilter     = "filter"
	optionNameDirFilter  = "dirFilter"
)

// EntryFunc decides whether an entry passes a filter. It receives the context of the listing, so a callback can call
// vfs.Entry.InfoContext, and it can fail. A non-nil error ends the listing.
type EntryFunc func(ctx context.Context, e vfs.Entry) (bool, error)

type (
	prefixOption     string
	recursiveOption  struct{}
	maxDepthOption   int
	filesOnlyOption  struct{}
	dirsOnlyOption   struct{}
	startAfterOption string
	afterOption      struct{ entry vfs.Entry }
	limitOption      int
	globOption       string
	matchOption      struct{ re *regexp.Regexp }
	filterOption     struct{ fn EntryFunc }
	dirFilterOption  struct{ fn EntryFunc }
)

func (prefixOption) ListOptionName() string     { return optionNamePrefix }
func (recursiveOption) ListOptionName() string  { return optionNameRecursive }
func (maxDepthOption) ListOptionName() string   { return optionNameMaxDepth }
func (filesOnlyOption) ListOptionName() string  { return optionNameFilesOnly }
func (dirsOnlyOption) ListOptionName() string   { return optionNameDirsOnly }
func (startAfterOption) ListOptionName() string { return optionNameStartAfter }
func (afterOption) ListOptionName() string      { return optionNameStartAfter }
func (limitOption) ListOptionName() string      { return optionNameLimit }
func (globOption) ListOptionName() string       { return optionNameGlob }
func (matchOption) ListOptionName() string      { return optionNameMatch }
func (filterOption) ListOptionName() string     { return optionNameFilter }
func (dirFilterOption) ListOptionName() string  { return optionNameDirFilter }

// Prefix restricts the listing to entries whose relative path starts with p. A prefix containing "/" is relative to
// the listed location, so Prefix("to/rep") matches "to/report.csv". An empty p is the same as omitting the option.
// Prefix cannot be combined with [Glob].
func Prefix(p string) options.ListOption { return prefixOption(p) }

// Recursive lists every file and directory beneath the location instead of only its immediate children.
func Recursive() options.ListOption { return recursiveOption{} }

// MaxDepth stops descent so that no entry deeper than n path segments below the location is yielded. n must be at
// least 1, and MaxDepth requires [Recursive]. MaxDepth cannot be combined with [Glob].
func MaxDepth(n int) options.ListOption { return maxDepthOption(n) }

// FilesOnly restricts the listing to files. It does not stop a recursive listing from descending into directories.
// FilesOnly cannot be combined with [DirsOnly].
func FilesOnly() options.ListOption { return filesOnlyOption{} }

// DirsOnly restricts the listing to directories. DirsOnly cannot be combined with [FilesOnly].
func DirsOnly() options.ListOption { return dirsOnlyOption{} }

// StartAfter yields only entries whose order key (see [OrderKey]) sorts after relPath. A trailing "/" on relPath
// denotes a directory. An empty relPath is the same as omitting the option, so a paging loop can pass its cursor
// unconditionally. StartAfter cannot be combined with [After].
func StartAfter(relPath string) options.ListOption { return startAfterOption(relPath) }

// After yields only entries that sort after e, which is the cursor for the next page of a paged listing. It is
// StartAfter with the order key of e. After cannot be combined with [StartAfter].
func After(e vfs.Entry) options.ListOption { return afterOption{entry: e} }

// Limit ends the listing after n entries have been yielded. Errors do not count toward the limit. n must be at least
// 1.
func Limit(n int) options.ListOption { return limitOption(n) }

// Glob yields only entries whose relative path matches pattern under path.Match rules, at whatever depth the pattern
// names: "*/data.csv" matches "x/data.csv" but not "x/y/data.csv". The pattern must be a relative path with no empty,
// "." or ".." segments. Glob cannot be combined with [Prefix], [Recursive] or [MaxDepth], because it implies them.
func Glob(pattern string) options.ListOption { return globOption(pattern) }

// Match yields only entries whose relative path matches re. It does not affect descent.
func Match(re *regexp.Regexp) options.ListOption { return matchOption{re: re} }

// Filter yields only entries for which fn returns true. It does not affect descent. To stat an entry or to report a
// failure from the callback, use [FilterContext].
func Filter(fn func(e vfs.Entry) bool) options.ListOption {
	if fn == nil {
		return filterOption{}
	}
	return filterOption{fn: func(_ context.Context, e vfs.Entry) (bool, error) { return fn(e), nil }}
}

// FilterContext is [Filter] with a callback that receives the listing's context and can fail. An error from fn ends
// the listing and is never treated as the entry being filtered out.
func FilterContext(fn EntryFunc) options.ListOption { return filterOption{fn: fn} }

// DirFilter prunes descent in a recursive listing: nothing beneath a directory for which fn returns false is yielded.
// The directory itself is still yielded, subject to the other options. To stat an entry or to report a failure from
// the callback, use [DirFilterContext].
func DirFilter(fn func(e vfs.Entry) bool) options.ListOption {
	if fn == nil {
		return dirFilterOption{}
	}
	return dirFilterOption{fn: func(_ context.Context, e vfs.Entry) (bool, error) { return fn(e), nil }}
}

// DirFilterContext is [DirFilter] with a callback that receives the listing's context and can fail. An error from fn
// ends the listing and is never treated as the directory being pruned.
func DirFilterContext(fn EntryFunc) options.ListOption { return dirFilterOption{fn: fn} }

func (o prefixOption) apply(b *builder) error {
	if o != "" && !validRelPath(string(o)) {
		return invalidf("prefix %q must be a relative path without empty, \".\" or \"..\" segments", string(o))
	}
	b.q.Prefix = string(o)
	b.prefixed = o != ""
	return nil
}

func (recursiveOption) apply(b *builder) error { //nolint:unparam // cannot fail; the error satisfies queryOption
	b.q.Recursive = true
	b.recursive = true
	return nil
}

func (o maxDepthOption) apply(b *builder) error {
	if o < 1 {
		return invalidf("MaxDepth must be at least 1, got %d", int(o))
	}
	b.q.MaxDepth = int(o)
	b.maxDepth = true
	return nil
}

func (filesOnlyOption) apply(b *builder) error { //nolint:unparam // cannot fail; the error satisfies queryOption
	b.q.Kind = KindFiles
	b.files = true
	return nil
}

func (dirsOnlyOption) apply(b *builder) error { //nolint:unparam // cannot fail; the error satisfies queryOption
	b.q.Kind = KindDirs
	b.dirs = true
	return nil
}

func (o startAfterOption) apply(b *builder) error {
	if o != "" && !validRelPath(string(o)) {
		return invalidf("StartAfter %q must be a relative path without empty, \".\" or \"..\" segments", string(o))
	}
	b.q.StartAfter = string(o)
	return nil
}

func (o afterOption) apply(b *builder) (err error) {
	b.q.StartAfter, err = afterKey(o.entry)
	return err
}

func (o limitOption) apply(b *builder) error {
	if o < 1 {
		return invalidf("Limit must be at least 1, got %d", int(o))
	}
	b.q.Limit = int(o)
	return nil
}

func (o globOption) apply(b *builder) error {
	g, err := compileGlob(string(o))
	if err != nil {
		return err
	}
	b.q.Glob = string(o)
	b.glob = &g
	return nil
}

func (o matchOption) apply(b *builder) error {
	if o.re == nil {
		return invalidf("Match requires a non-nil regexp")
	}
	b.q.Match = o.re
	return nil
}

func (o filterOption) apply(b *builder) error {
	if o.fn == nil {
		return invalidf("Filter requires a non-nil function")
	}
	b.q.Filter = o.fn
	return nil
}

func (o dirFilterOption) apply(b *builder) error {
	if o.fn == nil {
		return invalidf("DirFilter requires a non-nil function")
	}
	b.q.Descend = o.fn
	return nil
}
