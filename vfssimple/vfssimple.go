package vfssimple

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	_ "github.com/c2fo/vfs/v6/backend/all" // register all backends
	"github.com/c2fo/vfs/v6/backend/azure"
	"github.com/c2fo/vfs/v6/backend/mem"
	"github.com/c2fo/vfs/v6/backend/os"
)

var (
	ErrMissingAuthority = errors.New("unable to determine uri authority ([user@]host[:port]) for network-based scheme")
	ErrMissingScheme    = errors.New("unable to determine uri scheme")
	ErrRegFsNotFound    = errors.New("no matching registered filesystem found")
	ErrBlankURI         = errors.New("uri is blank")
)

// NewLocation is a convenience function that allows for instantiating a location based on a uri string. Any
// backend file system is supported, though some may require prior configuration. See the docs for
// specific requirements of each
func NewLocation(uri string) (vfs.Location, error) {
	fs, host, path, err := parseSupportedURI(uri)
	if err != nil {
		return nil, fmt.Errorf("unable to create vfs.Location for uri %q: %w", uri, err)
	}

	return fs.NewLocation(host, path)
}

// NewFile is a convenience function that allows for instantiating a file based on a uri string. Any
// backend file system is supported, though some may require prior configuration. See the docs for
// specific requirements of each.
func NewFile(uri string) (vfs.File, error) {
	fs, host, path, err := parseSupportedURI(uri)
	if err != nil {
		return nil, fmt.Errorf("unable to create vfs.File for uri %q: %w", uri, err)
	}

	return fs.NewFile(host, path)
}

// parseURI attempts to parse a URI and validate that it returns required results
func parseURI(uri string) (scheme, authority, path string, err error) {
	// return early if blank uri
	if uri == "" {
		err = ErrBlankURI
		return
	}

	// parse URI
	var u *url.URL
	u, err = url.Parse(uri)
	if err != nil {
		err = fmt.Errorf("unknown url.Parse error: %w", err)
		return
	}

	// validate schema
	scheme = u.Scheme
	if u.Scheme == "" {
		err = ErrMissingScheme
		return
	}

	// validate authority
	authority = u.Host
	path = u.Path
	if azure.IsValidURI(u) {
		authority, path, err = azure.ParsePath(path)
	}

	if u.User.String() != "" {
		authority = fmt.Sprintf("%s@%s", u.User, u.Host)
	}
	// network-based schemes require authority, but not file:// or mem://
	if authority == "" && !(scheme == os.Scheme || scheme == mem.Scheme) {
		return "", "", "", ErrMissingAuthority
	}

	return
}

// parseSupportedURI checks if URI matches any backend name as prefix, capturing the longest(most specific) match found.
// See doc.go Registered Backend Resoltion seciton for examples.
func parseSupportedURI(uri string) (vfs.FileSystem, string, string, error) {
	_, authority, path, err := parseURI(uri)
	if err != nil {
		return nil, "", "", err
	}

	var longest string
	backends := backend.RegisteredBackends()
	for _, backendName := range backends {
		if strings.HasPrefix(uri, backendName) {
			// The first match always becomes the longest
			if longest == "" {
				longest = backendName
				continue
			}

			// we found a longer (more specific) backend prefix matching URI
			if len(backendName) > len(longest) {
				longest = backendName
			}
		}
	}

	if longest == "" {
		err = ErrRegFsNotFound
	}

	return backend.Backend(longest), authority, path, err
}
