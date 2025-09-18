package utils_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/utils"
)

/**********************************
 ************TESTS*****************
 **********************************/

type errorsSuite struct {
	suite.Suite
}

// TestErrorWrapFunctions tests all error wrap functions with both nil and non-nil errors
func (s *errorsSuite) TestErrorWrapFunctions() {
	testError := errors.New("test error")

	testCases := []struct {
		name        string
		wrapFunc    func(error) error
		expectedMsg string
	}{
		{
			name:        "WrapReadError",
			wrapFunc:    utils.WrapReadError,
			expectedMsg: "read error: test error",
		},
		{
			name:        "WrapSeekError",
			wrapFunc:    utils.WrapSeekError,
			expectedMsg: "seek error: test error",
		},
		{
			name:        "WrapWriteError",
			wrapFunc:    utils.WrapWriteError,
			expectedMsg: "write error: test error",
		},
		{
			name:        "WrapCloseError",
			wrapFunc:    utils.WrapCloseError,
			expectedMsg: "close error: test error",
		},
		{
			name:        "WrapTouchError",
			wrapFunc:    utils.WrapTouchError,
			expectedMsg: "touch error: test error",
		},
		{
			name:        "WrapExistsError",
			wrapFunc:    utils.WrapExistsError,
			expectedMsg: "exists error: test error",
		},
		{
			name:        "WrapSizeError",
			wrapFunc:    utils.WrapSizeError,
			expectedMsg: "size error: test error",
		},
		{
			name:        "WrapLastModifiedError",
			wrapFunc:    utils.WrapLastModifiedError,
			expectedMsg: "lastModified error: test error",
		},
		{
			name:        "WrapDeleteError",
			wrapFunc:    utils.WrapDeleteError,
			expectedMsg: "delete error: test error",
		},
		{
			name:        "WrapCopyToLocationError",
			wrapFunc:    utils.WrapCopyToLocationError,
			expectedMsg: "copyToLocation error: test error",
		},
		{
			name:        "WrapCopyToFileError",
			wrapFunc:    utils.WrapCopyToFileError,
			expectedMsg: "copyToFile error: test error",
		},
		{
			name:        "WrapMoveToLocationError",
			wrapFunc:    utils.WrapMoveToLocationError,
			expectedMsg: "moveToLocation error: test error",
		},
		{
			name:        "WrapMoveToFileError",
			wrapFunc:    utils.WrapMoveToFileError,
			expectedMsg: "moveToFile error: test error",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name+"_WithError", func() {
			err := tc.wrapFunc(testError)
			s.Require().Error(err, "should return an error when given a non-nil error")
			s.Require().EqualError(err, tc.expectedMsg, "error message should be properly wrapped")
		})

		s.Run(tc.name+"_WithNil", func() {
			result := tc.wrapFunc(nil)
			s.Require().NoError(result, "should return nil when given a nil error")
		})
	}
}

// TestErrorWrapFunctionsWithUnwrap tests that wrapped errors can be unwrapped
func (s *errorsSuite) TestErrorWrapFunctionsWithUnwrap() {
	originalError := errors.New("original error")

	testCases := []struct {
		name     string
		wrapFunc func(error) error
	}{
		{"WrapReadError", utils.WrapReadError},
		{"WrapSeekError", utils.WrapSeekError},
		{"WrapWriteError", utils.WrapWriteError},
		{"WrapCloseError", utils.WrapCloseError},
		{"WrapTouchError", utils.WrapTouchError},
		{"WrapExistsError", utils.WrapExistsError},
		{"WrapSizeError", utils.WrapSizeError},
		{"WrapLastModifiedError", utils.WrapLastModifiedError},
		{"WrapDeleteError", utils.WrapDeleteError},
		{"WrapCopyToLocationError", utils.WrapCopyToLocationError},
		{"WrapCopyToFileError", utils.WrapCopyToFileError},
		{"WrapMoveToLocationError", utils.WrapMoveToLocationError},
		{"WrapMoveToFileError", utils.WrapMoveToFileError},
	}

	for _, tc := range testCases {
		s.Run(tc.name+"_Unwrap", func() {
			wrappedError := tc.wrapFunc(originalError)
			s.Require().Error(wrappedError, "wrapped error should not be nil")

			// Test that the original error can be unwrapped
			s.Require().ErrorIs(wrappedError, originalError, "should be able to unwrap to original error")
		})
	}
}

func TestErrors(t *testing.T) {
	suite.Run(t, new(errorsSuite))
}
