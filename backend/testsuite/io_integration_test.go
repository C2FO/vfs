//go:build vfsintegration
// +build vfsintegration

package testsuite

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/vfssimple"
)

type OSWrapper struct {
	filename   string
	file       *os.File
	exists     bool
	seekCalled bool
}

func NewOSWrapper(absPath string) *OSWrapper {
	exists := fileExists(absPath)
	return &OSWrapper{
		filename: absPath,
		exists:   exists,
	}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func (o *OSWrapper) Read(b []byte) (int, error) {
	if !o.exists {
		return 0, errors.New("file not found")
	}
	if o.file == nil {
		file, err := os.OpenFile(o.filename, os.O_RDWR, 0600)
		if err != nil {
			return 0, err
		}
		o.file = file
	}
	return o.file.Read(b)
}

func (o *OSWrapper) Write(b []byte) (int, error) {
	if o.file == nil {
		flags := os.O_RDWR | os.O_CREATE | os.O_TRUNC
		if o.seekCalled {
			flags = os.O_RDWR | os.O_CREATE
		}
		file, err := os.OpenFile(o.filename, flags, 0600) //nolint:gosec
		if err != nil {
			return 0, err
		}
		o.file = file
		o.exists = true
	}

	return o.file.Write(b)
}

func (o *OSWrapper) Seek(offset int64, whence int) (int64, error) {
	if !o.exists {
		return 0, errors.New("file not found")
	}

	if o.file == nil {
		file, err := os.OpenFile(o.filename, os.O_RDWR, 0600)
		if err != nil {
			return 0, err
		}
		o.file = file
	}
	o.seekCalled = true
	return o.file.Seek(offset, whence)
}

func (o *OSWrapper) Close() error {
	if !o.exists {
		return nil
	}
	err := o.file.Close()
	if err != nil {
		return err
	}
	o.file = nil
	return nil
}

func (o *OSWrapper) Name() string {
	return path.Base(o.filename)
}

func (o *OSWrapper) URI() string {
	return o.filename
}

func (o *OSWrapper) Delete(_ ...options.DeleteOption) error {
	return os.Remove(o.URI())
}

type ReadWriteSeekCloseURINamer interface {
	io.ReadWriteSeeker
	io.Closer
	Name() string
	URI() string
	Delete(opts ...options.DeleteOption) error
}

type ioTestSuite struct {
	suite.Suite
	testLocations map[string]vfs.Location
	localDir      string
}

/*
The following example shows how to setup the test suite to run against a local directory for unix file baseline

// setup local tests
osTemp, err := os.MkdirTemp("", "vfs-io-test")
s.Require().NoError(err)

// add baseline OS test
loc := osTemp + "/"
uris = append(uris, loc)

// add vfs os test
loc = "file://" + loc
uris = append(uris, loc)

*/

func (s *ioTestSuite) SetupSuite() {
	uris := make([]string, 0)

	// add VFS_INTEGRATION_LOCATIONS tests
	locs := os.Getenv("VFS_INTEGRATION_LOCATIONS")
	uris = append(uris, strings.Split(locs, ";")...)

	s.testLocations = make(map[string]vfs.Location)
	for idx := range uris {
		if strings.HasPrefix(uris[idx], "/") {
			s.localDir = uris[idx]
		} else {
			l, err := vfssimple.NewLocation(uris[idx])
			s.Require().NoError(err)
			switch l.FileSystem().Scheme() {
			case "file":
				s.testLocations[l.FileSystem().Scheme()] = CopyOsLocation(l)
			case "s3":
				s.testLocations[l.FileSystem().Scheme()] = CopyS3Location(l)
			case "sftp":
				s.testLocations[l.FileSystem().Scheme()] = CopySFTPLocation(l)
			case "gs":
				s.testLocations[l.FileSystem().Scheme()] = CopyGSLocation(l)
			case "mem":
				s.testLocations[l.FileSystem().Scheme()] = CopyMemLocation(l)
			case "https":
				s.testLocations[l.FileSystem().Scheme()] = CopyAzureLocation(l)
			case "ftp":
				s.testLocations[l.FileSystem().Scheme()] = CopyFTPLocation(l)
			default:
				panic(fmt.Sprintf("unknown scheme: %s", l.FileSystem().Scheme()))
			}
		}
	}
}

func (s *ioTestSuite) TestFileOperations() {
	if s.localDir != "" {
		s.Run("local", func() {
			s.testFileOperations(s.localDir)
		})
	}
	for scheme, location := range s.testLocations {
		s.Run(scheme, func() {
			s.testFileOperations(location.URI())
		})
	}
}

type TestCase struct {
	description       string
	sequence          string
	fileAlreadyExists bool
	expectFailure     bool
	expectedResults   string
}

// unless seek or read is called first, writes should replace a file (not edit)

func (s *ioTestSuite) testFileOperations(testPath string) {
	testCases := []TestCase{
		// Read, Close file
		{
			"Read, Close, file exists",
			"R(all);C()",
			true,
			false,
			"some text",
		},
		{
			"Read, CLose, file does not exist",
			"R(all);C()",
			false,
			true,
			"",
		},

		// Read, Seek, Read, Close
		{
			"Read, Seek, Read, Close, file exists",
			"R(4);S(0,0);R(4);C()",
			true,
			false,
			"some text",
		},

		// Write, Close
		{
			"Write, Close, file does not exist",
			"W(abc);C()",
			false,
			false,
			"abc",
		},
		{
			"Write, Close, file exists",
			"W(abc);C()",
			true,
			false,
			"abc",
		},

		// Write, Seek, Write, Close
		{
			"Write, Seek, Write, Close, file does not exist",
			"W(this and that);S(0,0);W(that);C()",
			false,
			false,
			"that and that",
		},
		{
			"Write, Seek, Write, Close, file exists",
			"W(this and that);S(0,0);W(that);C()",
			true,
			false,
			"that and that",
		},

		// Seek
		{
			"Seek, Close - file does not exist",
			"S(2,0);C()",
			false,
			true,
			"",
		},
		{
			"Seek, Close - file exists",
			"S(2,0);C()",
			true,
			false,
			"some text",
		},
		{
			"Seek, Write, Close, file exists",
			"S(5,0);W(new text);C()",
			true,
			false,
			"some new text",
		},

		// Seek, Read, Close
		{
			"Seek, Read, Close, file does not exist",
			"S(5,0);R(4);C()",
			false,
			true,
			"",
		},
		{
			"Seek, Read, Close, file exists",
			"S(5,0);R(4);C()",
			true,
			false,
			"some text",
		},

		// Read, Write, Close
		{
			"Read, Write, Close, file does not exist",
			"R(5);W(new text);C()",
			false,
			true,
			"",
		},
		{
			"Read, Write, Close, file exists",
			"R(5);W(new text);C()",
			true,
			false,
			"some new text",
		},

		// Read, Seek, Write, Close
		{
			"Read, Seek, Write, Close, file does not exist",
			"R(2);S(3,1);W(new text);C()",
			false,
			true,
			"",
		},
		{
			"Read, Seek, Write, Close, file exists",
			"R(2);S(3,1);W(new text);C()",
			true,
			false,
			"some new text",
		},

		// Write, Seek, Read, Close
		{
			"Write, Seek, Read, Close, file does not exist",
			"W(new text);S(0,0);R(5);C()",
			false,
			false,
			"new text",
		},
		{
			"Write, Seek, Read, Close, file exists",
			"W(new text);S(0,0);R(5);C()",
			true,
			false,
			"new text",
		},
	}

	defer s.teardownTestLocation(s.T(), testPath)
	for _, tc := range testCases {
		s.Run(tc.description, func() {
			testFileName := "testfile.txt"

			// run in a closure so we can defer teardown
			func() {
				// Setup vfs environment
				file, err := s.setupTestFile(tc.fileAlreadyExists, testPath, testFileName) // Implement this setup function
				defer func() {
					if file != nil {
						_ = file.Close()
						_ = file.Delete()
					}
				}()
				s.Require().NoError(err)

				// Use vfs to execute the sequence of operations described by the description
				actualContents, err := executeSequence(s.T(), file, tc.sequence) // Implement this function

				// Assert expected outcomes
				if tc.expectFailure && err == nil {
					s.Failf("%s: expected failure but got success", tc.description)
				}

				if err != nil && !tc.expectFailure {
					s.Failf("%s: expected success but got failure: %v", tc.description, err)
				}

				if tc.expectedResults != actualContents {
					s.Failf("%s: expected results %s but got %s", tc.description, tc.expectedResults, actualContents)
				}
			}()
		})
	}
}

//nolint:gocyclo
func executeSequence(t *testing.T, file ReadWriteSeekCloseURINamer, sequence string) (string, error) {
	// split sequence by semicolon
	commands := strings.Split(sequence, ";")
	var commandErr error
SEQ:
	for _, command := range commands {
		// parse command
		commandName, commandArgs := parseCommand(t, command)

		switch commandName {
		case "R":
			if commandArgs[0] == "all" {
				// Read entire file
				_, commandErr = io.ReadAll(file)
				if commandErr != nil {
					break SEQ
				}
			} else {
				// convert arg 0 to uint64
				bytesize, err := strconv.ParseUint(commandArgs[0], 10, 64)
				if err != nil {
					t.Fatalf("invalid bytesize: %s", commandArgs[0])
				}

				// Read file
				b := make([]byte, bytesize)
				_, commandErr = file.Read(b)
				if commandErr != nil {
					break SEQ
				}
			}
		case "W":
			// Write to file
			_, commandErr = file.Write([]byte(commandArgs[0]))
			if commandErr != nil {
				break SEQ
			}
		case "S":
			// expect 2 args for offset and whence
			if len(commandArgs) != 2 {
				t.Fatalf("invalid number of args for Seek: %d", len(commandArgs))
			}
			// convert args
			offset, err := strconv.ParseInt(commandArgs[0], 10, 64)
			if err != nil {
				t.Fatalf("invalid offset: %s", commandArgs[0])
			}
			whence, err := strconv.Atoi(commandArgs[1])
			if err != nil {
				t.Fatalf("invalid whence: %s", commandArgs[1])
			}
			// Seek
			_, commandErr = file.Seek(offset, whence)
			if commandErr != nil {
				break SEQ
			}
		case "C":
			// Close
			commandErr = file.Close()
			if commandErr != nil {
				break SEQ
			}
		}
	}
	// success so compare file contents to expected results
	if commandErr != nil {
		return "", commandErr
	}

	var f io.ReadCloser

	switch assertedFile := file.(type) {
	case *OSWrapper:
		var err error
		f, err = os.Open(assertedFile.URI())
		if err != nil {
			t.Fatalf("error opening file: %s", err.Error())
		}
	case vfs.File:
		var err error
		f, err = assertedFile.Location().NewFile(assertedFile.Name())
		if err != nil {
			t.Fatalf("error opening file: %s", err.Error())
		}
	}
	defer func() { _ = f.Close() }()
	// Read entire file
	contents, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("error reading file: %s", err.Error())
	}
	return string(contents), nil
}

