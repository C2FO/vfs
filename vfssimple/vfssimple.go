package vfssimple

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	_ "github.com/c2fo/vfs/v6/backend/all" // register all backends
	"github.com/c2fo/vfs/v6/backend/azure"
)

// NewLocation is a convenience function that allows for instantiating a location based on a uri string. Any
// backend file system is supported, though some may require prior configuration. See the docs for
// specific requirements of each
func NewLocation(uri string) (vfs.Location, error) {
	fs, host, path, err := parseSupportedURI(uri)
	if err != nil {
		return nil, err
	}

	return fs.NewLocation(host, path)
}

// NewFile is a convenience function that allows for instantiating a file based on a uri string. Any
// backend file system is supported, though some may require prior configuration. See the docs for
// specific requirements of each.
func NewFile(uri string) (vfs.File, error) {
	fs, host, path, err := parseSupportedURI(uri)
	if err != nil {
		return nil, err
	}

	return fs.NewFile(host, path)
}

func parseSupportedURI(uri string) (vfs.FileSystem, string, string, error) {
	var err error
	var u *url.URL
	u, err = url.Parse(uri)
	if err != nil {
		return nil, "", "", err
	}
	host := u.Host
	path := u.Path
	user := u.User
	if user.String() != "" {
		host = fmt.Sprintf("%s@%s", user, host)
	}

	var fs vfs.FileSystem
	for _, backendScheme := range backend.RegisteredBackends() {
		// Azure
		if azure.IsValidURI(u) {
			host, path, err = azure.ParsePath(u.Path)
		}

		// Object-level backend
		if strings.HasPrefix(uri, backendScheme) {
			fs = backend.Backend(backendScheme)
			break
		}
		// Bucket-level backend
		volume := fmt.Sprintf("%s://%s/", u.Scheme, u.Host)
		if volume == backendScheme {
			fs = backend.Backend(backendScheme)
			break
		}

		// Location-level backend
		if isInPath(u, volume, backendScheme) {
			fs = backend.Backend(backendScheme)
			break
		}

		// Scheme-level backend
		if u.Scheme == backendScheme {
			fs = backend.Backend(backendScheme)
		}
	}

	if fs == nil {
		err = fmt.Errorf("%s is an unsupported uri scheme", u.Scheme)
	}

	return fs, host, path, err
}

// isInPath will crawl down the provided url's path to see if it matches the registered root value
// Example:
//	 url: s3://bucket/root/path/to/file.txt
//   root: s3://bucket/root/   <== Registered URI for file system.
//
//   This would return true because the target url is within the root.
func isInPath(u *url.URL, volume, root string) bool {
	path := u.Path
	paths := strings.Split(path, string(os.PathSeparator))
	for i := range paths {
		path := fmt.Sprintf("%s%s/", volume, filepath.Join(paths[0:i]...))
		if path == root {
			return true
		}
	}
	return false
}
