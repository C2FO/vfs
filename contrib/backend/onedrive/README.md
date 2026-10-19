# OneDrive Backend for VFS

This package provides a VFS backend for Microsoft OneDrive using the Microsoft Graph REST API.
It talks to Graph over `net/http` directly (authentication uses `azidentity`), so it does not pull
in the Microsoft Graph SDK.

## Account Types

This backend supports two types of Microsoft accounts:

| Account Type | Description | Supported Auth Methods |
|--------------|-------------|------------------------|
| **Personal (Consumer)** | Personal Microsoft/Office 365 accounts (outlook.com, hotmail.com, live.com) | Device Code, Interactive Browser, Access Token |
| **Work/School (Azure AD)** | Organization accounts managed by Azure AD | All methods including Client Credentials |

> **Note**: Client Credentials (service principal) authentication only works with work/school accounts. Personal accounts require interactive authentication.

## Installation

```bash
go get github.com/c2fo/vfs/contrib/backend/onedrive
```

## Usage

### Basic Usage

```go
import (
    "github.com/c2fo/vfs/contrib/backend/onedrive"
)

func main() {
    // Create filesystem with access token
    fs := onedrive.NewFileSystem(
        onedrive.WithAccessToken("your-oauth-token"),
    )

    // Create a location
    location, err := fs.NewLocation("", "/Documents/")
    if err != nil {
        log.Fatal(err)
    }

    // List files
    files, err := location.List()
    if err != nil {
        log.Fatal(err)
    }

    for _, file := range files {
        fmt.Println(file)
    }
}
```

### Authentication Methods

#### 1. Device Code (Personal & Work Accounts) ⭐ Recommended for Personal OneDrive

Ideal for CLI tools and scenarios where a browser isn't available on the same machine.
The user visits a URL and enters a code to authenticate:

```go
// For personal Microsoft accounts (consumer OneDrive)
fs := onedrive.NewFileSystem(
    onedrive.WithDeviceCode("your-client-id", "consumers", nil),
)

// For work/school accounts
fs := onedrive.NewFileSystem(
    onedrive.WithDeviceCode("your-client-id", "organizations", nil),
)

// For both account types (user chooses at sign-in)
fs := onedrive.NewFileSystem(
    onedrive.WithDeviceCode("your-client-id", "common", nil),
)

// With custom callback to handle the device code message
fs := onedrive.NewFileSystem(
    onedrive.WithDeviceCode("your-client-id", "consumers", func(ctx context.Context, msg string) error {
        fmt.Println(msg) // "To sign in, use a web browser to open https://microsoft.com/devicelogin and enter code XXXXXXX"
        return nil
    }),
)
```

#### 2. Interactive Browser (Personal & Work Accounts)

Opens a browser window for user sign-in. Best for desktop applications:

```go
// For personal Microsoft accounts
fs := onedrive.NewFileSystem(
    onedrive.WithInteractiveBrowser("your-client-id", "consumers", ""),
)

// For work/school accounts  
fs := onedrive.NewFileSystem(
    onedrive.WithInteractiveBrowser("your-client-id", "organizations", ""),
)

// With custom redirect URL
fs := onedrive.NewFileSystem(
    onedrive.WithInteractiveBrowser("your-client-id", "common", "http://localhost:8080"),
)
```

#### 3. Access Token (All Account Types)

If you already have an OAuth2 access token from your own auth flow:

```go
fs := onedrive.NewFileSystem(
    onedrive.WithAccessToken(os.Getenv("VFS_ONEDRIVE_ACCESS_TOKEN")),
)
```

#### 4. Client Credentials (Work/School Accounts Only)

For server-to-server scenarios using Azure AD application credentials.
**Does not work with personal Microsoft accounts**:

```go
fs := onedrive.NewFileSystem(
    onedrive.WithClientCredentials(
        os.Getenv("VFS_ONEDRIVE_TENANT_ID"),
        os.Getenv("VFS_ONEDRIVE_CLIENT_ID"),
        os.Getenv("VFS_ONEDRIVE_CLIENT_SECRET"),
    ),
)
```

#### 5. Default Azure Credential (Work/School Accounts)

Uses the Azure SDK's default credential chain (environment variables, managed identity, etc.):

```go
fs := onedrive.NewFileSystem(
    onedrive.WithDefaultCredential(),
)
```

### Drive Selection

By default, the backend accesses the authenticated user's OneDrive. For application permissions, you must specify which drive to access:

#### Access a Specific User's Drive

