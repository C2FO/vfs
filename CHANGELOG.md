# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
## [Unreleased]

## [6.0.1] - 2021-11-07
### Fixed
- fixed #103 Ensures that sftp backend closed underlying SSH connection on autodisconnect, not just SFTP subsystem.

## [6.0.0] - 2021-09-29
### Changed
- Modified sftp Key Exchange option to accept an array instead of a string, allowing multiple kex algorithms

### Upgrade steps

With v6.0.0, sftp.Options struct changed to accept an array of Key Exchange algorithms rather than a string. To update, change the syntax of the auth commands. 
```
"keyExchanges":"diffie-hellman-group-a256"
```
becomes 
```
"keyExchanges":["diffie-hellman-group-a256"]
```

## [5.10.0] - 2021-09-16
### Changed
- Modified S3 file.go so that on the initial read when the remote file is downloaded, a temporary file is locally created
using concurrent go routines to download parts of the file vs. a single request to download the whole object
### Fixed
- Fixed #100 Rolled back Seek validation before Copy or Move in SFTP backend due to bug on some SFTP servers and how we cache open "file handles"

## [5.9.0] - 2021-09-07
### Added
- Add support (and tests) for Go 1.17. Updated dependencies.
- Updated golanci.yml config.
### Fixed
- fixed broken backend integration test.

## [5.8.0] - 2021-08-24
### Fixed
- fixed #82 Return error when CopyTo/MoveTo functions are called when Seek offset is not (0,0) for all backends,
not just GCS.
- fixed #84 where sftp connections were never disconnected in a long-lived app.  Added Close to sftp backend Client interface.  Close client automatically 10 seconds (configurable in Options) after connecting unless reset by calling some server request action.
### Deprecated
- Deprecating utils.TouchCopy (#89).
### Added
- Added utils.TouchCopyBuffered(#89). TouchCopyBuffered uses a min buffer size of 256MB and allows for a
  custom buffer size to be set with filesystem options. Improves large file transfer with negligible impact on smaller file transfers.

## [5.7.0] - 2021-07-23
### Added
- Add support of keyexchanges algorithm as a sftp option

## [5.6.0] - 2021-07-19
### Fixed
- fixed #71 Listing a Google Cloud Storage bucket does not return things in the root of the bucket
- fixed #78 Return error when CopyTo/MoveTo functions are called when Seek offset is not (0,0) for GS
### Performance
- fixed #74 utils.TouchCopy() unnecessarily calls file.Size()

## [5.5.7] - 2021-05-12
### Fixed
- fixed: do not offset number of bytes read for memfs (#70) Jeffrey Larson <jeffothy@gmail.com>
- fixed S3 VFS backend throws an error when you open then close a file.  shivanigaurh-c2fo
- fixed minor markdown doc formatting of type headers
### Added
- Add Azure Blob Storage backend. Dusty McIlvain <dustin.mcilvain@c2fo.com>
- Add github actions for testing and linting (golangci-lint)

## [5.5.6] - 2021-02-07
### Fixed
- fixed failing test due to CI asking for mod tidy.
- fixed ineffectual assignment failures keeping vfs from achieving 100% in goreportcard.
- fixed #48 bug where 'invalid cross-device link' error occurs when renaming os files that are found on different devices/volumes including temp dirs.
- fixed #53 bug where s3 file tests were failing due to a deferred removal of a temp file.

### Security
- Upgraded dependencies.  Required regen of S3 API mock due to a couple new methods.
  SSH returns a different error  text since Go 1.14 (only affects 1 test).

## [5.5.5] - 2020-12-11
### Fixed
- fixes #49 - Update sftp.Options struct tags

## [5.5.4] - 2020-07-06
### Fixed
- fixes #21 - OS file.Location().ChangeDir(something/) should not affect location
- fixes #44 - make S3 waitUntilFileExists Exists error more descriptive of root error
- fixes #45 - fix ASCII diagram alignment for SFTP doc
### Added
- Improved test coverage for os.FileSystem

## [5.5.3] - 2020-05-11
### Fixed
- Addresses an issue where writes/touch calls on the vfs.File backend did not work properly on AWS-hosted SFTP environments. (See https://github.com/pkg/sftp/pull/310/files)

## [5.5.2] - 2020-04-23
### Fixed
- Ensure that writing truncates existing file. Fixes #40

## [5.5.1] - 2020-02-20
### Fixed
- Ensure that spaces (url encoded or not) in filenames and paths work as expected when copying (particularly for s3 to s3 native copyObject). Fixes #36.

## [5.5.0] - 2019-09-09
### Added
- SFTP backend vfs implementation

## [5.4.0] - 2019-08-24
### Added
- Added fallback to TouchCopy for S3/GCS implementations for CopyTo and MoveTo operations between files and locations which use different authentication and region configurations. When possible, the directory copy/move operations will be used by the underlying clients.

## [5.3.0] - 2019-08-24
### Added
- ACL can now be passed in as an s3.Option value. See 
https://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html#canned-acl for values.

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
