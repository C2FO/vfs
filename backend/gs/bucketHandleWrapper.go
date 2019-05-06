package gs

import (
	"cloud.google.com/go/storage"
	"context"
	"github.com/c2fo/vfs/v3"
)

type BucketHandle interface {
	Attrs(ctx context.Context) (*storage.BucketAttrs, error)
}

type BucketHandleWrapper interface {
	BucketHandle
	WrappedObjects(ctx context.Context, q *storage.Query) ObjectIteratorWrapper
}

type RetryBucketHandler struct {
	Retry   vfs.Retry
	handler *storage.BucketHandle
}

func (r *RetryBucketHandler) Attrs(ctx context.Context) (*storage.BucketAttrs, error) {
	return bucketAttributeRetry(r.Retry, func() (*storage.BucketAttrs, error) {
		return r.handler.Attrs(ctx)
	})
}

func (r *RetryBucketHandler) WrappedObjects(ctx context.Context, q *storage.Query) ObjectIteratorWrapper {
	return &RetryObjectIterator{Retry: r.Retry, iterator: r.handler.Objects(ctx, q)}
}

type ObjectIteratorWrapper interface {
	Next() (*storage.ObjectAttrs, error)
}

type RetryObjectIterator struct {
	Retry    vfs.Retry
	iterator *storage.ObjectIterator
}

func (r *RetryObjectIterator) Next() (*storage.ObjectAttrs, error) {
	return objectAttributeRetry(r.Retry, func() (*storage.ObjectAttrs, error) {
		return r.iterator.Next()
	})
}

func bucketAttributeRetry(retry vfs.Retry, attrFunc func() (*storage.BucketAttrs, error)) (*storage.BucketAttrs, error) {
	var attrs *storage.BucketAttrs
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
	return attrs, nil
}
