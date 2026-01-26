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
	_ "github.com/c2fo/vfs/v7/backend/all" // register all core backends
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
		s.Run(scheme, func() {
			RunConformanceTests(s.T(), location, opts)
		})
	}
}

func TestVFS(t *testing.T) {
	suite.Run(t, new(vfsTestSuite))
}
