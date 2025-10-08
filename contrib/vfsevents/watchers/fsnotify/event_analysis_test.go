package fsnotify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7/vfssimple"
)

// TestEventAnalysis analyzes different file writing patterns to understand
// the underlying filesystem event sequences
func TestEventAnalysis(t *testing.T) {
	tempDir := t.TempDir()

	location, err := vfssimple.NewLocation(fileURL(tempDir))
	require.NoError(t, err)

	watcher, err := NewFSNotifyWatcher(location)
	require.NoError(t, err)

	events := make(chan vfsevents.Event, 100)
	errors := make(chan error, 10)

	eventHandler := func(event vfsevents.Event) {
		events <- event
	}

	errorHandler := func(err error) {
		errors <- err
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler)
	require.NoError(t, err)
	defer func() { _ = watcher.Stop() }()

	// Test different file writing patterns
	testPatterns := []struct {
		name string
		fn   func(string) error
	}{
		{"os_WriteFile_atomic", testAtomicWrite},
		{"Open_Write_Close_single", testSingleWrite},
		{"Open_Multiple_Writes_Close", testMultipleWrites},
		{"Create_Append_pattern", testAppendPattern},
		{"Truncate_and_rewrite", testTruncateRewrite},
	}

	for i, pattern := range testPatterns {
		testFile := filepath.Join(tempDir, fmt.Sprintf("test_%d_%s.txt", i, pattern.name))

		fmt.Printf("\n=== TESTING: %s ===\n", pattern.name)

		err := pattern.fn(testFile)
		require.NoError(t, err)

		// Collect events for this file
		eventCount := 0
		fmt.Printf("Collecting events for 500ms...\n")
		timeout := time.After(500 * time.Millisecond)

	eventLoop:
		for {
			select {
			case event := <-events:
				if strings.Contains(event.URI, testFile) {
					eventCount++
					fmt.Printf("EVENT: %s | %s | fsnotify_op=%s\n",
						event.Type.String(),
						filepath.Base(event.URI),
						event.Metadata["fsnotify_op"])
				}
			case <-timeout:
				break eventLoop
			}
		}

		fmt.Printf("Total events for %s: %d\n", pattern.name, eventCount)

		// Clean up
		_ = os.Remove(testFile)
	}
}

func testAtomicWrite(testFile string) error {
	return os.WriteFile(testFile, []byte("content"), 0600)
}

func testSingleWrite(testFile string) error {
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = f.WriteString("content")
	return err
}

func testMultipleWrites(testFile string) error {
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err = f.WriteString("part1"); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)

	_, err = f.WriteString("part2")
	return err
}

func testAppendPattern(testFile string) error {
	if err := os.WriteFile(testFile, []byte("initial"), 0600); err != nil {
		return err
	}

	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = f.WriteString(" appended")
	return err
}

func testTruncateRewrite(testFile string) error {
	if err := os.WriteFile(testFile, []byte("original"), 0600); err != nil {
		return err
	}

	f, err := os.OpenFile(testFile, os.O_TRUNC|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = f.WriteString("new")
	return err
}
