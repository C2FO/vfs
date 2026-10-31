package testsuite

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// listingFixture is the tree the listing tests run against: names chosen to expose ordering and encoding mistakes. The
// file "logs-2024.gz" must sort before the directory "logs", mixed-case, space and Unicode names must survive, and
// one file sits five levels deep. No two names differ only by case, because the fixture must also work on
// case-insensitive file systems.
var listingFixture = []string{
	"a-b.txt",
	"a.txt",
	"logs-2024.gz",
	"logs/app.log",
	"logs/old/2023.log",
	"logs/old/2024/jan/x.log",
	"Mixed/Case.TXT",
	"mixedCase/lower.txt",
	"with space/file name.txt",
	"\u00fcn\u00ef/c\u00f6d\u00e9.txt",
	"z.txt",
}

// listingTree is the fixture written under one location: the content of each file, and every order key the tree holds.
type listingTree struct {
	root    vfs.Location
	content map[string]string
	keys    []string
}

// RunListingTests tests that a backend's native listing, list.Entries over a location that implements
// vfs.EntryLister, yields the same entries, in the same order, with the same metadata as every other backend. It
// checks order, completeness, directories, metadata, resolution to files and locations, paging, depth, prefix, glob,
// early termination and cancellation.
//
// baseLoc must implement vfs.EntryLister, or RunListingTests fails. RunConformanceTests skips this suite for locations
// that do not, so a backend adopts it by implementing native listing.
func RunListingTests(t *testing.T, baseLoc vfs.Location, opts ...ConformanceOptions) {
	t.Helper()
	opt := ConformanceOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	require.Implements(t, (*vfs.EntryLister)(nil), baseLoc, "a backend runs the listing suite by listing natively")

	tree := newListingTree(t, baseLoc, opt)

	t.Run("Order", func(t *testing.T) { testListingOrder(t, tree) })
	t.Run("Entries", func(t *testing.T) { testListingEntries(t, tree, opt) })
	t.Run("Options", func(t *testing.T) { testListingOptions(t, tree) })
	t.Run("Paging", func(t *testing.T) { testListingPaging(t, tree) })
	t.Run("EarlyTermination", func(t *testing.T) { testListingEarlyTermination(t, tree) })
	t.Run("ReadWhileListing", func(t *testing.T) { testListingReadWhileListing(t, tree) })
	t.Run("Cancellation", func(t *testing.T) { testListingCancellation(t, tree) })
	t.Run("MissingLocation", func(t *testing.T) { testListingMissingLocation(t, tree) })
	if opt.CreateEmptyDirectory != nil {
		t.Run("EmptyDirectory", func(t *testing.T) { testListingEmptyDirectory(t, tree) })
	}
}

// newListingTree writes the fixture under a location of its own and removes it when the test ends.
func newListingTree(t *testing.T, baseLoc vfs.Location, opt ConformanceOptions) *listingTree {
	t.Helper()
	root, err := baseLoc.NewLocation("listingTests/")
	require.NoError(t, err)

	tree := &listingTree{root: root, content: map[string]string{}}
	t.Cleanup(func() { removeListingTree(t, tree, opt) })

	for _, rel := range listingFixture {
		body := strings.Repeat("x", len(rel))
		if opt.CreateEmptyDirectory != nil {
			dirLoc := root
			if dir := path.Dir(rel); dir != "." {
				dirLoc, err = root.NewLocation(dir + "/")
				require.NoError(t, err)
			}
			require.NoError(t, opt.CreateEmptyDirectory(dirLoc), "creating the directory of %s", rel)
		}
		f, err := root.NewFile(rel)
		require.NoError(t, err, "creating %s", rel)
		_, err = f.Write([]byte(body))
		require.NoError(t, err, "writing %s", rel)
		require.NoError(t, f.Close(), "closing %s", rel)
		tree.content[rel] = body
	}

	tree.keys = listingKeys(listingFixture)
	if opt.CreateEmptyDirectory != nil {
		emptyLoc, err := root.NewLocation("empty/")
		require.NoError(t, err)
		require.NoError(t, opt.CreateEmptyDirectory(emptyLoc), "creating the empty directory")
		tree.keys = listingKeys(append(slices.Clone(listingFixture), "empty/"))
	}
	return tree
}

func removeListingTree(t *testing.T, tree *listingTree, opt ConformanceOptions) {
	t.Helper()
	for rel := range tree.content {
		_ = tree.root.DeleteFile(rel)
	}
	if opt.RemoveDirectory != nil {
		assert.NoError(t, opt.RemoveDirectory(tree.root), "failed to clean up listing test location")
		return
	}
	if tree.root.FileSystem().Scheme() == "file" {
		assert.NoError(t, os.RemoveAll(tree.root.Path()), "failed to clean up listing test location")
	}
}

