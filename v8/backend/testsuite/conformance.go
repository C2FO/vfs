package testsuite

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/utils"
	vfs "github.com/c2fo/vfs/v8"
)

// ConformanceOptions configures conformance behavior for backends with known limitations.
type ConformanceOptions struct {
	// SkipTouchTimestampTest skips the Touch timestamp update assertion.
	SkipTouchTimestampTest bool

	// SkipFTPSpecificTests skips tests that assume non-FTP semantics (e.g. io.CopyBuffer paths).
	SkipFTPSpecificTests bool
}

// RunConformanceTests runs FileSystem, Location, and File conformance tests against baseLoc.
func RunConformanceTests(t *testing.T, baseLoc vfs.Location, opts ...ConformanceOptions) {
	t.Helper()
	opt := ConformanceOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	t.Run("FileSystem", func(t *testing.T) {
		RunFileSystemTests(t, baseLoc)
	})

	t.Run("Location", func(t *testing.T) {
		RunLocationTests(t, baseLoc)
	})

	t.Run("File", func(t *testing.T) {
		RunFileTests(t, baseLoc, opt)
	})
}

// RunFileSystemTests tests [vfs.FileSystem] validation and URI formatting.
func RunFileSystemTests(t *testing.T, baseLoc vfs.Location) {
	t.Helper()
	fs := baseLoc.FileSystem()

	authority := baseLoc.Authority().String()

	filepaths := map[string]bool{
		"/path/to/file.txt":    true,
		"/path/./to/file.txt":  true,
		"/path/../to/file.txt": true,
		"path/to/file.txt":     false,
		"./path/to/file.txt":   false,
		"../path/to/":          false,
		"/path/to/":            false,
		"":                     false,
	}
	for name, validates := range filepaths {
		file, err := fs.NewFile(authority, name)
		if validates {
			require.NoError(t, err, "scheme=%s name=%s", fs.Scheme(), name)
			expected := buildExpectedURI(fs, authority, path.Clean(name))
			assert.Equal(t, expected, file.URI(), "uri mismatch")
		} else {
			require.Error(t, err, "expect validation error scheme=%s name=%s", fs.Scheme(), name)
		}
	}

	locpaths := map[string]bool{
		"/path/to/":         true,
		"/path/./to/":       true,
		"/path/../to/":      true,
		"path/to/":          false,
		"./path/to/":        false,
		"../path/to/":       false,
		"/path/to/file.txt": false,
		"":                  false,
	}
	for name, validates := range locpaths {
		loc, err := fs.NewLocation(authority, name)
		if validates {
			require.NoError(t, err, "scheme=%s name=%s", fs.Scheme(), name)
			expected := buildExpectedURI(fs, authority, utils.EnsureTrailingSlash(path.Clean(name)))
			assert.Equal(t, expected, loc.URI(), "uri mismatch")
		} else {
			require.Error(t, err, "expect validation error scheme=%s name=%s", fs.Scheme(), name)
		}
	}
}

