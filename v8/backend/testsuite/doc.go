// Package testsuite provides shared conformance tests for [github.com/c2fo/vfs/v8] backends.
//
// Backends import this package from integration tests (typically behind the `vfsintegration`
// build tag), construct a [vfs.Location], and call [RunConformanceTests] and optionally
// [RunIOTests].
//
// Listing uses [vfs.Location.List] with [context.Context] and [iter.Seq2]; helpers such as
// [CollectList] materialize names for assertions. High-level copy/move operations from v7 live
// on [vfs.FileTransfers] in v8; this suite focuses on core [vfs.File], [vfs.Location], and
// [vfs.FileSystem] behavior plus optional scripted I/O when files implement [io.Seeker].
package testsuite
