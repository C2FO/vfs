package v7adapter

import (
	"context"
	"errors"
	"fmt"

	vfsv7 "github.com/c2fo/vfs/v7"
	vfsv8 "github.com/c2fo/vfs/v8"
)

// FileTransfers wraps a v7 [vfsv7.File] to implement [vfsv8.FileTransfers].
type FileTransfers struct {
	inner vfsv7.File
}

// FileTransfersFromV7 wraps f. It returns nil if f is nil.
func FileTransfersFromV7(f vfsv7.File) vfsv8.FileTransfers {
	if f == nil {
		return nil
	}
	return &FileTransfers{inner: f}
}

// CopyToLocation delegates to the v7 file after unwrapping dst to a v7 location.
func (t *FileTransfers) CopyToLocation(ctx context.Context, dst vfsv8.Location) (vfsv8.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dstV7, err := requireV7Location(dst)
	if err != nil {
		return nil, err
	}
	out, err := t.inner.CopyToLocation(dstV7)
	if err != nil {
		return nil, err
	}
	return FileFromV7(out), nil
}

// CopyToFile delegates to the v7 file after unwrapping dst to a v7 file.
func (t *FileTransfers) CopyToFile(ctx context.Context, dst vfsv8.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dstV7, err := requireV7File(dst)
	if err != nil {
		return err
	}
	return t.inner.CopyToFile(dstV7)
}

// MoveToLocation delegates to the v7 file after unwrapping dst to a v7 location.
func (t *FileTransfers) MoveToLocation(ctx context.Context, dst vfsv8.Location) (vfsv8.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dstV7, err := requireV7Location(dst)
	if err != nil {
		return nil, err
	}
	out, err := t.inner.MoveToLocation(dstV7)
	if err != nil {
		return nil, err
	}
	return FileFromV7(out), nil
}

// MoveToFile delegates to the v7 file after unwrapping dst to a v7 file.
func (t *FileTransfers) MoveToFile(ctx context.Context, dst vfsv8.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dstV7, err := requireV7File(dst)
	if err != nil {
		return err
	}
	return t.inner.MoveToFile(dstV7)
}

func requireV7Location(dst vfsv8.Location) (vfsv7.Location, error) {
	if dst == nil {
		return nil, errors.New("v7adapter: nil destination location")
	}
	loc, ok := AsV7Location(dst)
	if !ok || loc == nil {
		return nil, fmt.Errorf("v7adapter: destination location must be a %T from v7adapter", (*Location)(nil))
	}
	return loc, nil
}

func requireV7File(dst vfsv8.File) (vfsv7.File, error) {
	if dst == nil {
		return nil, errors.New("v7adapter: nil destination file")
	}
	f, ok := AsV7File(dst)
	if !ok || f == nil {
		return nil, fmt.Errorf("v7adapter: destination file must be a %T from v7adapter", (*File)(nil))
	}
	return f, nil
}

var _ vfsv8.FileTransfers = (*FileTransfers)(nil)
