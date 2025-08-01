package fsnotify

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7/vfssimple"
)

type FSNotifyWatcherTestSuite struct {
	suite.Suite
	tempDir string
	watcher *FSNotifyWatcher
}

func (s *FSNotifyWatcherTestSuite) SetupTest() {
	// Create a temporary directory for testing
	var err error
	s.tempDir, err = os.MkdirTemp("", "fsnotify_test_*")
	s.Require().NoError(err)
}

func (s *FSNotifyWatcherTestSuite) TearDownTest() {
	// Stop watcher if running
	if s.watcher != nil {
		s.watcher.Stop()
		s.watcher = nil
	}

	// Clean up temporary directory
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
		s.tempDir = ""
	}
}

func (s *FSNotifyWatcherTestSuite) TestNewFSNotifyWatcher() {
	s.Run("Valid local location", func() {
		location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", s.tempDir))
		s.Require().NoError(err)

		watcher, err := NewFSNotifyWatcher(location)
		s.Assert().NoError(err)
		s.Assert().NotNil(watcher)
		s.Assert().Equal(location, watcher.location)
		s.Assert().False(watcher.recursive)
	})

	s.Run("With recursive option", func() {
		location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", s.tempDir))
		s.Require().NoError(err)

		watcher, err := NewFSNotifyWatcher(location, WithRecursive(true))
		s.Assert().NoError(err)
		s.Assert().NotNil(watcher)
		s.Assert().True(watcher.recursive)
	})

	s.Run("Nil location", func() {
		watcher, err := NewFSNotifyWatcher(nil)
		s.Assert().Error(err)
		s.Assert().Nil(watcher)
		s.Assert().Contains(err.Error(), "location cannot be nil")
	})

	s.Run("Non-local location", func() {
		location, err := vfssimple.NewLocation("s3://bucket/path/")
		s.Require().NoError(err)

		watcher, err := NewFSNotifyWatcher(location)
		s.Assert().Error(err)
		s.Assert().Nil(watcher)
		s.Assert().Contains(err.Error(), "fsnotify watcher only supports local filesystem locations")
	})
}

func (s *FSNotifyWatcherTestSuite) TestStartAndStop() {
	location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location)
	s.Require().NoError(err)

	s.Run("Valid start", func() {
		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			events <- event
		}

		errorHandler := func(err error) {
			errors <- err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := s.watcher.Start(ctx, eventHandler, errorHandler)
		s.Assert().NoError(err)

		// Give the watcher time to start
		time.Sleep(100 * time.Millisecond)

		// Create a test file
		testFile := filepath.Join(s.tempDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		s.Require().NoError(err)

		// Wait for event
		select {
		case event := <-events:
			s.Assert().Equal(vfsevents.EventCreated, event.Type)
			s.Assert().Contains(event.URI, "test.txt")
		case err := <-errors:
			s.Fail("Unexpected error: %v", err)
		case <-time.After(2 * time.Second):
			s.Fail("Timeout waiting for create event")
		}

		// Stop the watcher
		err = s.watcher.Stop()
		s.Assert().NoError(err)
	})

	s.Run("Already running", func() {
		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			events <- event
		}

		errorHandler := func(err error) {
			errors <- err
		}

		ctx := context.Background()

		// Start first time
		err := s.watcher.Start(ctx, eventHandler, errorHandler)
		s.Assert().NoError(err)

		// Try to start again
		err = s.watcher.Start(ctx, eventHandler, errorHandler)
		s.Assert().Error(err)
		s.Assert().Contains(err.Error(), "already running")

		// Clean up
		s.watcher.Stop()
	})
}

