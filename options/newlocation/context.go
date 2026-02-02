// Package newlocation provides options for creating new locations in a virtual filesystem.
package newlocation

import (
	"context"

	"github.com/c2fo/vfs/v7/options"
)

const optionNameNewLocationContext = "newLocationContext"

// WithContext returns Context implementation of NewLocationOption
func WithContext(ctx context.Context) options.NewLocationOption {
	return &Context{ctx}
}

// Context represents the NewLocationOption that is used to specify a context for created locations.
type Context struct{ context.Context }

// NewLocationOptionName returns the name of Context option
func (ct *Context) NewLocationOptionName() string {
	return optionNameNewLocationContext
}
