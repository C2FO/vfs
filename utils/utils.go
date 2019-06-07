package utils

import (
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/c2fo/vfs/v4"
)

const (
	// Windows constant represents a target operating system running a version of Microsoft Windows
	Windows = "windows"
	// BadFilePrefix constant is returned when path has leading slash or backslash
	BadFilePrefix = "expecting only a filename prefix, which may not include slashes or backslashes"
	// ErrBadAbsFilePath constant is returned when a file path is not absolute
	ErrBadAbsFilePath = "absolute file path is invalid - must include leading separator character and may not include trailing separator character"
	// ErrBadRelFilePath constant is returned when a file path is not relative
	ErrBadRelFilePath = "relative file path is invalid - may not include leading or trailing separator characters"
	// ErrBadAbsLocationPath constant is returned when a file path is not absolute
	ErrBadAbsLocationPath = "absolute location path is invalid - must include leading and trailing separator characters"
	// ErrBadRelLocationPath constant is returned when a file path is not relative
	ErrBadRelLocationPath = "relative location path is invalid - may not include leading separator character but must include trailing separator character"
)

// regex to ensure prefix doesn't have leading '/', '.', '..', etc...
var prefixCleanRegex = regexp.MustCompile("^[/.]*")

// regex to test whether the last character is a '/'
var hasTrailingSlash = regexp.MustCompile("/$")

// regex to test whether the first character is a '/'
var hasLeadingSlash = regexp.MustCompile("^/")

// RemoveTrailingSlash removes trailing slash, if any
func RemoveTrailingSlash(path string) string {
	return strings.TrimRight(path, "/")
}

// RemoveLeadingSlash removes leading slash, if any
func RemoveLeadingSlash(path string) string {
	return strings.TrimLeft(path, "/")
}

// ValidateAbsFilePath ensures that a file path has a leading slash but not a trailing slash
func ValidateAbsFilePath(name string) error {
	if !strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return errors.New(ErrBadAbsFilePath)
	}
	return nil
}

// ValidateRelFilePath ensures that a file path has neither leading nor trailing slashes
func ValidateRelFilePath(name string) error {
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return errors.New(ErrBadRelFilePath)
	}
	return nil
}

// ValidateAbsLocationPath ensure that a file path has both leading and trailing slashes
func ValidateAbsLocationPath(name string) error {
	if !strings.HasPrefix(name, "/") || !strings.HasSuffix(name, "/") {
		return errors.New(ErrBadAbsLocationPath)
	}
	return nil
}

// ValidateRelLocationPath ensure that a file path has no leading slash but has a trailing slash
func ValidateRelLocationPath(name string) error {
	if strings.HasPrefix(name, "/") || !strings.HasSuffix(name, "/") {
		return errors.New(ErrBadRelLocationPath)
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
	if hasTrailingSlash.MatchString(dir) {
		return dir
	}
	return dir + "/"
}

// EnsureLeadingSlash is like EnsureTrailingSlash except that it adds the leading slash if needed.
func EnsureLeadingSlash(dir string) string {
	if dir == "" || hasLeadingSlash.MatchString(dir) {
		return dir
	}
	return "/" + dir
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
