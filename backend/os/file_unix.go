//go:build !windows

package os

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// getVolumeOrDevice returns an identifier for the device containing the given path.
// On Unix, this is the device ID from stat. It walks up the directory tree until
// it finds an existing ancestor.
func getVolumeOrDevice(filePath string) (string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %w", filePath, err)
	}

	// Walk up until we find a path that exists
	p := absPath
	for {
		info, err := os.Stat(p)
		if err == nil {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return "", fmt.Errorf("failed to get syscall.Stat_t for %s", p)
			}
			return fmt.Sprintf("dev-%d", stat.Dev), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to stat %s: %w", p, err)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", fmt.Errorf("no existing ancestor found for %s", filePath)
		}
		p = parent
	}
}
