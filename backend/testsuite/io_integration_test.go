//go:build vfsintegration

// This file provides manual IO integration test runners for core backends.
// Individual backends (including contrib) should import the IO conformance test
// functions directly from this package.

package testsuite

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	_ "github.com/c2fo/vfs/v7/backend/all" // register all core backends
	"github.com/c2fo/vfs/v7/vfssimple"
)

type ioTestSuite struct {
	suite.Suite
	testLocations map[string]vfs.Location
	localDir      string
}

func (s *ioTestSuite) SetupSuite() {
	uris := make([]string, 0)

	// add VFS_INTEGRATION_LOCATIONS tests
	locs := os.Getenv("VFS_INTEGRATION_LOCATIONS")
	uris = append(uris, strings.Split(locs, ";")...)

	s.testLocations = make(map[string]vfs.Location)
	for idx := range uris {
		if uris[idx] == "" {
			continue
		}
		if strings.HasPrefix(uris[idx], "/") {
			s.localDir = uris[idx]
		} else {
			l, err := vfssimple.NewLocation(uris[idx])
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
}

func (s *ioTestSuite) TestFileOperations() {
	if s.localDir != "" {
		s.Run("local_baseline", func() {
			runIOTestsLocal(s.T(), s.localDir, DefaultIOTestCases())
		})
	}
	for scheme, location := range s.testLocations {
		s.Run(scheme, func() {
			RunIOTests(s.T(), location)
		})
	}
}

func TestIOTestSuite(t *testing.T) {
	suite.Run(t, new(ioTestSuite))
}
