/*
Package v7adapter wraps github.com/c2fo/vfs/v7 types so they satisfy the v8
github.com/c2fo/vfs/v8 interfaces for incremental migration.

Constructors

  - FileFromV7, LocationFromV7, FileSystemFromV7 wrap v7 values.
  - FileTransfersFromV7 exposes copy/move operations with context on top of a v7 file.

Options

  - NewFile and DeleteFile on wrapped locations and file systems accept v8 option
    values but do not translate them to v7 yet; the underlying v7 methods are called
    without extra v7 options. Use v7 APIs directly when backend-specific options are required.

Unwrapping

  - AsV7File, AsV7Location, and AsV7FileSystem recover v7 handles from adapter values.
*/
package v7adapter
