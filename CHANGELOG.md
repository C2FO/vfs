# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [5.2.3] - 2019-08-07
### Fixed
- The GS implementation of location.List() returned an empty string for files found inside a persistent "folder" object
created in the UI or by createing a zero-length object ending with "/".  These objects are now ignored.
- Fixed a gs-to-gs copy bug that became visible with integration tests aded in v5.2.2.

## [5.2.2] - 2019-07-17
### Fixed
- The OS implementation of file.MoveToLocation() now ensures that the target directory exists before attempting to move the file

## [5.2.1] - 2019-07-17
### Fixed
- Missing error check in utils.UpdateLastModifiedByMoving()
- Minor goreportcard checks failed. Now 100%

## [5.2.0] - 2019-07-16
### Added
- In-Memory backend vfs implementation
- Included the in-memory backend to the list of registered backends used in backend_integration_test
- Checking for existence at the start of various File functions (size, seek, etc) and returning and error if the file does not exist on the OS backend
- Tests in  backend_integration_test that ensure that operations done on non-existent files throw an error
- mem.md in vfs/docs, updated link to it in the README.md
### Fixed
- Relative path validation in utils.go now disallows empty names
### Changed
- utils_test.go now expects an empty string NOT to validate
- updated README to include "Touch()" definition under the File interface rather than the Location interface
- updated README to exclude "in-memory-backend" from the list of ideas


## [5.1.0] - 2019-07-08
### Added
- Added Touch() method to File interface and implemented in each backend.
- Increased test coverage in both os and s3 backends.

## [5.0.0] - 2019-06-18
### Changed
- Defined stricter definitions in interfaces, largely related to input requiements being more
URI compliant.
- FileSystem's NewFile and NewLocation required absolute paths.  Locations NewFile and NewLocation require relative paths.
- Added/Updated several new utils functions for URI validation across implementations.
- Ensure native methods are being used for atomic functions like copy or move/rename where possible
- Document expected behavior with empty readers in all copy and move function as well as io.Copy
- ListByPrefix now allows relative paths.
### Added
- Integration test suite for backened developers that ensures expected interface behaviors and tests interactions
between implementations.
- Examples of io.Copy usage in docs.
### Fixed
- Fixed [#10](https://github.com/C2FO/vfs/issues/10)
- Fixed vfssimple bug that would return os file system if the uri had the word 'file' anywhere in it even if was s3, like s3://bucket/path/to/file.txt
- Fixed bug where MoveToLocation was returning the old file with updated location rather
than a brand new file with new location.  Since File may represent a non-existent file, the original file should still be valid.

## [4.0.0] - 2019-05-22

### Release Notes

1. Added the Retry() method to the FileSystem interface, along with a no-op default retryer.
2. Integrated the newly defined retryer interface into the GCS implementation of VFS and now allow the retry method to be injected as a FileSystem option.
3. Integrated the request.Retryer from the s3 client as an injectable implementation for the S3 vfs.Option. 

### Upgrade steps

With v4.0.0, we introduced the ability to add a 'retry' option to each registered file system. This version bump will require an update to any existing FileSystem implementations. If a custom retryer is not required by the implemented file system, the default retryer can be returned.

```
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}
```

## [3.0.0] - 2019-04-22

Initial release for go modules compliant project.

## [2.1.4] - 2019-04-05
### Fixed
- Remove go modules from v2 since we don't prefix path with `/v2`.
  See github.com/c2fo/vfs/issues/14.
