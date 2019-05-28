package utils

import (
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/c2fo/vfs/v4"
)

const (
	// Windows constant represents a target operating system running a version of Microsoft Windows
	Windows = "windows"
	// BadFilePrefix constant is returned when path has leading slash or backslash
	BadFilePrefix = "expecting only a filename prefix, which may not include slashes or backslashes"
	ErrBadFilePath = "file path is invalid - must include leading separator character and may not include trailing separator character"
	ErrBadLocationPath = "location path is invalid - must include leading and trailing separator characters"
)

// regex to ensure prefix doesn't have leading '/', '.', '..', etc...
var prefixCleanRegex = regexp.MustCompile("^[/.]*")

// regex to test whether the last character is a '/'
var hasTrailingSlash = regexp.MustCompile("/$")

// AddTrailingSlash is a helper function accepts a path string and returns the path string with a trailing slash if
// there wasn't one.
func AddTrailingSlash(path string) string {

	runePath := []rune(path)
	lastRune := runePath[len(runePath)-1]

	slash := "/"
	if runtime.GOOS == Windows {
		slash = "\\"
	}

	//add trailing slash, if none
	if string(lastRune) != "/" && string(lastRune) != "\\" {
		path = path + slash
	}
	return path
}

// ValidateFile ensure that a file may not end with trailing slash and its path must being with a leading slash
func ValidateFilePath(name string) error {
	if !strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return errors.New(ErrBadFilePath)
	}
	return nil
}

// ValidateFile ensure that a file may not end with trailing slash and its path must being with a leading slash
func ValidateLocationPath(name string) error {
	if !strings.HasPrefix(name, "/") || !strings.HasSuffix(name, "/") {
		return errors.New(ErrBadLocationPath)
	}
	return nil
}

// GetFileURI returns a File URI
func GetFileURI(f vfs.File) string {
	return fmt.Sprintf("%s://%s%s", f.Location().FileSystem().Scheme(), f.Location().Volume(), f.Path())
}

// GetLocationURI returns a Location URI
func GetLocationURI(l vfs.Location) string {
	return fmt.Sprintf("%s://%s%s", l.FileSystem().Scheme(), l.Volume(), l.Path())
}

// EnsureTrailingSlash is like AddTrailingSlash but will only ever use / since it's use for web uri's, never an Windows OS path.
func EnsureTrailingSlash(dir string) string {
	if dir == "" || hasTrailingSlash.MatchString(dir) {
		return dir
	}
	return dir + "/"
}

// CleanPrefix resolves relative dot pathing, removing any leading . or / and removes any trailing /
func CleanPrefix(prefix string) string {
	prefix = path.Clean(prefix)
	return prefixCleanRegex.ReplaceAllString(prefix, "")
}

// ValidateFilePrefix performs a validation check on a prefix. The prefix should not include "/" or "\\" characters. An
// error is returned if either of those conditions are true.
func ValidateFilePrefix(filenamePrefix string) error {
	if strings.Contains(filenamePrefix, "/") || strings.Contains(filenamePrefix, "\\") {
		return errors.New(BadFilePrefix)
	}
	return nil
}

// TouchCopy is a wrapper around io.Copy which ensures that even empty source files (reader) will get written as an
// empty file. It guarantees a Write() call on the target file.
func TouchCopy(writer, reader vfs.File) error {
	if size, err := reader.Size(); err != nil {
		return err
	} else if size == 0 {
		_, err = writer.Write([]byte{})
		if err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(writer, reader); err != nil {
			return err
		}
	}
	return nil
}
