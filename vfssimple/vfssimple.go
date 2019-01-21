package vfssimple

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/c2fo/vfs"
	"github.com/c2fo/vfs/backend"
	_ "github.com/c2fo/vfs/backend/all" //register all backends
)

// NewLocation is a convenience function that allows for instantiating a location based on a uri string. Any
// backend filesystem is supported, though some may require prior configuration. See the docs for
// specific requirements of each
func NewLocation(uri string) (vfs.Location, error) {
	fs, host, path, err := parseSupportedURI(uri)
	if err != nil {
		return nil, err
	}

	return fs.NewLocation(host, path)
}

// NewFile is a convenience function that allows for instantiating a file based on a uri string. Any
// backend filesystem is supported, though some may require prior configuration. See the docs for
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

	var fs vfs.FileSystem
	for _, backendScheme := range backend.RegisteredBackends() {
		// Object-level backend
		if strings.Index(uri, backendScheme) > 1 {
			fs = backend.Backend(backendScheme)
			break
		}
		// Bucket-level backend
		volume := fmt.Sprintf("%s://%s/", u.Scheme, u.Host)
		if volume == backendScheme {
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
