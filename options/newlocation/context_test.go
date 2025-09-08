package newlocation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/options/newlocation"
)

func TestWithContext(t *testing.T) {
	is := require.New(t)

	ctx := context.WithoutCancel(t.Context())
	opt := newlocation.WithContext(ctx)

	nfc, ok := opt.(*newlocation.Context)
	is.True(ok)
	is.Equal(ctx, nfc.Context)
	is.Equal("newLocationContext", nfc.NewLocationOptionName())
}
