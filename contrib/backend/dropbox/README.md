# Dropbox Backend for VFS

A Virtual File System (VFS) backend implementation for Dropbox using the [unofficial Dropbox SDK for Go](https://github.com/dropbox/dropbox-sdk-go-unofficial).

## Features

- Full VFS interface implementation (`FileSystem`, `Location`, `File`)
- Read, write, seek, and delete operations
- Native copy and move operations within Dropbox
- Chunked uploads for large files (>150MB)
- Comprehensive error handling with wrapped errors
- Lazy client initialization
- Configurable via functional options

## Installation

```bash
go get github.com/c2fo/vfs/contrib/backend/dropbox
```

## Quick Start

### Basic Usage

```go
package main

import (
    "fmt"
    "io"
    "os"

    "github.com/c2fo/vfs/v7/backend"
    _ "github.com/c2fo/vfs/contrib/backend/dropbox"
)

func main() {
    // Get Dropbox filesystem from backend registry
    fs := backend.Backend(dropbox.Scheme)
    
    // Create a file
    file, err := fs.NewFile("", "/path/to/file.txt")
    if err != nil {
        panic(err)
    }
    
    // Write content
    _, err = file.Write([]byte("Hello, Dropbox!"))
    if err != nil {
        panic(err)
    }
    
    // Close to commit the write
    err = file.Close()
    if err != nil {
        panic(err)
    }
}
```

### Direct Initialization

```go
package main

import (
    "github.com/c2fo/vfs/contrib/backend/dropbox"
)

func main() {
    // Create filesystem with access token
    fs := dropbox.NewFileSystem(
        dropbox.WithAccessToken(os.Getenv("VFS_DROPBOX_ACCESS_TOKEN")),
        dropbox.WithChunkSize(8 * 1024 * 1024), // 8MB chunks
    )
    
    // Use the filesystem
    location, err := fs.NewLocation("", "/my/folder/")
    if err != nil {
        panic(err)
    }
    
    files, err := location.List()
    if err != nil {
        panic(err)
    }
    
    for _, filename := range files {
        fmt.Println(filename)
    }
}
```

## Authentication

The Dropbox backend supports two authentication methods: **Access Tokens** (for testing) and **OAuth2 Flow** (for production). Our backend is authentication-method agnostic - it simply requires a valid access token string, regardless of how you obtained it.

### Access Token vs OAuth2 Flow

#### Access Token (Testing/Personal Use)

**What it is**: A long-lived token you manually generate from the Dropbox App Console.

**When to use**:
- Testing and development
- Personal scripts and tools
- Single-user applications
- Quick prototypes

**Pros**:
- Simple - no code required to obtain
- Perfect for local development and testing

**Cons**:
- Token doesn't expire (security risk if leaked)
- Only works for your Dropbox account
- Not suitable for multi-user applications

#### OAuth2 Flow (Production/Multi-User Apps)

**What it is**: The standard OAuth2 authorization flow where each user authorizes your app through Dropbox's login page.

**When to use**:
- Production applications
- Multi-user applications
- Applications that access user data
- SaaS products

**Pros**:
- Each user gets their own token
- Tokens can be short-lived and refreshed (more secure)
- Users can revoke access anytime
- Industry-standard security practice

**Cons**:
- More complex to implement
- Requires web server to handle OAuth callback

**Note**: The OAuth2 flow implementation is outside the scope of this backend. You implement the OAuth2 flow in your application code, then pass the resulting access token to our backend.

### Quick Start: Access Token (Testing)

1. **Create a Dropbox App**
   - Go to https://www.dropbox.com/developers/apps
   - Click "Create App"
   - Choose "Scoped access"
   - Choose "Full Dropbox" or "App folder"
   - Name your app

2. **Configure Permissions** (Required)
   - Go to the **"Permissions"** tab in your app
   - Enable these scopes:
     - `files.metadata.read` - View metadata for files and folders
     - `files.content.read` - View content of files and folders
     - `files.content.write` - Create, edit, and delete files and folders
   - Click **"Submit"**

3. **Generate Access Token**
   - Go to the **"Settings"** tab
   - Scroll to "OAuth 2" section
   - Click "Generate" under "Generated access token"
   - Copy the token (it will only be shown once)
   - **Note**: If you change permissions later, regenerate the token

3. **Use the Token**

```go
// From environment variable (recommended)
token := os.Getenv("VFS_DROPBOX_ACCESS_TOKEN")
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(token),
)
```

### Production: OAuth2 Flow

For production applications with multiple users, implement the OAuth2 flow and pass the resulting token to the backend.

#### Step 1: Implement OAuth2 (Your Application)

```go
import (
    "context"
    "net/http"
    
    "golang.org/x/oauth2"
)

// Configure OAuth2
var oauth2Config = &oauth2.Config{
    ClientID:     os.Getenv("DROPBOX_CLIENT_ID"),
    ClientSecret: os.Getenv("DROPBOX_CLIENT_SECRET"),
    Endpoint: oauth2.Endpoint{
        AuthURL:  "https://www.dropbox.com/oauth2/authorize",
        TokenURL: "https://api.dropboxapi.com/oauth2/token",
    },
    RedirectURL: "https://yourapp.com/auth/dropbox/callback",
    Scopes:      []string{"files.content.read", "files.content.write"},
}

// Redirect user to Dropbox authorization page
func handleLogin(w http.ResponseWriter, r *http.Request) {
    url := oauth2Config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
    http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// Handle OAuth2 callback
func handleCallback(w http.ResponseWriter, r *http.Request) {
    code := r.URL.Query().Get("code")
    
    // Exchange authorization code for access token
    token, err := oauth2Config.Exchange(context.Background(), code)
    if err != nil {
        http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
        return
    }
    
    // Store token securely (database, session, etc.)
    // token.AccessToken is what you'll pass to VFS
    storeUserToken(token.AccessToken)
    
    http.Redirect(w, r, "/success", http.StatusTemporaryRedirect)
}
```

#### Step 2: Use Token with VFS Backend

```go
import (
    "github.com/c2fo/vfs/contrib/backend/dropbox"
)

// Retrieve user's token from your storage
userToken := getUserToken(userID)

// Pass it to VFS Dropbox backend
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(userToken),
)

// Use the filesystem as normal
file, err := fs.NewFile("", "/user-data.txt")
// ... rest of your code
```

#### Token Refresh (Optional but Recommended)

If using short-lived tokens with refresh tokens:

```go
// Check if token needs refresh
if token.Expiry.Before(time.Now()) {
    // Use oauth2.TokenSource to automatically refresh
    tokenSource := oauth2Config.TokenSource(context.Background(), token)
    newToken, err := tokenSource.Token()
    if err != nil {
        return err
    }
    
    // Update stored token
    storeUserToken(newToken.AccessToken)
}

// Use the fresh token
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(token.AccessToken),
)
```

### Both Methods Work Identically

The key point: **Our backend doesn't care how you obtained the token**. Whether you:
- Generated it manually in the Dropbox console
- Obtained it through OAuth2 flow
- Refreshed it using a refresh token

As long as you have a valid access token string, pass it to `WithAccessToken()` and the backend works identically.

```go
// These all work the same way:

// 1. Manual token (testing)
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken("manually-generated-token"),
)

// 2. OAuth2 token (production)
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(oauth2Token.AccessToken),
)

// 3. Refreshed token (production)
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(refreshedToken.AccessToken),
)
```

## Configuration Options

### WithAccessToken

Sets the OAuth2 access token for API authentication (required).

```go
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken("your-token"),
)
```

### WithChunkSize

Sets the chunk size for uploading large files. Default is 4MB.

```go
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(token),
    dropbox.WithChunkSize(8 * 1024 * 1024), // 8MB chunks
)
```

### WithTempDir

Sets the directory for temporary files during read/write operations. Defaults to `os.TempDir()`.

```go
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(token),
    dropbox.WithTempDir("/custom/temp/dir"),
)
```

### WithClient

Provides a custom Dropbox client. Useful for testing.

```go
fs := dropbox.NewFileSystem(
    dropbox.WithClient(mockClient),
)
```

## URI Format

```
dbx:///path/to/file.txt
dbx:///path/to/folder/
```

- **Scheme**: `dbx` (Dropbox's stock ticker symbol)
- **Authority**: Empty (Dropbox uses a single namespace per token)
- **Path**: Absolute Dropbox path starting with `/`

## API Limitations and Workarounds

The Dropbox API has several limitations that affect VFS operations:

### 1. No Range Reads ⚠️

**Limitation**: Dropbox does not support HTTP range requests.

**Impact**: Cannot seek within a file without downloading the entire file.

**Workaround**: The backend automatically downloads the entire file to a temporary location when `Read()` or `Seek()` is called. This allows full `io.Seeker` support but requires local disk space.

```go
file, _ := fs.NewFile("", "/large-file.bin")
defer file.Close()

// Downloads entire file to temp location
file.Seek(1000, io.SeekStart)
buf := make([]byte, 100)
file.Read(buf) // Reads from temp file
```

### 2. No Append Mode ⚠️

**Limitation**: Files must be uploaded in their entirety.

**Impact**: Cannot append to existing files.

**Workaround**: All write operations buffer to a temporary file and upload on `Close()`. For editing existing files, the backend downloads the file first if needed.

```go
file, _ := fs.NewFile("", "/file.txt")
defer file.Close()

// Writes to temp file
file.Write([]byte("content"))

// Uploads entire temp file to Dropbox
file.Close()
```

### 3. 150MB Upload Limit ⚠️

**Limitation**: Simple uploads are limited to 150MB.

**Impact**: Large files require chunked upload sessions.

**Workaround**: The backend automatically uses chunked uploads for files >150MB, uploading in configurable chunks (default 4MB).

```go
// Automatically uses chunked upload for large files
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(token),
    dropbox.WithChunkSize(8 * 1024 * 1024), // 8MB chunks
)

file, _ := fs.NewFile("", "/large-file.bin")
// Write 200MB of data - automatically chunked
```

### 4. Case-Insensitive Paths 📝

**Limitation**: Dropbox paths are case-insensitive but case-preserving.

**Impact**: `/path/File.txt` and `/path/file.txt` refer to the same file.

**Workaround**: None needed. Be aware of this behavior in your application logic.

### 5. No Empty Directories 📝

**Limitation**: Dropbox doesn't store empty folders as entities.

**Impact**: Empty directories don't exist; they're just path prefixes when files are present.

**Workaround**: `Location.Exists()` checks for folder metadata but may return false for empty folders.

### 6. Expensive Touch Operation ⚠️

**Limitation**: No API to update timestamp without modifying content.

**Impact**: `Touch()` on existing files requires download and re-upload.

**Timestamp Behavior**: Even with explicit `client_modified` set during upload, Dropbox may not update the file's timestamp when re-uploading identical content. This appears to be server-side optimization behavior.

**Workaround**: Avoid using `Touch()` on large existing files. Creating new files is efficient.

```go
file, _ := fs.NewFile("", "/file.txt")

// Efficient for new files
file.Touch() // Creates empty file

// Expensive for existing files (downloads + re-uploads)
existingFile, _ := fs.NewFile("", "/existing.txt")
existingFile.Touch() // Downloads, then re-uploads entire file
                      // Note: timestamp may not change if content is identical
```

## Examples

### Reading a File

```go
fs := dropbox.NewFileSystem(
    dropbox.WithAccessToken(token),
)

file, err := fs.NewFile("", "/data.txt")
if err != nil {
    return err
}
defer file.Close()

content, err := io.ReadAll(file)
if err != nil {
    return err
}

fmt.Println(string(content))
```

### Writing a File

```go
file, err := fs.NewFile("", "/output.txt")
if err != nil {
    return err
}
defer file.Close()

_, err = file.Write([]byte("Hello, World!"))
if err != nil {
    return err
}

// Must close to upload
return file.Close()
```

### Listing Files

```go
location, err := fs.NewLocation("", "/my-folder/")
if err != nil {
    return err
}

files, err := location.List()
if err != nil {
    return err
}

for _, filename := range files {
    fmt.Println(filename)
}
```

### Copying Files (Native)

```go
source, _ := fs.NewFile("", "/source.txt")
target, _ := fs.NewFile("", "/target.txt")

// Uses Dropbox's native CopyV2 API (efficient)
err := source.CopyToFile(target)
```

### Moving Files (Native)

```go
source, _ := fs.NewFile("", "/old-location.txt")
target, _ := fs.NewFile("", "/new-location.txt")

// Uses Dropbox's native MoveV2 API (efficient)
err := source.MoveToFile(target)
```

### Seeking in Files

```go
file, _ := fs.NewFile("", "/data.bin")
defer file.Close()

// Downloads entire file to temp location
file.Seek(1000, io.SeekStart)

buf := make([]byte, 100)
n, err := file.Read(buf)
```

## Performance Considerations

### Temporary Disk Space

The backend requires temporary disk space for:
- **Read operations**: Downloads entire file when `Read()` or `Seek()` is called
- **Write operations**: Buffers all writes until `Close()`

Ensure sufficient disk space is available, especially for large files.

### Network Usage

- **Every read requires full download**: Even seeking to read a small portion downloads the entire file
- **Every write requires full upload**: Entire file is uploaded on `Close()`
- **Touch on existing files**: Downloads then re-uploads entire file

### Memory Usage

The backend streams data through temporary files rather than holding it in memory, keeping memory usage low even for large files.

## Error Handling

All errors are wrapped with context using the VFS utility functions:

```go
file, err := fs.NewFile("", "/file.txt")
if err != nil {
    // Error includes context like "new file error"
    return fmt.Errorf("failed to create file: %w", err)
}
```

Common error scenarios:
- **Authentication errors**: Invalid or expired access token
- **Path not found**: File or folder doesn't exist
- **Rate limiting**: Too many API requests
- **Insufficient space**: Dropbox storage quota exceeded

## Testing

### Unit Tests

```bash
# Run all unit tests
go test -v ./contrib/backend/dropbox/

# Run specific test suite
go test -v ./contrib/backend/dropbox/ -run TestFileSystemTestSuite
```

### Integration Tests

Integration tests require a real Dropbox access token:

```bash
# Set your access token
export VFS_DROPBOX_ACCESS_TOKEN="your-token-here"

# Set test location
export VFS_INTEGRATION_LOCATIONS="dbx:///vfs-test/"

# Run integration tests
go test -v ./backend/testsuite/ -run TestIOSuite
```

## Thread Safety

Individual file and location objects are not thread-safe. Do not share them across goroutines without synchronization. However, you can safely create multiple `FileSystem`, `Location`, and `File` instances across different goroutines.

## Contributing

Contributions are welcome! Please ensure:
- All tests pass
- New features include tests
- Code follows Go best practices
- Documentation is updated

## License

This backend is part of the VFS library and follows the same license.

## Resources

- [VFS Documentation](https://github.com/c2fo/vfs)
- [Dropbox API Documentation](https://www.dropbox.com/developers/documentation/http/overview)
- [Dropbox SDK for Go](https://github.com/dropbox/dropbox-sdk-go-unofficial)