// listingKeys returns the sorted order key of every entry a set of paths implies: each file, and each directory,
// whether named by a trailing slash or implied by a file inside it.
func listingKeys(paths []string) []string {
	set := map[string]bool{}
	for _, p := range paths {
		isDir := strings.HasSuffix(p, "/")
		segs := strings.Split(strings.TrimSuffix(p, "/"), "/")
		for i := range segs {
			rel := strings.Join(segs[:i+1], "/")
			if i < len(segs)-1 || isDir {
				rel += "/"
			}
			set[rel] = true
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// keysWhere returns the tree's order keys for which keep is true, given the relative path and whether it is a
// directory.
func (tree *listingTree) keysWhere(keep func(rel string, dir bool) bool) []string {
	var out []string
	for _, key := range tree.keys {
		if rel := strings.TrimSuffix(key, "/"); keep(rel, strings.HasSuffix(key, "/")) {
			out = append(out, key)
		}
	}
	return out
}

func (tree *listingTree) list(t *testing.T, opts ...options.ListOption) []vfs.Entry {
	t.Helper()
	entries, err := list.Collect(list.Entries(t.Context(), tree.root, opts...))
	require.NoError(t, err)
	return entries
}

func orderKeys(entries []vfs.Entry) []string {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func depthOf(rel string) int { return strings.Count(rel, "/") + 1 }

func testListingOrder(t *testing.T, tree *listingTree) {
	t.Run("recursive listing is complete and in byte order", func(t *testing.T) {
		assert.Equal(t, tree.keys, orderKeys(tree.list(t, list.Recursive())))
	})

	t.Run("a file sorts before the directory it shares a prefix with", func(t *testing.T) {
		keys := orderKeys(tree.list(t, list.Recursive()))
		assert.Less(t, slices.Index(keys, "logs-2024.gz"), slices.Index(keys, "logs/"))
	})

	t.Run("non-recursive listing yields the immediate children", func(t *testing.T) {
		expected := tree.keysWhere(func(rel string, _ bool) bool { return depthOf(rel) == 1 })
		assert.Equal(t, expected, orderKeys(tree.list(t)))
	})

	t.Run("listing twice yields the same sequence", func(t *testing.T) {
		first := orderKeys(tree.list(t, list.Recursive()))
		second := orderKeys(tree.list(t, list.Recursive()))
		assert.Equal(t, first, second)
	})
}

func testListingEntries(t *testing.T, tree *listingTree, opt ConformanceOptions) {
	entries := tree.list(t, list.Recursive())
	require.NotEmpty(t, entries)

	t.Run("names and paths", func(t *testing.T) {
		for _, e := range entries {
			assert.Equal(t, path.Base(e.RelPath()), e.Name(), e.RelPath())
			assert.NotContains(t, e.RelPath(), "\\", e.RelPath())
			assert.False(t, strings.HasPrefix(e.RelPath(), "/") || strings.HasSuffix(e.RelPath(), "/"), e.RelPath())
			expectedPath := tree.root.Path() + e.RelPath()
			if e.IsDir() {
				expectedPath += "/"
			}
			assert.Equal(t, expectedPath, e.Path())
		}
	})

	t.Run("kinds", func(t *testing.T) {
		for _, e := range entries {
			if e.IsDir() {
				assert.Equal(t, fs.ModeDir, e.Type(), e.RelPath())
				continue
			}
			assert.Zero(t, e.Type()&fs.ModeDir, e.RelPath())
		}
	})

	t.Run("listed metadata matches the files", func(t *testing.T) {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			require.True(t, e.HasMetadata(), "%s was listed without metadata", e.RelPath())
			assert.Equal(t, uint64(len(tree.content[e.RelPath()])), e.Size(), e.RelPath())

			f, err := e.File()
			require.NoError(t, err)
			modTime, err := f.LastModified()
			require.NoError(t, err)
			assert.WithinDuration(t, *modTime, e.ModTime(), opt.ListModTimeTolerance, e.RelPath())

			info, err := e.Info()
			require.NoError(t, err)
			assert.Equal(t, int64(e.Size()), info.Size())
		}
	})

	t.Run("entries resolve to files and locations", func(t *testing.T) {
		for _, e := range entries {
			if e.IsDir() {
				loc, err := e.Location()
				require.NoError(t, err)
				assert.Equal(t, e.Path(), loc.Path())
				_, err = e.File()
				require.Error(t, err, "a directory entry is not a file")
				continue
			}
			f, err := e.File()
			require.NoError(t, err)
			assert.Equal(t, e.Path(), f.Path())
			_, err = e.Location()
			require.Error(t, err, "a file entry is not a location")
		}
	})

	t.Run("a listed file can be read", func(t *testing.T) {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			f, err := e.File()
			require.NoError(t, err)
			data, err := io.ReadAll(f)
			require.NoError(t, err)
			require.NoError(t, f.Close())
			assert.Equal(t, tree.content[e.RelPath()], string(data), e.RelPath())
		}
	})

	t.Run("an entry outlives the file it describes", func(t *testing.T) {
		rel := "z.txt"
		e := entryFor(t, entries, rel)
		f, err := e.File()
		require.NoError(t, err)
		t.Cleanup(func() { restoreListingFile(t, tree, rel) })
		require.NoError(t, f.Delete())

		exists, err := f.Exists()
		require.NoError(t, err)
		assert.False(t, exists, "the file is gone")
		info, err := e.Info()
		require.NoError(t, err, "listed metadata needs no request")
		assert.Equal(t, int64(len(tree.content[rel])), info.Size())
	})
}

func entryFor(t *testing.T, entries []vfs.Entry, rel string) vfs.Entry {
	t.Helper()
	for _, e := range entries {
		if e.RelPath() == rel {
			return e
		}
	}
	require.FailNow(t, "entry not listed", rel)
	return vfs.Entry{}
}

func restoreListingFile(t *testing.T, tree *listingTree, rel string) {
	t.Helper()
	f, err := tree.root.NewFile(rel)
	require.NoError(t, err)
	_, err = f.Write([]byte(tree.content[rel]))
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func testListingOptions(t *testing.T, tree *listingTree) {
	tests := []struct {
		name     string
		opts     []options.ListOption
		expected func(rel string, dir bool) bool
	}{
		{
			name:     "files only",
			opts:     []options.ListOption{list.Recursive(), list.FilesOnly()},
			expected: func(_ string, dir bool) bool { return !dir },
		},
		{
			name:     "directories only",
			opts:     []options.ListOption{list.Recursive(), list.DirsOnly()},
			expected: func(_ string, dir bool) bool { return dir },
		},
		{
			name:     "max depth",
			opts:     []options.ListOption{list.Recursive(), list.MaxDepth(2)},
			expected: func(rel string, _ bool) bool { return depthOf(rel) <= 2 },
		},
		{
			name:     "prefix",
			opts:     []options.ListOption{list.Recursive(), list.Prefix("logs")},
			expected: func(rel string, _ bool) bool { return strings.HasPrefix(rel, "logs") },
		},
		{
			name:     "nested prefix without recursion",
			opts:     []options.ListOption{list.Prefix("logs/o")},
			expected: func(rel string, _ bool) bool { return strings.HasPrefix(rel, "logs/o") && depthOf(rel) <= 2 },
		},
		{
			name:     "glob",
			opts:     []options.ListOption{list.Glob("*/*.log")},
			expected: func(rel string, _ bool) bool { ok, _ := path.Match("*/*.log", rel); return ok },
		},
		{
			name:     "cursor inside a subtree",
			opts:     []options.ListOption{list.Recursive(), list.StartAfter("logs/old/2024/jan/x.log")},
			expected: func(rel string, dir bool) bool { return orderKeyOf(rel, dir) > "logs/old/2024/jan/x.log" },
		},
		{
			name:     "cursor on a directory",
			opts:     []options.ListOption{list.Recursive(), list.StartAfter("logs/")},
			expected: func(rel string, dir bool) bool { return orderKeyOf(rel, dir) > "logs/" },
		},
	}
	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			expected := tree.keysWhere(tt.expected)
			require.NotEmpty(t, expected, "the case must select something")
			got, err := list.Collect(list.Entries(t.Context(), tree.root, tt.opts...))
			require.NoError(t, err)
			assert.Equal(t, expected, orderKeys(got))
		})
	}
}

