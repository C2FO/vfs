package ftp

import (
	"context"
	"testing"

	"github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/assert"
)

func TestWithClient(t *testing.T) {
	client := &ftp.ServerConn{}
	fs := &FileSystem{}

	opt := WithClient(client)
	opt.Apply(fs)

	assert.Equal(t, client, fs.ftpclient, "Client should be set correctly")
}

func TestWithOptions(t *testing.T) {
	options := Options{}
	fs := &FileSystem{}

	opt := WithOptions(options)
	opt.Apply(fs)

	assert.Equal(t, options, fs.options, "Options should be set correctly")
}

func TestWithContext(t *testing.T) {
	ctx := context.Background()
	fs := &FileSystem{}

	opt := WithContext(ctx)
	opt.Apply(fs)

	assert.Equal(t, ctx, fs.ctx, "Context should be set correctly")
}
