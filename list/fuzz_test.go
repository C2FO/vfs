package list_test

import (
	"context"
	"iter"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/options"
)

// fuzzNames is deliberately full of names whose order depends on how "/" compares with their neighbors.
var fuzzNames = []string{"a", "a-b", "a.b", "a0", "b", "ab", "x", "x0", "z"}

// fuzzGlobs are patterns of several depths, with and without meta characters in the literal part.
var fuzzGlobs = []string{"*", "a*", "?0", "*/*", "*/a", "a/*", "?/b", "*/*/*", "[ab]*/*", "a/b/*", "x*/z"}

// fuzzMatch is the expression the fuzzed Match option uses; it hits names with "-" or "0" in them.
var fuzzMatch = regexp.MustCompile(`[-0]`)

// fuzzPaths turns bytes into a set of paths: each byte picks a name for one segment, and two bits of it pick the
// path's depth and whether it is a folder marker.
func fuzzPaths(data []byte) []string {
	var paths []string
	for i := 0; i+2 < len(data); i += 3 {
		depth := int(data[i]%4) + 1
		var segs []string
		for j := range depth {
			segs = append(segs, fuzzNames[int(data[(i+1+j)%len(data)])%len(fuzzNames)])
		}
		p := strings.Join(segs, "/")
		if data[i]&0x80 != 0 {
			p += "/"
		}
		paths = append(paths, p)
	}
	return paths
}

