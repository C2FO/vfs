package utils

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/c2fo/vfs/v6"
)

const (
	// ErrBadAbsFilePath constant is returned when a file path is not absolute
	ErrBadAbsFilePath = "absolute file path is invalid - must include leading slash and may not include trailing slash"
	// ErrBadRelFilePath constant is returned when a file path is not relative
	ErrBadRelFilePath = "relative file path is invalid - may not include leading or trailing slashes"
	// ErrBadAbsLocationPath constant is returned when a file path is not absolute
	ErrBadAbsLocationPath = "absolute location path is invalid - must include leading and trailing slashes"
	// ErrBadRelLocationPath constant is returned when a file path is not relative
	ErrBadRelLocationPath = "relative location path is invalid - may not include leading slash but must include trailing slash"
	// TouchCopyMinBufferSize min buffer size used in TouchCopyBuffered in bytes
	TouchCopyMinBufferSize = 262144
)

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

// ValidateAbsoluteFilePath ensures that a file path has a leading slash but not a trailing slash
func ValidateAbsoluteFilePath(name string) error {
	if !strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return errors.New(ErrBadAbsFilePath)
	}
	return nil
}

// ValidateRelativeFilePath ensures that a file path has neither leading nor trailing slashes
func ValidateRelativeFilePath(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return errors.New(ErrBadRelFilePath)
	}
	return nil
}

// ValidateAbsoluteLocationPath ensure that a file path has both leading and trailing slashes
func ValidateAbsoluteLocationPath(name string) error {
	if !strings.HasPrefix(name, "/") || !strings.HasSuffix(name, "/") {
		return errors.New(ErrBadAbsLocationPath)
	}
	return nil
}

// ValidateRelativeLocationPath ensure that a file path has no leading slash but has a trailing slash
func ValidateRelativeLocationPath(name string) error {
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
	if hasLeadingSlash.MatchString(dir) {
		return dir
	}
	return "/" + dir
}

// Deprecated: Use TouchCopyBuffer Instead
// TouchCopy is a wrapper around io.Copy which ensures that even empty source files (reader) will get written as an
// empty file. It guarantees a Write() call on the target file.
func TouchCopy(writer io.Writer, reader io.Reader) error {
	size, err := io.Copy(writer, reader)
	if err != nil {
		return err
	}
	if size == 0 {
		_, err = writer.Write([]byte{})
		if err != nil {
			return err
		}
	}
	return nil
}

// TouchCopyBuffered is a wrapper around io.CopyBuffer which ensures that even empty source files (reader) will get written as an
// empty file. It guarantees a Write() call on the target file.
// bufferSize is in bytes and if is less than TouchCopyMinBufferSize will result in a buffer of size TouchCopyMinBufferSize
// bytes. If bufferSize is > TouchCopyMinBufferSize it will result in a buffer of size bufferSize bytes
func TouchCopyBuffered(writer io.Writer, reader io.Reader, bufferSize int) error {
	var buffer []byte
	var size int64
	var err error

	if bufferSize <= 0 {
		// Use TouchCopyMinBufferSize
		buffer = make([]byte, TouchCopyMinBufferSize)
		size, err = io.CopyBuffer(writer, reader, buffer)
	} else {
		// Otherwise use provided bufferSize
		buffer = make([]byte, bufferSize)
		size, err = io.CopyBuffer(writer, reader, buffer)
	}

	if err != nil {
		return err
	}
	if size == 0 {
		_, err = writer.Write([]byte{})
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateLastModifiedByMoving is used by some backends' Touch() method when a file already exists.
func UpdateLastModifiedByMoving(file vfs.File) error {
	// setup a tempfile
	tempfile, err := file.Location().
		NewFile(fmt.Sprintf("%s.%d", file.Name(), time.Now().UnixNano()))
	if err != nil {
		return err
	}

	// copy file file to tempfile
	err = file.CopyToFile(tempfile)
	if err != nil {
		return err
	}

	// move tempfile back to file
	err = tempfile.MoveToFile(file)
	if err != nil {
		return err
	}
	return nil
}
