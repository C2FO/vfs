package fsnotify

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7/vfssimple"
)

// Helper function to create cross-platform file:// URLs
func fileURL(path string) string {
	// Convert backslashes to forward slashes for Windows
	path = filepath.ToSlash(path)
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + path + "/"
}

// Helper function to get platform-appropriate timeouts
func getEventTimeout() time.Duration {
	switch runtime.GOOS {
	case "windows":
		return 5 * time.Second // Windows can be slower
	case "darwin":
		return 4 * time.Second // macOS sometimes needs more time
	default:
		return 3 * time.Second // Linux/Unix
	}
}

// Helper function to get platform-appropriate stabilization delay
func getStabilizationDelay() time.Duration {
	switch runtime.GOOS {
	case "windows":
		return 200 * time.Millisecond
	case "darwin":
		return 150 * time.Millisecond
	default:
		return 100 * time.Millisecond
	}
}

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
		_ = s.watcher.Stop() // Ignore error in test teardown
		s.watcher = nil
	}

	// Clean up temporary directory
	if s.tempDir != "" {
		err := os.RemoveAll(s.tempDir)
		s.Require().NoError(err)
		s.tempDir = ""
	}
}

func (s *FSNotifyWatcherTestSuite) TestNewFSNotifyWatcher() {
	s.Run("Valid local location", func() {
		location, err := vfssimple.NewLocation(fileURL(s.tempDir))
		s.Require().NoError(err)

		watcher, err := NewFSNotifyWatcher(location)
		s.Require().NoError(err)
		s.NotNil(watcher)
		s.Equal(location, watcher.location)
		s.False(watcher.recursive)
	})

	s.Run("With recursive option", func() {
		location, err := vfssimple.NewLocation(fileURL(s.tempDir))
		s.Require().NoError(err)

		watcher, err := NewFSNotifyWatcher(location, WithRecursive(true))
		s.Require().NoError(err)
		s.NotNil(watcher)
		s.True(watcher.recursive)
	})

	s.Run("Nil location", func() {
		watcher, err := NewFSNotifyWatcher(nil)
		s.Require().Error(err)
		s.Nil(watcher)
		s.Require().ErrorContains(err, "location cannot be nil")
	})

	s.Run("Non-local location", func() {
		location, err := vfssimple.NewLocation("s3://bucket/path/")
		s.Require().NoError(err)

		watcher, err := NewFSNotifyWatcher(location)
		s.Require().Error(err)
		s.Nil(watcher)
		s.Require().ErrorContains(err, "fsnotify watcher only supports local filesystem locations")
	})
}

