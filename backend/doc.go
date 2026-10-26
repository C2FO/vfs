/*
Package backend provides a means of allowing backend file systems to self-register on load via an init() call to
backend.Register("some name", vfs.FileSystem)

In this way, a caller of vfs backends can simply load the backend file system (and ONLY those needed) and begin using it:

	package main

	// import backend and each backend you intend to use
	import(
	    "github.com/c2fo/vfs/v7/backend"
	    "github.com/c2fo/vfs/v7/backend/os"
	    "github.com/c2fo/vfs/v7/backend/s3"
	)

	func main() {
	    // THEN begin using the file systems
	    osfile, err := backend.Backend(os.Scheme).NewFile("", "/path/to/file.txt")
	    if err != nil {
	        panic(err)
	    }

	    s3file, err := backend.Backend(s3.Scheme).NewFile("mybucket", "/some/file.txt")
	    if err != nil {
	        panic(err)
	    }

	    err = osfile.CopyToFile(s3file)
	    if err != nil {
	        panic(err)
	    }
	}

To load every core backend at once, import github.com/c2fo/vfs/v7/backend/all.

# Development

To create your own backend, you must create a package that implements the interfaces: vfs.FileSystem, vfs.Location, and vfs.File.
Then ensure it registers itself on load:

	package myexoticfilesystem

	import(
	    ...
	    "github.com/c2fo/vfs/v7"
	    "github.com/c2fo/vfs/v7/backend"
	)

	const Scheme = "exfs"

	// IMPLEMENT vfs interfaces
	...

	// register backend
	func init() {
	    backend.Register(Scheme, &MyExoticFileSystem{})
	}

Then to use it in some other package:

	package main

	import(
	    "github.com/c2fo/vfs/v7/backend"
	    "github.com/acme/myexoticfilesystem"
	)

	...

	func useNewBackend() error {
	    myExoticFs := backend.Backend(myexoticfilesystem.Scheme)
	    ...
	}

# Testing a Backend

To verify your backend implementation is correct, run the shared conformance suites from
github.com/c2fo/vfs/v7/backend/testsuite against it:

	//go:build vfsintegration

	package myexoticfilesystem

	import (
	    "testing"

	    "github.com/c2fo/vfs/v7/backend/testsuite"
	)

	func TestConformance(t *testing.T) {
	    fs := NewFileSystem() // pass any options your backend needs
	    location, err := fs.NewLocation("", "/test-path/")
	    if err != nil {
	        t.Fatal(err)
	    }
	    testsuite.RunConformanceTests(t, location)
	    testsuite.RunIOTests(t, location)
	}

Run with:

	go test -v -tags=vfsintegration ./...

See docs/conformance_tests.md in the repository for details, including how to run the suites against
Docker containers with the testcontainers module.
*/
package backend
