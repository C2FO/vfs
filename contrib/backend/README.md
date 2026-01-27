# Community Backends

This directory contains community-contributed VFS backends that are not officially supported as part of the core VFS library.

## Available Backends

| Backend | Scheme | Description |
|---------|--------|-------------|
| [Dropbox](./dropbox/README.md) | `dbx://` | Dropbox cloud storage |

## Using a Community Backend

Community backends are **not** automatically registered. You must explicitly import them:

```go
import (
    "github.com/c2fo/vfs/v7/vfssimple"
    _ "github.com/c2fo/vfs/contrib/backend/dropbox" // registers dbx:// scheme
)

func main() {
    // Now dbx:// URIs work with vfssimple
    file, err := vfssimple.NewFile("dbx:///path/to/file.txt")
    // ...
}
```

## Contributing a New Backend

Want to contribute a backend? Follow these guidelines:

### 1. Structure

Create a new directory under `contrib/backend/`:

```
contrib/backend/yourbackend/
├── go.mod                 # Separate module (see versioning below)
├── go.sum
├── README.md              # Documentation for your backend
├── doc.go                 # Package documentation
├── fileSystem.go          # vfs.FileSystem implementation
├── fileSystem_test.go     # Unit tests
├── location.go            # vfs.Location implementation
├── location_test.go
├── file.go                # vfs.File implementation
├── file_test.go
├── options.go             # Configuration options
├── conformance_test.go    # Integration tests (see Testing Requirements)
└── mocks/                 # Generated mocks for testing
    └── Client.go
```

### 2. Versioning

Each contrib backend has its own `go.mod` for independent versioning:

```go
module github.com/c2fo/vfs/contrib/backend/yourbackend

go 1.24

require (
    github.com/c2fo/vfs/v7 v7.13.0  // Depend on core VFS
    // ... your backend's dependencies
)
```

This allows:
- Independent release cycles
- Isolated dependencies (your backend's deps don't affect core VFS)
- Clear compatibility requirements

### 3. Interface Implementation

Implement the core VFS interfaces:

- **`vfs.FileSystem`** - Factory for locations and files
- **`vfs.Location`** - Directory/path operations
- **`vfs.File`** - File operations (implements `io.Reader`, `io.Writer`, `io.Seeker`, `io.Closer`)

Register your backend in `init()`:

```go
func init() {
    backend.Register(Scheme, NewFileSystem())
}
```

### 4. Testing Requirements

#### Unit Tests
- Use `testify.Suite` with mocked dependencies
- Use **Mockery** for generating mocks: add entry to root `.mockery.yaml`

#### Integration Tests (Conformance Tests)

The `backend/testsuite` package provides conformance tests that verify your backend correctly implements the VFS interfaces. **This is critical for ensuring consistent behavior across all backends.**

See the full documentation: [docs/conformance_tests.md](../../docs/conformance_tests.md)

**What's tested:**
- `RunConformanceTests` — FileSystem, Location, and File interface compliance
- `RunIOTests` — 18 different Read/Write/Seek sequences

**Example conformance test:**

```go
//go:build vfsintegration

package yourbackend

import (
    "os"
    "testing"
    "github.com/c2fo/vfs/v7/backend/testsuite"
)

func TestConformance(t *testing.T) {
    token := os.Getenv("YOUR_TOKEN")
    if token == "" {
        t.Skip("YOUR_TOKEN not set, skipping integration tests")
    }

    fs := NewFileSystem(WithAccessToken(token))
    location, err := fs.NewLocation("", "/vfs-test/")
    if err != nil {
        t.Fatalf("failed to create location: %v", err)
    }

    // Configure options for backend-specific limitations
    opts := testsuite.ConformanceOptions{
        SkipTouchTimestampTest: true,  // if backend has timestamp limitations
        SkipFTPSpecificTests:   false,
    }

    testsuite.RunConformanceTests(t, location, opts)
}

func TestIOConformance(t *testing.T) {
    token := os.Getenv("YOUR_TOKEN")
    if token == "" {
        t.Skip("YOUR_TOKEN not set, skipping integration tests")
    }

    fs := NewFileSystem(WithAccessToken(token))
    location, _ := fs.NewLocation("", "/vfs-test/")
    testsuite.RunIOTests(t, location)
}
```

**Run with:**
```bash
YOUR_TOKEN=xxx go test -v -tags=vfsintegration ./... -run TestConformance
```

**ConformanceOptions:**
| Option | When to Use |
|--------|-------------|
| `SkipTouchTimestampTest` | Backend doesn't update `LastModified` on identical content |
| `SkipFTPSpecificTests` | Backend has FTP-like IO limitations |

### 5. Documentation

Your README should include:
- Installation instructions
- Authentication/configuration
- Usage examples
- API limitations and workarounds
- Performance considerations

### 6. Code Style

Follow the existing patterns in core backends and other contrib packages:
- Functional options for configuration (`WithOptionName()`)
- Wrapped errors with context
- Comprehensive error handling

## Support Level

Community backends are maintained by the community and may not receive the same level of support as core backends. Issues and PRs are welcome, but response times may vary.

If a community backend gains significant adoption and a maintainer willing to provide ongoing support, it may be considered for promotion to a core backend.
