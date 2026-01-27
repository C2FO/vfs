// Package testcontainers provides conformance tests for VFS backend implementations.
//
// These tests can be imported by any backend (core or contrib) to verify
// correct implementation of the vfs.FileSystem, vfs.Location, and vfs.File interfaces.
//
// Usage:
//
//	//go:build vfsintegration
//
//	package mybackend
//
//	import (
//	    "testing"
//	    "github.com/c2fo/vfs/v7/testcontainers"
//	)
//
//	func TestConformance(t *testing.T) {
//	    fs := NewFileSystem(/* options */)
//	    loc, _ := fs.NewLocation("", "/test-path/")
//	    testcontainers.RunConformanceTests(t, loc)
//	}
package testcontainers

import (
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/utils"
)

// ConformanceOptions configures conformance test behavior
type ConformanceOptions struct {
	// SkipTouchTimestampTest skips the Touch timestamp update assertion.
	// Some backends (e.g., Dropbox) may not update timestamps when content is unchanged.
	SkipTouchTimestampTest bool

	// SkipFTPSpecificTests skips tests that don't work well with FTP
	SkipFTPSpecificTests bool
}

// RunConformanceTests runs all conformance tests against the provided location.
// This is the main entry point for backend conformance testing.
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

// RunFileSystemTests tests vfs.FileSystem interface conformance
func RunFileSystemTests(t *testing.T, baseLoc vfs.Location) {
	t.Helper()
	fs := baseLoc.FileSystem()

	// NewFile initializes a File on the specified Authority string at path 'absFilePath'.
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
		file, err := fs.NewFile(baseLoc.Authority().String(), name)
		if validates {
			require.NoError(t, err, "there should be no error")
			expected := buildExpectedURI(fs, baseLoc.Authority().String(), path.Clean(name))
			assert.Equal(t, expected, file.URI(), "uri's should match")
		} else {
			require.Error(t, err, "should have validation error for scheme[%s] and name[%s]", fs.Scheme(), name)
		}
	}

	// NewLocation initializes a Location on the specified authority with the given path.
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
		loc, err := fs.NewLocation(baseLoc.Authority().String(), name)
		if validates {
			require.NoError(t, err, "there should be no error")
			expected := buildExpectedURI(fs, baseLoc.Authority().String(), utils.EnsureTrailingSlash(path.Clean(name)))
			assert.Equal(t, expected, loc.URI(), "uri's should match")
		} else {
			require.Error(t, err, "should have validation error for scheme[%s] and name[%s]", fs.Scheme(), name)
		}
	}
}

