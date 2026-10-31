package list

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"slices"
	"strings"

	"github.com/c2fo/vfs/v7"
)

// maxStalledPages is how many consecutive pages without progress FlatKeys tolerates before it gives up, so a backend
// that keeps answering with the same keys cannot loop forever.
const maxStalledPages = 1000

// Object is one key of a flat key space, as an object store lists it.
type Object struct {
	// Key is the object's key relative to the listed location, with no leading slash. A key ending in "/" is a folder
	// marker and is folded into the directory it names. The empty key, the location's own marker, is ignored.
	Key string
	// Options supply the listed size and modification time, mode and native object. They are ignored for markers,
	// because directories never carry metadata.
	Options []vfs.EntryOption
}

// PageRequest describes the page FlatKeys needs next.
type PageRequest struct {
	// StartAfter is the key after which the page must start. A backend that keeps its own continuation token between
	// calls may follow the token instead, because FlatKeys always resumes immediately after the previous page.
	StartAfter string
	// Prefix is the relative path every wanted key starts with, or "".
	Prefix string
	// Delimited reports whether to roll keys up at the next "/" after Prefix and return the rolled-up directories as
	// Page.Prefixes. It is true for a non-recursive listing and false for a recursive one.
	Delimited bool
}

// Page is one page of a flat listing.
type Page struct {
	// Objects are the keys of the page, in ascending byte order.
	Objects []Object
	// Prefixes are the rolled-up directories of a delimited page, each ending in "/", in ascending byte order.
	Prefixes []string
	// Done reports that no page follows.
	Done bool
}

// PageFunc fetches the page described by req. FlatKeys calls it sequentially, never while the caller's loop body runs,
// and not again once the caller stops iterating. A location that does not exist is reported on the first call with an
// error matching fs.ErrNotExist.
type PageFunc func(ctx context.Context, req PageRequest) (Page, error)

// FlatKeys lists a flat key space, such as an object store, from sorted pages of keys. It is the engine behind the
// native listing of s3, gs, azure and sqlfs.
//
// Keys that arrive in byte order already are the order every listing guarantees, so nothing is buffered beyond one
// page. Directories are synthesized from the "/" separators of keys, once each, whether or not a folder marker exists,
// at the position of their order key. A key that is out of order ends the listing with an error, since a backend whose
// order differs from byte order would otherwise return silently wrong results. FlatKeys applies q.Prefix, q.StartAfter,
// q.Depth and, in a recursive listing, q.Descend itself, whatever the backend did with the hints. It does not apply
// Kind, Glob, Match, Filter or Limit; list.Entries does.
//
// A key that cannot be an Entry, such as "a//b", is yielded as a zero Entry with an error matching fs.ErrInvalid and
// the listing continues. A failed page ends the listing with an error and a zero Entry, except that a location that does
// not exist yields nothing. Each page request is wrapped in retry, which may be nil, and is not attempted once ctx is
// done.
func FlatKeys(ctx context.Context, parent vfs.Location, q Query, pages PageFunc, retry RetryFunc) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		f := &flat{
			emitter: emitter{ctx: ctx, parent: parent, yield: yield},
			win:     newWindow(q),
			descend: descender(q),
		}
		f.run(pages, retry, PageRequest{StartAfter: q.StartAfter, Prefix: q.Prefix, Delimited: !q.Recursive})
	}
}

type flat struct {
	emitter
	win     window
	descend EntryFunc

	stack     []string // relative paths of the open directories, outermost first
	skipUnder string   // relative path of a directory whose contents are being skipped
	last      string   // order key of the last item processed
	started   bool     // whether any item has been processed
}

type flatItem struct {
	key    string
	opts   []vfs.EntryOption
	prefix bool // came from Page.Prefixes
}

func (f *flat) run(pages PageFunc, retry RetryFunc, req PageRequest) {
	stalled := 0
	for first := true; ; first = false {
		if f.canceled() {
			return
		}
		var page Page
		err := attempt(f.ctx, retry, func() (err error) {
			page, err = pages(f.ctx, req)
			return err
		})
		if err != nil {
			f.pageFailed(err, first)
			return
		}

		items := mergePage(page)
		progressed := false
		for i := range items {
			advanced, ok := f.process(&items[i])
			if !ok {
				return
			}
			progressed = progressed || advanced
		}
		if page.Done {
			return
		}

		if stalled = nextStall(stalled, progressed); stalled > maxStalledPages {
			f.fail(vfs.Entry{}, listError(f.parent.Path(), errors.New("page requests made no progress")))
			return
		}
		req.StartAfter = resumeAfter(req, items)
	}
}

func nextStall(stalled int, progressed bool) int {
	if progressed {
		return 0
	}
	return stalled + 1
}

