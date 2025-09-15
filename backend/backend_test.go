package backend

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/mocks"
)

/**********************************
 ************TESTS*****************
 **********************************/

type testSuite struct {
	suite.Suite
}

func (s *testSuite) TestBackend() {
	//
	m1 := mocks.NewFileSystem(s.T())
	Register("mock", m1)

	// register a new backend
	m2 := mocks.NewFileSystem(s.T())
	Register("new mock", m2)

	// register another backend
	m3 := mocks.NewFileSystem(s.T())
	Register("newest mock", m3)

	// get backend
	b := Backend("new mock")
	s.IsType((*mocks.FileSystem)(nil), b, "type is mocks.FileSystem")

	// check all RegisteredBackends names
	s.Len(RegisteredBackends(), 3, "found 3 backends")

	// Unregister a backend
	Unregister("newest mock")
	s.Len(RegisteredBackends(), 2, "found 2 backends")

	// Unregister all backends
	UnregisterAll()
	s.Empty(RegisteredBackends(), "found 0 backends")
}

func TestBackend(t *testing.T) {
	suite.Run(t, new(testSuite))
}
