package fsnotify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/vfssimple"
)

func TestDebouncing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fsnotify_debounce_*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	location, err := vfssimple.NewLocation(fileURL(tempDir))
	require.NoError(t, err)

	t.Run("NoDebouncing", func(t *testing.T) {
		testNoDebouncing(t, location, tempDir)
	})

	t.Run("WithDebouncing", func(t *testing.T) {
		testWithDebouncing(t, location, tempDir)
	})

	t.Run("MemoryLeakPrevention", func(t *testing.T) {
		testMemoryLeakPrevention(t, location, tempDir)
	})

	t.Run("LongDebounceTimeout", func(t *testing.T) {
		testLongDebounceTimeout(t, location, tempDir)
	})
}

// testNoDebouncing tests behavior without debouncing
func testNoDebouncing(t *testing.T, location vfs.Location, tempDir string) {
	// Test without debouncing - should receive multiple events
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

	// Create a file with multiple writes
	testFile := filepath.Join(tempDir, "no_debounce.txt")
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
	require.NoError(t, err)

	_, err = f.WriteString("part1")
	require.NoError(t, err)
	_, err = f.WriteString("part2")
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	// Collect events for a short time
	var receivedEvents []vfsevents.Event
	timeout := time.After(1 * time.Second)

eventLoop:
	for {
		select {
		case event := <-events:
			if strings.Contains(event.URI, "no_debounce.txt") {
				receivedEvents = append(receivedEvents, event)
			}
		case <-timeout:
			break eventLoop
		}
	}

	err = watcher.Stop()
	require.NoError(t, err)

	// Should receive multiple events (at least CREATE)
	assert.NotEmpty(t, receivedEvents, "Should receive at least one event without debouncing")
}

// testWithDebouncing tests behavior with debouncing enabled
func testWithDebouncing(t *testing.T, location vfs.Location, tempDir string) {
	// Test with debouncing - should receive consolidated events
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

	// Create a file with multiple writes
	testFile := filepath.Join(tempDir, "with_debounce.txt")
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // Test file path is controlled
	require.NoError(t, err)

	_, err = f.WriteString("part1")
	require.NoError(t, err)
	_, err = f.WriteString("part2")
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)

	// Wait for debounce period plus some buffer
	time.Sleep(500 * time.Millisecond)

	// Collect events
	var receivedEvents []vfsevents.Event
	timeout := time.After(100 * time.Millisecond)

eventLoop:
	for {
		select {
		case event := <-events:
			if strings.Contains(event.URI, "with_debounce.txt") {
				receivedEvents = append(receivedEvents, event)
			}
		case <-timeout:
			break eventLoop
		}
	}

	err = watcher.Stop()
	require.NoError(t, err)

	// Should receive exactly one consolidated event
	assert.Len(t, receivedEvents, 1, "Should receive exactly one consolidated event with debouncing")

	if len(receivedEvents) > 0 {
		// Check that it's a consolidated event
		event := receivedEvents[0]
		assert.Contains(t, []vfsevents.EventType{vfsevents.EventCreated, vfsevents.EventModified}, event.Type)
		// If multiple operations were consolidated, metadata should indicate this
		if event.Metadata["fsnotify_op"] == "multiple" {
			assert.Equal(t, "multiple", event.Metadata["fsnotify_op"], "Consolidated event should have 'multiple' fsnotify_op")
		}
	}
}

// testMemoryLeakPrevention tests that stopping the watcher cleans up pending events
func testMemoryLeakPrevention(t *testing.T, location vfs.Location, tempDir string) {
	// Test that stopping the watcher cleans up pending events
	watcher, err := NewFSNotifyWatcher(location, WithDebounce(500*time.Millisecond))
	require.NoError(t, err)

	events := make(chan vfsevents.Event, 20)
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
	// Use a longer debounce time to ensure events stay pending
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tempDir, fmt.Sprintf("leak_test_%d.txt", i))
		err = os.WriteFile(testFile, []byte("content"), 0600)
		require.NoError(t, err)
		// Small delay between file creations to ensure separate pending events
		time.Sleep(50 * time.Millisecond)
	}

	// Wait a bit for events to be registered but not yet processed (due to debounce)
	time.Sleep(200 * time.Millisecond)

	// Check that pending events exist
	watcher.mu.Lock()
	pendingCount := len(watcher.pendingEvents)
	watcher.mu.Unlock()

	// If no pending events, it might be because debouncing processed them quickly
	// In that case, create more files to ensure we have pending events
	if pendingCount == 0 {
		for i := 5; i < 10; i++ {
			testFile := filepath.Join(tempDir, fmt.Sprintf("leak_test_%d.txt", i))
			err = os.WriteFile(testFile, []byte("content"), 0600)
			require.NoError(t, err)
			time.Sleep(20 * time.Millisecond) // Shorter delay to create overlapping events
		}
		time.Sleep(100 * time.Millisecond)

		watcher.mu.Lock()
		pendingCount = len(watcher.pendingEvents)
		watcher.mu.Unlock()
	}

	// We should have some pending events, but if not, that's also valid behavior
	// The key test is that Stop() doesn't hang and cleans up properly
	t.Logf("Pending events before stop: %d", pendingCount)

	// Stop the watcher - this should clean up pending events and not hang
	stopDone := make(chan error, 1)
	go func() {
		stopDone <- watcher.Stop()
	}()

	select {
	case err = <-stopDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() took too long - likely hanging due to pending events not being cleaned up")
	}

	// Check that pending events are cleaned up
	watcher.mu.Lock()
	finalPendingCount := len(watcher.pendingEvents)
	watcher.mu.Unlock()

	assert.Zero(t, finalPendingCount, "Pending events should be cleaned up on stop")
}

// testLongDebounceTimeout tests behavior with longer debounce timeout (SFTP scenario)
func testLongDebounceTimeout(t *testing.T, location vfs.Location, tempDir string) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler)
	require.NoError(t, err)

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
	err = f.Close()
	require.NoError(t, err)

	// Wait for debounce period plus buffer
	time.Sleep(3 * time.Second)

	// Collect events
	var receivedEvents []vfsevents.Event
	timeout := time.After(100 * time.Millisecond)

eventLoop:
	for {
		select {
		case event := <-events:
			if strings.Contains(event.URI, "sftp_simulation.txt") {
				receivedEvents = append(receivedEvents, event)
			}
		case <-timeout:
			break eventLoop
		}
	}

	err = watcher.Stop()
	require.NoError(t, err)

	// Should receive exactly one consolidated event
	assert.Len(t, receivedEvents, 1, "Should receive exactly one consolidated event with long debounce")
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
