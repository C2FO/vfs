// Code generated by mockery v2.46.3. DO NOT EDIT.

package mocks

import (
	vfs "github.com/c2fo/vfs/v6"
	mock "github.com/stretchr/testify/mock"
)

// FileSystem is an autogenerated mock type for the FileSystem type
type FileSystem struct {
	mock.Mock
}

type FileSystem_Expecter struct {
	mock *mock.Mock
}

func (_m *FileSystem) EXPECT() *FileSystem_Expecter {
	return &FileSystem_Expecter{mock: &_m.Mock}
}

// Name provides a mock function with given fields:
func (_m *FileSystem) Name() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Name")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// FileSystem_Name_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Name'
type FileSystem_Name_Call struct {
	*mock.Call
}

// Name is a helper method to define mock.On call
func (_e *FileSystem_Expecter) Name() *FileSystem_Name_Call {
	return &FileSystem_Name_Call{Call: _e.mock.On("Name")}
}

func (_c *FileSystem_Name_Call) Run(run func()) *FileSystem_Name_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *FileSystem_Name_Call) Return(_a0 string) *FileSystem_Name_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *FileSystem_Name_Call) RunAndReturn(run func() string) *FileSystem_Name_Call {
	_c.Call.Return(run)
	return _c
}

// NewFile provides a mock function with given fields: volume, absFilePath
func (_m *FileSystem) NewFile(volume string, absFilePath string) (vfs.File, error) {
	ret := _m.Called(volume, absFilePath)

	if len(ret) == 0 {
		panic("no return value specified for NewFile")
	}

	var r0 vfs.File
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string) (vfs.File, error)); ok {
		return rf(volume, absFilePath)
	}
	if rf, ok := ret.Get(0).(func(string, string) vfs.File); ok {
		r0 = rf(volume, absFilePath)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(vfs.File)
		}
	}

	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(volume, absFilePath)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FileSystem_NewFile_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'NewFile'
type FileSystem_NewFile_Call struct {
	*mock.Call
}

// NewFile is a helper method to define mock.On call
//   - volume string
//   - absFilePath string
func (_e *FileSystem_Expecter) NewFile(volume interface{}, absFilePath interface{}) *FileSystem_NewFile_Call {
	return &FileSystem_NewFile_Call{Call: _e.mock.On("NewFile", volume, absFilePath)}
}

func (_c *FileSystem_NewFile_Call) Run(run func(volume string, absFilePath string)) *FileSystem_NewFile_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string))
	})
	return _c
}

func (_c *FileSystem_NewFile_Call) Return(_a0 vfs.File, _a1 error) *FileSystem_NewFile_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *FileSystem_NewFile_Call) RunAndReturn(run func(string, string) (vfs.File, error)) *FileSystem_NewFile_Call {
	_c.Call.Return(run)
	return _c
}

// NewLocation provides a mock function with given fields: volume, absLocPath
func (_m *FileSystem) NewLocation(volume string, absLocPath string) (vfs.Location, error) {
	ret := _m.Called(volume, absLocPath)

	if len(ret) == 0 {
		panic("no return value specified for NewLocation")
	}

	var r0 vfs.Location
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string) (vfs.Location, error)); ok {
		return rf(volume, absLocPath)
	}
	if rf, ok := ret.Get(0).(func(string, string) vfs.Location); ok {
		r0 = rf(volume, absLocPath)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(vfs.Location)
		}
	}

	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(volume, absLocPath)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FileSystem_NewLocation_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'NewLocation'
type FileSystem_NewLocation_Call struct {
	*mock.Call
}

// NewLocation is a helper method to define mock.On call
//   - volume string
//   - absLocPath string
func (_e *FileSystem_Expecter) NewLocation(volume interface{}, absLocPath interface{}) *FileSystem_NewLocation_Call {
	return &FileSystem_NewLocation_Call{Call: _e.mock.On("NewLocation", volume, absLocPath)}
}

func (_c *FileSystem_NewLocation_Call) Run(run func(volume string, absLocPath string)) *FileSystem_NewLocation_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string), args[1].(string))
	})
	return _c
}

func (_c *FileSystem_NewLocation_Call) Return(_a0 vfs.Location, _a1 error) *FileSystem_NewLocation_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *FileSystem_NewLocation_Call) RunAndReturn(run func(string, string) (vfs.Location, error)) *FileSystem_NewLocation_Call {
	_c.Call.Return(run)
	return _c
}

// Retry provides a mock function with given fields:
func (_m *FileSystem) Retry() vfs.Retry {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Retry")
	}

	var r0 vfs.Retry
	if rf, ok := ret.Get(0).(func() vfs.Retry); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(vfs.Retry)
		}
	}

	return r0
}

// FileSystem_Retry_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Retry'
type FileSystem_Retry_Call struct {
	*mock.Call
}

// Retry is a helper method to define mock.On call
func (_e *FileSystem_Expecter) Retry() *FileSystem_Retry_Call {
	return &FileSystem_Retry_Call{Call: _e.mock.On("Retry")}
}

func (_c *FileSystem_Retry_Call) Run(run func()) *FileSystem_Retry_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *FileSystem_Retry_Call) Return(_a0 vfs.Retry) *FileSystem_Retry_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *FileSystem_Retry_Call) RunAndReturn(run func() vfs.Retry) *FileSystem_Retry_Call {
	_c.Call.Return(run)
	return _c
}

// Scheme provides a mock function with given fields:
func (_m *FileSystem) Scheme() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Scheme")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// FileSystem_Scheme_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Scheme'
type FileSystem_Scheme_Call struct {
	*mock.Call
}

// Scheme is a helper method to define mock.On call
func (_e *FileSystem_Expecter) Scheme() *FileSystem_Scheme_Call {
	return &FileSystem_Scheme_Call{Call: _e.mock.On("Scheme")}
}

func (_c *FileSystem_Scheme_Call) Run(run func()) *FileSystem_Scheme_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *FileSystem_Scheme_Call) Return(_a0 string) *FileSystem_Scheme_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *FileSystem_Scheme_Call) RunAndReturn(run func() string) *FileSystem_Scheme_Call {
	_c.Call.Return(run)
	return _c
}

// NewFileSystem creates a new instance of FileSystem. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewFileSystem(t interface {
	mock.TestingT
	Cleanup(func())
}) *FileSystem {
	mock := &FileSystem{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
