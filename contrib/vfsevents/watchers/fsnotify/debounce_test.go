package fsnotify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7/vfssimple"
)

func TestDebouncing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fsnotify_debounce_*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	location, err := vfssimple.NewLocation(fileURL(tempDir))
	require.NoError(t, err)

	t.Run("NoDebouncing", func(t *testing.T) {
		// Test without debouncing - should get multiple events
		watcher, err := NewFSNotifyWatcher(location)
		require.NoError(t, err)

		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			events <- event
		}

		errorHandler := func(err error) {
			errors <- err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = watcher.Start(ctx, eventHandler, errorHandler)
		require.NoError(t, err)
		defer func() { _ = watcher.Stop() }()

		// Create a file with multiple writes
		testFile := filepath.Join(tempDir, "no_debounce.txt")
		f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
		require.NoError(t, err)

		_, err = f.WriteString("part1")
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		_, err = f.WriteString("part2")
		require.NoError(t, err)
		_ = f.Close()

		// Should receive multiple events
		eventCount := 0
		timeout := time.After(500 * time.Millisecond)

	eventLoop:
		for {
			select {
			case <-events:
				eventCount++
			case <-timeout:
				break eventLoop
			case <-time.After(100 * time.Millisecond):
				break eventLoop
			}
		}

		assert.Greater(t, eventCount, 1, "Should receive multiple events without debouncing")
	})

	t.Run("WithDebouncing", func(t *testing.T) {
		// Test with debouncing - should get single consolidated event
		watcher, err := NewFSNotifyWatcher(location, WithDebounce(200*time.Millisecond))
		require.NoError(t, err)

		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			events <- event
		}

		errorHandler := func(err error) {
			errors <- err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = watcher.Start(ctx, eventHandler, errorHandler)
		require.NoError(t, err)
		defer func() { _ = watcher.Stop() }()

		// Create a file with multiple writes
		testFile := filepath.Join(tempDir, "with_debounce.txt")
		f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
		require.NoError(t, err)

		_, err = f.WriteString("part1")
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond) // Less than debounce duration

		_, err = f.WriteString("part2")
		require.NoError(t, err)
		_ = f.Close()

		// Wait for debounce period plus buffer
		time.Sleep(400 * time.Millisecond)

		// Should receive exactly one event
		eventCount := 0
		var finalEvent vfsevents.Event
		timeout := time.After(100 * time.Millisecond)

	eventLoop:
		for {
			select {
			case event := <-events:
				eventCount++
				finalEvent = event
			case <-timeout:
				break eventLoop
			}
		}

		assert.Equal(t, 1, eventCount, "Should receive exactly one debounced event")
		assert.Equal(t, vfsevents.EventCreated, finalEvent.Type, "Final event should be Created")
		assert.Equal(t, "multiple", finalEvent.Metadata["fsnotify_op"], "Should indicate multiple operations")
	})

	t.Run("MemoryLeakPrevention", func(t *testing.T) {
		// Test that pending events are cleaned up properly
		watcher, err := NewFSNotifyWatcher(location, WithDebounce(1*time.Second))
		require.NoError(t, err)

		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			events <- event
		}

		errorHandler := func(err error) {
			errors <- err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = watcher.Start(ctx, eventHandler, errorHandler)
		require.NoError(t, err)

		// Create multiple files to populate the pending events map
		for i := 0; i < 5; i++ {
			testFile := filepath.Join(tempDir, fmt.Sprintf("leak_test_%d.txt", i))
			err = os.WriteFile(testFile, []byte("content"), 0600)
			require.NoError(t, err)
		}

		// Check that pending events exist
		watcher.mu.Lock()
		pendingCount := len(watcher.pendingEvents)
		watcher.mu.Unlock()

		assert.Greater(t, pendingCount, 0, "Should have pending events")

		// Stop the watcher - this should clean up pending events
		err = watcher.Stop()
		require.NoError(t, err)

		// Check that pending events are cleaned up
		watcher.mu.Lock()
		finalPendingCount := len(watcher.pendingEvents)
		watcher.mu.Unlock()

		assert.Equal(t, 0, finalPendingCount, "Pending events should be cleaned up on stop")
	})

	t.Run("LongDebounceTimeout", func(t *testing.T) {
		// Test with longer debounce timeout to simulate SFTP scenario
		watcher, err := NewFSNotifyWatcher(location, WithDebounce(2*time.Second))
		require.NoError(t, err)

		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			events <- event
		}

		errorHandler := func(err error) {
			errors <- err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err = watcher.Start(ctx, eventHandler, errorHandler)
		require.NoError(t, err)
		defer func() { _ = watcher.Stop() }()

		// Simulate SFTP-like behavior: create file, wait, append more
		testFile := filepath.Join(tempDir, "sftp_simulation.txt")

		// Initial write
		err = os.WriteFile(testFile, []byte("initial"), 0600)
		require.NoError(t, err)

		// Wait 1 second (less than debounce), then append
		time.Sleep(1 * time.Second)
		f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
		require.NoError(t, err)
		_, err = f.WriteString(" appended")
		require.NoError(t, err)
		_ = f.Close()

		// Wait for debounce period plus buffer
		time.Sleep(3 * time.Second)

		// Should receive exactly one consolidated event
		eventCount := 0
		timeout := time.After(100 * time.Millisecond)

	eventLoop:
		for {
			select {
			case <-events:
				eventCount++
			case <-timeout:
				break eventLoop
			}
		}

		assert.Equal(t, 1, eventCount, "Should receive exactly one event after long debounce")
	})
}

func TestDebouncingEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fsnotify_debounce_edge_*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	location, err := vfssimple.NewLocation(fileURL(tempDir))
	require.NoError(t, err)

	t.Run("CreateThenDelete", func(t *testing.T) {
		// Test create followed by delete - should result in delete event
		watcher, err := NewFSNotifyWatcher(location, WithDebounce(200*time.Millisecond))
		require.NoError(t, err)

		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			events <- event
		}

		errorHandler := func(err error) {
			errors <- err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = watcher.Start(ctx, eventHandler, errorHandler)
		require.NoError(t, err)
		defer func() { _ = watcher.Stop() }()

		// Create then quickly delete
		testFile := filepath.Join(tempDir, "create_delete.txt")
		err = os.WriteFile(testFile, []byte("content"), 0600)
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond) // Less than debounce
		err = os.Remove(testFile)
		require.NoError(t, err)

		// Wait for debounce period
		time.Sleep(400 * time.Millisecond)

		// Should receive delete event (delete wins over create)
		eventCount := 0
		var finalEvent vfsevents.Event
		timeout := time.After(100 * time.Millisecond)

	eventLoop:
		for {
			select {
			case event := <-events:
				eventCount++
				finalEvent = event
			case <-timeout:
				break eventLoop
			}
		}

		assert.Equal(t, 1, eventCount, "Should receive exactly one event")
		if eventCount > 0 {
			assert.Equal(t, vfsevents.EventDeleted, finalEvent.Type, "Delete should win over create")
		}
	})
}