```go
fs := onedrive.NewFileSystem(
    onedrive.WithClientCredentials(tenantID, clientID, clientSecret),
    onedrive.WithUserDrive("user@example.com"),
)
```

#### Access a SharePoint Site's Drive

```go
fs := onedrive.NewFileSystem(
    onedrive.WithClientCredentials(tenantID, clientID, clientSecret),
    onedrive.WithSiteDrive("site-id", "drive-id"),
)
```

### Tenant ID Values

| Value | Description |
|-------|-------------|
| `"consumers"` | Personal Microsoft accounts only |
| `"organizations"` | Work/school accounts only |
| `"common"` | Both personal and work/school (user chooses) |
| `"your-tenant-id"` | Specific Azure AD tenant (work/school only) |

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithDeviceCode(clientID, tenantID, callback)` | Device code flow (personal/work) | - |
| `WithInteractiveBrowser(clientID, tenantID, redirectURL)` | Browser sign-in (personal/work) | - |
| `WithAccessToken(token)` | OAuth2 access token | - |
| `WithClientCredentials(tenant, client, secret)` | Azure AD credentials (work only) | - |
| `WithDefaultCredential()` | Use Azure default credential chain | false |
| `WithUserDrive(userID)` | Access a specific user's drive | - |
| `WithSiteDrive(siteID, driveID)` | Access a SharePoint site drive | - |
| `WithDriveID(driveID)` | Access a specific drive by ID | - |
| `WithChunkSize(size)` | Chunk size for large file uploads (rounded down to a multiple of 320 KiB) | 5 MiB |
| `WithTempDir(dir)` | Temporary directory for buffering | os.TempDir() |
| `WithRetryCount(count)` | Retry attempts for 429 and 5xx responses | 3 |
| `WithBaseURL(url)` | Graph API base URL (national clouds, proxies, or a test server) | `https://graph.microsoft.com/v1.0` |
| `WithHTTPClient(client)` | HTTP client used for all Graph requests | `http.DefaultClient` |
| `WithClient(client)` | Custom `Client` implementation (for testing) | - |

## URI Format

```
onedrive:///path/to/file.txt
onedrive:///path/to/folder/
```

The authority is always empty as OneDrive uses a single namespace per drive.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `VFS_ONEDRIVE_ACCESS_TOKEN` | OAuth2 access token |
| `VFS_ONEDRIVE_TENANT_ID` | Azure AD tenant ID |
| `VFS_ONEDRIVE_CLIENT_ID` | Azure AD application ID |
| `VFS_ONEDRIVE_CLIENT_SECRET` | Azure AD application secret |

## Limitations

### Upload Size Limits

- **Simple uploads**: Up to 4 MiB
- **Large files**: Automatically use upload sessions with chunked uploads

### API Rate Limits

Microsoft Graph API has rate limits. The backend retries throttled (429) and 503 responses for every
request, and 408/500/502/504 responses and network errors for idempotent requests only (so a `POST`
that may have been processed is never replayed). `Retry-After` is honored up to 5 minutes; a longer
request fails immediately instead of retrying early, which would only prolong throttling. Otherwise
retries back off exponentially. An interrupted upload-session chunk resumes from the offset Graph
reports rather than failing the upload.

### Folders Are Not Files

`Delete`, `CopyToFile`/`CopyToLocation` and `MoveToFile`/`MoveToLocation` return an error when the
path is a folder. Graph would otherwise delete, copy or move the folder and everything in it.

### Cancellation and Timeouts

VFS file operations take no `context.Context`, so requests cannot be canceled by the caller. A copy
waits up to 10 minutes for Graph to finish. Use `WithHTTPClient` with a client `Timeout` to bound
individual requests.

### Copy

