package backend

import (
	"github.com/c2fo/vfs/v4/mocks"
	"testing"

	"github.com/stretchr/testify/suite"
)

/**********************************
 ************TESTS*****************
 **********************************/

type testSuite struct {
	suite.Suite
}

func (s *testSuite) TestBackend() {
	//
	m1 := &mocks.FileSystem{}
	Register("mock", m1)

	// register a new backend
	m2 := &mocks.FileSystem{}
	Register("new mock", m2)

	// register another backend
	m3 := &mocks.FileSystem{}
	Register("newest mock", m3)

	// return backend
	b := Backend("new mock")
	s.IsType(&mocks.FileSystem{}, b, "type is mocks.Filesystem")

	// check all RegisteredBackends names
	s.Len(RegisteredBackends(), 3, "found 3 backends")

	//Unregister a backend
	Unregister("newest mock")
	s.Len(RegisteredBackends(), 2, "found 2 backends")

	//Unregister all backends
	UnregisterAll()
	s.Len(RegisteredBackends(), 0, "found 0 backends")
}

func TestBackend(t *testing.T) {
	suite.Run(t, new(testSuite))
}
