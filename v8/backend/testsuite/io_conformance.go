package testsuite

import (
	"context"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	vfs "github.com/c2fo/vfs/v8"
)

// ioSeqFile is the minimal surface used by [ExecuteSequence] for scripted I/O tests.
type ioSeqFile interface {
	io.ReadWriteSeeker
	io.Closer
	vfs.File
}

// IOTestCase defines a single scripted I/O scenario.
type IOTestCase struct {
	Description       string
	Sequence          string
	FileAlreadyExists bool
	ExpectFailure     bool
	ExpectedResults   string
}

// DefaultIOTestCases returns the standard IO test matrix (requires [io.Seeker] on the file handle).
func DefaultIOTestCases() []IOTestCase {
	return []IOTestCase{
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
		{
			Description:       "Read, Seek, Read, Close, file exists",
			Sequence:          "R(4);S(0,0);R(4);C()",
			FileAlreadyExists: true,
			ExpectFailure:     false,
			ExpectedResults:   "some text",
		},
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

// RunIOTests runs scripted IO conformance tests against location. The backend must return
// files that implement [io.Seeker]; otherwise tests are skipped.
func RunIOTests(t *testing.T, location vfs.Location) {
	t.Helper()
	ctx := context.Background()

	probe, err := location.NewFile(".vfs-io-probe.txt")
	require.NoError(t, err)
	if _, ok := probe.(io.Seeker); !ok {
		require.NoError(t, probe.Close())
		t.Skip("backend file does not implement io.Seeker; skipping IO sequence tests")
	}
	require.NoError(t, probe.Close())

	runIOTestsWithCases(t, ctx, location.URI(), location, DefaultIOTestCases())
}

func runIOTestsWithCases(t *testing.T, ctx context.Context, testPath string, location vfs.Location, testCases []IOTestCase) {
	t.Helper()
	defer teardownTestLocation(t, ctx, testPath, location)

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			testFileName := "testfile.txt"

			func() {
				file, err := setupTestFile(tc.FileAlreadyExists, location, testFileName)
				defer func() {
					if file != nil {
						_ = file.Close()
						_ = location.DeleteFile(testFileName)
					}
				}()
				require.NoError(t, err)

				seqFile, ok := file.(ioSeqFile)
				if !ok {
					t.Fatalf("file must implement io.ReadWriteSeeker and vfs.File")
				}

				actualContents, err := ExecuteSequence(t, seqFile, tc.Sequence)

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

func setupTestFile(existsBefore bool, location vfs.Location, filename string) (vfs.File, error) {
	f, err := location.NewFile(filename)
	if err != nil {
		return nil, err
	}

	if existsBefore {
		if _, err = f.Write([]byte("some text")); err != nil {
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}

	return f, nil
}

func teardownTestLocation(t *testing.T, ctx context.Context, _ string, location vfs.Location) {
	t.Helper()
	names, err := CollectList(ctx, location)
	if err != nil {
		t.Logf("warning: error listing files for cleanup: %v", err)
		return
	}
	for _, name := range names {
		if err := location.DeleteFile(name); err != nil {
			t.Logf("warning: error deleting file %s: %v", name, err)
		}
	}
}

// ExecuteSequence executes a semicolon-separated command sequence and returns persisted contents.
//
//nolint:gocyclo
func ExecuteSequence(t *testing.T, file ioSeqFile, sequence string) (string, error) {
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

	f, err := file.Location().NewFile(file.Name())
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
