package testsuite

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
)

// ReadWriteSeekCloseURINamer interface for IO testing
type ReadWriteSeekCloseURINamer interface {
	io.ReadWriteSeeker
	io.Closer
	Name() string
	URI() string
	Delete(opts ...options.DeleteOption) error
}

// IOTestCase defines a single IO test scenario
type IOTestCase struct {
	Description       string
	Sequence          string
	FileAlreadyExists bool
	ExpectFailure     bool
	ExpectedResults   string
}

// DefaultIOTestCases returns the standard set of IO test cases
func DefaultIOTestCases() []IOTestCase {
	return []IOTestCase{
		// Read, Close file
		{
			Description:       "Read, Close, file exists",
			Sequence:          "R(all);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "some text",
		},
		{
			Description:       "Read, Close, file does not exist",
			Sequence:          "R(all);C()",
			FileAlreadyExists: false,
			ExpectFailure:     true,
			ExpectedResults:   "",
		},

		// Read, Seek, Read, Close
		{
			Description:       "Read, Seek, Read, Close, file exists",
			Sequence:          "R(4);S(0,0);R(4);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "some text",
		},

		// Write, Close
		{
			Description:       "Write, Close, file does not exist",
			Sequence:          "W(abc);C()",
			FileAlreadyExists: false,
			ExpectFailure:     false,
			ExpectedResults:   "abc",
		},
		{
			Description:       "Write, Close, file exists",
			Sequence:          "W(abc);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "abc",
		},

		// Write, Seek, Write, Close
		{
			Description:       "Write, Seek, Write, Close, file does not exist",
			Sequence:          "W(this and that);S(0,0);W(that);C()",
			FileAlreadyExists: false,
			ExpectFailure:     false,
			ExpectedResults:   "that and that",
		},
		{
			Description:       "Write, Seek, Write, Close, file exists",
			Sequence:          "W(this and that);S(0,0);W(that);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "that and that",
		},

		// Seek
		{
			Description:       "Seek, Close - file does not exist",
			Sequence:          "S(2,0);C()",
			FileAlreadyExists: false,
			ExpectFailure:     true,
			ExpectedResults:   "",
		},
		{
			Description:       "Seek, Close - file exists",
			Sequence:          "S(2,0);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "some text",
		},
		{
			Description:       "Seek, Write, Close, file exists",
			Sequence:          "S(5,0);W(new text);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "some new text",
		},

		// Seek, Read, Close
		{
			Description:       "Seek, Read, Close, file does not exist",
			Sequence:          "S(5,0);R(4);C()",
			FileAlreadyExists: false,
			ExpectFailure:     true,
			ExpectedResults:   "",
		},
		{
			Description:       "Seek, Read, Close, file exists",
			Sequence:          "S(5,0);R(4);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "some text",
		},

		// Read, Write, Close
		{
			Description:       "Read, Write, Close, file does not exist",
			Sequence:          "R(5);W(new text);C()",
			FileAlreadyExists: false,
			ExpectFailure:     true,
			ExpectedResults:   "",
		},
		{
			Description:       "Read, Write, Close, file exists",
			Sequence:          "R(5);W(new text);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "some new text",
		},

		// Read, Seek, Write, Close
		{
			Description:       "Read, Seek, Write, Close, file does not exist",
			Sequence:          "R(2);S(3,1);W(new text);C()",
			FileAlreadyExists: false,
			ExpectFailure:     true,
			ExpectedResults:   "",
		},
		{
			Description:       "Read, Seek, Write, Close, file exists",
			Sequence:          "R(2);S(3,1);W(new text);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "some new text",
		},

		// Write, Seek, Read, Close
		{
			Description:       "Write, Seek, Read, Close, file does not exist",
			Sequence:          "W(new text);S(0,0);R(5);C()",
			FileAlreadyExists: false,
			ExpectFailure:     false,
			ExpectedResults:   "new text",
		},
		{
			Description:       "Write, Seek, Read, Close, file exists",
			Sequence:          "W(new text);S(0,0);R(5);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "new text",
		},
	}
}

// RunIOTests runs IO conformance tests against the provided location
func RunIOTests(t *testing.T, location vfs.Location) {
	t.Helper()
	runIOTestsWithCases(t, location.URI(), location, DefaultIOTestCases())
}

