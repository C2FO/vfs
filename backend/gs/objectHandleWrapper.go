package gs

import (
	"cloud.google.com/go/storage"
	"context"
	"github.com/c2fo/vfs/v3"
	"google.golang.org/api/iterator"
)

// ObjectHandleWrapper is an interface which contains a subset of the functions provided
// by storage.ObjectHandler. Any function normally called directly by storage.ObjectHandler
// should be added to this interface to allow for proper retry wrapping of the functions
// which call the GCS API.
type ObjectHandleWrapper interface {
	NewWriter(ctx context.Context) *storage.Writer
	NewReader(ctx context.Context) (*storage.Reader, error)
	Attrs(ctx context.Context) (*storage.ObjectAttrs, error)
	Delete(ctx context.Context) error
}

// ObjectHandleCopier is a unique, wrapped type which should mimic the behavior of ObjectHandler, but with
// modified return types. Each function that returns a sub type that also should be wrapped should be added
// to this interface with the 'Wrapped' prefix.
type ObjectHandleCopier interface {
	ObjectHandleWrapper
	WrappedCopierFrom(src *storage.ObjectHandle) CopierWrapper
	ObjectHandle() *storage.ObjectHandle
}

// CopierWrapper is an interface which contains a subset of the functions provided by storage.Copier.
type CopierWrapper interface {
	Run(ctx context.Context) (*storage.ObjectAttrs, error)
	ContentType(string)
}

type RetryObjectHandler struct {
	Retry   vfs.Retry
	handler *storage.ObjectHandle
}

func (r *RetryObjectHandler) ObjectHandle() *storage.ObjectHandle {
	return r.handler
}

func (r *RetryObjectHandler) NewWriter(ctx context.Context) *storage.Writer {
	return r.handler.NewWriter(ctx)
}

func (r *RetryObjectHandler) NewReader(ctx context.Context) (*storage.Reader, error) {
	var reader *storage.Reader
	if err := r.Retry(func() error {
		var retryErr error
		reader, retryErr = r.handler.NewReader(ctx)
		if retryErr != nil {
			return retryErr
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return reader, nil
}

func (r *RetryObjectHandler) Attrs(ctx context.Context) (*storage.ObjectAttrs, error) {
	return objectAttributeRetry(r.Retry, func() (*storage.ObjectAttrs, error) {
		return r.handler.Attrs(ctx)
	})
}

func (r *RetryObjectHandler) Delete(ctx context.Context) error {
	if err := r.Retry(func() error {
		if retryErr := r.handler.Delete(ctx); retryErr != nil {
			return retryErr
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (r *RetryObjectHandler) WrappedCopierFrom(src *storage.ObjectHandle) CopierWrapper {
	return &Copier{copier: r.handler.CopierFrom(src), Retry: r.Retry}
}

type Copier struct {
	copier *storage.Copier
	Retry  vfs.Retry
}

func (c *Copier) Run(ctx context.Context) (*storage.ObjectAttrs, error) {
	return objectAttributeRetry(c.Retry, func() (*storage.ObjectAttrs, error) {
		return c.copier.Run(ctx)
	})
}

func objectAttributeRetry(retry vfs.Retry, attrFunc func() (*storage.ObjectAttrs, error)) (*storage.ObjectAttrs, error) {
	var attrs *storage.ObjectAttrs
	attrs, err := attrFunc()
	if err != nil && err != iterator.Done {
		if err := retry(func() error {
			var retryErr error
			attrs, retryErr = attrFunc()
			if retryErr != nil {
				return retryErr
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return attrs, err
}
