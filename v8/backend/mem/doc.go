/*
Package mem provides an in-memory implementation of [github.com/c2fo/vfs/v8] interfaces
for testing and ephemeral storage.

Behavior matches the v7 memory backend in spirit: a file does not appear in [vfs.File.Exists]
until it is written, [vfs.File.Touch] is called, or similar; [vfs.Location.List] returns
base names of files at that location.

See [README](README.md) for usage.
*/
package mem
