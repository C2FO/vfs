package ftp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataConnErr_Error(t *testing.T) {
	tests := []struct {
		err      dataConnErr
		expected string
	}{
		{singleOpInvalidDataconnType, "dataconn must be open for single op mode to conduct a single op action"},
		{readInvalidDataconnType, "dataconn must be open for read mode to conduct a read"},
		{writeInvalidDataconnType, "dataconn must be open for write mode to conduct a write"},
	}

	for _, tt := range tests {
		require.EqualError(t, tt.err, tt.expected)
	}
}
