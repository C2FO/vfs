package vfs

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopy_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var dst strings.Builder
	src := strings.NewReader("hello")

	_, err := Copy(ctx, &dst, src)
	require.Error(t, err)
}

func TestCopy_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var dst bytes.Buffer
	src := strings.NewReader("data")

	n, err := Copy(ctx, &dst, src)
	require.NoError(t, err)
	require.Equal(t, int64(4), n)
	require.Equal(t, "data", dst.String())
}
