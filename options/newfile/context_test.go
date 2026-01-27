package newfile_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/options/newfile"
)

func TestWithContext(t *testing.T) {
	is := require.New(t)

	ctx := context.WithoutCancel(t.Context())
	opt := newfile.WithContext(ctx)

	nfc, ok := opt.(*newfile.Context)
	is.True(ok)
	is.Equal(ctx, nfc.Context)
	is.Equal("newFileContext", nfc.NewFileOptionName())
}
