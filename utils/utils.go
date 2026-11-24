// Package utils provides utility functions for working with file paths, URIs, and other common operations.
package utils

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/c2fo/vfs/v7"
)

var (
	// ErrBadAbsFilePath is returned when a file path is not absolute
	ErrBadAbsFilePath = errors.New("absolute file path is invalid - must include leading slash and may not include trailing slash")
	// ErrBadRelFilePath is returned when a file path is not relative
	ErrBadRelFilePath = errors.New("relative file path is invalid - may not include leading or trailing slashes")
	// ErrBadAbsLocationPath is returned when a file path is not absolute
	ErrBadAbsLocationPath = errors.New("absolute location path is invalid - must include leading and trailing slashes")
	// ErrBadRelLocationPath is returned when a file path is not relative
	ErrBadRelLocationPath = errors.New("relative location path is invalid - may not include leading slash but must include trailing slash")
	// ErrBadPrefix is returned when a prefix is not relative or ends in / or is empty
	ErrBadPrefix = errors.New("prefix is invalid - may not include leading or trailing slashes and may not be empty")
)

const (
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
		return ErrBadAbsFilePath
	}
	return nil
}

// ValidateRelativeFilePath ensures that a file path has neither leading nor trailing slashes
func ValidateRelativeFilePath(name string) error {
	if name == "" || name == "." || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return ErrBadRelFilePath
	}
	return nil
}

// ValidateAbsoluteLocationPath ensure that a file path has both leading and trailing slashes
func ValidateAbsoluteLocationPath(name string) error {
	if !strings.HasPrefix(name, "/") || !strings.HasSuffix(name, "/") {
		return ErrBadAbsLocationPath
	}
	return nil
}

// ValidateRelativeLocationPath ensure that a file path has no leading slash but has a trailing slash
func ValidateRelativeLocationPath(name string) error {
	if strings.HasPrefix(name, "/") || !strings.HasSuffix(name, "/") {
		return ErrBadRelLocationPath
	}
	return nil
}

// ValidatePrefix ensures that a prefix path has neither leading nor trailing slashes
// may not be empty but unlike relative file path, *may* be simply "."
func ValidatePrefix(prefix string) error {
	if prefix == "" || strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/") {
		return ErrBadPrefix
	}
	return nil
}

// GetFileURI returns a File URI
func GetFileURI(f vfs.File) string {
	return fmt.Sprintf("%s://%s%s", f.Location().FileSystem().Scheme(), f.Location().Authority().String(), f.Path())
}

// GetLocationURI returns a Location URI
func GetLocationURI(l vfs.Location) string {
	return fmt.Sprintf("%s://%s%s", l.FileSystem().Scheme(), l.Authority().String(), l.Path())
}

// EnsureTrailingSlash is like AddTrailingSlash but will only ever use / since it's use for web uri's, never a Windows OS path.
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

// PathToURI takes a relative or absolute path and returns an OS URI.
// We assume non-scheme path is an OS File or Location.
// We assume URI authority is empty.
// We assume relative paths are relative to the pwd (program's working directory)
//
// /absolute/path/to/file.txt : file:///absolute/path/to/file.txt
// /some/absolute/path/       : file:///absolute/path/
// relative/path/to/file.txt  : file:///absolute/path/with/relative/path/to/file.txt
// relative/path/             : file:///absolute/path/with/relative/path/
func PathToURI(p string) (string, error) {
	if p == "" {
		p = "/"
	}
	var URI string

	// parse path
	u, err := url.Parse(p)
	if err != nil {
		return "", err
	}

	// if scheme is found, it's already a URI
	if u.Scheme != "" {
		return p, nil
	}

	absPath := p
	if p[0] != '/' {
		// make absolute path (if not already)
		absPath, err = filepath.Abs(p)
		if err != nil {
			return "", err
		}
		if runtime.GOOS == "windows" {
			absPath = "/" + absPath
		}
	}

	absPath = filepath.ToSlash(absPath)

	// Abs() strips trailing slashes so add back if original path had slash
	if p[len(p)-1:] == "/" {
		absPath = EnsureTrailingSlash(absPath)
	}

	URI = "file://" + absPath

	return URI, err
}

