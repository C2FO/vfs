package gs

import (
	"context"

	"cloud.google.com/go/storage"

	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNameClient  = "client"
	optionNameOptions = "options"
	optionNameContext = "context"
)

// WithClient returns clientOpt implementation of NewFileOption
//
// WithClient is used to explicitly specify a Client to use for the filesystem.
// The client is used to interact with the S3 service.
func WithClient(c *storage.Client) options.NewFileSystemOption {
	return &clientOpt{
		client: c,
	}
}

type clientOpt struct {
	client *storage.Client
}

func (ct *clientOpt) SetClient(fs *FileSystem) {
	fs.client = ct.client
}

func (ct *clientOpt) NewFileSystemOptionName() string {
	return optionNameClient
}

// WithOptions returns optionsOpt implementation of NewFileOption
//
// WithOptions is used to specify options for the filesystem.
// The options are used to configure the filesystem.
func WithOptions(options Options) options.NewFileSystemOption {
	return &optionsOpt{
		options: options,
	}
}

type optionsOpt struct {
	options Options
}

func (o *optionsOpt) SetOptions(fs *FileSystem) {
	fs.options = o.options
}

func (o *optionsOpt) NewFileSystemOptionName() string {
	return optionNameOptions
}

// WithContext returns contextOpt implementation of NewFileOption
//
// WithContext is used to specify a context for the filesystem.
// The context is used to control the lifecycle of the filesystem.
func WithContext(ctx context.Context) options.NewFileSystemOption {
	return &contextOpt{
		ctx: ctx,
	}
}

type contextOpt struct {
	ctx context.Context
}

func (c *contextOpt) SetContext(fs *FileSystem) {
	fs.ctx = c.ctx
}

func (c *contextOpt) NewFileSystemOptionName() string {
	return optionNameContext
}
