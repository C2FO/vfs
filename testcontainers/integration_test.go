//go:build vfsintegration

package testcontainers

import (
	"context"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"golang.org/x/sync/errgroup"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/ftp"
	ftptypes "github.com/c2fo/vfs/v7/backend/ftp/types"
	"github.com/c2fo/vfs/v7/backend/sftp"
	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/vfssimple"
)

// backendTarget is a single provisioned backend to run the shared suites
// against. name is a unique, human-readable subtest label (two targets can
// share a scheme, e.g. S3Mock serves s3 under both an SSE-off and an SSE-on
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
// "s3://s3mockbucket/" -> "s3-s3mockbucket". Slashes are collapsed so the name
// does not create spurious nested subtests.
func targetName(uri string) string {
	name := strings.NewReplacer("://", "-", "/", "-", ":", "-", "@", "-").Replace(uri)
	return strings.Trim(name, "-")
}

// optionsFor returns the conformance options appropriate for a scheme. FTP
// cannot seek within a write and does not preserve modified timestamps the way
// the other backends do, so those cases are skipped for it.
func optionsFor(scheme string) testsuite.ConformanceOptions {
	opts := testsuite.ConformanceOptions{
		SkipFTPSpecificTests:   scheme == "ftp",
		SkipTouchTimestampTest: scheme == "ftp",
		// S3 lists modification times and answers HeadObject with different precision, so a listed time can differ
		// from File.LastModified by under a second.
		ListModTimeTolerance: listModTimeTolerance(scheme),
	}
	// sftp and ftp have real directories, so the listing tests also check an empty one.
	if scheme == "sftp" || scheme == "ftp" {
		opts.CreateEmptyDirectory = createDirectory
	}
	if scheme == "sftp" {
		opts.RemoveDirectory = removeSFTPDirectory
	}
	return opts
}

// createDirectory creates dir and any missing parents over sftp or ftp, and succeeds when it exists.
func createDirectory(dir vfs.Location) error {
	switch fs := dir.FileSystem().(type) {
	case *sftp.FileSystem:
		client, err := fs.Client(dir.Authority())
		if err != nil {
			return err
		}
		return client.MkdirAll(dir.Path())
	case *ftp.FileSystem:
		dc, err := fs.DataConn(context.Background(), dir.Authority(), ftptypes.SingleOp, nil)
		if err != nil {
			return err
		}
		// FTP has no recursive make or exists check: make each level, and accept a failure when the level can be listed.
		cur := ""
		for _, seg := range strings.Split(strings.Trim(dir.Path(), "/"), "/") {
			cur = path.Join(cur, seg)
			p := "/" + cur
			if err := dc.MakeDir(p); err != nil {
				if _, listErr := dc.List(p); listErr != nil {
					return err
				}
			}
		}
		return nil
	}
	return nil
}

// removeSFTPDirectory removes dir and everything under it.
func removeSFTPDirectory(dir vfs.Location) error {
	fs, ok := dir.FileSystem().(*sftp.FileSystem)
	if !ok {
		return nil
	}
	client, err := fs.Client(dir.Authority())
	if err != nil {
		return err
	}
	var remove func(p string) error
	remove = func(p string) error {
		infos, err := client.ReadDir(p)
		if err != nil {
			return err
		}
		for _, info := range infos {
			child := path.Join(p, info.Name())
			if info.IsDir() {
				if err := remove(child); err != nil {
					return err
				}
				continue
			}
			if err := client.Remove(child); err != nil {
				return err
			}
		}
		return client.Remove(p)
	}
	return remove(path.Clean(dir.Path()))
}

func listModTimeTolerance(scheme string) time.Duration {
	if scheme == "s3" {
		return time.Second
	}
	return 0
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
