package list_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/options"
)

// fuzzNames is deliberately full of names whose order depends on how "/" compares with their neighbors.
var fuzzNames = []string{"a", "a-b", "a.b", "a0", "b", "ab", "x", "x0", "z"}

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

// fuzzReference computes the expected order keys without either engine: every entry in the tree, sorted, then
// narrowed by the window.
func fuzzReference(paths []string, q list.Query) []string {
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

	var out []string
	for key := range set {
		rel := strings.TrimSuffix(key, "/")
		inDepth := q.Depth() == 0 || strings.Count(rel, "/") < q.Depth()
		if inDepth && strings.HasPrefix(rel, q.Prefix) && key > q.StartAfter {
			out = append(out, key)
		}
	}
	slices.Sort(out)
	return out
}

func fuzzOptions(data []byte, paths []string) []options.ListOption {
	if len(data) < 4 || len(paths) == 0 {
		return nil
	}
	var opts []options.ListOption
	recursive := data[0]&1 != 0
	if recursive {
		opts = append(opts, list.Recursive())
		if depth := int(data[1] % 4); depth > 0 {
			opts = append(opts, list.MaxDepth(depth))
		}
	}
	pick := paths[int(data[2])%len(paths)]
	plain := strings.TrimSuffix(pick, "/")
	if data[0]&2 != 0 {
		opts = append(opts, list.Prefix(plain[:int(data[3])%(len(plain)+1)]))
	}
	if data[0]&4 != 0 {
		opts = append(opts, list.StartAfter(pick[:int(data[3])%(len(pick)+1)]))
	}
	return opts
}

// FuzzEnginesAgree checks that WalkDirs, FlatKeys at any page size, and an independent computation list the same
// entries in the same order for any tree and any window.
func FuzzEnginesAgree(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09})
	f.Add([]byte{0x85, 0x00, 0x01, 0x02, 0x03, 0x04, 0x91, 0x02, 0x00, 0x01, 0x03, 0x01})
	f.Add([]byte{0x03, 0x01, 0x00, 0x05, 0x40, 0x01, 0x01, 0x01, 0x01, 0x82, 0x04, 0x03, 0x02})
	f.Add([]byte{0x07, 0x02, 0x05, 0x02, 0x10, 0x20, 0x30, 0x41, 0x52, 0x63, 0x74, 0x85, 0x96, 0xa7, 0xb8})

	f.Fuzz(func(t *testing.T, data []byte) {
		paths := fuzzPaths(data)
		q, err := list.ParseQuery(fuzzOptions(data, paths))
		if err != nil {
			t.Skip()
		}
		expected := fuzzReference(paths, q)

		loc := mocks.NewLocation(t)
		loc.EXPECT().Path().Return("/root/").Maybe()

		dirs := &fakeDirs{tree: treeFromPaths(paths...)}
		walked := drain(list.WalkDirs(t.Context(), loc, q, dirs.readDir, nil))
		require.Empty(t, errsOf(walked))
		require.Equal(t, nonNil(expected), nonNil(keysOf(walked)), "WalkDirs, paths %q", paths)

		keys := slices.Clone(paths)
		slices.Sort(keys)
		for _, pageSize := range []int{0, 1, 2, 5} {
			bucket := &fakeBucket{keys: keys, pageSize: pageSize}
			flat := drain(list.FlatKeys(t.Context(), loc, q, bucket.pages, nil))
			require.Empty(t, errsOf(flat))
			require.Equal(t, nonNil(expected), nonNil(keysOf(flat)), "FlatKeys page size %d, paths %q", pageSize, paths)
		}
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
