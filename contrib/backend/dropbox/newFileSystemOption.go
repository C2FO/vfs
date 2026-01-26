package dropbox

import (
	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNameAccessToken = "accessToken"
	optionNameChunkSize   = "chunkSize"
	optionNameTempDir     = "tempDir"
	optionNameRetryCount  = "retryCount"
	optionNameClient      = "client"
)

// WithAccessToken sets the OAuth2 access token for Dropbox API authentication.
func WithAccessToken(token string) options.NewFileSystemOption[FileSystem] {
	return &accessTokenOpt{token: token}
}

type accessTokenOpt struct {
	token string
}

func (o *accessTokenOpt) Apply(fs *FileSystem) {
	fs.options.AccessToken = o.token
}

func (o *accessTokenOpt) NewFileSystemOptionName() string {
	return optionNameAccessToken
}

// WithChunkSize sets the chunk size for uploading large files.
// Files larger than 150MB are uploaded using chunked sessions with this chunk size.
// Default is 4MB.
func WithChunkSize(size int64) options.NewFileSystemOption[FileSystem] {
	return &chunkSizeOpt{size: size}
}

type chunkSizeOpt struct {
	size int64
}

func (o *chunkSizeOpt) Apply(fs *FileSystem) {
	fs.options.ChunkSize = o.size
}

func (o *chunkSizeOpt) NewFileSystemOptionName() string {
	return optionNameChunkSize
}

// WithTempDir sets the directory for temporary files used during read/write operations.
// Defaults to os.TempDir() if not specified.
func WithTempDir(dir string) options.NewFileSystemOption[FileSystem] {
	return &tempDirOpt{dir: dir}
}

type tempDirOpt struct {
	dir string
}

func (o *tempDirOpt) Apply(fs *FileSystem) {
	fs.options.TempDir = o.dir
}

func (o *tempDirOpt) NewFileSystemOptionName() string {
	return optionNameTempDir
}

// WithRetryCount sets the number of retry attempts for transient errors.
// Default is 3.
func WithRetryCount(count int) options.NewFileSystemOption[FileSystem] {
	return &retryCountOpt{count: count}
}

type retryCountOpt struct {
	count int
}

func (o *retryCountOpt) Apply(fs *FileSystem) {
	fs.options.RetryCount = o.count
}

func (o *retryCountOpt) NewFileSystemOptionName() string {
	return optionNameRetryCount
}

// WithClient sets a custom Dropbox client. Useful for testing or when you need
// to provide a pre-configured client.
func WithClient(client Client) options.NewFileSystemOption[FileSystem] {
	return &clientOpt{client: client}
}

type clientOpt struct {
	client Client
}

func (o *clientOpt) Apply(fs *FileSystem) {
	fs.client = o.client
}

func (o *clientOpt) NewFileSystemOptionName() string {
	return optionNameClient
}