// TouchCopy is a wrapper around io.Copy which ensures that even empty source files (reader) will get written as an
// empty file. It guarantees a Write() call on the target file.
//
// Deprecated: Use TouchCopyBuffer Instead
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

// SequentialWriter is implemented by files that stage writes locally (for example s3 and gs) and can skip that staging
// when the caller promises one sequential write stream followed by Close.
type SequentialWriter interface {
	// BeginSequentialWrite tells the file that a single sequential write stream is about to start and that it will not
	// be sought or read until it is closed. It returns a function that ends the hint, which is always non-nil and safe
	// to call even when the file ignored the hint (for example because it was already written to).
	BeginSequentialWrite() (end func())
}

// BeginSequentialWrite calls SequentialWriter.BeginSequentialWrite when w implements it. Otherwise it does nothing.
// The returned function is always non-nil and should be called, typically with defer, when the write stream is done.
func BeginSequentialWrite(w io.Writer) (end func()) {
	if sw, ok := w.(SequentialWriter); ok {
		if e := sw.BeginSequentialWrite(); e != nil {
			return e
		}
	}
	return func() {}
}

// WriteAborter is implemented by files (for example s3 and gs) whose in-flight upload can be abandoned when a caller's
// write stream stops early with an error, so a subsequent Close reports err instead of committing a truncated object.
type WriteAborter interface {
	// AbortWrite abandons any active write session on the file and latches err so subsequent operations and Close
	// report it without committing partial data. It is a no-op when err is nil or no write has started.
	AbortWrite(err error)
}

// AbortWrite calls WriteAborter.AbortWrite when err is non-nil and w implements WriteAborter. Otherwise it does
// nothing. Callers streaming from a fallible reader into an s3 or gs file (for example with io.Copy and a deferred
// Close) can call AbortWrite on copy failure so Close does not commit a partial object.
func AbortWrite(w io.Writer, err error) {
	if err == nil {
		return
	}
	if wa, ok := w.(WriteAborter); ok {
		wa.AbortWrite(err)
	}
}

// TouchCopyBuffered is a wrapper around io.CopyBuffer which ensures that even empty source files (reader) will get written as an
// empty file. It guarantees a Write() call on the target file.
// bufferSize is in bytes and if is less than TouchCopyMinBufferSize will result in a buffer of size TouchCopyMinBufferSize
// bytes. If bufferSize is > TouchCopyMinBufferSize it will result in a buffer of size bufferSize bytes
//
// When writer is an s3 or gs file that has not been written, seeked or read yet, the copy streams to the remote without a
// local temporary file, and a mid-copy read or write failure abandons the upload so Close will not commit a partial object.
// Such a file cannot be sought or read until it is closed, so callers must Close writer after the copy, as every VFS
// CopyToFile implementation does.
func TouchCopyBuffered(writer io.Writer, reader io.Reader, bufferSize int) (err error) {
	var buffer []byte
	var size int64

	// a single sequential copy never needs the backend's local write buffer; if the copy fails after writing began,
	// abandon any in-flight upload rather than letting a deferred Close commit a truncated object
	defer BeginSequentialWrite(writer)()
	defer func() { AbortWrite(writer, err) }()

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

	// copy file to tempfile
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

// SeekTo is a helper function for Seek. It takes the current position, offset, whence, and length of the file
// and returns the new position. It also checks for invalid offsets and returns an error if one is found.
func SeekTo(length, position, offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, vfs.ErrSeekInvalidWhence
	case io.SeekStart:
		// this actually does nothing since the new position just becomes the offset but is here for completeness
	case io.SeekCurrent:
		offset += position
	case io.SeekEnd:
		offset += length
	}
	if offset < 0 {
		return 0, vfs.ErrSeekInvalidOffset
	}

	return offset, nil
}

// EncodeURI ensure that a uri is properly percent-encoded
func EncodeURI(scheme, username, hostport, path string) string {
	u := &url.URL{
		Scheme: scheme,
		User:   url.User(username),
		Host:   hostport,
		Path:   path,
	}

	return u.String()
}

// Ptr returns a pointer to the given value.
func Ptr[T any](value T) *T {
	return new(value)
}
