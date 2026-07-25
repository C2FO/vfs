# VFS testcontainers integration suite

This module runs the canonical VFS backend **conformance** and **IO** suites
(from [`backend/testsuite`](../backend/testsuite)) against real backend servers
spun up as disposable Docker containers via
[testcontainers-go](https://golang.testcontainers.org/).

Unlike the manual integration path in `backend/testsuite` (driven by
`VFS_INTEGRATION_LOCATIONS`), this module provisions the servers automatically,
so **no external credentials or pre-provisioned buckets are required**. It
contains no test logic of its own — it only stands up containers, registers the
backends, and delegates to the shared suites, so behavior is validated by a
single source of truth.

See [`doc.go`](./doc.go) for the package-level godoc.

## Requirements

- A running **Docker daemon**.
- The `vfsintegration` build tag (these tests do not run in the default unit
  suite).

## Running

From this directory:

```bash
go test -tags=vfsintegration -race -timeout 30m ./...
```

The whole suite (all backends, conformance + IO) typically finishes in well
under a minute once images are cached.

### Local notes

- **Ryuk / restricted Docker sockets:** testcontainers uses a "Ryuk" reaper
  container that bind-mounts the Docker socket. In sandboxed Docker setups
  (e.g. Colima) that mount can be disallowed. If Ryuk fails to start, disable it
  and rely on the suite's own `testcontainers.CleanupContainer` cleanup:

  ```bash
  TESTCONTAINERS_RYUK_DISABLED=true go test -tags=vfsintegration ./...
  ```

- **FTP passive ports:** the FTP container publishes a fixed passive-port range
  on identical host ports (passive FTP requires predictable, matching ports).
  A crashed prior run can leave a container holding those ports; remove stale
  containers if a run fails to provision.

## Backends and images

| Scheme | Container image | Notes |
|--------|-----------------|-------|
| `s3` | `minio/minio` | Registered twice: server-side encryption off and on |
| `azure` | `mcr.microsoft.com/azure-storage/azurite` | |
| `gs` | `fsouza/fake-gcs-server` | |
| `sftp` | `atmoz/sftp` | |
| `ftp` | `fauria/vsftpd` | `REVERSE_LOOKUP_ENABLE=NO` to avoid a reverse-DNS login stall |
| `mem`, `os` | *(none)* | Registered directly; exercise the suites without a container |

Exact image tags/digests are pinned in
[`containers_test.go`](./containers_test.go); bump them deliberately.

## CI

A dedicated `Integration (testcontainers)` workflow
([`.github/workflows/integration.yml`](../.github/workflows/integration.yml))
runs this module on every PR and on `main`, so backend conformance is validated
automatically without manual setup or credentials.

## This is not a published module

`testcontainers/` is test-only: all logic lives in `_test.go` files, so there is
nothing importable. Its separate `go.mod` exists purely to keep the heavy
`testcontainers-go`/docker dependency tree out of the core module. It has **no
`CHANGELOG.md` on purpose** — releasegen treats every directory with a changelog
as an independently released+tagged module, and this one is not released. Its
changes are recorded in the repository-root `CHANGELOG.md`.

## Adding a backend

The reusable contract is **`backend/testsuite`**, not this module. Any backend
that implements the VFS interfaces can be driven by `RunConformanceTests` /
`RunIOTests`. What differs is how you stand up something to test against:

1. **Backends with a containerizable server + a custom-endpoint option**
   (object stores, SFTP/FTP servers, DB-backed stores, …). Point the backend at
   the container and register it, then delegate to the suites — exactly what the
   `register*` helpers in [`containers_test.go`](./containers_test.go) do for the
   core backends.

2. **SaaS / HTTP-API backends with no local emulator** (e.g. Dropbox, and the
   in-progress OneDrive backend). These have no "server" image to run, but their
   REST APIs can be stubbed with a
   [WireMock](https://wiremock.org/) container
   (there is a WireMock testcontainers module and image). This works **only if
   the backend can be pointed at a custom base URL/endpoint** — a prerequisite
   the backend must expose in its options. With that in place, a WireMock
   container serving recorded/stubbed responses lets the same conformance suites
   run with no credentials.

### Where the test should live

- **Core backends** are wired here, in this module.
- **Contrib backends** should add their container/WireMock integration tests in
  **their own module** (importing `testcontainers-go` and `backend/testsuite`)
  rather than here. This keeps each backend's dependency tree isolated and avoids
  coupling the core test harness to every contrib backend. See
  [`contrib/backend/README.md`](../contrib/backend/README.md).

## Attribution

This module is derived from the community contribution in
[C2FO/vfs#294](https://github.com/C2FO/vfs/pull/294) ("Integration testing with
Testcontainers") by [Nathan Baulch](https://github.com/NathanBaulch). That
proposal was reviewed, reworked, and split into three focused changes:

- [#346](https://github.com/C2FO/vfs/pull/346) — the Azure/FTP backend bug fixes
  it depended on.
- [#347](https://github.com/C2FO/vfs/pull/347) — the shared conformance-suite
  improvements (`ConformanceOptions`, IO EOF handling).
- this module — the testcontainers integration itself.
