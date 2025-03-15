package sftp

import "github.com/c2fo/vfs/v7/options"

const (
	optionNameClient  = "client"
	optionNameOptions = "options"
)

// WithClient returns clientOpt implementation of NewFileOption
//
// WithClient is used to explicitly specify a Client to use for the filesystem.
// The client is used to interact with the S3 service.
func WithClient(c Client) options.NewFileSystemOption {
	return &clientOpt{
		client: c,
	}
}

type clientOpt struct {
	client Client
}

func (ct *clientOpt) SetClient(fs *FileSystem) {
	fs.sftpclient = ct.client
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
