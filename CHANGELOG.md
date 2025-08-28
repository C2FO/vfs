# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Fixed
- Minor whitespace cleanup. 

## [v7.8.1] - 2025-08-07
### Fixed
- Fixed typed nil pointer bug in sftp backend.  Fixes #274.

## [v7.8.0] - 2025-08-06
### Added
- updated github actions workflows to support multi-modules.

## [v7.7.0] - 2025-08-05
### Added
- Updated changelog.sh to include checks for mono go repo
- Added .prenup.yml
### Fixed
- fix golangci-lint issues (for v3)

## [v7.6.0] - 2025-08-04
### Added
- Updated mockery config to be v3 compatible.

### Fixed
- Fixes #268. Allow memory filesystem file.Seek to seek to larger than size position.

## [v7.5.1] - 2025-07-24
### Fixed
- Update error wrapping functions to check for nil errors. Fixes #266.

## [v7.5.0] - 2025-07-08
### Added
- Add ability to suppress AWS SDK v2 "WARN Response has no supported checksum. Not validating response payload."
### Securtiy
- Updated dependencies and addressed dependabot issue #18 (CVE-2025-22868).

## [v7.4.1] - 2025-05-05
### Security
- Update dependencies to address a Cross-site Scripting vulnerability in golang.org/x/net. See [CVE-2025-12345](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2025-12345) for details.

## [v7.4.0] - 2025-04-25
### Added
- contrib/lockfile which provides a portable advisory locking mechanism for VFS files that works across different backends.
### Security
- Updated golangci-lint config (v2) and related gh action (v7).

## [v7.3.0] - 2025-04-15
### Added
- Add more error wrapping to public functions. Fixes #176.

### Fixed
- Remove redundant err check. Fixes #252.

### Security
- Update jwt/v5 to v5.2.2.

## [v7.2.0] - 2025-04-11
### Added
- Added Username field to SFTP and FTP Options structs with environment variable support (VFS_SFTP_USERNAME and VFS_FTP_USERNAME). Fixes #242.

## [v7.1.0] - 2025-03-19
### Added
- Add GetClient functions for each backend to allow for direct access to the underlying client.  Fixes #245.
- Added go-test-coverage action and remove codecove action since it hasn't been uploading for a while.