func (s *FSNotifyWatcherTestSuite) TestStartAndStop() {
	location, err := vfssimple.NewLocation(fileURL(s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location)
	s.Require().NoError(err)

	s.Run("Valid start", func() {
		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			fmt.Printf("TEST DEBUG: Handler received event - URI: %q, Type: %s\n", event.URI, event.Type)
			events <- event
		}

		errorHandler := func(err error) {
			fmt.Printf("TEST DEBUG: Error handler called: %v\n", err)
			errors <- err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := s.watcher.Start(ctx, eventHandler, errorHandler)
		s.Require().NoError(err)

		// Give the watcher time to start
		time.Sleep(getStabilizationDelay())

		// Create a test file
		testFile := filepath.Join(s.tempDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0600)
		s.Require().NoError(err)

		// Wait for event
		select {
		case event := <-events:
			s.Equal(vfsevents.EventCreated, event.Type)
			s.Equal("file://"+testFile, event.URI)
		case err := <-errors:
			s.Require().NoError(err)
		case <-time.After(getEventTimeout()):
			s.Fail("Timeout waiting for create event")
		}

		// Stop the watcher
		err = s.watcher.Stop()
		s.Require().NoError(err)
	})

	s.Run("Already running", func() {
		events := make(chan vfsevents.Event, 10)
		errors := make(chan error, 10)

		eventHandler := func(event vfsevents.Event) {
			fmt.Printf("TEST DEBUG: Handler received event - URI: %q, Type: %s\n", event.URI, event.Type)
			events <- event
		}

		errorHandler := func(err error) {
			fmt.Printf("TEST DEBUG: Error handler called: %v\n", err)
			errors <- err
		}

		ctx := context.Background()

		// Start first time
		err := s.watcher.Start(ctx, eventHandler, errorHandler)
		s.Require().NoError(err)

		// Try to start again
		err = s.watcher.Start(ctx, eventHandler, errorHandler)
		s.Require().Error(err)
		s.Require().ErrorContains(err, "already running")

		// Clean up
		_ = s.watcher.Stop() // Ignore error in test cleanup
	})
}

//nolint:gocyclo
func (s *FSNotifyWatcherTestSuite) TestFileOperations() {
	location, err := vfssimple.NewLocation(fileURL(s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location)
	s.Require().NoError(err)

	events := make(chan vfsevents.Event, 10)
	errors := make(chan error, 10)

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("TEST DEBUG: Handler received event - URI: %q, Type: %s\n", event.URI, event.Type)
		events <- event
	}

	errorHandler := func(err error) {
		fmt.Printf("TEST DEBUG: Error handler called: %v\n", err)
		errors <- err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = s.watcher.Start(ctx, eventHandler, errorHandler)
	s.Require().NoError(err)
	defer func() { _ = s.watcher.Stop() }() // Ignore error in test cleanup

	// Give the watcher time to start
	time.Sleep(getStabilizationDelay())

	// Create file
	s.Run("Create file", func() {
		testFile := filepath.Join(s.tempDir, "create_test.txt")
		err := os.WriteFile(testFile, []byte("test content"), 0600)
		s.Require().NoError(err)

		select {
		case event := <-events:
			s.Equal(vfsevents.EventCreated, event.Type)
			s.Equal("file://"+testFile, event.URI)
			s.Equal(testFile, event.Metadata["path"])
		case <-time.After(getEventTimeout()):
			s.Fail("Timeout waiting for create event")
		}
	})

	s.Run("Modify file", func() {
		testFile := filepath.Join(s.tempDir, "modify_test.txt")

		// Create file first
		err := os.WriteFile(testFile, []byte("initial content"), 0600)
		s.Require().NoError(err)

		// Wait for create event and drain any additional events
		eventCount := 0
		timeout := time.After(getEventTimeout())
		createReceived := false

	drainLoop:
		for {
			select {
			case event := <-events:
				eventCount++
				if event.Type == vfsevents.EventCreated {
					createReceived = true
				}
				// Continue draining for a short time to catch any additional events
				time.Sleep(50 * time.Millisecond)
			case <-time.After(100 * time.Millisecond):
				// No more events, break out
				break drainLoop
			case <-timeout:
				if !createReceived {
					s.Fail("Timeout waiting for create event")
				}
				break drainLoop
			}
		}

		s.True(createReceived, "Should have received create event")

		// Add a small delay before modifying to ensure filesystem stability
		time.Sleep(getStabilizationDelay())

		// Modify the file
		err = os.WriteFile(testFile, []byte("modified content"), 0600)
		s.Require().NoError(err)

		// Wait for modify event - be more flexible about event count
		modifyReceived := false
		timeout = time.After(getEventTimeout())

	modifyLoop:
		for {
			select {
			case event := <-events:
				if event.Type == vfsevents.EventModified {
					modifyReceived = true
					s.Equal("file://"+testFile, event.URI)
					s.Equal(testFile, event.Metadata["path"])
					break modifyLoop
				}
				// Continue waiting for modify event, ignore other events
			case <-timeout:
				break modifyLoop
			}
		}

		s.True(modifyReceived, "Should have received modify event")
	})

	s.Run("Delete file", func() {
		testFile := filepath.Join(s.tempDir, "delete_test.txt")

		// Create file first
		err := os.WriteFile(testFile, []byte("content to delete"), 0600)
		s.Require().NoError(err)

		// Wait for create event
		select {
		case <-events:
		case <-time.After(getEventTimeout()):
			s.Fail("Timeout waiting for create event")
		}

		// Give a small delay to ensure the file is fully written
		time.Sleep(getStabilizationDelay())

		// Delete the file
		err = os.Remove(testFile)
		s.Require().NoError(err)

		// Wait for delete event with longer timeout for macOS
		// Note: fsnotify behavior can vary by platform - some may generate REMOVE, others RENAME
		eventReceived := false
		timeout := time.After(getEventTimeout() * 2)

		for !eventReceived {
			select {
			case event := <-events:
				// Accept both delete and rename events as valid for file removal
				if event.Type == vfsevents.EventDeleted {
					s.Equal("file://"+testFile, event.URI)
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
	location, err := vfssimple.NewLocation(fileURL(s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location, WithRecursive(true))
	s.Require().NoError(err)

	events := make(chan vfsevents.Event, 10)
	errors := make(chan error, 10)

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("TEST DEBUG: Handler received event - URI: %q, Type: %s\n", event.URI, event.Type)
		events <- event
	}

	errorHandler := func(err error) {
		fmt.Printf("TEST DEBUG: Error handler called: %v\n", err)
		errors <- err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = s.watcher.Start(ctx, eventHandler, errorHandler)
	s.Require().NoError(err)
	defer func() { _ = s.watcher.Stop() }() // Ignore error in test cleanup

	// Give the watcher time to start
	time.Sleep(getStabilizationDelay())

	s.Run("Create subdirectory and file", func() {
		// Create subdirectory
		subDir := filepath.Join(s.tempDir, "subdir")
		err := os.Mkdir(subDir, 0750)
		s.Require().NoError(err)

		// Wait for directory create event
		select {
		case event := <-events:
			s.Equal(vfsevents.EventCreated, event.Type)
			s.Equal("file://"+subDir, event.URI)
		case <-time.After(getEventTimeout()):
			s.Fail("Timeout waiting for directory create event")
		}

		// Give time for the new directory to be added to the watch list
		time.Sleep(getStabilizationDelay())

		// Create file in subdirectory
		testFile := filepath.Join(subDir, "nested_file.txt")
		err = os.WriteFile(testFile, []byte("nested content"), 0600)
		s.Require().NoError(err)

		// Wait for file create event
		select {
		case event := <-events:
			s.Equal(vfsevents.EventCreated, event.Type)
			s.Equal("file://"+testFile, event.URI)
		case <-time.After(getEventTimeout()):
			s.Fail("Timeout waiting for nested file create event")
		}
	})
}

func (s *FSNotifyWatcherTestSuite) TestEventFiltering() {
	location, err := vfssimple.NewLocation(fileURL(s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location)
	s.Require().NoError(err)

	events := make(chan vfsevents.Event, 10)
	errors := make(chan error, 10)

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("TEST DEBUG: Handler received event - URI: %q, Type: %s\n", event.URI, event.Type)
		events <- event
	}

	errorHandler := func(err error) {
		fmt.Printf("TEST DEBUG: Error handler called: %v\n", err)
		errors <- err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start with event filter - only .txt files
	err = s.watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			result := strings.HasSuffix(e.URI, ".txt")
			fmt.Printf("TEST DEBUG: Filter called for URI %q, result: %t\n", e.URI, result)
			return result
		}),
	)
	s.Require().NoError(err)
	defer func() { _ = s.watcher.Stop() }() // Ignore error in test cleanup

	// Give the watcher time to start
	time.Sleep(getStabilizationDelay())

	s.Run("Filtered events", func() {
		s.runEventFilteringTest(events)
	})
}

func (s *FSNotifyWatcherTestSuite) runEventFilteringTest(events chan vfsevents.Event) {
	// Create a .txt file (should be processed)
	txtFile := filepath.Join(s.tempDir, "test.txt")
	fmt.Printf("TEST DEBUG: Creating .txt file at: %q\n", txtFile)
	err := os.WriteFile(txtFile, []byte("txt content"), 0600)
	s.Require().NoError(err)
	fmt.Printf("TEST DEBUG: .txt file created successfully\n")

	// Give more time for the event to be processed on Windows
	time.Sleep(getStabilizationDelay())

	// Create a .log file (should be filtered out)
	logFile := filepath.Join(s.tempDir, "test.log")
	fmt.Printf("TEST DEBUG: Creating .log file at: %q\n", logFile)
	err = os.WriteFile(logFile, []byte("log content"), 0600)
	s.Require().NoError(err)
	fmt.Printf("TEST DEBUG: .log file created successfully\n")

	// Wait for events and verify filtering
	txtEventReceived, logEventReceived, eventCount := s.collectAndVerifyEvents(events)

	fmt.Printf("TEST DEBUG: Final state - txtEventReceived: %t, logEventReceived: %t, eventCount: %d\n",
		txtEventReceived, logEventReceived, eventCount)
	s.True(txtEventReceived, "Should have received .txt file event")
	s.False(logEventReceived, "Should not have received .log file event")
}

func (s *FSNotifyWatcherTestSuite) collectAndVerifyEvents(events chan vfsevents.Event) (bool, bool, int) {
	txtEventReceived := false
	logEventReceived := false
	timeout := time.After(getEventTimeout())
	eventCount := 0
	maxEvents := 10 // Prevent infinite loop

	fmt.Printf("TEST DEBUG: Starting event collection loop, timeout: %v\n", getEventTimeout())

eventLoop:
	for eventCount < maxEvents {
		select {
		case event := <-events:
			eventCount++
			fmt.Printf("TEST DEBUG: Event loop received event #%d - URI: %q, Type: %s\n", eventCount, event.URI, event.Type)

			if s.isTextFileEvent(event.URI) {
				txtEventReceived = true
				fmt.Printf("TEST DEBUG: Marking txtEventReceived = true\n")
				// Accept both Created and Modified events for .txt files
				s.True(event.Type == vfsevents.EventCreated || event.Type == vfsevents.EventModified,
					"Expected Created or Modified event for .txt file, got: %s", event.Type)
			} else if s.isLogFileEvent(event.URI) {
				logEventReceived = true
				fmt.Printf("TEST DEBUG: Marking logEventReceived = true - THIS SHOULD NOT HAPPEN!\n")
				s.Fail("Received unexpected event", event)
			} else {
				fmt.Printf("TEST DEBUG: Received unexpected event (not test.txt or test.log): %+v\n", event)
			}

			// If we got the txt event, check for additional events
			if txtEventReceived && s.checkForAdditionalEvents(events, &logEventReceived, &eventCount) {
				break eventLoop
			}
		case <-time.After(100 * time.Millisecond):
			// Short timeout between events
			if txtEventReceived {
				fmt.Printf("TEST DEBUG: Short timeout reached, but txt event received\n")
				break eventLoop
			}
			fmt.Printf("TEST DEBUG: Short timeout, continuing to wait for events\n")
		case <-timeout:
			fmt.Printf("TEST DEBUG: Main timeout reached, breaking loop\n")
			break eventLoop
		}
	}

	return txtEventReceived, logEventReceived, eventCount
}

func (s *FSNotifyWatcherTestSuite) isTextFileEvent(uri string) bool {
	return strings.HasSuffix(uri, "/test.txt") || strings.HasSuffix(uri, "\\test.txt")
}

func (s *FSNotifyWatcherTestSuite) isLogFileEvent(uri string) bool {
	return strings.HasSuffix(uri, "/test.log") || strings.HasSuffix(uri, "\\test.log")
}

func (s *FSNotifyWatcherTestSuite) checkForAdditionalEvents(events chan vfsevents.Event, logEventReceived *bool, eventCount *int) bool {
	// Continue for a bit longer to catch any potential log events
	select {
	case event2 := <-events:
		*eventCount++
		fmt.Printf("TEST DEBUG: Additional event #%d - URI: %q, Type: %s\n", *eventCount, event2.URI, event2.Type)
		if s.isLogFileEvent(event2.URI) {
			*logEventReceived = true
			fmt.Printf("TEST DEBUG: Marking logEventReceived = true - THIS SHOULD NOT HAPPEN!\n")
			s.Fail("Received unexpected event", event2)
		}
	case <-time.After(200 * time.Millisecond):
		// No additional events, good
		fmt.Printf("TEST DEBUG: No additional events after 200ms\n")
	}
	return true
}

func (s *FSNotifyWatcherTestSuite) TestStatusCallback() {
	location, err := vfssimple.NewLocation(fileURL(s.tempDir))
	s.Require().NoError(err)

	s.watcher, err = NewFSNotifyWatcher(location)
	s.Require().NoError(err)

	events := make(chan vfsevents.Event, 10)
	errors := make(chan error, 10)
	statuses := make(chan vfsevents.WatcherStatus, 10)

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("TEST DEBUG: Handler received event - URI: %q, Type: %s\n", event.URI, event.Type)
		events <- event
	}

	errorHandler := func(err error) {
		fmt.Printf("TEST DEBUG: Error handler called: %v\n", err)
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
	defer func() { _ = s.watcher.Stop() }() // Ignore error in test cleanup

	// Should receive initial status
	select {
	case status := <-statuses:
		s.True(status.Running)
		s.Zero(status.EventsProcessed)
	case <-time.After(getEventTimeout()):
		s.Fail("Timeout waiting for initial status")
	}

	// Create a file to trigger an event
	testFile := filepath.Join(s.tempDir, "status_test.txt")
	err = os.WriteFile(testFile, []byte("status test"), 0600)
	s.Require().NoError(err)

	// Should receive event
	select {
	case <-events:
	case <-time.After(getEventTimeout()):
		s.Fail("Timeout waiting for event")
	}

	// Should receive updated status
	select {
	case status := <-statuses:
		s.True(status.Running)
		s.Equal(int64(1), status.EventsProcessed)
		s.False(status.LastEventTime.IsZero())
	case <-time.After(getEventTimeout()):
		s.Fail("Timeout waiting for updated status")
	}
}

func TestFSNotifyWatcherTestSuite(t *testing.T) {
	suite.Run(t, new(FSNotifyWatcherTestSuite))
}

// Example demonstrates basic usage of FSNotify watcher for monitoring local filesystem events
func Example() {
	// Create a temporary directory for demonstration
	tempDir, err := os.MkdirTemp("", "fsnotify_example_*")
	if err != nil {
		log.Printf("Failed to create temp dir: %v", err)
		return
	}
	defer func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			log.Printf("Failed to remove temp dir: %v", err)
		}
	}()

	// Create VFS location for local filesystem
	location, err := vfssimple.NewLocation(fileURL(tempDir))
	if err != nil {
		log.Printf("Failed to create VFS location: %v", err)
		return
	}

	// Create FSNotify watcher
	watcher, err := NewFSNotifyWatcher(location)
	if err != nil {
		log.Printf("Failed to create FSNotify watcher: %v", err)
		return
	}

	// Set up event and error handlers
	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("FSNotify Event: %s - %s\n", event.Type, event.URI)
	}

	errorHandler := func(err error) {
		fmt.Printf("FSNotify Error: %v\n", err)
	}

	// Start watching with a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler)
	if err != nil {
		log.Printf("Failed to start FSNotify watcher: %v", err)
		return
	}

	// Stop watching
	err = watcher.Stop(vfsevents.WithTimeout(10 * time.Second))
	if err != nil {
		log.Printf("Failed to stop FSNotify watcher: %v", err)
		return
	}
}

// ExampleNewFSNotifyWatcher_withRecursive demonstrates recursive filesystem watching
func ExampleNewFSNotifyWatcher_withRecursive() {
	// Create a temporary directory for demonstration
	tempDir, err := os.MkdirTemp("", "fsnotify_recursive_*")
	if err != nil {
		log.Printf("Failed to create temp dir: %v", err)
		return
	}
	defer func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			log.Printf("Failed to remove temp dir: %v", err)
		}
	}()

	// Create VFS location
	location, err := vfssimple.NewLocation(fileURL(tempDir))
	if err != nil {
		log.Printf("Failed to create VFS location: %v", err)
		return
	}

	// Create FSNotify watcher with recursive monitoring
	watcher, err := NewFSNotifyWatcher(location, WithRecursive(true))
	if err != nil {
		log.Printf("Failed to create FSNotify watcher: %v", err)
		return
	}

	// Set up event and error handlers
	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("Recursive FSNotify Event: %s - %s\n", event.Type, event.URI)
	}

	errorHandler := func(err error) {
		fmt.Printf("Recursive FSNotify Error: %v\n", err)
	}

	// Start watching with advanced options
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler,
		// Only process .txt files and deletions
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			return strings.HasSuffix(e.URI, ".txt") || // Only .txt files
				e.Type == vfsevents.EventDeleted // Always process deletions
		}),
		vfsevents.WithStatusCallback(func(status vfsevents.WatcherStatus) {
			if status.EventsProcessed > 0 {
				fmt.Printf("FSNotify processed %d events\n", status.EventsProcessed)
			}
		}),
	)
	if err != nil {
		log.Printf("Failed to start FSNotify watcher: %v", err)
		return
	}

	// Graceful shutdown
	err = watcher.Stop(vfsevents.WithTimeout(10 * time.Second))
	if err != nil {
		log.Printf("Failed to stop FSNotify watcher: %v", err)
		return
	}
}