func orderKeyOf(rel string, dir bool) string {
	if dir {
		return rel + "/"
	}
	return rel
}

func testListingPaging(t *testing.T, tree *listingTree) {
	tests := []struct {
		name string
		opts []options.ListOption
	}{
		{name: "recursive", opts: []options.ListOption{list.Recursive()}},
		{name: "non-recursive", opts: nil},
		{name: "files only", opts: []options.ListOption{list.Recursive(), list.FilesOnly()}},
		{name: "directories only", opts: []options.ListOption{list.Recursive(), list.DirsOnly()}},
		{name: "prefix", opts: []options.ListOption{list.Recursive(), list.Prefix("logs")}},
		{name: "glob", opts: []options.ListOption{list.Glob("*/*.log")}},
		{name: "max depth", opts: []options.ListOption{list.Recursive(), list.MaxDepth(2)}},
	}
	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			full := orderKeys(tree.list(t, tt.opts...))
			require.NotEmpty(t, full)

			for _, size := range []int{1, 2, 3, 100} {
				var got []string
				cursor := list.StartAfter("")
				for {
					page := tree.list(t, append(slices.Clone(tt.opts), list.Limit(size), cursor)...)
					if len(page) == 0 {
						break
					}
					require.LessOrEqual(t, len(page), size)
					got = append(got, orderKeys(page)...)
					cursor = list.After(page[len(page)-1])
				}
				assert.Equal(t, full, got, "page size %d", size)
			}
		})
	}
}