func (s *FSNotifyWatcherTestSuite) TestFileOperations() {
	location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location)
	s.Require().NoError(err)

	events := make(chan vfsevents.Event, 10)
	errors := make(chan error, 10)

	eventHandler := func(event vfsevents.Event) {
		events <- event
	}

	errorHandler := func(err error) {
		errors <- err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = s.watcher.Start(ctx, eventHandler, errorHandler)
	s.Require().NoError(err)
	defer s.watcher.Stop()

	// Give the watcher time to start
	time.Sleep(100 * time.Millisecond)

	s.Run("Create file", func() {
		testFile := filepath.Join(s.tempDir, "create_test.txt")
		err := os.WriteFile(testFile, []byte("test content"), 0644)
		s.Require().NoError(err)

		select {
		case event := <-events:
			s.Assert().Equal(vfsevents.EventCreated, event.Type)
			s.Assert().Contains(event.URI, "create_test.txt")
			s.Assert().Contains(event.Metadata["path"], "create_test.txt")
		case <-time.After(2 * time.Second):
			s.Fail("Timeout waiting for create event")
		}
	})

	s.Run("Modify file", func() {
		testFile := filepath.Join(s.tempDir, "modify_test.txt")

		// Create file first
		err := os.WriteFile(testFile, []byte("initial content"), 0644)
		s.Require().NoError(err)

		// Wait for create event
		select {
		case <-events:
		case <-time.After(2 * time.Second):
			s.Fail("Timeout waiting for create event")
		}

		// Modify the file
		err = os.WriteFile(testFile, []byte("modified content"), 0644)
		s.Require().NoError(err)

		select {
		case event := <-events:
			s.Assert().Equal(vfsevents.EventModified, event.Type)
			s.Assert().Contains(event.URI, "modify_test.txt")
		case <-time.After(2 * time.Second):
			s.Fail("Timeout waiting for modify event")
		}
	})

	s.Run("Delete file", func() {
		testFile := filepath.Join(s.tempDir, "delete_test.txt")

		// Create file first
		err := os.WriteFile(testFile, []byte("content to delete"), 0644)
		s.Require().NoError(err)

		// Wait for create event
		select {
		case <-events:
		case <-time.After(2 * time.Second):
			s.Fail("Timeout waiting for create event")
		}

		// Give a small delay to ensure the file is fully written
		time.Sleep(100 * time.Millisecond)

		// Delete the file
		err = os.Remove(testFile)
		s.Require().NoError(err)

		// Wait for delete event with longer timeout for macOS
		// Note: fsnotify behavior can vary by platform - some may generate REMOVE, others RENAME
		eventReceived := false
		timeout := time.After(5 * time.Second)

		for !eventReceived {
			select {
			case event := <-events:
				// Accept both delete and rename events as valid for file removal
				if event.Type == vfsevents.EventDeleted {
					s.Assert().Contains(event.URI, "delete_test.txt")
					eventReceived = true
				}
				// Continue waiting if it's not a delete event (might be a modify event from the delete operation)
			case <-timeout:
				s.Fail("Timeout waiting for delete event - this may be a platform-specific fsnotify behavior")
				return
			}
		}
	})
}

func (s *FSNotifyWatcherTestSuite) TestRecursiveWatching() {
	location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location, WithRecursive(true))
	s.Require().NoError(err)

	events := make(chan vfsevents.Event, 10)
	errors := make(chan error, 10)

	eventHandler := func(event vfsevents.Event) {
		events <- event
	}

	errorHandler := func(err error) {
		errors <- err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = s.watcher.Start(ctx, eventHandler, errorHandler)
	s.Require().NoError(err)
	defer s.watcher.Stop()

	// Give the watcher time to start
	time.Sleep(100 * time.Millisecond)

	s.Run("Create subdirectory and file", func() {
		// Create subdirectory
		subDir := filepath.Join(s.tempDir, "subdir")
		err := os.Mkdir(subDir, 0755)
		s.Require().NoError(err)

		// Wait for directory create event
		select {
		case event := <-events:
			s.Assert().Equal(vfsevents.EventCreated, event.Type)
			s.Assert().Contains(event.URI, "subdir")
		case <-time.After(2 * time.Second):
			s.Fail("Timeout waiting for directory create event")
		}

		// Give time for the new directory to be added to the watch list
		time.Sleep(200 * time.Millisecond)

		// Create file in subdirectory
		testFile := filepath.Join(subDir, "nested_file.txt")
		err = os.WriteFile(testFile, []byte("nested content"), 0644)
		s.Require().NoError(err)

		// Wait for file create event
		select {
		case event := <-events:
			s.Assert().Equal(vfsevents.EventCreated, event.Type)
			s.Assert().Contains(event.URI, "nested_file.txt")
		case <-time.After(2 * time.Second):
			s.Fail("Timeout waiting for nested file create event")
		}
	})
}

func (s *FSNotifyWatcherTestSuite) TestEventFiltering() {
	location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location)
	s.Require().NoError(err)

	events := make(chan vfsevents.Event, 10)
	errors := make(chan error, 10)

	eventHandler := func(event vfsevents.Event) {
		events <- event
	}

	errorHandler := func(err error) {
		errors <- err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start with event filter - only .txt files
	err = s.watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			return strings.HasSuffix(e.URI, ".txt")
		}),
	)
	s.Require().NoError(err)
	defer s.watcher.Stop()

	// Give the watcher time to start
	time.Sleep(100 * time.Millisecond)

	s.Run("Filtered events", func() {
		// Create a .txt file (should be processed)
		txtFile := filepath.Join(s.tempDir, "test.txt")
		err := os.WriteFile(txtFile, []byte("txt content"), 0644)
		s.Require().NoError(err)

		// Create a .log file (should be filtered out)
		logFile := filepath.Join(s.tempDir, "test.log")
		err = os.WriteFile(logFile, []byte("log content"), 0644)
		s.Require().NoError(err)

		// Should only receive the .txt file event
		select {
		case event := <-events:
			s.Assert().Equal(vfsevents.EventCreated, event.Type)
			s.Assert().Contains(event.URI, "test.txt")
		case <-time.After(2 * time.Second):
			s.Fail("Timeout waiting for filtered event")
		}

		// Should not receive any more events (the .log file should be filtered out)
		select {
		case event := <-events:
			s.Fail("Received unexpected event: %+v", event)
		case <-time.After(500 * time.Millisecond):
			// Expected - no more events should be received
		}
	})
}

