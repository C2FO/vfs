package sqlfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
)

// Sentinel errors returned (wrapped) by this package. Use errors.Is to test for them.
var (
	// ErrNotFound reports that a file does not exist. It wraps fs.ErrNotExist.
	ErrNotFound = fmt.Errorf("sqlfs: file not found: %w", fs.ErrNotExist)

	// ErrAlreadyExists reports that a file was created concurrently by another writer. It is always returned
	// together with ErrConflict.
	ErrAlreadyExists = errors.New("sqlfs: file already exists")

	// ErrConflict reports that another writer created, replaced, moved, or deleted the file since this handle
	// first observed it, or that content this handle depends on was reclaimed. Nothing was published.
	ErrConflict = errors.New("sqlfs: concurrent modification conflict")

	// ErrInvalidConfig reports an invalid filesystem option.
	ErrInvalidConfig = errors.New("sqlfs: invalid configuration")

	// ErrInvalidSchema reports a missing, outdated, or newer-than-supported database schema.
	ErrInvalidSchema = errors.New("sqlfs: invalid or incompatible schema")

	// ErrUnavailable reports that the database could not be reached.
	ErrUnavailable = errors.New("sqlfs: database unavailable")

	// ErrTimeout reports that a database operation exceeded the configured operation timeout. It wraps
	// context.DeadlineExceeded.
	ErrTimeout = fmt.Errorf("sqlfs: operation timed out: %w", context.DeadlineExceeded)

	// ErrCorrupt reports stored content that is inconsistent with its metadata.
	ErrCorrupt = errors.New("sqlfs: corrupt stored content")

	// errRetry is used internally to request that a whole transaction be run again.
	errRetry = errors.New("sqlfs: retry transaction")
)

// wrapErr adds operation context to err. Database errors are classified into the exported sentinels; the original
// error stays in the chain. Messages never include file content or connection strings.
func (fsys *FileSystem) wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict), errors.Is(err, ErrInvalidSchema),
		errors.Is(err, ErrInvalidConfig), errors.Is(err, ErrUnavailable), errors.Is(err, ErrTimeout),
		errors.Is(err, ErrCorrupt):
		return fmt.Errorf("sqlfs: %s: %w", op, err)
	case errors.Is(err, errRetry):
		return fmt.Errorf("sqlfs: %s: %w", op, ErrConflict)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("sqlfs: %s: %w: %w", op, ErrTimeout, err)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("sqlfs: %s: %w", op, err)
	case fsys.dialect.Classify(err) == KindUnavailable:
		return fmt.Errorf("sqlfs: %s: %w: %w", op, ErrUnavailable, err)
	default:
		return fmt.Errorf("sqlfs: %s: %w", op, err)
	}
}
