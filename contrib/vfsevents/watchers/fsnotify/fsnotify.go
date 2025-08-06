// Package fsnotify provides an implementation of the vfsevents.Watcher interface using fsnotify for real-time filesystem monitoring.
// It offers cross-platform filesystem event watching with support for recursive directory monitoring and event filtering.
package fsnotify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7"
)

// FSNotifyWatcher watches local filesystem directories for changes using fsnotify.
// It provides real-time event notifications for file system operations.
type FSNotifyWatcher struct {
	location   vfs.Location
	watcher    *fsnotify.Watcher
	recursive  bool
	cancel     context.CancelFunc
	mu         sync.Mutex
	wg         sync.WaitGroup
	watchPaths map[string]bool // Track watched directories
}

// Option represents a functional option for configuring the FSNotifyWatcher.
type Option func(*FSNotifyWatcher)

// WithRecursive enables recursive watching of subdirectories.
func WithRecursive(recursive bool) Option {
	return func(w *FSNotifyWatcher) {
		w.recursive = recursive
	}
}

// NewFSNotifyWatcher creates a new filesystem watcher for the given VFS location.
// The location must be a local filesystem path (file:// scheme).
func NewFSNotifyWatcher(location vfs.Location, opts ...Option) (*FSNotifyWatcher, error) {
	if location == nil {
		return nil, fmt.Errorf("location cannot be nil")
	}

	// Verify this is a local filesystem location FIRST
	if !strings.HasPrefix(location.URI(), "file://") {
		return nil, fmt.Errorf("fsnotify watcher only supports local filesystem locations (file:// scheme), got: %s", location.URI())
	}

	exists, err := location.Exists()
	if err != nil {
		return nil, fmt.Errorf("failed to check if location exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("location does not exist: %s", location.URI())
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	w := &FSNotifyWatcher{
		location:   location,
		watcher:    watcher,
		recursive:  false,
		watchPaths: make(map[string]bool),
	}

	for _, opt := range opts {
		opt(w)
	}

	return w, nil
}

// Start begins watching the filesystem location for changes.
func (w *FSNotifyWatcher) Start(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	errHandler vfsevents.ErrorHandlerFunc,
	opts ...vfsevents.StartOption) error {

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return fmt.Errorf("fsnotify watcher is already running")
	}

	// Process start options
	config := &vfsevents.StartConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Get the local path from the VFS location
	localPath := strings.TrimPrefix(w.location.URI(), "file://")
	localPath = strings.TrimSuffix(localPath, "/") // Remove trailing slash if present
	if localPath == "" {
		return fmt.Errorf("invalid file location URI: %s", w.location.URI())
	}

	// Add the main directory to watch
	if err := w.addWatchPath(localPath); err != nil {
		return fmt.Errorf("failed to add watch path: %w", err)
	}

	// If recursive, add all subdirectories
	if w.recursive {
		if err := w.addRecursiveWatchPaths(localPath); err != nil {
			return fmt.Errorf("failed to add recursive watch paths: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	// Status tracking
	var status vfsevents.WatcherStatus
	status.Running = true
	status.StartTime = time.Now()

	// Send initial status if callback is configured
	if config.StatusCallback != nil {
		config.StatusCallback(status)
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.watchLoop(ctx, handler, errHandler, &status, config)
	}()

	return nil
}

// Stop stops the filesystem watcher.
func (w *FSNotifyWatcher) Stop(opts ...vfsevents.StopOption) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel == nil {
		return fmt.Errorf("fsnotify watcher is not running")
	}

	// Process stop options
	config := &vfsevents.StopConfig{
		Timeout: 30 * time.Second, // Default timeout
		Force:   false,
	}
	for _, opt := range opts {
		opt(config)
	}

	w.cancel()
	w.cancel = nil

	// Handle graceful shutdown with timeout
	if config.Force {
		// Force immediate shutdown
		if err := w.watcher.Close(); err != nil {
			return fmt.Errorf("error closing fsnotify watcher: %w", err)
		}
		return nil
	}

	// Wait for the watch goroutine to exit with timeout
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if err := w.watcher.Close(); err != nil {
			return fmt.Errorf("error closing fsnotify watcher: %w", err)
		}
		return nil
	case <-time.After(config.Timeout):
		if err := w.watcher.Close(); err != nil {
			return fmt.Errorf("timeout waiting for fsnotify watcher to stop after %v, and error closing watcher: %w", config.Timeout, err)
		}
		return fmt.Errorf("timeout waiting for fsnotify watcher to stop after %v", config.Timeout)
	}
}

// addWatchPath adds a single path to the watcher.
func (w *FSNotifyWatcher) addWatchPath(path string) error {
	if w.watchPaths[path] {
		return nil // Already watching this path
	}

	if err := w.watcher.Add(path); err != nil {
		return fmt.Errorf("failed to add watch path %s: %w", path, err)
	}

	w.watchPaths[path] = true
	return nil
}

// addRecursiveWatchPaths adds all subdirectories to the watcher.
func (w *FSNotifyWatcher) addRecursiveWatchPaths(rootPath string) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only watch directories
		if info != nil && info.IsDir() {
			return w.addWatchPath(path)
		}

		return nil
	})
}