func runIOTestsWithCases(t *testing.T, testPath string, location vfs.Location, testCases []IOTestCase) {
	t.Helper()
	defer teardownTestLocation(t, testPath, location)

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			testFileName := "testfile.txt"

			func() {
				file, err := setupTestFile(tc.FileAlreadyExists, location, testFileName)
				defer func() {
					if file != nil {
						_ = file.Close()
						_ = file.Delete()
					}
				}()
				require.NoError(t, err)

				actualContents, err := ExecuteSequence(t, file, tc.Sequence)

				if tc.ExpectFailure && err == nil {
					t.Fatalf("%s: expected failure but got success", tc.Description)
				}

				if err != nil && !tc.ExpectFailure {
					t.Fatalf("%s: expected success but got failure: %v", tc.Description, err)
				}

				if tc.ExpectedResults != actualContents {
					t.Fatalf("%s: expected results %s but got %s", tc.Description, tc.ExpectedResults, actualContents)
				}
			}()
		})
	}
}

func setupTestFile(existsBefore bool, location vfs.Location, filename string) (ReadWriteSeekCloseURINamer, error) {
	f, err := location.NewFile(filename)
	if err != nil {
		return nil, err
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

func teardownTestLocation(t *testing.T, _ string, location vfs.Location) {
	t.Helper()
	files, err := location.List()
	if err != nil {
		t.Logf("warning: error listing files for cleanup: %v", err)
		return
	}
	for _, file := range files {
		err := location.DeleteFile(file)
		if err != nil {
			t.Logf("warning: error deleting file %s: %v", file, err)
		}
	}
}

// ExecuteSequence executes a sequence of IO operations and returns the final file contents
//
//nolint:gocyclo
func ExecuteSequence(t *testing.T, file ReadWriteSeekCloseURINamer, sequence string) (string, error) {
	t.Helper()
	commands := strings.Split(sequence, ";")
	var commandErr error
SEQ:
	for _, command := range commands {
		commandName, commandArgs := parseCommand(t, command)

		switch commandName {
		case "R":
			if commandArgs[0] == "all" {
				_, commandErr = io.ReadAll(file)
				if commandErr != nil {
					break SEQ
				}
			} else {
				bytesize, err := strconv.ParseUint(commandArgs[0], 10, 64)
				if err != nil {
					t.Fatalf("invalid bytesize: %s", commandArgs[0])
				}
				b := make([]byte, bytesize)
				_, commandErr = file.Read(b)
				if commandErr != nil {
					break SEQ
				}
			}
		case "W":
			_, commandErr = file.Write([]byte(commandArgs[0]))
			if commandErr != nil {
				break SEQ
			}
		case "S":
			if len(commandArgs) != 2 {
				t.Fatalf("invalid number of args for Seek: %d", len(commandArgs))
			}
			offset, err := strconv.ParseInt(commandArgs[0], 10, 64)
			if err != nil {
				t.Fatalf("invalid offset: %s", commandArgs[0])
			}
			whence, err := strconv.Atoi(commandArgs[1])
			if err != nil {
				t.Fatalf("invalid whence: %s", commandArgs[1])
			}
			_, commandErr = file.Seek(offset, whence)
			if commandErr != nil {
				break SEQ
			}
		case "C":
			commandErr = file.Close()
			if commandErr != nil {
				break SEQ
			}
		}
	}

	if commandErr != nil {
		return "", commandErr
	}

	vfsFile, ok := file.(vfs.File)
	if !ok {
		t.Fatalf("file must implement vfs.File")
	}
	f, err := vfsFile.Location().NewFile(vfsFile.Name())
	if err != nil {
		t.Fatalf("error opening file: %s", err.Error())
	}
	defer func() { _ = f.Close() }()

	contents, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("error reading file: %s", err.Error())
	}
	return string(contents), nil
}

var commandArgsRegex = regexp.MustCompile(`^([a-zA-Z0-9]+)\((.*)\)$`)

func parseCommand(t *testing.T, command string) (string, []string) {
	t.Helper()
	results := commandArgsRegex.FindStringSubmatch(command)
	if len(results) != 3 {
		t.Fatalf("invalid command string: %s", command)
	}
	args := strings.Split(results[2], ",")
	return results[1], args
}
