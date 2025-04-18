package ftp

import (
	"github.com/c2fo/vfs/v7/backend/ftp/types"
	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNameFTPClient = "ftpclient"
	optionNameOptions   = "options"
	optionNameDataConn  = "dataconn"
)

// WithClient returns clientOpt implementation of NewFileOption
//
// WithClient is used to explicitly specify a Client to use for the filesystem.
// The client is used to interact with the FTP service.
func WithClient(c types.Client) options.NewFileSystemOption[FileSystem] {
	return &clientOpt{
		client: c,
	}
}

type clientOpt struct {
	client types.Client
}

// Apply applies the client to the filesystem
func (ct *clientOpt) Apply(fs *FileSystem) {
	fs.ftpclient = ct.client
}

// NewFileSystemOptionName returns the name of the option
func (ct *clientOpt) NewFileSystemOptionName() string {
	return optionNameFTPClient
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

// Apply applies the options to the filesystem
func (o *optionsOpt) Apply(fs *FileSystem) {
	fs.options = o.options
}

// NewFileSystemOptionName returns the name of the option
func (o *optionsOpt) NewFileSystemOptionName() string {
	return optionNameOptions
}

// WithDataConn returns dataconnOpt implementation of NewFileOption
//
// WithDataConn is used to specify a DataConn to use for the filesystem.
// The dataconn is used to interact with the FTP service.
func WithDataConn(dc types.DataConn) options.NewFileSystemOption[FileSystem] {
	return &dataConnOpt{
		dataconn: dc,
	}
}

type dataConnOpt struct {
	dataconn types.DataConn
}

// Apply applies the dataconn to the filesystem
func (d *dataConnOpt) Apply(fs *FileSystem) {
	fs.dataconn = d.dataconn
}

// NewFileSystemOptionName returns the name of the option
func (d *dataConnOpt) NewFileSystemOptionName() string {
	return optionNameDataConn
}