func (f *flat) pageFailed(err error, first bool) {
	if f.canceled() {
		return
	}
	if first && errors.Is(err, fs.ErrNotExist) {
		return
	}
	f.fail(vfs.Entry{}, listError(f.parent.Path(), err))
}

// mergePage returns the objects and rolled-up directories of a page as one sequence in key order.
func mergePage(p Page) []flatItem {
	items := make([]flatItem, 0, len(p.Objects)+len(p.Prefixes))
	for i := range p.Objects {
		items = append(items, flatItem{key: p.Objects[i].Key, opts: p.Objects[i].Options})
	}
	for _, prefix := range p.Prefixes {
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		items = append(items, flatItem{key: prefix, prefix: true})
	}
	slices.SortStableFunc(items, func(a, b flatItem) int { return strings.Compare(a.key, b.key) })
	return items
}

// pastDirectory sorts after every key inside a directory, and before any key that follows the directory, such as a
// sibling named like the directory plus a character above "/". It is the highest Unicode code point, so only a key
// with that exact character right after the slash could be skipped.
const pastDirectory = "\U0010FFFF"

// resumeAfter returns the key the next page starts after. After a rolled-up directory it is the directory's key
// plus pastDirectory, because StartAfter with the directory's own key would roll the same directory up again.
func resumeAfter(req PageRequest, items []flatItem) string {
	if len(items) == 0 {
		return req.StartAfter
	}
	lastItem := items[len(items)-1]
	next := lastItem.key
	if req.Delimited && lastItem.prefix {
		next += pastDirectory
	}
	if next < req.StartAfter {
		return req.StartAfter
	}
	return next
}

// process handles one item. advanced reports whether it sorted after every earlier item, and ok is false once the
// listing must end.
func (f *flat) process(it *flatItem) (advanced, ok bool) {
	key := it.key
	if f.started && key < f.last {
		f.fail(vfs.Entry{}, listError(key, fmt.Errorf("backend returned key %q after %q: keys must be in byte order", key, f.last)))
		return false, false
	}
	advanced = !f.started || key > f.last
	f.last, f.started = key, true
	if !advanced || key == "" || key <= f.win.after {
		return advanced, true
	}
	if f.skipUnder != "" {
		if strings.HasPrefix(key, f.skipUnder+"/") {
			return advanced, true
		}
		f.skipUnder = ""
	}

	dir := strings.HasSuffix(key, "/")
	rel := strings.TrimSuffix(key, "/")
	if !validKey(rel) {
		if f.win.wantsEntry(rel, dir) {
			return advanced, f.fail(vfs.Entry{}, invalidKey(key))
		}
		return advanced, true
	}
	return advanced, f.place(rel, dir, it.opts)
}

// validKey reports whether rel, a key without a trailing slash, is a valid relative path.
func validKey(rel string) bool {
	return rel != "." && fs.ValidPath(rel)
}

// place yields the directories implied by rel that have not been yielded yet, then the file at rel unless it is a
// directory. It returns false once the listing must end.
func (f *flat) place(rel string, dir bool, opts []vfs.EntryOption) bool {
	segs := strings.Split(rel, "/")
	nDirs := len(segs) - 1
	if dir {
		nDirs = len(segs)
	}
	for i := range nDirs {
		dirRel := strings.Join(segs[:i+1], "/")
		if i < len(f.stack) && f.stack[i] == dirRel {
			continue
		}
		f.stack = append(f.stack[:i], dirRel)
		if cont, descend := f.openDir(dirRel); !cont {
			return false
		} else if !descend {
			f.skipUnder = dirRel
			return true
		}
	}
	f.stack = f.stack[:nDirs]
	if dir || !f.win.wantsEntry(rel, false) {
		return true
	}
	e, err := vfs.NewFileEntry(f.parent, rel, opts...)
	if err != nil {
		return f.fail(vfs.Entry{}, listError(rel, err))
	}
	return f.entry(e)
}

// openDir yields the directory at rel if it is in the window and decides whether to look inside it. cont is false once
// the listing must end, and descend is false when nothing inside the directory is wanted.
func (f *flat) openDir(rel string) (cont, descend bool) {
	e, err := vfs.NewDirEntry(f.parent, rel)
	if err != nil {
		return f.fail(vfs.Entry{}, listError(rel, err)), false
	}
	if f.win.wantsEntry(rel, true) && !f.entry(e) {
		return false, false
	}
	if !f.win.opens(rel) {
		return true, false
	}
	if f.descend == nil {
		return true, true
	}
	keep, err := f.descend(f.ctx, e)
	if err != nil {
		f.fail(vfs.Entry{}, listError(e.Path(), err))
		return false, false
	}
	return true, keep
}
