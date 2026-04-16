/*
Package os provides a [vfs.FileSystem] implementation backed by the host operating system's
file APIs ([os] package, path semantics).

Construct a [FileSystem] with [NewFileSystem] and optional [WithTempDir] for temporary files
during writes (same rationale as the v7 OS backend).

Typical usage:

	fs := os.NewFileSystem()
	loc, err := fs.NewLocation("", "/var/tmp/")
	f, err := fs.NewFile("", "/var/tmp/example.txt")

URIs use scheme "file" per RFC 8089; listing uses [vfs.Location.List] with [context.Context].

See also the [v8 module README](../../README.md) and [backend index](../README.md).
*/
package os