// RunLocationTests tests vfs.Location interface conformance
func RunLocationTests(t *testing.T, baseLoc vfs.Location) {
	t.Helper()

	srcLoc, err := baseLoc.NewLocation("locTestSrc/")
	require.NoError(t, err, "there should be no error")
	defer func() {
		// clean up srcLoc after test for OS
		if srcLoc.FileSystem().Scheme() == "file" {
			exists, err := srcLoc.Exists()
			require.NoError(t, err)
			if exists {
				require.NoError(t, os.RemoveAll(srcLoc.Path()), "failed to clean up location test srcLoc")
			}
		}
	}()

	// NewLocation is an initializer for a new Location relative to the existing one.
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
			require.NoError(t, err, "there should be no error")
			expected := buildExpectedURI(srcLoc.FileSystem(), baseLoc.Authority().String(),
				utils.EnsureTrailingSlash(path.Clean(path.Join(srcLoc.Path(), name))))
			assert.Equal(t, expected, loc.URI(), "uri's should match")
		} else {
			require.Error(t, err, "should have validation error for scheme and name: %s : %s", srcLoc.FileSystem().Scheme(), name)
		}
	}

	// NewFile will instantiate a vfs.File instance at or relative to the current location's path.
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
			require.NoError(t, err, "there should be no error")
			expected := buildExpectedURI(srcLoc.FileSystem(), srcLoc.Authority().String(), path.Clean(path.Join(srcLoc.Path(), name)))
			assert.Equal(t, expected, file.URI(), "uri's should match")
		} else {
			require.Error(t, err, "should have validation error for scheme and name: %s : +%s+", srcLoc.FileSystem().Scheme(), name)
		}
	}

	// ChangeDir / NewLocation tests
	cdTestLoc, err := srcLoc.NewLocation("chdirTest/")
	require.NoError(t, err)

	_, err = cdTestLoc.NewLocation("")
	require.Error(t, err, "empty string should error")
	_, err = cdTestLoc.NewLocation("/home/")
	require.Error(t, err, "absolute path should error")
	_, err = cdTestLoc.NewLocation("file.txt")
	require.Error(t, err, "file should error")
	cdTestLoc, err = cdTestLoc.NewLocation("l1dir1/./l2dir1/../l2dir2/")
	require.NoError(t, err, "should be no error for relative path")

	// Path returns absolute location path
	assert.True(t, strings.HasSuffix(cdTestLoc.Path(), "locTestSrc/chdirTest/l1dir1/l2dir2/"), "should end with dot dirs resolved")
	assert.True(t, strings.HasPrefix(cdTestLoc.Path(), "/"), "should start with slash (abs path)")

	// URI returns the fully qualified URI for the Location
	assert.True(t, strings.HasSuffix(cdTestLoc.URI(), "locTestSrc/chdirTest/l1dir1/l2dir2/"), "should end with dot dirs resolved")
	prefix := cdTestLoc.FileSystem().Scheme() + "://"
	assert.True(t, strings.HasPrefix(cdTestLoc.URI(), prefix), "should start with schema and abs slash")

	// Exists
	exists, err := baseLoc.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "baseLoc location exists check")

	// setup list tests
	f1, err := srcLoc.NewFile("file1.txt")
	require.NoError(t, err)
	_, err = f1.Write([]byte("this is a test file"))
	require.NoError(t, err)
	require.NoError(t, f1.Close())

	f2, err := srcLoc.NewFile("file2.txt")
	require.NoError(t, err)
	require.NoError(t, f1.CopyToFile(f2))
	require.NoError(t, f1.Close())

	f3, err := srcLoc.NewFile("self.txt")
	require.NoError(t, err)
	require.NoError(t, f1.CopyToFile(f3))
	require.NoError(t, f1.Close())

	subLoc, err := srcLoc.NewLocation("somepath/")
	require.NoError(t, err)

	f4, err := subLoc.NewFile("that.txt")
	require.NoError(t, err)
	require.NoError(t, f1.CopyToFile(f4))
	require.NoError(t, f1.Close())

	// List
	files, err := srcLoc.List()
	require.NoError(t, err)
	assert.Len(t, files, 3, "list srcLoc location")

	files, err = subLoc.List()
	require.NoError(t, err)
	assert.Len(t, files, 1, "list subLoc location")
	assert.Equal(t, "that.txt", files[0], "returned basename")

	files, err = cdTestLoc.List()
	require.NoError(t, err)
	assert.Empty(t, files, "non-existent location")

	// ListByPrefix
	files, err = srcLoc.ListByPrefix("file")
	require.NoError(t, err)
	assert.Len(t, files, 2, "list srcLoc location matching prefix")

	files, err = srcLoc.ListByPrefix("s")
	require.NoError(t, err)
	assert.Len(t, files, 1, "list srcLoc location")
	assert.Equal(t, "self.txt", files[0], "returned only file basename, not subdir matching prefix")

	files, err = srcLoc.ListByPrefix("somepath/t")
	require.NoError(t, err)
	assert.Len(t, files, 1, "list 'somepath' location relative to srcLoc")
	assert.Equal(t, "that.txt", files[0], "returned only file basename, using relative prefix")

	files, err = cdTestLoc.List()
	require.NoError(t, err)
	assert.Empty(t, files, "non-existent location")

	// ListByRegex
	files, err = srcLoc.ListByRegex(regexp.MustCompile("^f"))
	require.NoError(t, err)
	assert.Len(t, files, 2, "list srcLoc location matching prefix")

	files, err = srcLoc.ListByRegex(regexp.MustCompile(`.txt$`))
	require.NoError(t, err)
	assert.Len(t, files, 3, "list srcLoc location matching prefix")

	files, err = srcLoc.ListByRegex(regexp.MustCompile(`Z`))
	require.NoError(t, err)
	assert.Empty(t, files, "list srcLoc location matching prefix")

	// DeleteFile
	require.NoError(t, srcLoc.DeleteFile(f1.Name()), "deleteFile file1")
	require.NoError(t, srcLoc.DeleteFile(f2.Name()), "deleteFile file2")
	require.NoError(t, srcLoc.DeleteFile(f3.Name()), "deleteFile self.txt")
	require.NoError(t, srcLoc.DeleteFile("somepath/that.txt"), "deleted relative path")

	// should error if file doesn't exist
	require.Error(t, srcLoc.DeleteFile(f1.Path()), "deleteFile trying to delete a file already deleted")
}