// RunLocationTests tests [vfs.Location] navigation, [vfs.List], and [vfs.Location.DeleteFile].
func RunLocationTests(t *testing.T, baseLoc vfs.Location) {
	t.Helper()
	ctx := context.Background()

	srcLoc, err := baseLoc.NewLocation("locTestSrc/")
	require.NoError(t, err)

	defer func() {
		if baseLoc.FileSystem().Scheme() == "file" {
			exists, err := srcLoc.Exists()
			require.NoError(t, err)
			if exists {
				require.NoError(t, os.RemoveAll(srcLoc.Path()), "cleanup srcLoc")
			}
		}
	}()

	locpaths := map[string]bool{
		"/path/to/":         false,
		"/path/./to/":       false,
		"/path/../to/":      false,
		"path/to/":          true,
		"./path/to/":        true,
		"../path/to/":       true,
		"/path/to/file.txt": false,
		"":                  false,
	}
	for name, validates := range locpaths {
		loc, err := srcLoc.NewLocation(name)
		if validates {
			require.NoError(t, err)
			expected := buildExpectedURI(srcLoc.FileSystem(), baseLoc.Authority().String(),
				utils.EnsureTrailingSlash(path.Clean(path.Join(srcLoc.Path(), name))))
			assert.Equal(t, expected, loc.URI())
		} else {
			require.Error(t, err)
		}
	}

	filepaths := map[string]bool{
		"/path/to/file.txt":    false,
		"/path/./to/file.txt":  false,
		"/path/../to/file.txt": false,
		"path/to/file.txt":     true,
		"./path/to/file.txt":   true,
		"../path/to/":          false,
		"../path/to/file.txt":  true,
		"/path/to/":            false,
		"":                     false,
	}
	for name, validates := range filepaths {
		file, err := srcLoc.NewFile(name)
		if validates {
			require.NoError(t, err)
			expected := buildExpectedURI(srcLoc.FileSystem(), srcLoc.Authority().String(),
				path.Clean(path.Join(srcLoc.Path(), name)))
			assert.Equal(t, expected, file.URI())
		} else {
			require.Error(t, err)
		}
	}

	cdTestLoc, err := srcLoc.NewLocation("chdirTest/")
	require.NoError(t, err)

	_, err = cdTestLoc.NewLocation("")
	require.Error(t, err)
	_, err = cdTestLoc.NewLocation("/home/")
	require.Error(t, err)
	_, err = cdTestLoc.NewLocation("file.txt")
	require.Error(t, err)

	cdTestLoc, err = cdTestLoc.NewLocation("l1dir1/./l2dir1/../l2dir2/")
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(cdTestLoc.Path(), "locTestSrc/chdirTest/l1dir1/l2dir2/"))
	assert.True(t, strings.HasPrefix(cdTestLoc.Path(), "/"))
	assert.True(t, strings.HasSuffix(cdTestLoc.URI(), "locTestSrc/chdirTest/l1dir1/l2dir2/"))
	prefix := cdTestLoc.FileSystem().Scheme() + "://"
	assert.True(t, strings.HasPrefix(cdTestLoc.URI(), prefix))

	exists, err := baseLoc.Exists()
	require.NoError(t, err)
	assert.True(t, exists)

	f1, err := srcLoc.NewFile("file1.txt")
	require.NoError(t, err)
	_, err = f1.Write([]byte("this is a test file"))
	require.NoError(t, err)
	require.NoError(t, f1.Close())

	f2, err := srcLoc.NewFile("file2.txt")
	require.NoError(t, err)
	_, err = f2.Write([]byte("this is a test file"))
	require.NoError(t, err)
	require.NoError(t, f2.Close())

	f3, err := srcLoc.NewFile("self.txt")
	require.NoError(t, err)
	_, err = f3.Write([]byte("this is a test file"))
	require.NoError(t, err)
	require.NoError(t, f3.Close())

	subLoc, err := srcLoc.NewLocation("somepath/")
	require.NoError(t, err)

	f4, err := subLoc.NewFile("that.txt")
	require.NoError(t, err)
	_, err = f4.Write([]byte("this is a test file"))
	require.NoError(t, err)
	require.NoError(t, f4.Close())

	files, err := CollectList(ctx, srcLoc)
	require.NoError(t, err)
	assert.Len(t, files, 3)

	files, err = CollectList(ctx, subLoc)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "that.txt", files[0])

	files, err = CollectList(ctx, cdTestLoc)
	require.NoError(t, err)
	assert.Empty(t, files)

	files, err = CollectList(ctx, srcLoc, vfs.WithPrefix("file"))
	require.NoError(t, err)
	assert.Len(t, files, 2)

	files, err = CollectList(ctx, srcLoc, vfs.WithPrefix("s"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "self.txt", files[0])

	files, err = CollectList(ctx, srcLoc, vfs.WithPrefix("somepath/t"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "that.txt", files[0])

	files, err = CollectList(ctx, cdTestLoc)
	require.NoError(t, err)
	assert.Empty(t, files)

	files, err = CollectList(ctx, srcLoc, vfs.WithRegexp(regexp.MustCompile("^f")))
	require.NoError(t, err)
	assert.Len(t, files, 2)

	files, err = CollectList(ctx, srcLoc, vfs.WithRegexp(regexp.MustCompile(`.txt$`)))
	require.NoError(t, err)
	assert.Len(t, files, 3)

	files, err = CollectList(ctx, srcLoc, vfs.WithRegexp(regexp.MustCompile(`Z`)))
	require.NoError(t, err)
	assert.Empty(t, files)

	require.NoError(t, srcLoc.DeleteFile(f1.Name()))
	require.NoError(t, srcLoc.DeleteFile(f2.Name()))
	require.NoError(t, srcLoc.DeleteFile(f3.Name()))
	require.NoError(t, srcLoc.DeleteFile("somepath/that.txt"))

	require.Error(t, srcLoc.DeleteFile(f1.Path()))
}

// RunFileTests runs core [vfs.File] conformance: I/O, metadata, Stat, Touch, and cleanup via [vfs.Location.DeleteFile].
func RunFileTests(t *testing.T, baseLoc vfs.Location, opts ConformanceOptions) {
	t.Helper()

	srcLoc, err := baseLoc.NewLocation("fileTestSrc/")
	require.NoError(t, err)

	defer func() {
		if srcLoc.FileSystem().Scheme() == "file" {
			exists, err := srcLoc.Exists()
			require.NoError(t, err)
			if exists {
				require.NoError(t, os.RemoveAll(srcLoc.Path()), "cleanup file test srcLoc")
			}
		}
	}()

	srcFile, err := srcLoc.NewFile("srcFile.txt")
	require.NoError(t, err)

	sz, err := srcFile.Write([]byte("this is a test\n"))
	require.NoError(t, err)
	assert.Equal(t, 15, sz)
	sz, err = srcFile.Write([]byte("and more text"))
	require.NoError(t, err)
	assert.Equal(t, 13, sz)

	require.NoError(t, srcFile.Close())

	exists, err := srcFile.Exists()
	require.NoError(t, err)
	assert.True(t, exists)

	assert.Equal(t, "srcFile.txt", srcFile.Name())

	assert.Equal(t, path.Join(baseLoc.Path(), "fileTestSrc/srcFile.txt"), srcFile.Path())

	assert.Equal(t, baseLoc.URI()+"fileTestSrc/srcFile.txt", srcFile.URI())
	assert.Equal(t, baseLoc.URI()+"fileTestSrc/srcFile.txt", srcFile.String())

	st, err := srcFile.Stat()
	require.NoError(t, err)
	assert.Equal(t, "srcFile.txt", st.Name())

	b, err := srcFile.Size()
	require.NoError(t, err)
	assert.Equal(t, uint64(28), b)

	tm, err := srcFile.LastModified()
	require.NoError(t, err)
	require.NotNil(t, tm)

	str, err := io.ReadAll(srcFile)
	require.NoError(t, err)
	assert.Equal(t, "this is a test\nand more text", string(str))

	if seeker, ok := srcFile.(io.Seeker); ok {
		offset, err := seeker.Seek(3, 0)
		require.NoError(t, err)
		assert.Equal(t, int64(3), offset)

		str, err = io.ReadAll(srcFile)
		require.NoError(t, err)
		assert.Equal(t, "s is a test\nand more text", string(str))
		require.NoError(t, srcFile.Close())

		_, err = seeker.Seek(0, 0)
		require.NoError(t, err)

		if !opts.SkipFTPSpecificTests && srcLoc.FileSystem().Scheme() != "ftp" {
			copyFile1, err := srcLoc.NewFile("copyFile1.txt")
			require.NoError(t, err)
			buffer := make([]byte, utils.TouchCopyMinBufferSize)
			n, err := io.CopyBuffer(copyFile1, srcFile, buffer)
			require.NoError(t, err)
			assert.Equal(t, int64(28), n)
			require.NoError(t, copyFile1.Close())
			ex, err := copyFile1.Exists()
			require.NoError(t, err)
			assert.True(t, ex)
			require.NoError(t, srcLoc.DeleteFile("copyFile1.txt"))
		}
	} else {
		require.NoError(t, srcFile.Close())

		if !opts.SkipFTPSpecificTests && srcLoc.FileSystem().Scheme() != "ftp" {
			srcAgain, err := srcLoc.NewFile("srcFile.txt")
			require.NoError(t, err)
			copyFile1, err := srcLoc.NewFile("copyFile1.txt")
			require.NoError(t, err)
			buffer := make([]byte, utils.TouchCopyMinBufferSize)
			n, err := io.CopyBuffer(copyFile1, srcAgain, buffer)
			require.NoError(t, err)
			assert.Equal(t, int64(28), n)
			require.NoError(t, copyFile1.Close())
			require.NoError(t, srcAgain.Close())
			ex, err := copyFile1.Exists()
			require.NoError(t, err)
			assert.True(t, ex)
			require.NoError(t, srcLoc.DeleteFile("copyFile1.txt"))
		}
	}

	touchedFile, err := srcLoc.NewFile("touch.txt")
	require.NoError(t, err)
	defer func() { _ = srcLoc.DeleteFile("touch.txt") }()

	ex, err := touchedFile.Exists()
	require.NoError(t, err)
	assert.False(t, ex)

	require.NoError(t, touchedFile.Touch())
	ex, err = touchedFile.Exists()
	require.NoError(t, err)
	assert.True(t, ex)

	size, err := touchedFile.Size()
	require.NoError(t, err)
	assert.Zero(t, size)

	if !opts.SkipTouchTimestampTest {
		modified, err := touchedFile.LastModified()
		require.NoError(t, err)
		modifiedDeref := *modified
		time.Sleep(2 * time.Second)
		require.NoError(t, touchedFile.Touch())
		newModified, err := touchedFile.LastModified()
		require.NoError(t, err)
		assert.Greater(t, *newModified, modifiedDeref)
	}

	require.NoError(t, srcLoc.DeleteFile("srcFile.txt"))
	exists, err = srcFile.Exists()
	require.NoError(t, err)
	assert.False(t, exists)

	badFile, err := srcLoc.NewFile("thisFileDoesNotExist")
	require.NoError(t, err)

	ex, err = badFile.Exists()
	require.NoError(t, err)
	assert.False(t, ex)

	badSize, err := badFile.Size()
	require.Error(t, err)
	assert.Zero(t, badSize)

	_, err = badFile.LastModified()
	require.Error(t, err)

	if sk, ok := badFile.(io.Seeker); ok {
		seeked, err := sk.Seek(-1, 2)
		require.Error(t, err)
		assert.Zero(t, seeked)
	}

	_, err = badFile.Read(make([]byte, 1))
	require.Error(t, err)
}

func buildExpectedURI(fs vfs.FileSystem, authorityStr, p string) string {
	return fmt.Sprintf("%s://%s%s", fs.Scheme(), authorityStr, p)
}
