package list_test

import (
	"context"
	"io/fs"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
)

// result is one value yielded by a listing.
type result struct {
	entry vfs.Entry
	err   error
}

func drain(seq iter.Seq2[vfs.Entry, error]) []result {
	var out []result
	for e, err := range seq {
		out = append(out, result{entry: e, err: err})
	}
	return out
}

// keysOf returns the order keys of the entries yielded without an error.
func keysOf(rs []result) []string {
	var out []string
	for _, r := range rs {
		if r.err == nil {
			out = append(out, list.OrderKey(r.entry))
		}
	}
	return out
}

func errsOf(rs []result) []error {
	var out []error
	for _, r := range rs {
		if r.err != nil {
			out = append(out, r.err)
		}
	}
	return out
}

// treeFromPaths builds the children of every directory in a tree. A path ending in "/" is an empty directory.
func treeFromPaths(paths ...string) map[string][]list.Child {
	tree := map[string][]list.Child{}
	add := func(dir, name string, isDir bool) {
		for _, c := range tree[dir] {
			if c.Name == name && c.IsDir == isDir {
				return
			}
		}
		tree[dir] = append(tree[dir], list.Child{Name: name, IsDir: isDir})
	}
	for _, p := range paths {
		explicitDir := strings.HasSuffix(p, "/")
		segs := strings.Split(strings.TrimSuffix(p, "/"), "/")
		for i, name := range segs {
			dir := strings.Join(segs[:i], "/")
			isDir := i < len(segs)-1 || explicitDir
			add(dir, name, isDir)
		}
	}
	return tree
}

// fakeDirs serves a tree to WalkDirs and records every read.
type fakeDirs struct {
	tree   map[string][]list.Child
	errs   map[string]error
	reads  []string
	onRead func(relDir string)
}

func (f *fakeDirs) readDir(_ context.Context, relDir string) ([]list.Child, error) {
	f.reads = append(f.reads, relDir)
	if f.onRead != nil {
		f.onRead(relDir)
	}
	if err := f.errs[relDir]; err != nil {
		return nil, err
	}
	children, ok := f.tree[relDir]
	if !ok && relDir == "" {
		return nil, fs.ErrNotExist
	}
	// Return a copy in reverse order so the engine cannot rely on input order.
	out := slices.Clone(children)
	slices.Reverse(out)
	return out, nil
}

// fakeBucket serves sorted keys to FlatKeys the way an object store does: it honors Prefix, StartAfter and Delimited,
// and it rolls StartAfter inside a directory up into the same common prefix again, which is what S3 does.
type fakeBucket struct {
	keys     []string
	pageSize int
	requests []list.PageRequest
	pageErr  func(call int) error
	mutate   func(call int, p *list.Page)
}

func (b *fakeBucket) pages(_ context.Context, req list.PageRequest) (list.Page, error) {
	call := len(b.requests)
	b.requests = append(b.requests, req)
	if b.pageErr != nil {
		if err := b.pageErr(call); err != nil {
			return list.Page{}, err
		}
	}

	var page list.Page
	count := 0
	lastPrefix := ""
	more := false
	for _, key := range b.keys {
		if !strings.HasPrefix(key, req.Prefix) || key <= req.StartAfter {
			continue
		}
		rest := key[len(req.Prefix):]
		idx := strings.IndexByte(rest, '/')
		if req.Delimited && idx >= 0 {
			prefix := req.Prefix + rest[:idx+1]
			if prefix == lastPrefix {
				continue
			}
			if b.full(count) {
				more = true
				break
			}
			lastPrefix = prefix
			page.Prefixes = append(page.Prefixes, prefix)
			count++
			continue
		}
		if b.full(count) {
			more = true
			break
		}
		page.Objects = append(page.Objects, list.Object{Key: key})
		count++
	}
	page.Done = !more
	if b.mutate != nil {
		b.mutate(call, &page)
	}
	return page, nil
}

func (b *fakeBucket) full(count int) bool { return b.pageSize > 0 && count >= b.pageSize }

// sortedKeys returns the bucket keys for paths, sorted in byte order.
func sortedKeys(paths ...string) []string {
	keys := slices.Clone(paths)
	slices.Sort(keys)
	return keys
}

// modTime returns a fixed modification time.
func (s *FlatKeysTestSuite) modTime() time.Time {
	return time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
}
