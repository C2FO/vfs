//go:build vfsintegration && (linux || darwin)

package testcontainers

import (
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// limitFileSize lowers the soft RLIMIT_FSIZE so large local writes fail with EFBIG, which is how a full or
// quota-limited temp volume looks to the write buffer. It returns a restore function.
func (s *containerSuite) limitFileSize(limit uint64) func() {
	var orig syscall.Rlimit
	s.Require().NoError(syscall.Getrlimit(syscall.RLIMIT_FSIZE, &orig))

	// exceeding the limit raises SIGXFSZ, which kills the process unless it is ignored
	signal.Ignore(syscall.SIGXFSZ)
	s.Require().NoError(syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: limit, Max: orig.Max}))

	return func() {
		_ = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &orig)
		signal.Reset(syscall.SIGXFSZ)
	}
}

func (s *containerSuite) writeWithFileSizeLimit(w io.Writer, data []byte, limit uint64) error {
	restore := s.limitFileSize(limit)
	defer restore()

	_, err := w.Write(data)
	return err
}

// TestBufferedWriteFailureCommitsNothing checks that when the local write buffer fails mid-stream, the error hints at
// streaming and the partially streamed object is never committed to the bucket.
func (s *containerSuite) TestBufferedWriteFailureCommitsNothing() {
	const limit = 1 << 20

	for _, tgt := range s.objectTargets() {
		s.Run(tgt.name, func() {
			tmp := s.isolateTempDir()

			// confirm the limit is enforced on this platform before relying on it
			probe, err := os.CreateTemp(tmp, "probe")
			s.Require().NoError(err)
			probeErr := s.writeWithFileSizeLimit(probe, make([]byte, 2*limit), limit)
			_ = probe.Close()
			_ = os.Remove(probe.Name())
			if probeErr == nil {
				s.T().Skip("RLIMIT_FSIZE is not enforced on this platform")
			}

			name := streamName("abort")
			f, err := tgt.location.NewFile(name)
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = f.Delete() })

			_, err = f.Write(make([]byte, limit/2))
			s.Require().NoError(err)

			err = s.writeWithFileSizeLimit(f, make([]byte, 2*limit), limit)
			s.Require().Error(err)
			s.Contains(err.Error(), "newfile.WithStreamingWrite()")
			s.requireEmptyDir(tmp)

			s.Require().Error(f.Close(), "Close must report the failed write")

			// abandoning the upload is asynchronous, so watch for a short while
			for range 20 {
				exists, err := f.Exists()
				s.Require().NoError(err)
				s.Require().False(exists, "no truncated object may be committed")
				time.Sleep(100 * time.Millisecond)
			}
		})
	}
}