// testListingReadWhileListing reads every file from inside the loop, and reads one after leaving the loop early. A
// backend that shares one connection between listing and transfers must not let either break the listing or the read.
func testListingReadWhileListing(t *testing.T, tree *listingTree) {
	t.Run("reading each file inside the loop", func(t *testing.T) {
		var keys []string
		for e, err := range list.Entries(t.Context(), tree.root, list.Recursive()) {
			require.NoError(t, err)
			keys = append(keys, list.OrderKey(e))
			if e.IsDir() {
				continue
			}
			f, err := e.File()
			require.NoError(t, err)
			data, err := io.ReadAll(f)
			require.NoError(t, err, e.RelPath())
			require.NoError(t, f.Close())
			assert.Equal(t, tree.content[e.RelPath()], string(data), e.RelPath())
		}
		assert.Equal(t, tree.keys, keys)
	})

	t.Run("reading after leaving the loop early", func(t *testing.T) {
		var first vfs.Entry
		for e, err := range list.Entries(t.Context(), tree.root, list.Recursive(), list.FilesOnly()) {
			require.NoError(t, err)
			first = e
			break
		}
		f, err := first.File()
		require.NoError(t, err)
		data, err := io.ReadAll(f)
		require.NoError(t, err)
		require.NoError(t, f.Close())
		assert.Equal(t, tree.content[first.RelPath()], string(data))

		again, err := list.Collect(list.Entries(t.Context(), tree.root, list.Recursive()))
		require.NoError(t, err)
		assert.Len(t, again, len(tree.keys), "a listing after the read is complete")
	})
}

func testListingEarlyTermination(t *testing.T, tree *listingTree) {
	baseline := runtime.NumGoroutine()
	for range 20 {
		for _, err := range list.Entries(t.Context(), tree.root, list.Recursive()) {
			require.NoError(t, err)
			break
		}
	}

	// A listing that keeps working after its loop stopped would hold a goroutine, and 20 listings would hold 20, so the
	// count must come back to within a few of where it started. The margin absorbs unrelated goroutines.
	const margin = 5
	assert.Eventually(t, func() bool { return runtime.NumGoroutine() <= baseline+margin }, 5*time.Second, 20*time.Millisecond,
		"stopping a listing early left goroutines running")

	// The backend stays usable after a loop stops early.
	f, err := tree.root.NewFile("a.txt")
	require.NoError(t, err)
	data, err := io.ReadAll(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	assert.Equal(t, tree.content["a.txt"], string(data))
}

func testListingCancellation(t *testing.T, tree *listingTree) {
	t.Run("canceled before the listing starts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		var errs []error
		for _, err := range list.Entries(ctx, tree.root, list.Recursive()) {
			if err != nil {
				errs = append(errs, err)
			}
		}
		require.Len(t, errs, 1)
		assert.ErrorIs(t, errs[0], context.Canceled)
	})

	t.Run("canceled after the first entry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		var seen int
		var last error
		for _, err := range list.Entries(ctx, tree.root, list.Recursive()) {
			if err != nil {
				last = err
				continue
			}
			seen++
			cancel()
		}
		assert.Equal(t, 1, seen, "no entry is yielded after cancellation")
		assert.ErrorIs(t, last, context.Canceled)
	})
}

func testListingMissingLocation(t *testing.T, tree *listingTree) {
	missing, err := tree.root.NewLocation("no-such-location/")
	require.NoError(t, err)

	entries, err := list.Collect(list.Entries(t.Context(), missing, list.Recursive()))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func testListingEmptyDirectory(t *testing.T, tree *listingTree) {
	entries := tree.list(t, list.Recursive())
	e := entryFor(t, entries, "empty")
	assert.True(t, e.IsDir())

	loc, err := e.Location()
	require.NoError(t, err)
	inside, err := list.Collect(list.Entries(t.Context(), loc, list.Recursive()))
	require.NoError(t, err)
	assert.Empty(t, inside, "an empty directory has no entries")

	t.Run("it is listed once, where its order key puts it", func(t *testing.T) {
		assert.Equal(t, 1, strings.Count(strings.Join(orderKeys(entries), "\n"), "empty/"))
	})
}
