package azure

import (
	"context"

	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNameClient  = "client"
	optionNameOptions = "options"
	optionNameContext = "context"
)

// WithClient returns clientSetter implementation of NewFileOption
//
// WithClient is used to explicitly specify a Client to use for the filesystem.
// The client is used to interact with the Azure service.
func WithClient(c Client) options.NewFileSystemOption[FileSystem] {
	return &clientOpt{
		client: c,
	}
}

type clientOpt struct {
	client Client
}

// Apply applies the client to the filesystem
func (ct *clientOpt) Apply(fs *FileSystem) {
	fs.client = ct.client
}

// NewFileSystemOptionName returns the name of the option
func (ct *clientOpt) NewFileSystemOptionName() string {
	return optionNameClient
}

// WithOptions returns optionsSetter implementation of NewFileOption
//
// WithOptions is used to specify options for the filesystem.
// The options are used to configure the filesystem.
func WithOptions(opts Options) options.NewFileSystemOption[FileSystem] {
	return &optionsOpt{
		options: opts,
	}
}

type optionsOpt struct {
	options Options
}

// Apply applies the options to the filesystem
func (o *optionsOpt) Apply(fs *FileSystem) {
	fs.options = &o.options
}

// NewFileSystemOptionName returns the name of the option
func (o *optionsOpt) NewFileSystemOptionName() string {
	return optionNameOptions
}

// WithContext returns a context option implementation of NewFileOption
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

// Apply applies the context to the filesystem
func (c *contextOpt) Apply(fs *FileSystem) {
	fs.ctx = c.ctx
}

// NewFileSystemOptionName returns the name of the option
func (c *contextOpt) NewFileSystemOptionName() string {
	return optionNameContext
}
