# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Fixed
- A failed upload on `Close` keeps the written content so `Close` can be retried; `Delete` discards it.
- The upload-session chunk that reaches the end of the file must return the committed file, otherwise the file is looked up and `Close` fails unless it has the full size. A `WithChunkSize` above Graph's 60 MiB limit fails before the upload starts.
- A concurrently created folder name that turns out to be a file is no longer used as a parent folder.
- Authentication options are last-one-wins: applying device code or interactive browser after client credentials (or the reverse) no longer keeps the earlier credentials.
- Client credentials without `WithUserDrive`, `WithSiteDrive`, or `WithDriveID` fail with an error naming those options instead of a Graph `/me/drive` error.
- Copy and move onto an existing file fall back to a streamed overwrite when Graph reports `nameAlreadyExists`, as personal accounts do because they ignore `conflictBehavior=replace`.
- Access tokens are cached until five minutes before expiry, so credentials that spawn a process per token (Azure CLI) are not invoked on every request.

### Added
- Initial commit of the OneDrive community-contributed backend (`onedrive://`), backed by a small Microsoft Graph REST client over `net/http` with `azidentity` authentication (access token, client credentials, device code, interactive browser, default credential) and user, site, or drive selection.
- `WithBaseURL` and `WithHTTPClient` options for pointing the backend at proxies or a test server.
- Unit tests using mockery mocks of `types.Client`/`types.UploadSession`, WireMock testcontainers tests of the Graph protocol (`vfsintegration` tag), and a credentialed conformance test against a real drive.
