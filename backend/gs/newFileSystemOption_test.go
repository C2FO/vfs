package gs

import (
	"reflect"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
)

func TestWithClient(t *testing.T) {
	client := &storage.Client{}
	fs := &FileSystem{}

	opt := WithClient(client)
	opt.Apply(fs)

	assert.Equal(t, client, fs.client, "Client should be set correctly")
}

func TestWithOptions(t *testing.T) {
	options := Options{}
	fs := &FileSystem{}

	opt := WithOptions(options)
	opt.Apply(fs)

	assert.Equal(t, options, fs.options, "Options should be set correctly")
}

func TestWithContext(t *testing.T) {
	ctx := t.Context()
	fs := &FileSystem{}

	opt := WithContext(ctx)
	opt.Apply(fs)

	assert.Equal(t, ctx, fs.ctx, "Context should be set correctly")
}

func TestWithRetryer(t *testing.T) {
	retryer := func(wrapped func() error) error { return wrapped() }
	fs := &FileSystem{}

	opt := WithRetryer(retryer)
	opt.Apply(fs)

	assert.NotNil(t, fs.retryer, "Retryer should not be nil")
	assert.Equal(t, reflect.ValueOf(retryer).Pointer(), reflect.ValueOf(fs.retryer).Pointer(), "Retryer should be set correctly")
}
