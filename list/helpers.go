package list

import (
	"cmp"
	"container/heap"
	"iter"
	"slices"
	"strings"

	"github.com/c2fo/vfs/v7"
)

// Collect returns every entry seq yields, in order. It stops at the first error and returns it with no entries, so a
// caller that wants the entries a failed listing did produce, such as for an access report, ranges over the sequence
// instead. Collect holds the whole listing in memory.
func Collect(seq iter.Seq2[vfs.Entry, error]) ([]vfs.Entry, error) {
	var out []vfs.Entry
	for e, err := range seq {
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// TopN returns the n entries of seq that sort first under compare, in that order, keeping only n entries in memory however
// long the listing is. Entries that compare equal keep the order in which seq yielded them. To find the largest files,
// pass a comparator that sorts larger sizes first, such as one that swaps the arguments of [BySize]. TopN stops at the
// first error and returns it with no entries. It returns an error matching [ErrInvalidOption] if n is below 1 or compare
// is nil, without ranging over seq.
func TopN(seq iter.Seq2[vfs.Entry, error], n int, compare func(a, b vfs.Entry) int) ([]vfs.Entry, error) {
	if n < 1 {
		return nil, invalidf("TopN count must be at least 1, got %d", n)
	}
	if compare == nil {
		return nil, invalidf("TopN comparator is nil")
	}

	h := &worstFirst{cmp: compare}
	arrival := 0
	for e, err := range seq {
		if err != nil {
			return nil, err
		}
		r := ranked{entry: e, arrival: arrival}
		arrival++
		switch {
		case len(h.items) < n:
			heap.Push(h, r)
		case h.worse(h.items[0], r):
			h.items[0] = r
			heap.Fix(h, 0)
		}
	}

	slices.SortFunc(h.items, func(a, b ranked) int {
		if c := compare(a.entry, b.entry); c != 0 {
			return c
		}
		return a.arrival - b.arrival
	})
	out := make([]vfs.Entry, len(h.items))
	for i, r := range h.items {
		out[i] = r.entry
	}
	return out, nil
}

type ranked struct {
	entry   vfs.Entry
	arrival int
}

// worstFirst is a heap with the entry that sorts last at the root, so the root is the one to replace.
type worstFirst struct {
	items []ranked
	cmp   func(a, b vfs.Entry) int
}

// worse reports whether a sorts after b, with later arrivals sorting after earlier ones.
func (h *worstFirst) worse(a, b ranked) bool {
	if c := h.cmp(a.entry, b.entry); c != 0 {
		return c > 0
	}
	return a.arrival > b.arrival
}

func (h *worstFirst) Len() int           { return len(h.items) }
func (h *worstFirst) Less(i, j int) bool { return h.worse(h.items[i], h.items[j]) }
func (h *worstFirst) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *worstFirst) Push(x any)         { h.items = append(h.items, x.(ranked)) } //nolint:forcetypeassert // only ranked is pushed
func (h *worstFirst) Pop() any {
	last := h.items[len(h.items)-1]
	h.items = h.items[:len(h.items)-1]
	return last
}

// ByName compares entries by Name, then by relative path so that entries with the same name in different directories
// have a stable order. It is a comparator for slices.SortFunc and [TopN]; swap the arguments to reverse it.
func ByName(a, b vfs.Entry) int {
	if c := strings.Compare(a.Name(), b.Name()); c != 0 {
		return c
	}
	return strings.Compare(a.RelPath(), b.RelPath())
}

// BySize compares entries by their listed Size. Entries without metadata (see vfs.Entry.HasMetadata) have size 0. It is
// a comparator for slices.SortFunc and [TopN]; swap the arguments to reverse it.
func BySize(a, b vfs.Entry) int { return cmp.Compare(a.Size(), b.Size()) }

// ByModTime compares entries by their listed ModTime. Entries without metadata have the zero time. It is a comparator
// for slices.SortFunc and [TopN]; swap the arguments to reverse it.
func ByModTime(a, b vfs.Entry) int { return a.ModTime().Compare(b.ModTime()) }
