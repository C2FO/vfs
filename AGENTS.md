# Agent Guidelines for VFS Repository

This document provides guidelines for AI agents working on the VFS repository, including upgrade policies, dependency management, and maintenance procedures.

## Table of Contents
- [Open Source Hygiene](#open-source-hygiene)
- [Development Guidelines](#development-guidelines)
- [Integration Testing](#integration-testing)
- [CHANGELOG and PR Process](#changelog-and-pr-process)
- [Pull Requests and Code Review](#pull-requests-and-code-review)
- [Go Version Policy](#go-version-policy)
- [Dependency Upgrades](#dependency-upgrades)
- [GitHub Actions Maintenance](#github-actions-maintenance)
- [Module Management](#module-management)
- [CI/CD Configuration](#cicd-configuration)

---

## Open Source Hygiene

VFS is a public, open source project.

- **Never reference internal company details** in code, comments, commit messages, PR descriptions, or issues. This includes internal ticket IDs (e.g. Jira keys), internal service names, private repositories, bucket names, hostnames, and customer names.
- Describe the problem in terms of VFS behavior instead. If the work came from an internal ticket, open a GitHub issue and reference that.
- **Do not add AI attribution** anywhere: no `Co-authored-by:` trailers for AI tools, no "Generated with ..." lines, and no AI mentions in commits, PR descriptions, issues, code comments, or docs.
- Do not commit local planning or tooling artifacts such as `openspec/`, `graphify-out/`, scratch plan documents, or editor/agent config directories.

---

## Development Guidelines

### Testing Requirements

#### Mandatory Test Coverage
- **All new code MUST have tests**
- Aim for coverage as close to 100% as is practicable
- Minimum thresholds (enforced by CI via `.testcoverage.yml`):
  - **Total project coverage:** 80%
  - **Package coverage:** 63%
  - **File coverage:** 52%
- Thresholds only ratchet up. Never lower one to make CI pass; add tests instead.

#### Test Organization
- **Use `testify/suite` for organizing related tests**
  - One test suite per major component/struct being tested
  - Naming: `[ComponentName]TestSuite` (e.g., `S3BackendTestSuite`)
  - Use `SetupTest()` and `TearDownTest()` for test isolation
  - **Exception:** testify/suite may not be suitable for benchmarks or simple unit tests

#### Test Style
- **Prefer suite functions and facilities over external libraries**
  - Use suite's built-in methods for temp files, fixtures, etc.
  - Use `s.Assert()` and `s.Require()` from suite rather than standalone packages
  - Keep test dependencies minimal

- **Use table-driven tests where possible**
  - Slice of anonymous structs with `name`, inputs, and expected outputs
  - Use descriptive test case names that explain the scenario
  - Example:
    ```go
    tests := []struct {
        name           string
        input          string
        expectedOutput string
        expectedError  string
    }{
        {
            name:           "Success case - normal operation",
            input:          "valid",
            expectedOutput: "result",
        },
        {
            name:          "Error case - invalid input",
            input:         "invalid",
            expectedError: "expected error message",
        },
    }

    for _, tt := range tests {
        s.Run(tt.name, func() {
            result, err := functionUnderTest(tt.input)
            if tt.expectedError != "" {
                s.Require().Error(err)
                s.Assert().Contains(err.Error(), tt.expectedError)
            } else {
                s.Require().NoError(err)
                s.Assert().Equal(tt.expectedOutput, result)
            }
        })
    }
    ```

#### Race Detection
- CI runs `go test -race ./...` for every module. Run it locally before pushing concurrency-related changes.

#### Cross-Platform Behavior
CI runs on Linux, macOS, and Windows. Common Windows pitfalls:
- `file://` URIs for drive-letter paths have the form `file:///C:/path/...`. Convert between URIs and native paths explicitly rather than stripping or prepending `file://`.
- `-race` requires cgo, and cgo on Windows needs a valid, writable `TEMP`/`TMP`. The Windows TMPDIR step in `go.yml` must use PowerShell syntax when writing to `$GITHUB_ENV`.

### Code Quality

#### Linting
- **All code MUST pass `golangci-lint`**
- Run before committing: `make lint` (runs every module with `--build-tags=vfsintegration`)
- Configuration: `.golangci.yml`
- Linter runs on all modules independently in CI
- `nolintlint` is enabled: only add a `//nolint:<linter>` directive when that linter actually reports the line, and always include a reason

#### Pre-commit Hook
- `.prenup.yaml` configures [prenup](https://github.com/c2fo/prenup) to run tests, `golangci-lint`, and `releasegen validate` on the modules touched by a commit.
- The `releasegen validate` step can't open a linked git worktree (`repository does not exist`). When committing from a worktree, use `git commit --no-verify`; the CHANGELOG is still validated in CI.

#### Code Style
- Follow standard Go idioms and conventions
- Use `gofmt` and `goimports` for formatting
- Exported functions and types must have documentation comments
- Keep functions focused and small
- Use early returns to reduce nesting

#### Error Handling
- Handle all errors explicitly - no silent failures
- Use wrapped errors with context: `fmt.Errorf("operation failed: %w", err)`
- Provide meaningful error messages with context
- Never ignore error return values

### API and Interface Design
- Prefer small, focused interfaces over large ones
- Define interfaces in consuming packages, not implementing packages
- **Inject dependencies through functional options** on the backend constructor (e.g. `NewFileSystem(opts...)`), not through replaceable package-level globals
- Keep option patterns uniform across backends. General options apply to every backend; backend-specific options live in that backend's package.
- Do not force a specific logging library on users. Accept a minimal interface or callback instead.
- Avoid breaking changes to the v7 public API. When a breaking change is unavoidable, it must be planned and called out as a `BREAKING CHANGE` in the CHANGELOG.

### File Organization
- Group related functionality in packages
- Use subdirectories for different implementations
- Place mocks in `mocks/` subdirectories
- One package per major feature

### Mocking
- **Use `mockery` v3 for generating mocks with EXPECT() pattern**
- **No manual mocks or fakes** (unless absolutely necessary; see [Integration Testing](#integration-testing) for stateful HTTP fakes)
- **Prefer EXPECT() pattern for mock setup** for better readability and type safety
- Mocks are configured in `.mockery.yaml` and generated into `mocks/` subdirectories. Add new interfaces to `.mockery.yaml` and run `mockery`.
- Example EXPECT() usage:
  ```go
  mockClient := mocks.NewMockSQSClient(t)
  mockClient.EXPECT().ReceiveMessage(mock.Anything, mock.MatchedBy(func(input *sqs.ReceiveMessageInput) bool {
      return *input.QueueUrl == queueURL
  })).Return(&sqs.ReceiveMessageOutput{Messages: messages}, nil)
  ```

### Documentation
- All exported types, functions, and methods must have godoc comments
- A package doc comment must sit directly above the `package` clause, or it won't attach to the package on pkg.go.dev
- Include usage examples for complex functionality
- Update README.md when adding new features
- A new backend needs its own `README.md`, plus a link from the root `README.md` (and from `contrib/backend/README.md` for contrib backends)
- Document breaking changes in CHANGELOG.md

#### Generated API Docs
- The core module's `docs/*.md` API pages are generated from godoc with [gomarkdoc](https://github.com/princjef/gomarkdoc). The package-to-page mapping is `DOC_PAGES` in the `Makefile`.
- After changing doc comments or exported symbols, run `make docs` and commit the result. The `Docs` workflow runs `make docs-check` and fails the PR when a page is out of date.
- Never hand-edit content between the `<!-- gomarkdoc:embed:start -->` and `<!-- gomarkdoc:embed:end -->` markers. Text outside the markers is preserved, and `docs/conformance_tests.md` is fully hand-written.
- `.gomarkdoc.yml` and the template overrides in `.gomarkdoc/` drop source-line links so the output doesn't change whenever code moves. The overrides are copies of the templates from the pinned gomarkdoc version.
- Contrib modules are documented by their READMEs, not by generated pages.

### Resource Management
- Use context for cancellation throughout
- Prevent resource leaks with proper cleanup (use `defer`)
- Be mindful of goroutine lifecycle and synchronization

---

## Integration Testing

### Build Tag
- Integration tests use the build tag `vfsintegration`
- Run with: `go test -tags=vfsintegration ./...`
- Tests that need credentials read them from environment variables and **skip** when they are not set

### Conformance Suites
- `backend/testsuite` provides shared conformance and IO conformance suites. Every backend, core or contrib, should run them.
- See `docs/conformance_tests.md` for details.

### testcontainers
- The `testcontainers/` module runs the conformance suites against real servers in disposable Docker containers. It runs in `integration.yml` (Linux only) on PRs and nightly.
- Prefer testcontainers for any backend with a self-hostable server (S3-compatible, SFTP, FTP, Azure, GCS emulators, PostgreSQL, etc.).
- SaaS/HTTP-API backends (Dropbox, OneDrive, Google Drive) should use a stateful in-process fake for conformance; use WireMock or recorded stubs only for stateless error paths. See `contrib/backend/README.md` and `testcontainers/README.md`.
- Pin container images by digest and apply the same [10-day rule](#update-policy) used for GitHub Actions.
- Locally, Docker is provided by Colima. Make sure it is running before running these tests.

### Real-Service Tests
- Tests against real accounts (e.g. `VFS_INTEGRATION_LOCATIONS="s3://bucket/path/"`, OneDrive, Google Drive) are run manually with credentials.
- When a test uses interactive auth such as a device-code flow, run it so the sign-in prompt is visible (do not pipe output through `grep` or similar), and tell the user the code/URL they need.

---

## CHANGELOG and PR Process

### CHANGELOG Requirements

#### All PRs MUST Update CHANGELOG.md
- **Every PR requires an update to the `## [Unreleased]` section** of the affected module's CHANGELOG.md, including CI-only and docs-only changes
- `releasegen validate` enforces this in CI and in the pre-commit hook (`require_changelog_entry` in `.releasegen.yaml`)
- PRs are **always submitted without a new version number**
- Version numbers are added automatically when the PR is merged to main
- Add entries **inside** `## [Unreleased]`, directly under that heading, never below an already-released version

#### Per-Module CHANGELOGs
- Each Go module has its own CHANGELOG.md next to its `go.mod`. Changes to a contrib module go in that module's CHANGELOG; everything else goes in the root CHANGELOG.
- A new module's CHANGELOG must start with the same preamble as the existing ones (title, Keep a Changelog and Semantic Versioning links, then `## [Unreleased]`).

#### CHANGELOG Format
The CHANGELOG follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

**Section Headings** (case-insensitive):
- `### Added` - New features or functionality
- `### Changed` - Changes to existing functionality (breaking changes only)
- `### Removed` - Removed features or functionality (breaking changes only)
- `### Deprecated` - Features marked for future removal
- `### Security` - Security-related updates or fixes
- `### Fixed` - Bug fixes
- `### Documentation` - Documentation-only changes (custom type, see `.releasegen.yaml`)
- `### Performance` - Performance improvements (custom type, see `.releasegen.yaml`)

#### Breaking Changes
- **`### Changed` and `### Removed` sections are ONLY for breaking changes**
- **MUST include the exact phrase "BREAKING CHANGE" in the description**
- If a change is not breaking, use `### Added` (for new features) or `### Fixed` (for bug fixes)
- Example:
  ```markdown
  ### Changed
  - **BREAKING CHANGE**: Modified API behavior to return errors instead of panicking
  ```

#### Version Bump Rules
Changes determine the next semantic version:
- **Major bump**: `### Changed` or `### Removed` sections with "BREAKING CHANGE"
- **Minor bump**: `### Added`, `### Deprecated`, `### Security`, or `### Performance` sections
- **Patch bump**: Only `### Fixed` or `### Documentation` sections

#### Example CHANGELOG Entry
```markdown
## [Unreleased]

### Added
- New S3 backend option for custom endpoint configuration

### Fixed
- Fixed file handle leak in os backend when operations fail
- Corrected path separator handling on Windows
```

### PR Submission Guidelines
1. Add changes under `## [Unreleased]` section in the affected module's CHANGELOG.md
2. Use appropriate section headings based on change type
3. **Do NOT add version numbers** - these are added during the merge/release process
4. Be clear and specific in change descriptions
5. Mark breaking changes with "BREAKING CHANGE" text

---

## Pull Requests and Code Review

### Working With PRs
- If a PR is already open for the branch, push to it. Do not open a second PR for the same work.
- When reworking a third-party contributor's PR, push to their branch when possible. If a new PR replaces theirs, credit them and link the original PR(s) in the description and CHANGELOG.
- Keep dependency bumps in their own PR rather than folding them into feature work.

### Review Comments (Copilot and Human)
- Validate each comment against the code before changing anything. Not every suggestion is correct.
- When asked to "validate" comments, report which ones you agree with and ask before making changes.
- After addressing a comment, reply briefly and resolve the thread. Leave threads you disagree with open, with an explanation.

### Performing Code Reviews
- Separate actionable findings from neutral observations. Each finding should state the problem, its impact, and a concrete fix.
- State your confidence and how a fix was verified, especially for regressions.
- Be conservative with core backends that production users depend on (SFTP in particular). Prove that fixes cause no regressions with tests rather than reasoning alone.

---

## Go Version Policy

### Compatibility Requirements
VFS guarantees compatibility with **the latest Go version minus 1 minor version**.

**Example:** If Go 1.27.x is the latest release, VFS supports:
- Go 1.27.x (latest minor version)
- Go 1.26.x (latest - 1 minor version)

Every `go.mod` specifies the **older** of the two supported minor versions, at its latest patch release (e.g., `go 1.26.7`), ensuring compatibility with both.

The patch level matters: CI runs with `GOTOOLCHAIN=local` and `setup-go` `check-latest: true`, so a `go` directive newer than the latest released patch fails CI. Use a full patch version when it contains a needed security fix.

### Upgrade Process

#### Core Module First
1. Update `go.mod` in the root module to the latest patch of the latest-1 minor version
2. Update all configuration files that reference Go versions
3. Run `go mod tidy` in `testcontainers/` (it builds against local core via `replace`)
4. Release the core VFS version
5. **Then** update contrib modules (they import a published core version, so core must be released first)

#### Configuration Files to Update
When updating Go versions, update these files:

- **All `go.mod` files** - `go` directive
- **`.golangci.yml`** - `run.go` field (latest-1 minor version)
- **`.github/workflows/go.yml`** - `matrix.go-version` array (latest-1 and latest)
- **`.github/workflows/golangci-lint.yml`** - `go-version` field
- **`.github/workflows/go-test-coverage.yml`** - `go-version` field
- **`.github/workflows/codeql.yml`** - `go-version` field
- **`.github/workflows/integration.yml`** - `go-version` field
- **`.github/workflows/docs.yml`** - `go-version` field

#### Contrib Modules (Update After Core Release)
- `contrib/lockfile/go.mod`
- `contrib/vfsevents/go.mod`
- `contrib/backend/dropbox/go.mod`
- `contrib/backend/gdrive/go.mod`
- `contrib/backend/onedrive/go.mod`
- `contrib/backend/sqlfs/go.mod`
- Any other contrib modules

---

## Dependency Upgrades

### General Policy
- Update dependencies regularly for security and bug fixes
- Test thoroughly after dependency updates
- Document breaking changes in CHANGELOG.md
- Do not use deprecated modules. If a dependency is deprecated (e.g. `go` prints a deprecation warning), open an issue to migrate.

### Go Dependencies
```bash
# Update all dependencies
go get -u -t ./...
go mod tidy

# Update specific dependency
go get -u github.com/example/package@latest
```

### Watch the `go` Directive
`go get -u` raises the `go` directive to the highest version required by any dependency. After upgrading:
1. Check that the `go` directive is still at the supported floor (see [Go Version Policy](#go-version-policy))
2. If it was raised, find the dependency responsible and pin it to the newest version compatible with the floor
3. Restore the directive with `go mod edit -go=<version>` and run `go mod tidy`

### testcontainers Module
`testcontainers/` uses `replace github.com/c2fo/vfs/v7 => ../`, so any dependency change in core requires `go mod tidy` in `testcontainers/` too, or CI fails with `updates to go.mod needed`.

---

## GitHub Actions Maintenance

### SHA Pinning Policy
All GitHub Actions **must be pinned to specific commit SHAs** for security and reproducibility.

**Format:** `uses: owner/action@<sha> # <version-tag>`

**Example:** `uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6.0.2`

### Update Policy
- **Select the newest version that is at least 10 days old**
- If the latest version is too recent (< 10 days), check for intermediate versions
- Allow time for community vetting of new releases
- Check for security advisories before updating
- Always verify release dates before updating

### How to Update Actions

1. **Find Available Versions**
   - Visit the action's repository releases page
   - Example: `https://github.com/actions/checkout/releases`
   - Check the CHANGELOG if available for release dates

2. **Select Newest Eligible Version**
   - Identify all versions newer than current
   - Find the **newest version that is at least 10 days old**
   - If the latest is too recent, look for intermediate versions
   - This allows time for community feedback and security review

3. **Get the Commit SHA**
   - Navigate to the selected tag/release on GitHub
   - Click on the commit hash to get the full 40-character SHA
   - Copy the full SHA (not the abbreviated version)

4. **Update the Workflow File**
   ```yaml
   # Before
   uses: actions/checkout@old-sha # v6.0.1

   # After
   uses: actions/checkout@new-sha # v6.0.2
   ```

5. **Test the Workflow**
   - Create a PR to test the updated action
   - Verify all workflows pass successfully

### Workflow Files
- `.github/workflows/go.yml` - Main test workflow (multi-module, multi-OS, multi-Go-version, `-race`)
- `.github/workflows/golangci-lint.yml` - Linting workflow (per module)
- `.github/workflows/codeql.yml` - Security scanning
- `.github/workflows/go-test-coverage.yml` - Test coverage enforcement
- `.github/workflows/integration.yml` - testcontainers conformance (PRs and nightly)
- `.github/workflows/docs.yml` - Fails PRs whose generated `docs/*.md` pages are out of date (`make docs-check`)
- `.github/workflows/validate-changelog.yml` - CHANGELOG validation via `releasegen validate`
- `.github/workflows/releasegen.yml` - Versions, tags, and publishes releases on merge to `main`

---

## Module Management

### Repository Structure
VFS uses a **multi-module** repository structure:

```
vfs/
├── go.mod                           # Core VFS module
├── testcontainers/go.mod            # Container-based conformance tests (replace ../)
├── contrib/
│   ├── lockfile/go.mod              # Lockfile contrib module
│   ├── vfsevents/go.mod             # Events contrib module
│   └── backend/
│       ├── dropbox/go.mod           # Dropbox backend
│       ├── gdrive/go.mod            # Google Drive backend
│       ├── onedrive/go.mod          # OneDrive backend
│       └── sqlfs/go.mod             # SQL FS backend
```

### Module Dependencies
- **Contrib modules depend on a published core VFS version**
- Core must be released **before** updating contrib module imports
- `testcontainers` always builds against local core via `replace`
- Each module has independent versioning and its own CHANGELOG.md

### Release Process
Releases are automated by `releasegen.yml`:
1. Each PR adds entries under `## [Unreleased]` in the affected module CHANGELOG(s)
2. On merge to `main`, releasegen computes each changed module's next version, promotes the entries into a versioned section, tags the commit (e.g. `v7.14.0`, `contrib/lockfile/v1.2.0`), and publishes a GitHub release
3. Do not create tags or version sections by hand

### Testing All Modules
```bash
# Run tests for all modules
make test

# Run linter for all modules
make lint

# Manual testing
for dir in $(find . -name go.mod -exec dirname {} \;); do
    echo "Testing $dir"
    (cd "$dir" && go test -race ./...)
done
```

---

## CI/CD Configuration

### Dynamic Module Detection
The CI workflows automatically detect all modules using:

```yaml
- name: Find all modules
  run: |
    modules=$(find . -name go.mod -exec dirname {} \; | jq --raw-input --slurp --compact-output 'split("\n")[:-1]')
    echo "modules=$modules" >> $GITHUB_OUTPUT
```

This ensures new modules are automatically tested without workflow updates.

### Test Matrix
- **Go Versions:** latest-1 and latest (see [Go Version Policy](#go-version-policy))
- **Operating Systems:** ubuntu-latest, macos-latest, windows-latest
- **Modules:** All modules with `go.mod` files on pushes to `main` and manual runs. On pull requests, `.github/scripts/affected-modules.sh` selects only the affected modules:
  - a module is affected when a file under its directory changes (nested modules own their files)
  - a module with a local `replace` directive (e.g. `testcontainers`) is affected when the replaced module is
  - changes to `.github/workflows/`, `.github/scripts/`, `.golangci.yml`, `.testcoverage.yml`, or `go.work*` test every module
  - markdown-only changes test nothing (the matrix is skipped and the gate still passes)

### Required Check Gates
Matrix jobs fan out into a changing set of checks that can't be listed in branch rulesets. Each matrix workflow ends with a single gate job that rulesets require instead:
- `Test (required)` in `go.yml`
- `golangci-lint (required)` in `golangci-lint.yml`

New matrix workflows should follow the same pattern (`needs: <matrix job>`, `if: always()`, fail unless the result is `success`).

### Concurrency Control
Workflows use concurrency groups to prevent resource waste:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}
```

### Test Coverage
- Minimum coverage requirements defined in `.testcoverage.yml`
- Enforced via `go-test-coverage` action

---

## Security

### Dependency Scanning
- CodeQL runs on all PRs and weekly schedule
- Dependabot alerts should be triaged into GitHub issues and fixed in their own PRs

### Action Security
- All actions pinned to commit SHAs (not tags)
- Regular review of action updates
- 10-day waiting period for new releases

---

## Maintenance Checklist

### Monthly
- [ ] Check for Go version updates
- [ ] Review dependency updates (`go list -u -m all`)
- [ ] Check GitHub Actions for updates (>10 days old)
- [ ] Review security advisories and Dependabot alerts

### Quarterly
- [ ] Update Go to latest-1 minor version
- [ ] Update all dependencies
- [ ] Review and update CI/CD workflows
- [ ] Update this AGENTS.md document
- [ ] When upgrading gomarkdoc (pinned in the `Makefile`), re-copy its `type` and `func` templates into `.gomarkdoc/` and reapply the link removal

---

## Common Tasks

### Adding a New Core Backend
1. Create the package under `backend/` with a `doc.go` package comment and a `Scheme` constant
2. Register it in `backend/all` and add a `conformance_test.go` that runs the shared suites
3. Add a `pkg:docs/<name>.md` entry to `DOC_PAGES` in the `Makefile`, then run `make docs`
4. Link the new page from the root `README.md`

### Adding a New Contrib Module
1. Create directory under `contrib/`
2. Initialize `go.mod` with dependency on a published core VFS version, using the same `go` directive as core
3. Add `CHANGELOG.md` with the standard preamble and an `## [Unreleased]` entry
4. Add `README.md` with usage documentation, and link it from the root `README.md` and `contrib/backend/README.md`
5. Implement required interfaces
6. Add comprehensive tests, including the shared conformance suites
7. CI will automatically detect and test the new module

### Updating Core VFS Version in Contrib
```bash
cd contrib/yourmodule
go get github.com/c2fo/vfs/v7@v7.14.0
go mod tidy
```

### Running Integration Tests
```bash
# With build tags
go test -tags=vfsintegration ./...

# Against real locations
VFS_INTEGRATION_LOCATIONS="s3://bucket/vfs_test/" go test -v -tags=vfsintegration ./backend/testsuite/...

# testcontainers (requires Docker)
cd testcontainers
go test -tags=vfsintegration ./...
```

---

## References

- [Go Release Policy](https://go.dev/doc/devel/release)
- [GitHub Actions Security](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions)
- [golangci-lint Documentation](https://golangci-lint.run/)
- [releasegen](https://github.com/c2fo/releasegen)
- [prenup](https://github.com/c2fo/prenup)
- [VFS CHANGELOG](./CHANGELOG.md)
