package vfs

import (
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"runtime"
	"strings"
)

const (
	Windows       = "windows"
	BadFilePrefix = "expecting only a filename prefix, which may not include slashes or backslashes"
)

// regexp matching an initial slash, used to check for them in paths
var prefixSlashRegex = regexp.MustCompile("^[/.]*")

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

// GetFile returns a File URI
func GetFileURI(f File) string {
	return fmt.Sprintf("%s://%s%s", f.Location().FileSystem().Scheme(), f.Location().Volume(), f.Path())
}

// GetFile returns a Location URI
func GetLocationURI(l Location) string {
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

// Performs a validation check on a prefix. The prefix should not include "/" or "\\" characters. An
// error is returned if either of those conditions are true.
func ValidateFilePrefix(filenamePrefix string) error {
	if strings.Contains(filenamePrefix, "/") || strings.Contains(filenamePrefix, "\\") {
		return errors.New(BadFilePrefix)
	}
	return nil
}

// Methods to ensure consistency between implementations

func StandardizePath(path string) string {
	if prefixSlashRegex.MatchString(path) {
		return path
	} else {
		return "/" + path
	}
}

// TouchCopy is a wrapper around io.Copy which ensures that even empty source files (reader) will get written as an
// empty file. It guarantees a Write() call on the target file.
func TouchCopy(writer File, reader File) error {
	if size, err := reader.Size(); err != nil {
		return err
	} else if size == 0 {
		writer.Write([]byte{})
	} else {
		if _, err := io.Copy(writer, reader); err != nil {
			return err
		}
	}
	return nil
}