// RunFileTests tests vfs.File interface conformance
func RunFileTests(t *testing.T, baseLoc vfs.Location, opts ConformanceOptions) {
	t.Helper()

	srcLoc, err := baseLoc.NewLocation("fileTestSrc/")
	require.NoError(t, err)
	defer func() {
		// clean up srcLoc after test for OS
		if srcLoc.FileSystem().Scheme() == "file" {
			exists, err := srcLoc.Exists()
			require.NoError(t, err)
			if exists {
				require.NoError(t, os.RemoveAll(srcLoc.Path()), "failed to clean up file test srcLoc")
			}
		}
	}()

	// setup srcFile
	srcFile, err := srcLoc.NewFile("srcFile.txt")
	require.NoError(t, err)

	// io.Writer
	sz, err := srcFile.Write([]byte("this is a test\n"))
	require.NoError(t, err)
	assert.Equal(t, 15, sz)
	sz, err = srcFile.Write([]byte("and more text"))
	require.NoError(t, err)
	assert.Equal(t, 13, sz)

	// io.Closer
	err = srcFile.Close()
	require.NoError(t, err)

	// Exists
	exists, err := srcFile.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "file exists")

	// Name
	assert.Equal(t, "srcFile.txt", srcFile.Name(), "name test")

	// Path
	assert.Equal(t, path.Join(baseLoc.Path(), "fileTestSrc/srcFile.txt"), srcFile.Path(), "path test")

	// URI
	assert.Equal(t, baseLoc.URI()+"fileTestSrc/srcFile.txt", srcFile.URI(), "uri test")

	// fmt.Stringer
	assert.Equal(t, baseLoc.URI()+"fileTestSrc/srcFile.txt", srcFile.String(), "string(er) explicit test")
	var stringer fmt.Stringer = srcFile
	assert.Equal(t, baseLoc.URI()+"fileTestSrc/srcFile.txt", stringer.String(), "string(er) implicit test")

	// Size
	b, err := srcFile.Size()
	require.NoError(t, err)
	assert.Equal(t, uint64(28), b)

	// LastModified
	tm, err := srcFile.LastModified()
	require.NoError(t, err)
	assert.IsType(t, (*time.Time)(nil), tm, "last modified returned *time.Time")

	// Exists (again)
	exists, err = srcFile.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "file exists")

	// io.Reader and io.Seeker
	str, err := io.ReadAll(srcFile)
	require.NoError(t, err)
	assert.Equal(t, "this is a test\nand more text", string(str), "read was successful")

	offset, err := srcFile.Seek(3, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), offset, "seek was successful")

	str, err = io.ReadAll(srcFile)
	require.NoError(t, err)
	assert.Equal(t, "s is a test\nand more text", string(str), "read after seek")
	err = srcFile.Close()
	require.NoError(t, err)

	// CopyToLocation - test copying to same location
	dstLoc, err := baseLoc.NewLocation("dstLoc/")
	require.NoError(t, err)
	if dstLoc.FileSystem().Scheme() == "file" {
		t.Cleanup(func() {
			exists, err := dstLoc.Exists()
			require.NoError(t, err)
			if exists {
				require.NoError(t, os.RemoveAll(dstLoc.Path()), "failed to clean up file test dstLoc")
			}
		})
	}

	_, err = srcFile.Seek(0, 0)
	require.NoError(t, err)
	dst, err := srcFile.CopyToLocation(dstLoc)
	require.NoError(t, err)
	exists, err = dst.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "dst file should now exist")
	exists, err = srcFile.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "src file should still exist")

	// CopyToFile
	dstFile1, err := dstLoc.NewFile("dstFile1.txt")
	require.NoError(t, err)
	exists, err = dstFile1.Exists()
	require.NoError(t, err)
	assert.False(t, exists, "dstFile1 file should not yet exist")
	_, err = srcFile.Seek(0, 0)
	require.NoError(t, err)
	err = srcFile.CopyToFile(dstFile1)
	require.NoError(t, err)
	exists, err = dstFile1.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "dstFile1 file should now exist")
	exists, err = srcFile.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "src file should still exist")

	// io.Copy tests (skip for FTP)
	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	copyFile1, err := srcLoc.NewFile("copyFile1.txt")
	require.NoError(t, err)

	if !opts.SkipFTPSpecificTests && srcLoc.FileSystem().Scheme() != "ftp" {
		exists, err = copyFile1.Exists()
		require.NoError(t, err)
		assert.False(t, exists, "copyFile1 should not yet exist locally")

		_, err = srcFile.Seek(0, 0)
		require.NoError(t, err)
		b1, err := io.CopyBuffer(copyFile1, srcFile, buffer)
		require.NoError(t, err)
		assert.Equal(t, int64(28), b1)
		err = copyFile1.Close()
		require.NoError(t, err)

		exists, err = copyFile1.Exists()
		require.NoError(t, err)
		assert.Truef(t, exists, "%s should now exist locally", copyFile1)
		err = copyFile1.Close()
		require.NoError(t, err)
	} else {
		// ensure copyFile1 exists for later tests
		err = copyFile1.Touch()
		require.NoError(t, err)
	}

	copyFile2, err := srcLoc.NewFile("copyFile2.txt")
	require.NoError(t, err)

	if !opts.SkipFTPSpecificTests && srcLoc.FileSystem().Scheme() != "ftp" {
		exists, err = copyFile2.Exists()
		require.NoError(t, err)
		assert.False(t, exists, "copyFile2 should not yet exist locally")

		_, err = srcFile.Seek(0, 0)
		require.NoError(t, err)
		buffer = make([]byte, utils.TouchCopyMinBufferSize)
		b2, err := io.CopyBuffer(copyFile2, srcFile, buffer)
		require.NoError(t, err)
		assert.Equal(t, int64(28), b2)

		err = copyFile2.Close()
		require.NoError(t, err)
		exists, err = copyFile2.Exists()
		require.NoError(t, err)
		assert.True(t, exists, "copyFile2 should now exist locally")
		err = copyFile2.Close()
		require.NoError(t, err)
	} else {
		err = copyFile2.Touch()
		require.NoError(t, err)
	}

	// MoveToLocation tests
	fileForNew, err := srcLoc.NewFile("fileForNew.txt")
	require.NoError(t, err)

	if !opts.SkipFTPSpecificTests && srcLoc.FileSystem().Scheme() != "ftp" {
		_, err = srcFile.Seek(0, 0)
		require.NoError(t, err)
		buffer = make([]byte, utils.TouchCopyMinBufferSize)
		_, err = io.CopyBuffer(fileForNew, srcFile, buffer)
		require.NoError(t, err)
		err = fileForNew.Close()
		require.NoError(t, err)

		newLoc, err := dstLoc.NewLocation("doesnotexist/")
		require.NoError(t, err)
		dstCopyNew, err := fileForNew.MoveToLocation(newLoc)
		require.NoError(t, err)
		exists, err = dstCopyNew.Exists()
		require.NoError(t, err)
		assert.True(t, exists)
		require.NoError(t, dstCopyNew.Delete())
	}

	dstCopy1, err := copyFile1.MoveToLocation(dstLoc)
	require.NoError(t, err)
	exists, err = dstCopy1.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "dstCopy1 file should now exist")
	exists, err = copyFile1.Exists()
	require.NoError(t, err)
	assert.False(t, exists, "copyFile1 should no longer exist locally")

	// MoveToFile
	dstCopy2, err := dstLoc.NewFile("dstFile2.txt")
	require.NoError(t, err)
	exists, err = dstCopy2.Exists()
	require.NoError(t, err)
	assert.False(t, exists, "dstCopy2 file should not yet exist")
	err = copyFile2.MoveToFile(dstCopy2)
	require.NoError(t, err)
	exists, err = copyFile2.Exists()
	require.NoError(t, err)
	assert.False(t, exists, "copyFile2 should no longer exist locally")
	exists, err = dstCopy2.Exists()
	require.NoError(t, err)
	assert.True(t, exists, "dstCopy2 file should now exist")

	// clean up files
	require.NoError(t, dst.Delete())
	require.NoError(t, dstFile1.Delete())
	require.NoError(t, dstCopy1.Delete())
	require.NoError(t, dstCopy2.Delete())

	// MoveToFile with spaces in path
	tests := []struct {
		Path, Filename string
	}{
		{Path: "file/", Filename: "has space.txt"},
		{Path: "file/", Filename: "has%20encodedSpace.txt"},
		{Path: "path has/", Filename: "space.txt"},
		{Path: "path%20has/", Filename: "encodedSpace.txt"},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			srcSpaces, err := srcLoc.NewFile(path.Join(test.Path, test.Filename))
			require.NoError(t, err)
			b, err := srcSpaces.Write([]byte("something"))
			require.NoError(t, err)
			assert.Equal(t, 9, b, "byte count is correct")
			err = srcSpaces.Close()
			require.NoError(t, err)

			testDestLoc, err := dstLoc.NewLocation(test.Path)
			require.NoError(t, err)

			dstSpaces, err := srcSpaces.MoveToLocation(testDestLoc)
			require.NoError(t, err)
			exists, err := dstSpaces.Exists()
			require.NoError(t, err)
			assert.True(t, exists, "dstSpaces should now exist")
			exists, err = srcSpaces.Exists()
			require.NoError(t, err)
			assert.False(t, exists, "srcSpaces should no longer exist")
			assert.True(t,
				strings.HasSuffix(dstSpaces.URI(), path.Join(test.Path, test.Filename)),
				"destination file %s ends with source string for %s", dstSpaces.URI(), path.Join(test.Path, test.Filename),
			)

			newSrcSpaces, err := dstSpaces.MoveToLocation(srcSpaces.Location())
			require.NoError(t, err)
			exists, err = newSrcSpaces.Exists()
			require.NoError(t, err)
			assert.True(t, exists, "newSrcSpaces should now exist")
			exists, err = dstSpaces.Exists()
			require.NoError(t, err)
			assert.False(t, exists, "dstSpaces should no longer exist")
			hasSuffix := strings.HasSuffix(newSrcSpaces.URI(), path.Join(test.Path, test.Filename))
			assert.True(t, hasSuffix, "destination file %s ends with source string for %s", dstSpaces.URI(), path.Join(test.Path, test.Filename))

			require.NoError(t, newSrcSpaces.Delete())
			exists, err = newSrcSpaces.Exists()
			require.NoError(t, err)
			assert.False(t, exists, "newSrcSpaces should now exist")
		})
	}

	// Touch tests
	touchedFile, err := srcLoc.NewFile("touch.txt")
	require.NoError(t, err)
	defer func() { _ = touchedFile.Delete() }()
	exists, err = touchedFile.Exists()
	require.NoError(t, err)
	assert.Falsef(t, exists, "%s shouldn't yet exist", touchedFile)

	err = touchedFile.Touch()
	require.NoError(t, err)
	exists, err = touchedFile.Exists()
	require.NoError(t, err)
	assert.Truef(t, exists, "%s now exists", touchedFile)

	size, err := touchedFile.Size()
	require.NoError(t, err)
	assert.Zerof(t, size, "%s should be empty", touchedFile)

	// Touch timestamp update test (optional)
	if !opts.SkipTouchTimestampTest {
		modified, err := touchedFile.LastModified()
		require.NoError(t, err)
		modifiedDeRef := *modified
		time.Sleep(2 * time.Second)
		err = touchedFile.Touch()
		require.NoError(t, err)
		newModified, err := touchedFile.LastModified()
		require.NoError(t, err)
		assert.Greaterf(t, *newModified, modifiedDeRef, "touch updated modified date for %s", touchedFile)
	}

	// Delete
	require.NoError(t, srcFile.Delete())
	exists, err = srcFile.Exists()
	require.NoError(t, err)
	assert.False(t, exists, "file no longer exists")

	// Operations on non-existent file should error
	srcFile, err = srcLoc.NewFile("thisFileDoesNotExist")
	require.NoError(t, err, "unexpected error creating file")

	exists, err = srcFile.Exists()
	require.NoError(t, err)
	assert.False(t, exists, "file should not exist")

	size, err = srcFile.Size()
	require.Error(t, err, "expected error because file does not exist")
	assert.Zero(t, size)

	_, err = srcFile.LastModified()
	require.Error(t, err, "expected error because file does not exist")

	seeked, err := srcFile.Seek(-1, 2)
	require.Error(t, err, "expected error because file does not exist")
	assert.Zero(t, seeked)

	_, err = srcFile.Read(make([]byte, 1))
	require.Error(t, err, "expected error because file does not exist")
}

func buildExpectedURI(fs vfs.FileSystem, authorityStr, p string) string {
	return fmt.Sprintf("%s://%s%s", fs.Scheme(), authorityStr, p)
}