Graph copies run asynchronously. `CopyToFile`/`CopyToLocation` wait for the copy to finish (polling
Graph's monitor URL) before returning. Copies and moves overwrite an existing destination. When the
destination folder does not exist yet, the content is streamed instead.

### `%` in Folder Names

When an upload creates missing parent folders, Graph percent-decodes their names twice (a folder
named `a%20b` would be created as `a b`). To keep names literal, uploads whose parent path contains
`%` create the missing folders explicitly first, which costs a few extra requests.

### Path Length

OneDrive has a 400-character path length limit.

### No Range Writes

OneDrive upload sessions support range writes, but for simplicity this implementation uses temporary file buffering (same approach as Dropbox backend).

### Touch Behavior

Graph has no way to update a timestamp alone, so touching an existing file downloads it and uploads
it again. A write made by someone else between the download and the upload is overwritten.

## App Registration Setup

### For Personal Microsoft Accounts (Consumer OneDrive)

1. Go to [Azure Portal](https://portal.azure.com) > App registrations
2. Click "New registration"
3. Configure:
   - **Name**: Your app name
   - **Supported account types**: Select **"Personal Microsoft accounts only"** or **"Accounts in any organizational directory and personal Microsoft accounts"**
   - **Redirect URI**: 
     - For device code: Leave blank (not required)
     - For interactive browser: Add `http://localhost` (or your custom redirect)
4. Note your **Application (client) ID**

#### Configure API Permissions (Personal Accounts)

Add Microsoft Graph **delegated** permissions:
- `Files.ReadWrite` - Read and write user's files
- `User.Read` - Sign in and read user profile
- `offline_access` - Maintain access (for refresh tokens)

> **Important**: Personal accounts only support delegated permissions, not application permissions.

### For Work/School Accounts (Azure AD)

1. Go to [Azure Portal](https://portal.azure.com)
2. Navigate to Azure Active Directory > App registrations
3. Click "New registration"
4. Configure:
   - **Name**: Your app name
   - **Supported account types**: Choose based on your needs
   - **Redirect URI**: (optional, for OAuth flow)

#### Configure API Permissions (Work/School)

Add Microsoft Graph permissions:

**Delegated permissions** (user context - device code, interactive browser):
- `Files.ReadWrite`
- `Files.ReadWrite.All`

**Application permissions** (service principal - client credentials):
- `Files.ReadWrite.All`
- `Sites.ReadWrite.All` (for SharePoint)

#### Create Client Secret (for Client Credentials only)

1. Go to Certificates & secrets
2. Click "New client secret"
3. Copy the secret value

#### Grant Admin Consent

For application permissions, an admin must grant consent.

## Testing

### Unit Tests

```bash
go test ./...
```

Unit tests use mockery mocks of `types.Client` (regenerate with `mockery` from this directory) and an
`httptest` server for the Graph REST client.

### WireMock Integration Tests

`wiremock_test.go` starts a [WireMock](https://wiremock.org/) container with
[testcontainers-go](https://golang.testcontainers.org/) and drives the real `FileSystem` against
stubbed Graph responses (via `WithBaseURL`), covering drive resolution, uploads and upload sessions,
download redirects, pagination, async copy polling, move, delete, and throttling retries. They need
Docker and skip when it isn't available:

```bash
go test -tags=vfsintegration -run TestWireMockTestSuite ./...
```

### Real OneDrive Tests

`TestConformance` runs the shared VFS conformance and IO suites against a real drive, and
`TestRealDriveTestSuite` covers what they don't: upload sessions with large files, copy and move
overwriting an existing file, the folder guard, and names containing characters such as `%`, `#`, `+`
and `&`. Both skip unless credentials are configured:

```bash
# Set credentials
export VFS_ONEDRIVE_ACCESS_TOKEN="your-token"
# OR
export VFS_ONEDRIVE_TENANT_ID="your-tenant"
export VFS_ONEDRIVE_CLIENT_ID="your-client-id"
export VFS_ONEDRIVE_CLIENT_SECRET="your-secret"

# Optional: custom test path
export VFS_ONEDRIVE_TEST_PATH="/vfs-integration-test/"

# Run conformance tests
go test -v -tags=vfsintegration ./... -run TestConformance
```

## Examples

### Read a File

```go
file, err := fs.NewFile("", "/Documents/report.txt")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

content, err := io.ReadAll(file)
if err != nil {
    log.Fatal(err)
}
fmt.Println(string(content))
```

### Write a File

```go
file, err := fs.NewFile("", "/Documents/new-file.txt")
if err != nil {
    log.Fatal(err)
}

_, err = file.Write([]byte("Hello, OneDrive!"))
if err != nil {
    log.Fatal(err)
}

err = file.Close() // Upload happens on Close
if err != nil {
    log.Fatal(err)
}
```

### Copy a File

```go
srcFile, _ := fs.NewFile("", "/Documents/source.txt")
destLocation, _ := fs.NewLocation("", "/Backup/")

newFile, err := srcFile.CopyToLocation(destLocation)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Copied to:", newFile.URI())
```

### List Files with Filter

```go
location, _ := fs.NewLocation("", "/Documents/")

// List files with prefix
files, err := location.ListByPrefix("report-")
if err != nil {
    log.Fatal(err)
}

// List files matching regex
regex := regexp.MustCompile(`.*\.pdf$`)
pdfFiles, err := location.ListByRegex(regex)
```

## License

This package is part of VFS and is released under the same license.
