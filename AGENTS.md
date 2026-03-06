# Agent Guidelines for VFS Repository

This document provides guidelines for AI agents working on the VFS repository, including upgrade policies, dependency management, and maintenance procedures.

## Table of Contents
- [Development Guidelines](#development-guidelines)
- [CHANGELOG and PR Process](#changelog-and-pr-process)
- [Go Version Policy](#go-version-policy)
- [Dependency Upgrades](#dependency-upgrades)
- [GitHub Actions Maintenance](#github-actions-maintenance)
- [Module Management](#module-management)

---

## Development Guidelines

### Testing Requirements

#### Mandatory Test Coverage
- **All new code MUST have tests**
- Aim for coverage as close to 100% as is practicable
- Minimum thresholds (enforced by CI):
  - **Total project coverage:** 80%
  - **Package coverage:** 63%
  - **File coverage:** 52%

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
        name          string
        input         string
        expectedOutput string
        expectedError string
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

#### Integration Tests
- Use build tag `vfsintegration` for integration tests
- Run with: `go test -tags=vfsintegration ./...`
- Integration tests may require external services or credentials

### Code Quality

#### Linting
- **All code MUST pass `golangci-lint`**
- Run before committing: `make lint`
- Configuration: `.golangci.yml`
- Linter runs on all modules independently in CI

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

### Interface Design
- Prefer small, focused interfaces over large ones
- Define interfaces in consuming packages, not implementing packages
- Use dependency injection for testability

### File Organization
- Group related functionality in packages
- Use subdirectories for different implementations
- Place mocks in `mocks/` subdirectories
- One package per major feature

### Mocking
- **Use `mockery` v3 for generating mocks with EXPECT() pattern**
- **No manual mocks or fakes** (unless absolutely necessary)
- **Prefer EXPECT() pattern for mock setup** for better readability and type safety
- Place mocks in dedicated `mocks/` subdirectories
- Generate mocks with: `//go:generate mockery --name=InterfaceName --output=./mocks`
- Example EXPECT() usage:
  ```go
  mockClient := mocks.NewMockSQSClient(t)
  mockClient.EXPECT().ReceiveMessage(mock.Anything, mock.MatchedBy(func(input *sqs.ReceiveMessageInput) bool {
      return *input.QueueUrl == queueURL
  })).Return(&sqs.ReceiveMessageOutput{Messages: messages}, nil)
  ```

### Documentation
- All exported types, functions, and methods must have godoc comments
- Include usage examples for complex functionality
- Update README.md when adding new features
- Document breaking changes in CHANGELOG.md

### Performance
- Implement graceful shutdown with timeouts
- Use context for cancellation throughout
- Prevent resource leaks with proper cleanup (use `defer`)
- Be mindful of goroutine lifecycle and synchronization

---

## CHANGELOG and PR Process

### CHANGELOG Requirements

#### All PRs MUST Update CHANGELOG.md
- **Every PR requires an update to the `## [Unreleased]` section** of CHANGELOG.md
- PRs are **always submitted without a new version number**
- Version numbers are added automatically when the PR is merged to main

#### CHANGELOG Format
The CHANGELOG follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

**Standard Section Headings** (case-insensitive):
- `### Added` - New features or functionality
- `### Changed` - Changes to existing functionality (breaking changes only)
- `### Removed` - Removed features or functionality (breaking changes only)
- `### Deprecated` - Features marked for future removal
- `### Security` - Security-related updates or fixes
- `### Fixed` - Bug fixes

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
- **Minor bump**: `### Added`, `### Deprecated`, or `### Security` sections
- **Patch bump**: Only `### Fixed` sections

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
1. Add changes under `## [Unreleased]` section in CHANGELOG.md
2. Use appropriate section headings based on change type
3. **Do NOT add version numbers** - these are added during the merge/release process
4. Be clear and specific in change descriptions
5. Mark breaking changes with "BREAKING CHANGE" text

---

## Go Version Policy

### Compatibility Requirements
VFS guarantees compatibility with **the latest Go version minus 1 minor version**.

**Example:** If Go 1.26.1 is the latest release, VFS supports:
- Go 1.26.x (latest minor version)
- Go 1.25.x (latest - 1 minor version)

The `go.mod` file specifies the older of the two supported versions (e.g., `go 1.25.8`), ensuring compatibility with both.

### Upgrade Process

#### Core Module First
1. Update `go.mod` in the root module to the latest Go version (e.g., `go 1.25.8`)
2. Update all configuration files that reference Go versions
3. Release the core VFS version
4. **Then** update contrib modules (they import core, so core must be released first)

#### Configuration Files to Update
When updating Go versions, update these files:

- **`go.mod`** - Main module Go version
- **`.golangci.yml`** - `run.go` field
- **`.github/workflows/go.yml`** - `matrix.go-version` array
- **`.github/workflows/golangci-lint.yml`** - `go-version` field
- **`.github/workflows/codeql.yml`** - `go-version` field

#### Contrib Modules (Update After Core Release)
- `contrib/lockfile/go.mod`
- `contrib/vfsevents/go.mod`
- `contrib/backend/dropbox/go.mod`
- Any other contrib modules

---

## Dependency Upgrades

### General Policy
- Update dependencies regularly for security and bug fixes
- Test thoroughly after dependency updates
- Document breaking changes in CHANGELOG.md

### Go Dependencies
```bash
# Update all dependencies
go get -u -t ./...
go mod tidy

# Update specific dependency
go get -u github.com/example/package@latest
```

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
- `.github/workflows/go.yml` - Main test workflow (multi-module, multi-OS, multi-Go-version)
- `.github/workflows/golangci-lint.yml` - Linting workflow
- `.github/workflows/codeql.yml` - Security scanning
- `.github/workflows/go-test-coverage.yml` - Test coverage enforcement
- `.github/workflows/ensure_changelog.yml` - CHANGELOG validation

---

## Module Management

### Repository Structure
VFS uses a **multi-module** repository structure:

```
vfs/
├── go.mod                           # Core VFS module
├── contrib/
│   ├── lockfile/go.mod             # Lockfile contrib module
│   ├── vfsevents/go.mod            # Events contrib module
│   └── backend/
│       └── dropbox/go.mod          # Dropbox backend contrib module
```

### Module Dependencies
- **Contrib modules depend on core VFS**
- Core must be released **before** updating contrib module imports
- Each module has independent versioning

### Release Process
1. Update and test core VFS module
2. Create release tag for core (e.g., `v7.14.0`)
3. Update contrib modules to reference new core version
4. Test contrib modules
5. Create release tags for contrib modules (e.g., `contrib/lockfile/v1.2.0`)

### Testing All Modules
```bash
# Run tests for all modules
make test

# Run linter for all modules
make lint

# Manual testing
for dir in $(find . -name go.mod -exec dirname {} \;); do
    echo "Testing $dir"
    (cd "$dir" && go test ./...)
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
- **Go Versions:** 1.25, 1.26 (latest-1 and latest)
- **Operating Systems:** ubuntu-latest, macos-latest, windows-latest
- **Modules:** All modules with `go.mod` files

### Concurrency Control
Workflows use concurrency groups to prevent resource waste:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}
```

---

## Code Quality

### golangci-lint Configuration
- Configuration: `.golangci.yml`
- Go version must match latest supported version
- Runs on all modules independently
- Build tags: `vfsintegration` for integration tests

**Run the linter:**
```bash
golangci-lint run --verbose --max-same-issues 0 ./...
```

### Test Coverage
- Minimum coverage requirements defined in `.testcoverage.yml`
- Enforced via `go-test-coverage` action
- Coverage reports generated for all modules

---

## Security

### Dependency Scanning
- CodeQL runs on all PRs and weekly schedule
- Scans for security vulnerabilities in Go code
- Requires `GH_CI_READ` secret for private dependencies

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
- [ ] Review security advisories

### Quarterly
- [ ] Update Go to latest stable version
- [ ] Update all dependencies
- [ ] Review and update CI/CD workflows
- [ ] Update this AGENTS.md document

### Before Major Release
- [ ] Update CHANGELOG.md
- [ ] Run full test suite on all platforms
- [ ] Verify all modules build successfully
- [ ] Update version tags
- [ ] Create GitHub release with notes

---

## Common Tasks

### Adding a New Contrib Module
1. Create directory under `contrib/`
2. Initialize `go.mod` with dependency on core VFS
3. Add README.md with usage documentation
4. Implement required interfaces
5. Add comprehensive tests
6. CI will automatically detect and test the new module

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

# Specific backend
cd backend/s3
go test -tags=vfsintegration ./...
```

---

## References

- [Go Release Policy](https://go.dev/doc/devel/release)
- [GitHub Actions Security](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions)
- [golangci-lint Documentation](https://golangci-lint.run/)
- [VFS CHANGELOG](./CHANGELOG.md)