var commandArgsRegex = regexp.MustCompile(`^([a-zA-Z0-9]+)\((.*)\)$`)

// takes command string in the form of <command name>(<args>) and returns the command name and args
func parseCommand(t *testing.T, command string) (string, []string) {
	// parse command string
	results := commandArgsRegex.FindStringSubmatch(command)
	if len(results) != 3 {
		t.Fatalf("invalid command string: %s", command)
	}

	// split args by comma
	args := strings.Split(results[2], ",")

	return results[1], args
}

func (s *ioTestSuite) setupTestFile(existsBefore bool, loc, filename string) (ReadWriteSeekCloseURINamer, error) {
	var f ReadWriteSeekCloseURINamer
	var err error
	// Create file
	if strings.HasPrefix(loc, "/") {
		f = NewOSWrapper(loc + filename)
	} else {
		scheme := strings.Split(loc, ":")[0]
		// Write something to the file
		f, err = s.testLocations[scheme].NewFile(filename)
		if err != nil {
			return nil, err
		}
	}
	if existsBefore {
		_, err = f.Write([]byte("some text"))
		if err != nil {
			return nil, err
		}
		err = f.Close()
		if err != nil {
			return nil, err
		}
	}

	return f, nil
}

func (s *ioTestSuite) teardownTestLocation(t *testing.T, testPath string) {
	if strings.HasPrefix(testPath, "/") {
		err := os.RemoveAll(testPath)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		scheme := strings.Split(testPath, ":")[0]
		// Write something to the file
		loc := s.testLocations[scheme]
		files, err := loc.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			err := loc.DeleteFile(file)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestIOTestSuite(t *testing.T) {
	suite.Run(t, new(ioTestSuite))
}
