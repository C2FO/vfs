package backend

import (
	"fmt"
	"io"

	"github.com/c2fo/vfs/v6"
)

func ValidateCopySeekPosition(f vfs.File) error {
	// validate seek is at 0,0 before doing copy
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to determine current cursor offset: %w", err)
	}
	if offset != 0 {
		return vfs.CopyToNotPossible
	}

	return nil
}
