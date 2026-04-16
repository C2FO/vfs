package ftp

import (
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v8/backend/ftp/types"
)

const (
	optionNameFTPClient      = "ftpclient"
	optionNameOptions        = "options"
	optionNameDataConn       = "dataconn"
	optionNameClientGetter   = "clientGetter"
	optionNameDataConnGetter = "dataConnGetter"
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

// WithClientGetter returns a NewFileSystemOption that supplies how the FTP control client is obtained.
// When unset, NewFileSystem uses GetClient.
func WithClientGetter(g ClientGetter) options.NewFileSystemOption[FileSystem] {
	return &clientGetterOpt{getter: g}
}

type clientGetterOpt struct {
	getter ClientGetter
}

// Apply sets the client getter on the filesystem.
func (o *clientGetterOpt) Apply(fs *FileSystem) {
	fs.clientGetter = o.getter
}

// NewFileSystemOptionName returns the name of the option
func (o *clientGetterOpt) NewFileSystemOptionName() string {
	return optionNameClientGetter
}

// WithDataConnGetter returns a NewFileSystemOption that supplies how read/write/single-op data connections are obtained.
// When unset, the default implementation is used.
func WithDataConnGetter(g DataConnGetter) options.NewFileSystemOption[FileSystem] {
	return &dataConnGetterOpt{getter: g}
}

type dataConnGetterOpt struct {
	getter DataConnGetter
}

// Apply sets the data connection getter on the filesystem.
func (o *dataConnGetterOpt) Apply(fs *FileSystem) {
	fs.dataConnGetter = o.getter
}

// NewFileSystemOptionName returns the name of the option
func (o *dataConnGetterOpt) NewFileSystemOptionName() string {
	return optionNameDataConnGetter
}