// fuzzKeys returns the order key of every entry in the tree the paths describe, sorted: every file, and every
// directory, whether explicit or implied by a path.
func fuzzKeys(paths []string) []string {
	set := map[string]bool{}
	for _, p := range paths {
		isDir := strings.HasSuffix(p, "/")
		segs := strings.Split(strings.TrimSuffix(p, "/"), "/")
		for i := range segs {
			rel := strings.Join(segs[:i+1], "/")
			if i < len(segs)-1 || isDir {
				set[rel+"/"] = true
			} else {
				set[rel] = true
			}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// fuzzPruned reports whether the entry at rel lies under a directory the fuzzed DirFilter rejects, which is any
// directory named "a" that is asked about: one inside the depth limit.
func fuzzPruned(rel string, q list.Query) bool {
	if q.Descend == nil || !q.Recursive {
		return false
	}
	segs := strings.Split(rel, "/")
	for i := range len(segs) - 1 {
		if segs[i] == "a" && (q.Depth() == 0 || i+1 < q.Depth()) {
			return true
		}
	}
	return false
}

// fuzzWindow computes what an engine yields without either engine: the entries inside the prefix, cursor and depth,
// minus those under a pruned directory.
func fuzzWindow(keys []string, q list.Query) []string {
	var out []string
	for _, key := range keys {
		rel := strings.TrimSuffix(key, "/")
		inDepth := q.Depth() == 0 || strings.Count(rel, "/") < q.Depth()
		if inDepth && strings.HasPrefix(rel, q.Prefix) && key > q.StartAfter && !fuzzPruned(rel, q) {
			out = append(out, key)
		}
	}
	return out
}

// fuzzResult computes what Entries yields: the window narrowed by kind, regexp and limit.
func fuzzResult(keys []string, q list.Query) []string {
	var out []string
	for _, key := range fuzzWindow(keys, q) {
		isDir := strings.HasSuffix(key, "/")
		if (q.Kind == list.KindFiles && isDir) || (q.Kind == list.KindDirs && !isDir) {
			continue
		}
		rel := strings.TrimSuffix(key, "/")
		if q.Match != nil && !q.Match.MatchString(rel) {
			continue
		}
		if q.Glob != "" {
			if ok, err := path.Match(q.Glob, rel); err != nil || !ok {
				continue
			}
		}
		out = append(out, key)
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out
}

// fuzzOptions picks listing options from the bytes: recursion and depth, a prefix and cursor taken from the tree, a
// glob, a kind, a regexp, a directory filter that rejects directories named "a", and a limit.
func fuzzOptions(data []byte, paths []string) []options.ListOption {
	if len(data) < 4 || len(paths) == 0 {
		return nil
	}
	var opts []options.ListOption
	if data[0]&0x80 != 0 {
		// A glob implies its own prefix, recursion and depth, and so excludes those options.
		opts = append(opts, list.Glob(fuzzGlobs[int(data[2])%len(fuzzGlobs)]))
	} else if data[0]&1 != 0 {
		opts = append(opts, list.Recursive())
		if depth := int(data[1] % 4); depth > 0 {
			opts = append(opts, list.MaxDepth(depth))
		}
	}
	pick := paths[int(data[2])%len(paths)]
	plain := strings.TrimSuffix(pick, "/")
	if data[0]&0x80 == 0 && data[0]&2 != 0 {
		opts = append(opts, list.Prefix(plain[:int(data[3])%(len(plain)+1)]))
	}
	if data[0]&4 != 0 {
		opts = append(opts, list.StartAfter(pick[:int(data[3])%(len(pick)+1)]))
	}
	switch {
	case data[0]&8 != 0:
		opts = append(opts, list.FilesOnly())
	case data[0]&16 != 0:
		opts = append(opts, list.DirsOnly())
	}
	if data[0]&32 != 0 {
		opts = append(opts, list.Match(fuzzMatch))
	}
	if data[0]&64 != 0 {
		opts = append(opts, list.DirFilter(func(e vfs.Entry) bool { return e.Name() != "a" }))
	}
	if limit := int(data[1]>>4) % 5; limit > 0 {
		opts = append(opts, list.Limit(limit))
	}
	return opts
}

// fuzzEntries builds the entries of a whole tree, as a backend that ignores every hint would yield them.
func fuzzEntries(t *testing.T, loc vfs.Location, keys []string) []result {
	t.Helper()
	out := make([]result, 0, len(keys))
	for _, key := range keys {
		build, rel := vfs.NewFileEntry, key
		if strings.HasSuffix(key, "/") {
			build, rel = vfs.NewDirEntry, strings.TrimSuffix(key, "/")
		}
		e, err := build(loc, rel)
		require.NoError(t, err)
		out = append(out, result{entry: e})
	}
	return out
}

// fuzzLocation is a location whose native listing is serve.
type serveFunc = func(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error]

func fuzzLocation(t *testing.T, base *mocks.Location, serve serveFunc) nativeLocation {
	t.Helper()
	lister := mocks.NewEntryLister(t)
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).RunAndReturn(serve).Maybe()
	lister.EXPECT().ListEntries(mock.Anything).RunAndReturn(serve).Maybe()
	return nativeLocation{Location: base, lister: lister}
}

// FuzzEnginesAgree checks, for any tree and any options, that:
//   - WalkDirs, FlatKeys at any page size, and an independent computation yield the same window of the tree;
//   - Entries returns the same result over a backend that ignores every hint, over WalkDirs and over FlatKeys, and
//     that result equals an independent computation of every option.
func FuzzEnginesAgree(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09})
	f.Add([]byte{0x85, 0x00, 0x01, 0x02, 0x03, 0x04, 0x91, 0x02, 0x00, 0x01, 0x03, 0x01})
	f.Add([]byte{0x03, 0x01, 0x00, 0x05, 0x40, 0x01, 0x01, 0x01, 0x01, 0x82, 0x04, 0x03, 0x02})
	f.Add([]byte{0x07, 0x02, 0x05, 0x02, 0x10, 0x20, 0x30, 0x41, 0x52, 0x63, 0x74, 0x85, 0x96, 0xa7, 0xb8})
	f.Add([]byte{0x80, 0x00, 0x03, 0x00, 0x01, 0x01, 0x01, 0x02, 0x02, 0x02, 0x02, 0x00})
	f.Add([]byte{0x80, 0x00, 0x04, 0x00, 0x00, 0x01, 0x02, 0x03, 0x01, 0x02, 0x04, 0x02})
	f.Add([]byte{0x49, 0x31, 0x00, 0x05, 0x00, 0x00, 0x00, 0x01, 0x01, 0x01, 0x01, 0x02, 0x02, 0x03})
	f.Add([]byte{0x75, 0x42, 0x01, 0x03, 0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x04, 0x08, 0x00, 0x85, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		paths := fuzzPaths(data)
		opts := fuzzOptions(data, paths)
		q, err := list.ParseQuery(opts)
		if err != nil {
			t.Skip()
		}
		keys := fuzzKeys(paths)
		window := fuzzWindow(keys, q)
		expected := fuzzResult(keys, q)

		base := mocks.NewLocation(t)
		base.EXPECT().Path().Return("/root/").Maybe()

		dirs := &fakeDirs{tree: treeFromPaths(paths...)}
		walked := drain(list.WalkDirs(t.Context(), base, q, dirs.readDir, nil))
		require.Empty(t, errsOf(walked))
		require.Equal(t, nonNil(window), nonNil(keysOf(walked)), "WalkDirs, paths %q", paths)

		objectKeys := slices.Clone(paths)
		slices.Sort(objectKeys)
		for _, pageSize := range []int{0, 1, 2, 5} {
			bucket := &fakeBucket{keys: objectKeys, pageSize: pageSize}
			flat := drain(list.FlatKeys(t.Context(), base, q, bucket.pages, nil))
			require.Empty(t, errsOf(flat))
			require.Equal(t, nonNil(window), nonNil(keysOf(flat)), "FlatKeys page size %d, paths %q", pageSize, paths)
		}

		sources := map[string]serveFunc{
			"hint-ignoring backend": func(context.Context, ...options.ListOption) iter.Seq2[vfs.Entry, error] {
				return func(yield func(vfs.Entry, error) bool) {
					for _, r := range fuzzEntries(t, base, keys) {
						if !yield(r.entry, nil) {
							return
						}
					}
				}
			},
			"WalkDirs": func(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
				own, _ := list.ParseQuery(opts)
				return list.WalkDirs(ctx, base, own, (&fakeDirs{tree: treeFromPaths(paths...)}).readDir, nil)
			},
			"FlatKeys": func(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
				own, _ := list.ParseQuery(opts)
				bucket := &fakeBucket{keys: objectKeys, pageSize: 2}
				return list.FlatKeys(ctx, base, own, bucket.pages, nil)
			},
		}
		for name, serve := range sources {
			got := drain(list.Entries(t.Context(), fuzzLocation(t, base, serve), opts...))
			require.Empty(t, errsOf(got), name)
			require.Equal(t, nonNil(expected), nonNil(keysOf(got)), "Entries over %s, paths %q", name, paths)
		}
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
