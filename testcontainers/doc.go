/*
Package testcontainers runs the canonical VFS backend conformance and IO test
suites (from github.com/c2fo/vfs/v7/backend/testsuite) against real backend
servers spun up as disposable Docker containers.

Unlike the manual integration suite in backend/testsuite (driven by the
VFS_INTEGRATION_LOCATIONS environment variable), this module provisions the
servers automatically via testcontainers-go, so no external credentials or
pre-provisioned buckets are required.

The tests are gated behind the "vfsintegration" build tag and require a running
Docker daemon. They do not run as part of the default unit-test suite. Run them
explicitly with:

	go test -tags=vfsintegration ./...

Each supported backend is emulated by a well-known image:

	s3    -> minio/minio (registered twice: SSE-off and SSE-on)
	azure -> mcr.microsoft.com/azure-storage/azurite
	gs    -> fsouza/fake-gcs-server
	sftp  -> atmoz/sftp
	ftp   -> fauria/vsftpd

This module intentionally contains no test logic of its own: it only stands up
containers and delegates to the shared conformance suites so that backend
behavior is validated by a single source of truth.

# Attribution

This module is derived from the community contribution in C2FO/vfs#294
("Integration testing with Testcontainers") by Nathan Baulch
(https://github.com/NathanBaulch). That proposal was reviewed, reworked, and
split into three focused changes: the backend bug fixes it depended on
(C2FO/vfs#346), the shared conformance-suite improvements (C2FO/vfs#347), and
this testcontainers integration module.
*/
package testcontainers
