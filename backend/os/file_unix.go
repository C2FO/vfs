//go:build !windows

package os

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// getVolumeOrDevice returns an identifier for the device containing the given path.
// On Unix, it's the device ID from stat.
func getVolumeOrDevice(filePath string) (string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		// If path doesn't exist, try its directory
		if os.IsNotExist(err) {
			absDir, dirErr := filepath.Abs(filepath.Dir(filePath))
			if dirErr != nil {
				return "", fmt.Errorf("failed to get absolute path for %s or its directory: %w", filePath, dirErr)
			}
			absPath = absDir
		} else {
			return "", fmt.Errorf("failed to get absolute path for %s: %w", filePath, err)
		}
	}

	// Unix-like systems
	info, err := os.Stat(absPath)
	if err != nil {
		// If the path itself doesn't exist (like a target file), stat its directory
		if os.IsNotExist(err) {
			parentInfo, parentErr := os.Stat(filepath.Dir(absPath))
			if parentErr != nil {
				// If dir also doesn't exist, we can't determine device ID
				return "", fmt.Errorf("failed to stat path %s and its parent %s: %w", absPath, filepath.Dir(absPath), parentErr)
			}
			info = parentInfo
		} else {
			return "", fmt.Errorf("failed to stat path %s: %w", absPath, err)
		}
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("failed to get syscall.Stat_t for %s", absPath)
	}
	// Use device ID as the identifier
	return fmt.Sprintf("dev-%d", stat.Dev), nil
}
