package testsuite

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
)

// newMemIOLocation returns an isolated in-memory location for IO testing.
func newMemIOLocation(t *testing.T) vfs.Location {
	t.Helper()
	loc, err := mem.NewFileSystem().NewLocation("testvolume", "/vfs-io-test/")
	require.NoError(t, err)
	return loc
}

// TestRunIOTests_Mem exercises the full IO conformance suite against the
// in-memory backend, which supports every sequence (including partial writes).
// This gives the conformance runner, ExecuteSequence, and the helpers real
// coverage in the default (non-vfsintegration) test run.
func TestRunIOTests_Mem(t *testing.T) {
	RunIOTests(t, newMemIOLocation(t))
}

// TestRunIOTests_MemSkipFTP runs the suite with SkipFTPSpecificTests set,
// exercising the skip path. The in-memory backend supports the skipped
// sequences, so the run must still pass with those cases skipped.
func TestRunIOTests_MemSkipFTP(t *testing.T) {
	RunIOTests(t, newMemIOLocation(t), ConformanceOptions{SkipFTPSpecificTests: true})
}

// TestDefaultIOTestCases_SeekWithinWriteFlag locks the mapping between IO test
// cases and the RequiresSeekWithinWrite flag so a renamed description or a new
// case can't silently change which sequences FTP-style backends skip.
func TestDefaultIOTestCases_SeekWithinWriteFlag(t *testing.T) {
	seekWithinWrite := make(map[string]bool)
	for _, tc := range DefaultIOTestCases() {
		seekWithinWrite[tc.Description] = tc.RequiresSeekWithinWrite
	}

	want := map[string]bool{
		"Write, Seek, Write, Close, file does not exist": true,
		"Write, Seek, Write, Close, file exists":         true,
	}

	trueCount := 0
	for desc, flagged := range seekWithinWrite {
		if flagged {
			trueCount++
			require.Truef(t, want[desc], "unexpected case flagged RequiresSeekWithinWrite: %q", desc)
		}
	}
	require.Equalf(t, len(want), trueCount,
		"expected exactly %d cases flagged RequiresSeekWithinWrite, got %d", len(want), trueCount)
}

// eofInjectingFile wraps a vfs.File and returns io.EOF alongside the final
// bytes of a fully-satisfied read, simulating backends (e.g. s3) that signal
// EOF on the read that reaches the end of the file rather than on the next one.
type eofInjectingFile struct {
	vfs.File
}

func (f *eofInjectingFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if err == nil && n > 0 && n == len(p) {
		return n, io.EOF
	}
	return n, err
}

// TestExecuteSequence_EOFOnFullRead verifies that a fixed-size read returning
// io.EOF together with data is treated as a successful read: the sequence must
// continue (so the trailing Close runs) and report no error.
func TestExecuteSequence_EOFOnFullRead(t *testing.T) {
	loc := newMemIOLocation(t)

	seed, err := loc.NewFile("eoftest.txt")
	require.NoError(t, err)
	_, err = seed.Write([]byte("some text"))
	require.NoError(t, err)
	require.NoError(t, seed.Close())

	readFile, err := loc.NewFile("eoftest.txt")
	require.NoError(t, err)
	wrapped := &eofInjectingFile{File: readFile}

	// R(9) reads all 9 bytes and the wrapper injects io.EOF; the sequence must
	// still reach C() and succeed.
	contents, err := ExecuteSequence(t, wrapped, "R(9);C()")
	require.NoError(t, err)
	require.Equal(t, "some text", contents)
}
