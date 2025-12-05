package newfile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/options/newfile"
)

func TestWithContentType(t *testing.T) {
	opt := newfile.WithContentType("application/json")

	ct, ok := opt.(*newfile.ContentType)
	require.Truef(t, ok, "expected `*newfile.ContentType`, got %T", opt)
	assert.Equal(t, newfile.ContentType("application/json"), *ct)
	assert.Equal(t, "newFileContentType", ct.NewFileOptionName())
}
