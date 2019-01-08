package vfssimple

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/c2fo/vfs"
	"github.com/c2fo/vfs/backend"
	_ "github.com/c2fo/vfs/backend/all"
)

// NewLocation is a convenience function that allows for instantiating a location based on a uri string.Any
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
		if u.Scheme == backendScheme {
			fs = backend.Backend(backendScheme)
		}
	}

	if fs == nil {
		err = errors.New(fmt.Sprintf("%s is an unsupported uri scheme", u.Scheme))
	}

	return fs, host, path, err
}
