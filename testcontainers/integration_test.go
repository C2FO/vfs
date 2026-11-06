//go:build vfsintegration

package testcontainers

import (
	"context"
	"io"
	stdfs "io/fs"
	"path"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"golang.org/x/sync/errgroup"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/ftp"
	ftptypes "github.com/c2fo/vfs/v7/backend/ftp/types"
	"github.com/c2fo/vfs/v7/backend/sftp"
	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/iofs"
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
		for seg := range strings.SplitSeq(strings.Trim(dir.Path(), "/"), "/") {
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

// TestIOFS checks that iofs presents each backend as a valid fs.FS. testing/fstest.TestFS needs every directory to
// hold at least one file, because object stores cannot represent an empty directory.
func (s *containerSuite) TestIOFS() {
	tree := map[string]string{
		"a.txt":         "alpha",
		"css/site.css":  "body{}",
		"css/img/x.png": "png",
		"deep/er/f.go":  "package f",
	}
	for _, tgt := range s.targets {
		s.Run(tgt.name, func() {
			if tgt.scheme == "gs" {
				// gs.File.Read does not advance its position when a read returns data together with io.EOF, so
				// Seek(0, io.SeekCurrent) after reading to the end reports the wrong offset and fstest rejects the file
				// (issue 406).
				s.T().Skip("gs.File position is wrong after a read that returns io.EOF")
			}
			if tgt.scheme == "ftp" {
				// ftp.File.Seek(offset, io.SeekEnd) computes size-offset instead of size+offset (issue 407).
				s.T().Skip("ftp.File.Seek with io.SeekEnd uses the wrong sign")
			}
			root, err := tgt.location.NewLocation("iofs-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "/")
			s.Require().NoError(err)

			expected := make([]string, 0, len(tree))
			for name, content := range tree {
				f, err := root.NewFile(name)
				s.Require().NoError(err)
				// sftp and ftp have real directories, which a write does not create.
				s.Require().NoError(createDirectory(f.Location()))
				_, err = io.WriteString(f, content)
				s.Require().NoError(err)
				s.Require().NoError(f.Close())
				expected = append(expected, name)
				s.T().Cleanup(func() { _ = f.Delete() })
			}

			s.Require().NoError(fstest.TestFS(iofs.New(root), expected...))
		})
	}
}

// TestIOFSFileWins checks that a name that is both an object and a prefix is a single file. Only object stores allow it.
func (s *containerSuite) TestIOFSFileWins() {
	for _, tgt := range s.targets {
		if tgt.scheme != "s3" && tgt.scheme != "az" {
			continue
		}
		s.Run(tgt.name, func() {
			root, err := tgt.location.NewLocation("iofs-wins-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "/")
			s.Require().NoError(err)
			for name, content := range map[string]string{"amb": "file", "amb/child": "child"} {
				f, err := root.NewFile(name)
				s.Require().NoError(err)
				_, err = io.WriteString(f, content)
				s.Require().NoError(err)
				s.Require().NoError(f.Close())
				s.T().Cleanup(func() { _ = f.Delete() })
			}

			fsys := iofs.New(root)
			info, err := stdfs.Stat(fsys, "amb")
			s.Require().NoError(err)
			s.False(info.IsDir())

			entries, err := stdfs.ReadDir(fsys, ".")
			s.Require().NoError(err)
			s.Require().Len(entries, 1)
			s.False(entries[0].IsDir())

			data, err := stdfs.ReadFile(fsys, "amb/child")
			s.Require().NoError(err)
			s.Equal("child", string(data))
		})
	}
}

func TestContainerSuite(t *testing.T) {
	suite.Run(t, new(containerSuite))
}
