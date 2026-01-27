package testcontainers

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/vfssimple"
)

type vfsTestSuite struct {
	suite.Suite
	testLocations map[string]vfs.Location
}

func (s *vfsTestSuite) SetupSuite() {
	registers := []func(*testing.T) string{
		registerMem,
		registerOS,
		registerAtmoz,
		registerAzurite,
		registerGCSServer,
		registerLocalStack,
		registerMinio,
		registerVSFTPD,
	}
	uris := make([]string, len(registers))
	var wg sync.WaitGroup
	for i := range registers {
		wg.Go(func() { uris[i] = registers[i](s.T()) })
	}
	wg.Wait()

	s.testLocations = make(map[string]vfs.Location)
	for _, loc := range uris {
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

func registerMem(*testing.T) string {
	return "mem://test/"
}

func registerOS(t *testing.T) string {
	return fmt.Sprintf("file://%s/", filepath.ToSlash(t.TempDir()))
}

// TestScheme runs conformance tests for each configured backend
func (s *vfsTestSuite) TestScheme() {
	for scheme, location := range s.testLocations {
		fmt.Printf("************** TESTING scheme: %s **************\n", scheme)

		// Determine conformance options based on scheme
		opts := ConformanceOptions{
			SkipFTPSpecificTests:   scheme == "ftp",
			SkipTouchTimestampTest: scheme == "ftp",
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
