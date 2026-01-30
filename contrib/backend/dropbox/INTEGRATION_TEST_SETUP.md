# Dropbox Integration Test Setup

This guide explains how to run the VFS integration tests with the Dropbox backend.

## Prerequisites

1. **Dropbox Account**: You need a Dropbox account
2. **Access Token**: Generate an OAuth2 access token for your app

## Step 1: Create a Dropbox App

1. Go to https://www.dropbox.com/developers/apps
2. Click "Create App"
3. Choose:
   - **API**: Scoped access
   - **Access**: Full Dropbox (or App folder if you prefer isolated testing)
   - **Name**: Choose a name (e.g., "VFS Integration Tests")
4. Click "Create App"

### Configure Required Permissions

**CRITICAL**: After creating your app, you must enable the required permissions:

1. In your app's settings, go to the **"Permissions"** tab
2. Enable these scopes:
   - ✅ **files.metadata.read** - View metadata for files and folders
   - ✅ **files.content.read** - View content of files and folders
   - ✅ **files.content.write** - Create, edit, and delete files and folders
3. Click **"Submit"** at the bottom of the permissions page
4. **Important**: You must regenerate your access token after changing permissions

## Step 2: Generate Access Token

1. In your app's settings page, go to the **"Settings"** tab
2. Scroll to "OAuth 2" section
3. Under "Generated access token", click **"Generate"**
4. Copy the token (it will only be shown once)
5. **Important**: Keep this token secure and never commit it to source control

**Note**: If you changed permissions after generating a token, you must regenerate the token for the new permissions to take effect. Old tokens will continue to work with their original permissions only.

## Step 3: Set Environment Variables

```bash
# Set your Dropbox access token
export VFS_DROPBOX_ACCESS_TOKEN="your-access-token-here"

# Set the test location (must end with /)
export VFS_INTEGRATION_LOCATIONS="dbx:///vfs-test/"
```

### Multiple Backends

You can test multiple backends simultaneously by separating URIs with semicolons:

```bash
export VFS_INTEGRATION_LOCATIONS="dbx:///vfs-test/;s3://my-bucket/vfs-test/;file:///tmp/vfs-test/"
```

## Step 4: Create Test Directory

The integration tests will create files in the specified location. Ensure the directory exists or will be created:

```bash
# The test suite will create this directory if it doesn't exist
# For Dropbox, you can create it manually or let the tests create it
```

## Step 5: Run Integration Tests

```bash
# Run all integration tests
go test -v -tags=vfsintegration ./backend/testsuite/

# Run only Dropbox tests (if you set only dbx in VFS_INTEGRATION_LOCATIONS)
go test -v -tags=vfsintegration ./backend/testsuite/ -run TestIOSuite
```

## What the Tests Do

The integration tests (`io_integration_test.go`) run 24 different scenarios testing:

1. **Write Operations**
   - Creating new files
   - Overwriting existing files
   - Appending to files

2. **Read Operations**
   - Reading file content
   - Reading after writes
   - Handling EOF

3. **Seek Operations**
   - Seeking before/after reads
   - Seeking before/after writes
   - Different seek positions (start, current, end)

4. **Combined Operations**
   - Write → Seek → Read
   - Read → Seek → Write
   - Multiple operations in sequence

5. **Edge Cases**
   - Empty files
   - Large seeks
   - Cursor positioning

## Expected Behavior

All 24 test cases should pass. The tests validate that the Dropbox backend behaves consistently with other VFS backends for all I/O operations.

## Troubleshooting

### Authentication Errors

```
Error: access token is required for Dropbox authentication
```

**Solution**: Ensure `VFS_DROPBOX_ACCESS_TOKEN` is set correctly

### Permission Errors

```
Error: Your app is not permitted to access this endpoint because it does not have the required scope 'files.content.write'
```

**Solution**: 
1. Your Dropbox app is missing required permissions
2. Go to your app's **Permissions** tab at https://www.dropbox.com/developers/apps
3. Enable all three required scopes:
   - `files.metadata.read`
   - `files.content.read`
   - `files.content.write`
4. Click **"Submit"**
5. **Regenerate your access token** (Settings tab → OAuth 2 → Generate)
6. Update `VFS_DROPBOX_ACCESS_TOKEN` with the new token

### Path Not Found

```
Error: path/not_found
```

**Solution**: 
- Ensure the test path exists or the token has permissions to create it
- Check that the path in `VFS_INTEGRATION_LOCATIONS` is correct

### Rate Limiting

```
Error: too_many_requests
```

**Solution**: 
- Wait a moment and retry manually
- Consider implementing application-level retry logic for rate-limited requests

### Temp Directory Issues

```
Error: no space left on device
```

**Solution**: 
- The Dropbox backend downloads entire files to temp storage
- Ensure you have sufficient disk space in your temp directory
- Optionally set a custom temp dir with `WithTempDir()`

## Cleanup

After running tests, you may want to clean up the test directory:

```bash
# Manually delete the test directory from Dropbox web interface
# or use the VFS to clean it up programmatically
```

## Security Notes

1. **Never commit your access token** to source control
2. **Use short-lived tokens** when possible (implement OAuth2 flow)
3. **Restrict token permissions** to only what's needed for testing
4. **Rotate tokens regularly** for security
5. **Consider using app folders** instead of full Dropbox access for testing

## Continuous Integration

For CI/CD pipelines:

```yaml
# GitHub Actions example
- name: Run Dropbox Integration Tests
  env:
    VFS_DROPBOX_ACCESS_TOKEN: ${{ secrets.VFS_DROPBOX_ACCESS_TOKEN }}
    VFS_INTEGRATION_LOCATIONS: "dbx:///vfs-ci-test/"
  run: |
    go test -v -tags=vfsintegration ./backend/testsuite/
```

Store the access token as a secret in your CI system.

## Performance Expectations

Dropbox integration tests will be slower than local filesystem tests due to:
- Network latency
- Full file downloads for seek operations
- Full file uploads on close
- API rate limiting

This is expected behavior given the Dropbox API limitations.
