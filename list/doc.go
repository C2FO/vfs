/*
Package list streams the contents of a vfs.Location as vfs.Entry values, identically on every backend.

# Options

Listings are configured with options, each of which implements options.ListOption:

  - [Prefix], [Glob], [Match], [Filter] and [FilterContext] restrict which entries are yielded.
  - [Recursive], [MaxDepth], [DirFilter] and [DirFilterContext] control descent into subdirectories.
  - [FilesOnly] and [DirsOnly] restrict the kind of entry.
  - [StartAfter], [After] and [Limit] page through a listing.

Each option may be given at most once, and invalid combinations are rejected by [ParseQuery] with an error matching
[ErrInvalidOption] before any request is made. Options that this package does not define, such as backend-specific
options, are ignored.

# Ordering

Entries are yielded in ascending byte order of their order key (see [OrderKey]): the entry's relative path, with a
trailing "/" for directories. The same order holds on every backend, which makes [StartAfter] a portable cursor.

# Backend implementers

A backend that lists natively implements vfs.EntryLister and starts from [ParseQuery]. Every field of [Query] other
than Limit, Match and Filter is a hint the backend may use to avoid fetching entries that would be discarded.
Ignoring a hint is never a correctness bug, because the listing applies every option again to what a backend yields.
*/
package list
