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
	optionNameRetryer = "retryer"
)

// WithClient returns clientOpt implementation of NewFileOption
//
// WithClient is used to explicitly specify a Client to use for the filesystem.
// The client is used to interact with the S3 service.
func WithClient(c *storage.Client) options.NewFileSystemOption[FileSystem] {
	return &clientOpt{
		client: c,
	}
}

type clientOpt struct {
	client *storage.Client
}

func (ct *clientOpt) Apply(fs *FileSystem) {
	fs.client = ct.client
}

func (ct *clientOpt) NewFileSystemOptionName() string {
	return optionNameClient
}

// WithOptions returns optionsOpt implementation of NewFileOption
//
// WithOptions is used to specify options for the filesystem.
// The options are used to configure the filesystem.
func WithOptions(options Options) options.NewFileSystemOption[FileSystem] {
	return &optionsOpt{
		options: options,
	}
}

type optionsOpt struct {
	options Options
}

func (o *optionsOpt) Apply(fs *FileSystem) {
	fs.options = o.options
}

func (o *optionsOpt) NewFileSystemOptionName() string {
	return optionNameOptions
}

// WithContext returns contextOpt implementation of NewFileOption
//
// WithContext is used to specify a context for the filesystem.
// The context is used to control the lifecycle of the filesystem.
func WithContext(ctx context.Context) options.NewFileSystemOption[FileSystem] {
	return &contextOpt{
		ctx: ctx,
	}
}

type contextOpt struct {
	ctx context.Context
}

func (c *contextOpt) Apply(fs *FileSystem) {
	fs.ctx = c.ctx
}

func (c *contextOpt) NewFileSystemOptionName() string {
	return optionNameContext
}

type Retryer func(wrapped func() error) error

func WithRetryer(retryer Retryer) options.NewFileSystemOption[FileSystem] {
	return &retryerOpt{
		retryer: retryer,
	}
}

type retryerOpt struct {
	retryer Retryer
}

func (r *retryerOpt) Apply(fs *FileSystem) {
	fs.retryer = r.retryer
}

func (r *retryerOpt) NewFileSystemOptionName() string {
	return optionNameRetryer
}
