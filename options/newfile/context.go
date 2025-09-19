// Package newfile provides options for creating new files in a virtual filesystem.
package newfile

import (
	"context"

	"github.com/c2fo/vfs/v7/options"
)

const optionNameNewFileContext = "newFileContext"

// WithContext returns Context implementation of NewFileOption
func WithContext(ctx context.Context) options.NewFileOption {
	return &Context{ctx}
}

// Context represents the NewFileOption that is used to specify a context for created files.
type Context struct{ context.Context }

// NewFileOptionName returns the name of Context option
func (ct *Context) NewFileOptionName() string {
	return optionNameNewFileContext
}
