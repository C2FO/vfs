package fsnotify_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/contrib/vfsevents/watchers/fsnotify"
	"github.com/c2fo/vfs/v7/vfssimple"
)

// ExampleFSNotifyWatcher_debouncing demonstrates how to use event debouncing
// to consolidate multiple related filesystem events into single logical events.
func ExampleFSNotifyWatcher_debouncing() {
	// Create a VFS location for local filesystem
	location, err := vfssimple.NewLocation("file:///tmp/watch")
	if err != nil {
		log.Fatal(err)
	}

	// Create FSNotify watcher with 500ms debouncing and recursive watching
	watcher, err := fsnotify.NewFSNotifyWatcher(location,
		fsnotify.WithDebounce(500*time.Millisecond),
		fsnotify.WithRecursive(true))
	if err != nil {
		log.Fatal(err)
	}

	// Define event handler that shows debouncing in action
	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("Event: %s on %s\n", event.Type.String(), event.URI)

		// Check if this was a consolidated event from debouncing
		if op, exists := event.Metadata["fsnotify_op"]; exists {
			if op == "multiple" {
				fmt.Printf("  -> Consolidated multiple operations\n")
			} else {
				fmt.Printf("  -> Single operation: %s\n", op)
			}
		}
	}

	// Define error handler
	errorHandler := func(err error) {
		log.Printf("FSNotify error: %v", err)
	}

	// Start watching with event filtering for .txt files only
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			// Only process .txt files
			return len(e.URI) >= 4 && e.URI[len(e.URI)-4:] == ".txt"
		}))
	if err != nil {
		defer cancel()
		log.Printf("Failed to start FSNotify watcher: %v", err)
		return
	}

	// Stop watching when done
	defer func() { _ = watcher.Stop() }()

	fmt.Println("Watching /tmp/watch for .txt file changes with 500ms debouncing...")
	fmt.Println("Try creating, modifying, or deleting .txt files to see debouncing in action")

	// In a real application, you would keep the program running
	// select {}
}

// ExampleFSNotifyWatcher_networkFilesystem demonstrates debouncing configuration
// for network filesystems like SFTP where longer debounce periods are beneficial.
func ExampleFSNotifyWatcher_networkFilesystem() {
	location, err := vfssimple.NewLocation("file:///mnt/sftp-mount")
	if err != nil {
		log.Fatal(err)
	}

	// Use longer debounce period for network filesystems
	// where file writes may be paused and resumed
	watcher, err := fsnotify.NewFSNotifyWatcher(location,
		fsnotify.WithDebounce(2*time.Second))
	if err != nil {
		log.Fatal(err)
	}

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("Network FS Event: %s on %s\n",
			event.Type.String(), event.URI)
	}

	errorHandler := func(err error) {
		log.Printf("Network FS error: %v", err)
	}

	ctx := context.Background()
	if err := watcher.Start(ctx, eventHandler, errorHandler); err != nil {
		log.Printf("Failed to start FSNotify watcher: %v", err)
		return
	}

	defer func() { _ = watcher.Stop() }()

	fmt.Println("Watching network filesystem with 2s debouncing for delayed writes...")
}

// ExampleFSNotifyWatcher_buildTool demonstrates debouncing for build tools
// that need to avoid excessive rebuilds during rapid file changes.
func ExampleFSNotifyWatcher_buildTool() {
	location, err := vfssimple.NewLocation("file:///project/src")
	if err != nil {
		log.Fatal(err)
	}

	// Short debounce for responsive development feedback
	watcher, err := fsnotify.NewFSNotifyWatcher(location,
		fsnotify.WithDebounce(100*time.Millisecond),
		fsnotify.WithRecursive(true))
	if err != nil {
		log.Fatal(err)
	}

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("Source change detected: %s\n", event.URI)

		// Trigger build process
		fmt.Println("  -> Triggering incremental build...")
	}

	errorHandler := func(err error) {
		log.Printf("Build watcher error: %v", err)
	}

	ctx := context.Background()
	err = watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			// Only watch source files
			uri := e.URI
			return len(uri) >= 3 && (uri[len(uri)-3:] == ".go" ||
				uri[len(uri)-3:] == ".js" ||
				(len(uri) >= 4 && uri[len(uri)-4:] == ".tsx"))
		}))
	if err != nil {
		log.Printf("Failed to start FSNotify watcher: %v", err)
		return
	}

	defer func() { _ = watcher.Stop() }()

	fmt.Println("Build tool watching source files with 100ms debouncing...")
}