func (s *FSNotifyWatcherTestSuite) TestStatusCallback() {
	location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location)
	s.Require().NoError(err)

	events := make(chan vfsevents.Event, 10)
	errors := make(chan error, 10)
	statuses := make(chan vfsevents.WatcherStatus, 10)

	eventHandler := func(event vfsevents.Event) {
		events <- event
	}

	errorHandler := func(err error) {
		errors <- err
	}

	statusHandler := func(status vfsevents.WatcherStatus) {
		statuses <- status
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = s.watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithStatusCallback(statusHandler),
	)
	s.Require().NoError(err)
	defer s.watcher.Stop()

	// Should receive initial status
	select {
	case status := <-statuses:
		s.Assert().True(status.Running)
		s.Assert().Equal(int64(0), status.EventsProcessed)
	case <-time.After(2 * time.Second):
		s.Fail("Timeout waiting for initial status")
	}

	// Create a file to trigger an event
	testFile := filepath.Join(s.tempDir, "status_test.txt")
	err = os.WriteFile(testFile, []byte("status test"), 0644)
	s.Require().NoError(err)

	// Should receive event
	select {
	case <-events:
	case <-time.After(2 * time.Second):
		s.Fail("Timeout waiting for event")
	}

	// Should receive updated status
	select {
	case status := <-statuses:
		s.Assert().True(status.Running)
		s.Assert().Equal(int64(1), status.EventsProcessed)
		s.Assert().False(status.LastEventTime.IsZero())
	case <-time.After(2 * time.Second):
		s.Fail("Timeout waiting for updated status")
	}
}

func TestFSNotifyWatcherTestSuite(t *testing.T) {
	suite.Run(t, new(FSNotifyWatcherTestSuite))
}

// Example demonstrates basic usage of FSNotify watcher for monitoring local filesystem
func Example() {
	// Create a temporary directory for demonstration
	tempDir, err := os.MkdirTemp("", "fsnotify_example_*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create VFS location for local filesystem
	location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", tempDir))
	if err != nil {
		log.Fatalf("Failed to create VFS location: %v", err)
	}

	// Create FSNotify watcher
	watcher, err := NewFSNotifyWatcher(location)
	if err != nil {
		log.Fatalf("Failed to create FSNotify watcher: %v", err)
	}

	// Define event handler
	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("FSNotify Event: %s | %s\n", event.Type.String(), event.URI)

		// Access metadata
		if path, exists := event.Metadata["path"]; exists {
			fmt.Printf("Local path: %s\n", path)
		}
		if op, exists := event.Metadata["fsnotify_op"]; exists {
			fmt.Printf("FSNotify operation: %s\n", op)
		}
	}

	// Define error handler
	errorHandler := func(err error) {
		fmt.Printf("FSNotify watcher error: %v\n", err)
	}

	// Start watching
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler)
	if err != nil {
		log.Fatalf("Failed to start FSNotify watcher: %v", err)
	}

	// Stop watching
	watcher.Stop()
}

// ExampleNewFSNotifyWatcher_withRecursive demonstrates recursive filesystem watching
func ExampleNewFSNotifyWatcher_withRecursive() {
	// Create a temporary directory for demonstration
	tempDir, err := os.MkdirTemp("", "fsnotify_recursive_*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create VFS location
	location, err := vfssimple.NewLocation(fmt.Sprintf("file://%s/", tempDir))
	if err != nil {
		log.Fatalf("Failed to create VFS location: %v", err)
	}

	// Create FSNotify watcher with recursive watching
	watcher, err := NewFSNotifyWatcher(location, WithRecursive(true))
	if err != nil {
		log.Fatalf("Failed to create FSNotify watcher: %v", err)
	}

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("Recursive FSNotify Event: %s on %s\n", event.Type.String(), event.URI)

		// Show the local filesystem path
		if path, exists := event.Metadata["path"]; exists {
			fmt.Printf("Full path: %s\n", path)
		}
	}

	errorHandler := func(err error) {
		fmt.Printf("Error: %v\n", err)
	}

	ctx := context.Background()

	// Start with advanced configuration
	err = watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			// Only process certain file types
			return strings.HasSuffix(e.URI, ".txt") ||
				strings.HasSuffix(e.URI, ".log") ||
				e.Type == vfsevents.EventDeleted // Always process deletions
		}),
		vfsevents.WithStatusCallback(func(status vfsevents.WatcherStatus) {
			if status.EventsProcessed > 0 {
				fmt.Printf("FSNotify processed %d events\n", status.EventsProcessed)
			}
		}),
	)
	if err != nil {
		log.Fatalf("Failed to start FSNotify watcher: %v", err)
	}

	// Graceful shutdown
	watcher.Stop(vfsevents.WithTimeout(10 * time.Second))
}
