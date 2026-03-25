//go:build windows

package os

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// getVolumeOrDevice returns an identifier for the volume containing the given path.
// On Windows, this is the volume name (e.g., "C:"). It walks up the directory tree
// until it finds an existing ancestor to resolve the volume.
func getVolumeOrDevice(filePath string) (string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %w", filePath, err)
	}

	vol := filepath.VolumeName(absPath)
	vol = strings.TrimRight(vol, `\`)

	if vol == "" && len(absPath) > 1 && absPath[0] == '\\' && absPath[1] == '\\' {
		parts := strings.SplitN(strings.TrimPrefix(absPath, `\\`), `\`, 3)
		if len(parts) >= 2 {
			return `\\` + parts[0] + `\` + parts[1], nil
		}
	}

	if vol == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory to determine volume: %w", err)
		}
		vol = strings.TrimRight(filepath.VolumeName(cwd), `\`)
	}

	return vol, nil
}
