package testcontainers

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/vfssimple"
)

type ioTestSuite struct {
	suite.Suite
	testLocations map[string]vfs.Location
}

func (s *ioTestSuite) SetupSuite() {
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
	for idx := range uris {
		if uris[idx] == "" {
			continue
		}
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

func (s *ioTestSuite) TestFileOperations() {
	for scheme, location := range s.testLocations {
		s.Run(scheme, func() {
			RunIOTests(s.T(), location)
		})
	}
}

func TestIOTestSuite(t *testing.T) {
	suite.Run(t, new(ioTestSuite))
}
