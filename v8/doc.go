/*
Package vfs defines composable interfaces for virtual file systems used across
object stores, POSIX paths, SFTP, and in-memory implementations.

# Layers

  - [FileSystem] constructs [File] and [Location] handles from scheme, authority, and path.
  - [FileIdentity] and [LocationIdentity] model stable URIs without implying I/O.
  - [File] combines streaming ([io.Reader], [io.Writer], [io.Closer]) with [FileMeta].
  - Optional I/O: use type assertions to [io.Seeker], [FileReaderAt], and [FileWriterAt].
  - [Location] embeds [Lister] for unified listing via iterators (see below).

# Listing

[Lister.List] uses functional options ([WithPrefix], [WithRegexp], [WithRecursive], [WithPageSize])
instead of separate List/ListByPrefix/ListByRegex methods from earlier versions.

Each yield is an [Entry] with [EntryKind] describing whether the backend could classify
the name as file-like, directory-like, or unknown flat keys.

# Transfers

[Copy] provides a streaming helper. Rename, move, and server-side copy should use
backend-native APIs when available; [FileTransfers] documents the contextual shape
for implementations that expose copy/move as methods.

# POSIX alignment

Paths are absolute and cleaned per backend rules; URIs remain the portable interchange.
When semantics differ (for example object prefixes versus directories), backends document
behavior. [ErrNotSupported] signals missing capabilities.

# io/fs

[Entry.DirEntry] offers a lightweight [fs.DirEntry] view for interoperability with
[fs.WalkDir] helpers; it is not a substitute for mutating operations on [fs.FS].
*/
package vfs
