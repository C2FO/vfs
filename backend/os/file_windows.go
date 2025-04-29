//go:build windows

package os

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// getVolumeOrDevice returns an identifier for the volume containing the given path.
// On Windows, it's the volume name (e.g., "C:").
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

	vol := filepath.VolumeName(absPath)
	// Normalize C:\\ -> C:
	if len(vol) > 0 && vol[len(vol)-1] == '\\' {
		vol = vol[:len(vol)-1]
	}
	// If vol is empty (e.g., UNC path \\\\server\\share), return the cleaned UNC root
	if vol == "" && len(absPath) > 1 && absPath[0] == '\\' && absPath[1] == '\\' {
		parts := strings.SplitN(strings.TrimPrefix(absPath, `\\`), `\`, 3)
		if len(parts) >= 2 {
			return `\\` + parts[0] + `\` + parts[1], nil // \\\\server\\share
		}
	}
	// Handle cases like relative paths on current drive if vol is still empty
	if vol == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory to determine volume: %w", err)
		}
		vol = filepath.VolumeName(cwd)
		if len(vol) > 0 && vol[len(vol)-1] == '\\' {
			vol = vol[:len(vol)-1]
		}
	}
	return vol, nil
}
