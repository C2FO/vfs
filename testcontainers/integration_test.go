//go:build vfsintegration

package testcontainers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"golang.org/x/sync/errgroup"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/vfssimple"
)

// backendTarget is a single provisioned backend to run the shared suites
// against. name is a unique, human-readable subtest label (two targets can
// share a scheme, e.g. minio serves s3 under both an SSE-off and an SSE-on
// bucket, so scheme alone is not unique).
type backendTarget struct {
	name     string
	scheme   string
	location vfs.Location
}

// containerSuite provisions every supported backend once (as disposable
// containers) and then runs the shared conformance and IO suites against each.
// Both test methods reuse the same targets so the containers only start once.
type containerSuite struct {
	suite.Suite
	targets []backendTarget
}

func (s *containerSuite) SetupSuite() {
	// Skip the whole suite when there is no usable Docker daemon rather than
	// failing, so `go test -tags=vfsintegration ./...` is safe to run anywhere.
	testcontainers.SkipIfProviderIsNotHealthy(s.T())

	registers := allRegisters()
	uriGroups := make([][]string, len(registers))

	var g errgroup.Group
	// Cap concurrent container startups so a cold runner does not pull every
	// image at once, which would amplify Docker Hub rate-limit exposure.
	g.SetLimit(4)
	for i, reg := range registers {
		g.Go(func() error {
			uris, err := reg(s.T())
			if err != nil {
				return err
			}
			uriGroups[i] = uris
			return nil
		})
	}
	s.Require().NoError(g.Wait(), "failed to provision backend containers")

	for _, uris := range uriGroups {
		for _, uri := range uris {
			l, err := vfssimple.NewLocation(uri)
			s.Require().NoError(err)
			s.targets = append(s.targets, backendTarget{
				name:     targetName(uri),
				scheme:   l.FileSystem().Scheme(),
				location: l,
			})
		}
	}
}

// targetName turns a VFS URI into a subtest-friendly label, e.g.
// "s3://miniobucket/" -> "s3-miniobucket". Slashes are collapsed so the name
// does not create spurious nested subtests.
func targetName(uri string) string {
	name := strings.NewReplacer("://", "-", "/", "-", ":", "-", "@", "-").Replace(uri)
	return strings.Trim(name, "-")
}

// optionsFor returns the conformance options appropriate for a scheme. FTP
// cannot seek within a write and does not preserve modified timestamps the way
// the other backends do, so those cases are skipped for it.
func optionsFor(scheme string) testsuite.ConformanceOptions {
	return testsuite.ConformanceOptions{
		SkipFTPSpecificTests:   scheme == "ftp",
		SkipTouchTimestampTest: scheme == "ftp",
	}
}

func (s *containerSuite) TestConformance() {
	for _, tgt := range s.targets {
		s.Run(tgt.name, func() {
			testsuite.RunConformanceTests(s.T(), tgt.location, optionsFor(tgt.scheme))
		})
	}
}

func (s *containerSuite) TestIO() {
	for _, tgt := range s.targets {
		s.Run(tgt.name, func() {
			testsuite.RunIOTests(s.T(), tgt.location, optionsFor(tgt.scheme))
		})
	}
}

func TestContainerSuite(t *testing.T) {
	suite.Run(t, new(containerSuite))
}
