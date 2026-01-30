package dropbox

import (
	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNameAccessToken         = "accessToken"
	optionNameChunkSize           = "chunkSize"
	optionNameMaxSimpleUploadSize = "maxSimpleUploadSize"
	optionNameTempDir             = "tempDir"
	optionNameClient              = "client"
)

// WithAccessToken sets the OAuth2 access token for Dropbox API authentication.
func WithAccessToken(token string) options.NewFileSystemOption[FileSystem] {
	return &accessTokenOpt{token: token}
}

type accessTokenOpt struct {
	token string
}

// Apply implements options.NewFileSystemOption.
func (o *accessTokenOpt) Apply(fs *FileSystem) {
	fs.options.AccessToken = o.token
}

// NewFileSystemOptionName implements options.NewFileSystemOption.
func (o *accessTokenOpt) NewFileSystemOptionName() string {
	return optionNameAccessToken
}

// WithChunkSize sets the chunk size for uploading large files.
// Files larger than MaxSimpleUploadSize are uploaded using chunked sessions with this chunk size.
// Default is 4MB.
func WithChunkSize(size int64) options.NewFileSystemOption[FileSystem] {
	return &chunkSizeOpt{size: size}
}

type chunkSizeOpt struct {
	size int64
}

// Apply implements options.NewFileSystemOption.
func (o *chunkSizeOpt) Apply(fs *FileSystem) {
	fs.options.ChunkSize = o.size
}

// NewFileSystemOptionName implements options.NewFileSystemOption.
func (o *chunkSizeOpt) NewFileSystemOptionName() string {
	return optionNameChunkSize
}

// WithMaxSimpleUploadSize sets the threshold above which chunked uploads are used.
// Default is 150MB (Dropbox API limit). This can be lowered for testing purposes.
func WithMaxSimpleUploadSize(size int64) options.NewFileSystemOption[FileSystem] {
	return &maxSimpleUploadSizeOpt{size: size}
}

type maxSimpleUploadSizeOpt struct {
	size int64
}

// Apply implements options.NewFileSystemOption.
func (o *maxSimpleUploadSizeOpt) Apply(fs *FileSystem) {
	fs.options.MaxSimpleUploadSize = o.size
}

// NewFileSystemOptionName implements options.NewFileSystemOption.
func (o *maxSimpleUploadSizeOpt) NewFileSystemOptionName() string {
	return optionNameMaxSimpleUploadSize
}

// WithTempDir sets the directory for temporary files used during read/write operations.
// Defaults to os.TempDir() if not specified.
func WithTempDir(dir string) options.NewFileSystemOption[FileSystem] {
	return &tempDirOpt{dir: dir}
}

type tempDirOpt struct {
	dir string
}

// Apply implements options.NewFileSystemOption.
func (o *tempDirOpt) Apply(fs *FileSystem) {
	fs.options.TempDir = o.dir
}

// NewFileSystemOptionName implements options.NewFileSystemOption.
func (o *tempDirOpt) NewFileSystemOptionName() string {
	return optionNameTempDir
}

// WithClient sets a custom Dropbox client. Useful for testing or when you need
// to provide a pre-configured client.
func WithClient(client Client) options.NewFileSystemOption[FileSystem] {
	return &clientOpt{client: client}
}

type clientOpt struct {
	client Client
}

// Apply implements options.NewFileSystemOption.
func (o *clientOpt) Apply(fs *FileSystem) {
	fs.client = o.client
}

// NewFileSystemOptionName implements options.NewFileSystemOption.
func (o *clientOpt) NewFileSystemOptionName() string {
	return optionNameClient
}