// watchLoop is the main event processing loop.
//
//nolint:gocyclo
func (w *FSNotifyWatcher) watchLoop(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	errHandler vfsevents.ErrorHandlerFunc,
	status *vfsevents.WatcherStatus,
	config *vfsevents.StartConfig) {

	for {
		select {
		case <-ctx.Done():
			status.Running = false
			if config.StatusCallback != nil {
				config.StatusCallback(*status)
			}
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			fmt.Printf("DEBUG: Raw fsnotify event - Name: %q, Op: %s\n", event.Name, event.Op.String())

			vfsEvent := w.convertEvent(event)
			if vfsEvent != nil {
				fmt.Printf("DEBUG: Converted VFS event - URI: %q, Type: %s\n", vfsEvent.URI, vfsEvent.Type)

				// Apply event filter if configured before calling handler
				shouldProcess := true
				if config.EventFilter != nil {
					shouldProcess = config.EventFilter(*vfsEvent)
					fmt.Printf("DEBUG: Filter result for URI %q: %t\n", vfsEvent.URI, shouldProcess)
				} else {
					fmt.Printf("DEBUG: No event filter configured\n")
				}

				if shouldProcess {
					fmt.Printf("DEBUG: Calling handler for URI: %q\n", vfsEvent.URI)
					// Apply the handler
					handler(*vfsEvent)

					// Update status after successful event processing
					status.EventsProcessed++
					status.LastEventTime = time.Now()
					if config.StatusCallback != nil {
						config.StatusCallback(*status)
					}
				} else {
					fmt.Printf("DEBUG: Event filtered out, not calling handler for URI: %q\n", vfsEvent.URI)
				}

				// If recursive and a new directory was created, start watching it
				if w.recursive && event.Has(fsnotify.Create) {
					w.handleNewDirectory(event.Name)
				}

				// If a watched directory was deleted, clean up our tracking
				if event.Has(fsnotify.Remove) {
					w.handleDeletedDirectory(event.Name)
				}
			} else {
				fmt.Printf("DEBUG: convertEvent returned nil for fsnotify event: %q %s\n", event.Name, event.Op.String())
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}

			status.LastError = err
			if config.StatusCallback != nil {
				config.StatusCallback(*status)
			}
			errHandler(fmt.Errorf("fsnotify error: %w", err))
		}
	}
}

// convertEvent converts fsnotify events to vfsevents.Event.
func (w *FSNotifyWatcher) convertEvent(event fsnotify.Event) *vfsevents.Event {
	var eventType vfsevents.EventType

	switch {
	case event.Has(fsnotify.Create):
		eventType = vfsevents.EventCreated
	case event.Has(fsnotify.Write):
		eventType = vfsevents.EventModified
	case event.Has(fsnotify.Remove):
		eventType = vfsevents.EventDeleted
	case event.Has(fsnotify.Rename):
		// Treat rename as delete (the old name is gone)
		eventType = vfsevents.EventDeleted
	case event.Has(fsnotify.Chmod):
		// Treat chmod as modify
		eventType = vfsevents.EventModified
	default:
		// Unknown event type, skip
		return nil
	}

	// Convert local path back to VFS URI
	uri := "file://" + event.Name

	return &vfsevents.Event{
		URI:       uri,
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Metadata: map[string]string{
			"fsnotify_op": event.Op.String(),
			"path":        event.Name,
		},
	}
}

// handleNewDirectory adds a newly created directory to the watch list if recursive is enabled.
func (w *FSNotifyWatcher) handleNewDirectory(path string) {
	// Check if the new path is a directory
	// We'll try to add it to the watcher - if it fails, it's probably not a directory
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.addWatchPath(path); err != nil {
		// Silently ignore errors - the path might not be a directory or might not exist anymore
		return
	}
}

// handleDeletedDirectory removes a deleted directory from the watch list.
func (w *FSNotifyWatcher) handleDeletedDirectory(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Remove the deleted directory from the watch list
	delete(w.watchPaths, path)

	// If recursive, remove all subdirectories
	if w.recursive {
		w.removeRecursiveWatchPaths(path)
	}
}

// removeRecursiveWatchPaths removes all subdirectories from the watch list.
func (w *FSNotifyWatcher) removeRecursiveWatchPaths(rootPath string) {
	// Since the directory is already deleted, we can't walk it.
	// Instead, iterate through our watchPaths and remove any that are subdirectories.
	for watchedPath := range w.watchPaths {
		if strings.HasPrefix(watchedPath, rootPath+string(filepath.Separator)) {
			delete(w.watchPaths, watchedPath)
		}
	}
}
