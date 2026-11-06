/*
Package iofs presents a [vfs.Location] as a read-only [io/fs.FS].

It lets any VFS backend be used wherever the standard library accepts a filesystem, such as [net/http.FileServerFS],
[html/template.ParseFS], [io/fs.WalkDir] and [io/fs.Glob]:

	loc, _ := vfssimple.NewLocation("s3://bucket/assets/")
	fsys := iofs.New(loc)

	http.Handle("/", http.FileServerFS(fsys))
	data, err := fs.ReadFile(fsys, "config/app.json")

The returned filesystem also implements [io/fs.StatFS], [io/fs.ReadDirFS], [io/fs.ReadFileFS] and [io/fs.SubFS]. It
needs no change to any backend, and it never writes.

# Names

Names follow [io/fs.ValidPath]: unrooted, slash-separated, without "." or ".." elements, with "." naming the location
the filesystem was built on. Invalid names fail with an error matching [io/fs.ErrInvalid] before any request is made.

# Files and directories

A name is resolved by listing its parent directory with [github.com/c2fo/vfs/v7/list.Entries], so the same rules hold
on every backend, including those with no real directories. When a name matches both a file and a directory, which
object stores allow, the file wins. Object stores cannot represent an empty directory: a directory exists only while
something is stored beneath it.

A symbolic link is reported as the listing reports it: with [io/fs.ModeSymlink] and without being followed, so a link
to a directory is not a directory here, unlike with [os.DirFS]. A location that does not exist cannot be told apart from
an empty one when it is opened as ".", so ReadDir on it returns no entries and no error; names beneath it do not exist.

# Cost

Opening or statting a name costs one listing request. Entries returned by ReadDir carry the size and modification time
the backend listed, so walking a tree and reading entry metadata needs no per-file request on backends that list
natively. On a backend that does not, the first Info call for each entry costs the requests Size and LastModified
make. Prefer ReadDir over many Stat calls when metadata for a whole directory is needed.

# Context

[io/fs.FS] methods take no context. Use [WithContext] to supply the one used for listing; it defaults to
[context.Background]. Retries are left to each backend.
*/
package iofs
