package fsnotify

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/vfssimple"
)

// TestVfsPathToNativeOS covers the VFS-path -> native-OS-path conversion used before handing a
// path to fsnotify.Add, including the Windows drive-letter case from
// https://github.com/C2FO/vfs/issues/344. It is parameterized on GOOS so the Windows behavior
// can be verified without actually running on Windows.
func TestVfsPathToNativeOS(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		goos     string
		expected string
	}{
		{
			name:     "empty path",
			path:     "",
			goos:     "windows",
			expected: "",
		},
		{
			name:     "windows drive-letter path strips leading slash and uses backslashes",
			path:     "/C:/Temp/TestDebouncingEdgeCases3938112569/001",
			goos:     "windows",
			expected: `C:\Temp\TestDebouncingEdgeCases3938112569\001`,
		},
		{
			name:     "windows root of drive with trailing slash",
			path:     "/C:/",
			goos:     "windows",
			expected: `C:\`,
		},
		{
			name:     "windows root of drive without trailing slash keeps drive-root meaning",
			path:     "/C:",
			goos:     "windows",
			expected: `C:\`,
		},
		{
			name:     "non-windows GOOS is a no-op",
			path:     "/C:/Temp/foo",
			goos:     "linux",
			expected: "/C:/Temp/foo",
		},
		{
			name:     "unix path unaffected on non-windows GOOS",
			path:     "/tmp/foo/bar",
			goos:     "darwin",
			expected: "/tmp/foo/bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, vfsPathToNativeOS(tt.path, tt.goos))
		})
	}
}

// TestResolveWatchPathOS covers deriving the native watch path from a vfs.Location, including
// the Windows drive-letter case from https://github.com/C2FO/vfs/issues/344. It is parameterized
// on GOOS so the Windows behavior can be verified without actually running on Windows.
//
// Location.Path() is purely virtual bookkeeping (it never touches the real filesystem), so a
// Windows-style file:// URI can be built and inspected on any host OS.
func TestResolveWatchPathOS(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		goos        string
		expected    string
		expectedErr string
	}{
		{
			name:     "windows drive-letter location",
			uri:      "file:///C:/Temp/foo/",
			goos:     "windows",
			expected: `C:\Temp\foo`,
		},
		{
			name:     "windows drive-root location",
			uri:      "file:///C:/",
			goos:     "windows",
			expected: `C:\`,
		},
		{
			name:     "posix location",
			uri:      "file:///tmp/foo/",
			goos:     "linux",
			expected: "/tmp/foo",
		},
		{
			name:        "root location is invalid",
			uri:         "file:///",
			goos:        "linux",
			expectedErr: "invalid file location URI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := vfssimple.NewLocation(tt.uri)
			require.NoError(t, err)

			native, err := resolveWatchPathOS(loc, tt.goos)
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, native)
		})
	}
}

// TestNativePathToURIOS covers the native-OS-path -> "file://" URI conversion applied to
// fsnotify event paths, including the Windows drive-letter case from
// https://github.com/C2FO/vfs/issues/344. It is parameterized on GOOS so the Windows behavior
// can be verified without actually running on Windows.
func TestNativePathToURIOS(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		goos      string
		authority string
		expected  string
	}{
		{
			name:     "windows drive-letter path gains leading slash and forward slashes",
			path:     `C:\Temp\TestDebouncingEdgeCases3938112569\001\test.txt`,
			goos:     "windows",
			expected: "file:///C:/Temp/TestDebouncingEdgeCases3938112569/001/test.txt",
		},
		{
			name:     "windows root of drive",
			path:     `C:\`,
			goos:     "windows",
			expected: "file:///C:/",
		},
		{
			name:     "non-windows GOOS is a simple prefix",
			path:     "/tmp/foo/bar.txt",
			goos:     "linux",
			expected: "file:///tmp/foo/bar.txt",
		},
		{
			name:     "unix path unaffected on non-windows GOOS",
			path:     "/tmp/foo/bar.txt",
			goos:     "darwin",
			expected: "file:///tmp/foo/bar.txt",
		},
		{
			name:      "non-empty authority is preserved",
			path:      `C:\Temp\bar.txt`,
			goos:      "windows",
			authority: "myhost",
			expected:  "file://myhost/C:/Temp/bar.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, nativePathToURIOS(tt.path, tt.goos, tt.authority))
		})
	}
}

// TestPathConversionRoundTrip verifies that a VFS URI path round-trips through
// vfsPathToNativeOS and back through nativePathToURIOS to the original file:// URI, for both
// Windows drive-letter paths and POSIX-style paths.
func TestPathConversionRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		goos string
	}{
		{
			name: "windows drive-letter path",
			uri:  "file:///C:/Temp/TestDebouncingEdgeCases3938112569/001",
			goos: "windows",
		},
		{
			name: "posix path",
			uri:  "file:///tmp/TestDebouncingEdgeCases3938112569/001",
			goos: "linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vfsPath := tt.uri[len("file://"):]
			native := vfsPathToNativeOS(vfsPath, tt.goos)
			assert.Equal(t, tt.uri, nativePathToURIOS(native, tt.goos, ""))
		})
	}
}
