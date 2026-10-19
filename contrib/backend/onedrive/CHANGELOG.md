# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Initial commit of the OneDrive community-contributed backend (`onedrive://`), backed by a small Microsoft Graph REST client over `net/http` with `azidentity` authentication (access token, client credentials, device code, interactive browser, default credential) and user, site, or drive selection.
- `WithBaseURL` and `WithHTTPClient` options for pointing the backend at national clouds, proxies, or a test server.
- Unit tests using mockery mocks of `types.Client`/`types.UploadSession`, WireMock testcontainers tests of the Graph protocol (`vfsintegration` tag), and a credentialed conformance test against a real drive.
