package newfile

import "github.com/c2fo/vfs/v6/options"

const optionNameNewFileContentType = "newFileContentType"

// WithContentType returns ContentType implementation of NewFileOption
func WithContentType(contentType string) options.NewFileOption {
	ct := ContentType(contentType)
	return &ct
}

// ContentType represents the NewFileOption that is used to explicitly specify a content type on created files.
type ContentType string

// NewFileOptionName returns the name of ContentType option
func (ct *ContentType) NewFileOptionName() string {
	return optionNameNewFileContentType
}
