//go:build vfsintegration

// This file provides manual integration test runners for core backends.
// It uses vfssimple which auto-registers all core backends via backend/all.
// Individual backends (including contrib) should import the conformance test
// functions directly from this package.

package testsuite

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/gs"
	_ "github.com/c2fo/vfs/v7/backend/all" // register all core backends
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/vfssimple"
)

type vfsTestSuite struct {
	suite.Suite
	testLocations map[string]vfs.Location
}

func (s *vfsTestSuite) SetupSuite() {
	locs := os.Getenv("VFS_INTEGRATION_LOCATIONS")
	s.testLocations = make(map[string]vfs.Location)
	for _, loc := range strings.Split(locs, ";") {
		if loc == "" {
			continue
		}
		l, err := vfssimple.NewLocation(loc)
		s.Require().NoError(err)

		// For file:// locations, ensure directory exists
		if l.FileSystem().Scheme() == "file" {
			exists, err := l.Exists()
			if err != nil {
				panic(err)
			}
			if !exists {
				err := os.Mkdir(l.Path(), 0750)
				if err != nil {
					panic(err)
				}
			}
		}

		// Store location by scheme - no type assertion needed
		s.testLocations[l.FileSystem().Scheme()] = l
	}
}

// TestScheme runs conformance tests for each configured backend
func (s *vfsTestSuite) TestScheme() {
	for scheme, location := range s.testLocations {
		fmt.Printf("************** TESTING scheme: %s **************\n", scheme)

		// Determine conformance options based on scheme
		opts := ConformanceOptions{
			SkipFTPSpecificTests: scheme == "ftp",
		}

		// Run the exported conformance tests
		s.T().Run(scheme, func(t *testing.T) {
			RunConformanceTests(t, location, opts)
		})

		// Run GS-specific tests if applicable
		if scheme == "gs" {
			s.gsList(location)
		}
	}
}

// gsList is a GS-specific test for handling persistent "folders"
func (s *vfsTestSuite) gsList(baseLoc vfs.Location) {
	/*
		test description:
			When a persistent "folder" is created through the UI, it simply creates a zero length object
			with a trailing "/". The UI or gsutil knows to interpret these objects as folders but they are
			still just objects.  List(), in its current state, should ignore these objects.

		If we create the following objects:
			gs://bucket/some/path/to/myfolder/         -- Note that object base name is "myfolder/"
			gs://bucket/some/path/to/myfolder/file.txt

		List() from location "gs://bucket/some/path/to/myfolder/" should only return object name "file.txt";
		"myfolder/" should be ignored.
	*/

	// getting client since VFS doesn't allow a File ending with a slash
	client, err := baseLoc.FileSystem().(*gs.FileSystem).Client()
	s.Require().NoError(err)

	objHandle := client.
		Bucket("enterprise-test").
		Object(utils.RemoveLeadingSlash(baseLoc.Path() + "myfolder/"))

	ctx := s.T().Context()

	// write zero length object
	writer := objHandle.NewWriter(ctx)
	_, err = writer.Write([]byte(""))
	s.Require().NoError(err)
	s.Require().NoError(writer.Close())

	// create a file inside the "folder"
	f, err := baseLoc.NewFile("myfolder/file.txt")
	s.Require().NoError(err)

	_, err = f.Write([]byte("some text"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	// list "folder" should only return file.txt
	files, err := f.Location().List()
	s.Require().NoError(err)
	s.Len(files, 1, "check file count found")
	s.Equal("file.txt", files[0], "file.txt was found")

	// CLEAN UP
	s.Require().NoError(f.Delete(), "clean up file.txt")
	s.Require().NoError(objHandle.Delete(ctx))
}

func TestVFS(t *testing.T) {
	suite.Run(t, new(vfsTestSuite))
}