## [v7.0.1] - 2025-03-18
### Fixes
- [#243](https://github.com/C2FO/vfs/issues/243) - gs backend fails with cloud.google.com/go/storage v1.51.0 due to new wrapped errors.

## [v7.0.0] - 2025-03-17
### Added
- Deprecated usage of Volume and ChangeDir.  Added Authority (to replace Volume).  Fixes #235.
- Deprecate Retry() method in FileSystem interface and related types.
- Deprecate vfs.Option type in favor of specific backend options passed by NewFileSystemOption.
- Add NewFileSystemOption to allow for options, clients, etc to be passed to the FileSystem constructor.  Fixes #238.
- New README.md with logo

### Changed
- S3 backend now returns an s3.Client instead of an s3iface.ClientAPI. *Breaking Change*
- S3 backend s3.Option.Retry field is now an aws.Retryer instead of a (aws) request.Retry. *Breaking Change*
- Azure backend now uses the schema `az://` instead of `https://`. *Breaking Change*
- Azure backend authority is now the blob container name, rather than host + container name. See [README.md](README.md#azure-backend). *Breaking Change*
- GS backend removed use of vfs.Retry in favor of gs.Retryer, which is a more specific type and is now on the fs.Filesystem rather than gs.Options.  Now set with functional option gs.WithRetryer. *BreakingChange*

## [v7.0.0-pre5] - 2025-03-17
### Added
- Add NewFileSystemOption to allow for options, clients, etc to be passed to the FileSystem constructor.  Fixes #238.
- Deprecate Retry() method in FileSystem interface and related types.
- Deprecate vfs.Option type in favor of specific backend options passed by NewFileSystemOption.
- Removed gs backend use of vfs.Retry in favor of gs.Retryer, which is a more specific type and is now on the fs.Filesystem rather than gs.Options.

## [v7.0.0-pre4] - 2025-03-12
### Added
- Deprecated usage of Volume and ChangeDir.  Added Authority (to replace Volume).  Fixes #235.

## [v7.0.0-pre3] - 2025-03-09
### Changed
- Azure backend now uses the schema `az://` instead of `https://`. *Breaking Change*
- Azure backend authority is now the blob container name, rather than host + container name. See [README.md](README.md#azure-backend). *Breaking Change*

## [v7.0.0-pre2] - 2025-03-08
### Fixed
- Fixed go.mod and paths to reflect v7

## [v7.0.0-pre1] - 2025-03-07
### Changed
- S3 backend now returns an s3.Client instead of an s3iface.ClientAPI. *Breaking Change*
- S3 backend s3.Option.Retry field is now an aws.Retryer instead of a (aws) request.Retry. *Breaking Change*

### Security
- S3 backend now uses the `aws-sdk-go-v2` library instead of the deprecated `aws-sdk-go`.
- Updated dependencies.

## [v6.26.0] - 2025-02-19
### Added
- Add support for role based authentication in s3 backend.

### Security
- Update dependencies.
- Update go from 1.22 to 1.23 (since 1.24 is out now).

### Documentation
- Update README.md with go version policy.

## [6.25.1] - 2025-01-09
### Fixed
- The intersection of features #208 and #213 was missed (during rebase).

## [6.25.0] - 2025-01-08
### Added
- Added NewFile option for Content-Type.
- Windows support in the os backend.
### Fixed
- Ability to run all unit tests on Windows.
- Deprecated delete.WithDeleteAllVersions in favor of delete.WithAllVersions.
### Security
- Switched to new Azure SDK (from deprecated github.com/Azure/azure-storage-blob-go)
- Updated dependencies

## [6.24.0] - 2024-12-16
### Security
- updated dependencies

## [6.23.0] - 2024-12-06
### Security
- updated dependencies

## [6.22.0] - 2024-11-06
### Fixed
- [#214](https://github.com/C2FO/vfs/issues/214) Fix issue where s3 backend didn't reset `readEOFSeen` flag when resetting the file cursor during Seek operations.

## [6.21.0] - 2024-11-04
### Fixed
- Unit Test improvements: report underlying unit tests errors, always run test cases in a sub-test, always use test suite functions, use more specific assert functions where possible.
- General spelling and grammar corrections everywhere.
- Normalize whitespace: Removed trailing whitespace and superfluous new lines.
- Ensure all mocked types are included in mockery configuration and regenerate.

## [6.20.0] - 2024-10-15
### Added
- Add IncludeInsecureCiphers option to FTP backend.  Fixes #204.

## [6.19.0] - 2024-09-13
### Added
- Add ability to set file permissions after writing an SFTP file. Resolves #202.

## [6.18.0] - 2024-09-12
### Added
- Updated mocks to use mockery Expecter.  Resolves #200.

## [6.17.0] - 2024-09-10
### Added
- Added additionally-supported HostKeyAlgorithms for SFTP backend.  Resolves #198.

## [6.16.0] - 2024-08-24
### Security
- Fixes #196 - Update to latest go and dependencies
  - Update .gvmrc to 1.23.0.
  - Update go.mod min version to Go 1.23.
  - Update actions to test only supported Go versions (1.22, 1.23).
  - Updated dependencies.
  - Update golangci-lint.yml for latest golang-ci-lint version.

## [6.15.1] - 2024-06-18
### Added
- Fixed #193 - Correct method signature for EncodeAuthority to not return an error.
- Updated dependencies.

## [6.15.0] - 2024-06-18
### Added
- Fixed #191 - Add a util functions to encode authority strictly following RFC 3986.

## [6.14.3] - 2024-06-11
### Fixed
- Fixed #189 - Update utils authority package to handle proper encoding/decoding of uri with reserved characters.

## [6.14.2] - 2024-05-30
### Fixed
- Fixed #187 - Update to latest jlaffaye/ftp library to fix issue where FTPS connections were failing due to a bug in the library. Also updated dataconn to continue even if it fails to MakeDir.

## [6.14.1] - 2024-05-28
### Fixed
- Fixed #185 - location.Exists was checking if a list entry was a directory but it was only checking the first entry.

## [6.14.0] - 2024-05-15
### Security
- updated dependencies

## [6.13.2] - 2024-05-15
### Fixed
- Fixed #182 - location.Exists was checking the wrong dir for existence.  Now it checks the file's parent dir for the subdir's existence.
- Corrected where file.Close() occurs then file.Location().NewFile("somefile.txt") but was not respecting the connection reset. Now it does.

## [6.13.1] - 2024-04-24
### Fixed
- Fixed #180 - SFTP backend now only opens files for W (instead of RW) except after read/seek. This should fix issues where server permission are write only.

## [6.13.0] - 2024-03-09
### Fixed
- Fixed #177 - SFTP MoveToFile fails if destination file already exists
### Added
- Updated all backends to use error wrapping for Read, Write, Close, Seek. #174
- Update GS backend to comply with new io integration test suite. #172

## [6.12.1] - 2024-02-23
### Fixed
- Fixed #170 - Fix bug where mem backend doesn't remove fs entries when location.DeleteFile is called

## [6.12.0] - 2024-02-23
### Added
- Fixed #167 - Update CopyToFile to defer file close so cursor is reset.
- Fixes #146 - Update S3 backend to comply with new io integration test suite. Update to no longer write to memory.
- Fixes #156 - Update os backend to comply with new io integration test suite
- Fixes #160 - Update mem backend to comply with new io integration test suite
### Security
- Fixes #165 - Update to latest go and dependencies
  - Update .gvmrc to 1.22.0.
  - Update go.mod min version to Go 1.22.
  - Update actions to test only supported Go versions (1.21, 1.22)
  - Updated dependencies (ie, Google Cloud, Azure, AWS sdks)

## [6.11.4] - 2024-03-08
### Fixed
- Fixed #177 - SFTP MoveToFile fails if destination file already exists

## [6.11.3] - 2024-02-02
### Fixed
- Fixed #158 bug. Updated sftp backend to fix issue where some servers return a generic error message when a file is opened for RW on Read().

## [6.11.2] - 2024-01-30
### Fixed
- Fixed #154 bug. Updated sftp backend to overwrite except after read/seek.

## [6.11.1] - 2024-01-22
### Fixed
- Fixed #152 bug where s3 backend failed to read empty files

## [6.11.0] - 2024-01-22
### Added
- Added support for hmac-sha1 and hmac-sha1-96 and removed hmac-ripemd160

## [6.10.0] - 2023-12-28
### Fixed
- fixed #145 - Remove use of local temp file when reading/seeking from s3 files.  This should improve performance by allowing streaming reads from s3 files.
### Added
- fixes #149 - Add overridable default HostKeyAlgorithms, Ciphers, MACs, and KeyExchanges

## [6.9.1] - 2023-11-21
### Fixed
- fixed sftp backend to consider configured ports other than 22

## [6.9.0] - 2023-09-27

### Security
- Update .gvmrc to 1.21.1. Fixes #143.
- Update go.mod min version to Go 1.20.
- Update actions to test only supported Go versions (1.20, 1.21)
- Updated dependencies (ie, Google Cloud, Azure, AWS sdks)

## [6.8.0] - 2023-07-29
### Added
- ftp backend to support connection via FTP, FTPS (implicit TLS), and FTPES (explicit TLS).

## [6.7.0] - 2023-05-13

### Security
- Update go.mod min version to Go 1.18. Fixes #139.
- Update actions to test only supported Go versions (1.19, 1.20)
- Updated dependencies (ie, Google Cloud, Azure, AWS sdks)

## [6.6.0] - 2022-10-25
### Fixed
- fixed #129(cdd4143) Resolve failing golangci-lint - elliotwutingfeng
- fixed #133 Replaced usage of deprecated ioutil functions with os and io equivalents.

### Security
- Update go.mod min version to Go 1.18. Fixes #131.
- Update actions to test only supported Go versions (1.18, 1.19)
- Updated dependencies (ie, Google Cloud, Azure, AWS sdks)

## [6.5.2] - 2022-07-08
### Fixed
- fixed edge cases where in-memory files either weren't found when they should be, or didn't reset state on new writes

### Fixed
- Replaced pre-generated ssh keypair in sftp options tests with auto-generated pair to remediate false-positive security alerts for hard-coded credentials.

## [6.5.1] - 2022-07-08

### Fixed
- Fixed the azure deleteAllVersions to fetch versions using file path instead of location path

## [6.5.0] - 2022-07-08

### Added
- Add DeleteAllVersions option support for Azure to remove all versions of a file

### Fixed
- Fixed golangci-lint issues
- Fixed #90 - integration were tests failing on move for mem-to-mem move and bad mutex unlock call when doing mem to non-mem move

## [6.4.0] - 2022-06-27

### Added
- Add DeleteAllVersions option support for S3 to remove all versions of a file
- Add DeleteAllVersions option support for GS to remove all versions of a file

### Security
- Update dependencies
- Update .gvmrc

## [6.3.0] - 2022-05-16
### Security
- Update go.mod min version to Go 1.17
- Update actions/setup-go action to v3
- Update actions/checkout action to v3
- Update golangci/golangci-lint-action action to v3
- Update actions to test only supported Go versions (1.17, 1.18)
- Updated dependencies (ie, Google Cloud, Azure, AWS sdks)

## [6.2.0] - 2022-04-28
### Added
- Add S3 option ForcePathStyle.  Fixes #96
- Add S3 option DisableServerSideEncryption. Fixes #98

## [6.1.0] - 2022-02-18
### Added
- Added utils.PathToURI() func to change path to OS URI.  resolves #112

## [6.0.2] - 2021-12-03
### Fixed
- fixed linting issues with missing godoc on exported functions and new build tag formatting.
- fixed #92 (broken by #72) where calling ListByPrefix() was fail from non-root locations when calling file-level prefixes.
- fixed azure helper func for vfssimple, ensuring it works on File URIs in addition to Location URIs
- fixed #97 by updating vfssimple logic to ensure the most specific registered backend that matches a url is used, not just the first one it comes across.  Updated vfssimple docs.
- Added vfssimple tests.  Zero to 100% coverage.
- Fix codecov validation which got lost when converting to github actions.  Removed .travis.yml.

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
- Updated golangci.yml config.
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
created in the UI or by creating a zero-length object ending with "/".  These objects are now ignored.
- Fixed a gs-to-gs copy bug that became visible with integration tests added in v5.2.2.

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
- Defined stricter definitions in interfaces, largely related to input requirements being more
URI compliant.
- FileSystem's NewFile and NewLocation required absolute paths.  Locations NewFile and NewLocation require relative paths.
- Added/Updated several new utils functions for URI validation across implementations.
- Ensure native methods are being used for atomic functions like copy or move/rename where possible
- Document expected behavior with empty readers in all copy and move function as well as io.Copy
- ListByPrefix now allows relative paths.
### Added
- Integration test suite for backend developers that ensures expected interface behaviors and tests interactions
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
