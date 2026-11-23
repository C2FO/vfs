package writebuffer

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/suite"
)

type WriteBufferTestSuite struct {
	suite.Suite
	saved []Handler
}

func (s *WriteBufferTestSuite) SetupTest() {
	mu.Lock()
	defer mu.Unlock()
	s.saved = handlers
	handlers = nil
}

func (s *WriteBufferTestSuite) TearDownTest() {
	mu.Lock()
	defer mu.Unlock()
	handlers = s.saved
}

type claimable struct {
	io.Writer
	enabled bool
}

func (s *WriteBufferTestSuite) TestEnableEphemeralStreaming() {
	Register(func(w io.Writer) (func(), bool) {
		c, ok := w.(*claimable)
		if !ok {
			return nil, false
		}
		c.enabled = true
		return func() { c.enabled = false }, true
	})

	tests := []struct {
		name            string
		writer          io.Writer
		expectedClaimed bool
	}{
		{name: "claimed writer is enabled and then reset", writer: &claimable{Writer: io.Discard}, expectedClaimed: true},
		{name: "unclaimed writer gets a no-op reset", writer: &bytes.Buffer{}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			reset := EnableEphemeralStreaming(tt.writer)
			s.Require().NotNil(reset)
			if c, ok := tt.writer.(*claimable); ok {
				s.Equal(tt.expectedClaimed, c.enabled)
				reset()
				s.False(c.enabled)
				return
			}
			s.NotPanics(reset)
		})
	}
}

func (s *WriteBufferTestSuite) TestNoHandlers() {
	reset := EnableEphemeralStreaming(&bytes.Buffer{})
	s.Require().NotNil(reset)
	s.NotPanics(reset)
}

func TestWriteBufferTestSuite(t *testing.T) {
	suite.Run(t, new(WriteBufferTestSuite))
}
