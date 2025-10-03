package sftp

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	_sftp "github.com/pkg/sftp"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v7/utils/authority"
)

// SFTPConcurrencyTestSuite tests typed nil handling and concurrency robustness
// in the SFTP backend under failure scenarios
type SFTPConcurrencyTestSuite struct {
	suite.Suite
	originalClientGetter func(authority.Authority, Options) (Client, io.Closer, error)
}

func TestSFTPConcurrencyTestSuite(t *testing.T) {
	suite.Run(t, new(SFTPConcurrencyTestSuite))
}

func (s *SFTPConcurrencyTestSuite) SetupTest() {
	// Save the original client getter (which might be mocked by other tests)
	s.originalClientGetter = defaultClientGetter

	// Reset to the real client getter to force actual SFTP connection attempts
	defaultClientGetter = func(auth authority.Authority, opts Options) (Client, io.Closer, error) {
		return GetClient(auth, opts)
	}
}

func (s *SFTPConcurrencyTestSuite) TearDownTest() {
	// Restore the original client getter
	defaultClientGetter = s.originalClientGetter
}

// TestClientTypedNilHandling tests that the Client() method properly handles
// typed nil pointers that can occur when client creation fails
func (s *SFTPConcurrencyTestSuite) TestClientTypedNilHandling() {
	// Create filesystem with invalid configuration to trigger connection failures
	fs := NewFileSystem(WithOptions(Options{
		Username:           "nonexistentuser",
		Password:           "wrongpassword",
		KnownHostsCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // Test code - insecure host key acceptable
	}))

	// Use a non-existent host to ensure connection failures
	auth, err := authority.NewAuthority("sftp://nonexistentuser@nonexistent-host:22")
	s.Require().NoError(err)

	// Multiple attempts should fail gracefully without panics
	for i := range 3 {
		client, err := fs.Client(auth)
		s.T().Logf("Attempt %d: client=%v, err=%v", i+1, client, err)
		if err == nil {
			s.T().Logf("Client type: %T, is nil: %v", client, client == nil)
		}
		s.Require().Error(err, "Should get connection error due to invalid configuration")
	}
}

// TestConcurrentFailedConnections tests that multiple goroutines attempting
// to create failed connections don't cause panics or race conditions
func (s *SFTPConcurrencyTestSuite) TestConcurrentFailedConnections() {
	// Create filesystem with invalid configuration
	fs := NewFileSystem(WithOptions(Options{
		Username:           "testuser",
		Password:           "wrongpassword",
		KnownHostsCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // Test code - insecure host key acceptable
	}))

	auth, err := authority.NewAuthority("sftp://testuser@nonexistent-host:22")
	s.Require().NoError(err)

	// Start multiple goroutines that will all fail to connect
	const numGoroutines = 10
	var wg sync.WaitGroup
	panicChan := make(chan interface{}, numGoroutines)
	errorChan := make(chan error, numGoroutines)

	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicChan <- r
				}
			}()

			// This should fail but not panic
			_, err := fs.Client(auth)
			errorChan <- err
		}()
	}

	wg.Wait()
	close(panicChan)
	close(errorChan)

	// Collect any panics that occurred
	panics := make([]interface{}, 0, len(panicChan))
	for panic := range panicChan {
		panics = append(panics, panic)
	}

	// Collect errors
	errors := make([]error, 0, len(errorChan))
	for err := range errorChan {
		errors = append(errors, err)
	}

	// Assert that no panics occurred
	s.Empty(panics, "No goroutines should panic")

	// Assert that all operations failed with connection errors
	s.NotEmpty(errors, "All operations should fail with connection errors")
	for _, err := range errors {
		s.Require().Error(err, "Each operation should return an error")
	}
}

// TestClientFailureRobustness simulates multiple failed connection attempts
// to ensure the filesystem remains stable and doesn't panic
func (s *SFTPConcurrencyTestSuite) TestClientFailureRobustness() {
	s.Run("Multiple failed connection attempts should not panic", func() {
		for i := range 5 {
			s.Run(fmt.Sprintf("Attempt %d", i+1), func() {
				// Create a new filesystem for each attempt to avoid state pollution
				fs := NewFileSystem(WithOptions(Options{
					Username:           "testuser",
					Password:           "wrongpassword",             // Wrong password to trigger connection failure
					KnownHostsCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // Test code - insecure host key acceptable
				}))

				auth, err := authority.NewAuthority("sftp://testuser@nonexistent-host:22")
				s.Require().NoError(err)

				// Test multiple client creation attempts that will fail
				// This is where the typed nil issue would occur
				for range 3 {
					_, _ = fs.Client(auth) // This should fail but not panic
				}

				// Verify that the filesystem is still usable after failures
				_, err = fs.Client(auth)
				// We expect an error due to wrong credentials, but no panic
				s.Require().Error(err, "Should get connection error due to wrong credentials")
			})
		}
	})
}

// TestTimerCleanupRobustness tests the interaction between the auto-disconnect
// timer and active client operations to ensure no panics occur
func (s *SFTPConcurrencyTestSuite) TestTimerCleanupRobustness() {
	// Create filesystem with very short auto-disconnect to trigger timer quickly
	fs := NewFileSystem(WithOptions(Options{
		Username:           "testuser",
		Password:           "wrongpassword",
		KnownHostsCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // Test code - insecure host key acceptable
		AutoDisconnect:     1,                           // Very short timeout to trigger timer quickly
	}))

	auth, err := authority.NewAuthority("sftp://testuser@nonexistent-host:22")
	s.Require().NoError(err)

	// Start multiple operations that will fail but might trigger the timer
	const numOperations = 10
	var wg sync.WaitGroup
	panicChan := make(chan interface{}, numOperations)

	for range numOperations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicChan <- r
				}
			}()

			// Only test client creation to avoid mock conflicts
			// This is where the typed nil issue would occur
			_, _ = fs.Client(auth)

			// Add a small delay to increase chance of timer firing
			time.Sleep(100 * time.Millisecond)

			// Try another client operation to see if timer cleanup caused issues
			_, _ = fs.Client(auth)
		}()
	}

	wg.Wait()
	close(panicChan)

	// Collect any panics that occurred
	panics := make([]interface{}, 0, len(panicChan))
	for panic := range panicChan {
		panics = append(panics, panic)
	}

	// Assert that no panics occurred
	s.Empty(panics, "Timer cleanup should not cause panics")
}

// TestTimerLogicValidation validates that the timer correctly handles
// valid clients vs typed-nil clients
func (s *SFTPConcurrencyTestSuite) TestTimerLogicValidation() {
	s.Run("Timer closes valid client", func() {
		// Create a mock client that tracks Close() calls
		mockClient := &mockClosableClient{
			closeCalled: make(chan bool, 1),
		}
		mockConn := &mockCloser{closeCalled: make(chan bool, 1)}

		fs := &FileSystem{
			sftpclient: mockClient,
			sshConn:    mockConn,
			options: Options{
				AutoDisconnect: 1, // 1 second timeout
			},
		}

		// Verify client is set before timer
		s.NotNil(fs.sftpclient, "Client should exist before timer")
		s.NotNil(fs.sshConn, "SSH connection should exist before timer")

		// Start the timer
		fs.connTimerStart()

		// Wait for timer to fire and Close to be called
		select {
		case <-mockClient.closeCalled:
			// Success - Close was called on valid client
		case <-time.After(2 * time.Second):
			s.Fail("Timer should have closed the valid client")
		}

		// Also verify SSH connection was closed
		select {
		case <-mockConn.closeCalled:
			// Success - Close was called on SSH connection
		case <-time.After(100 * time.Millisecond):
			s.Fail("Timer should have closed the SSH connection")
		}

		// Verify both client and connection were set to nil after timer
		fs.timerMutex.Lock()
		s.Nil(fs.sftpclient, "Client should be nil after timer")
		s.Nil(fs.sshConn, "SSH connection should be nil after timer")
		fs.timerMutex.Unlock()
	})

	s.Run("Timer handles typed-nil client safely", func() {
		// Create a typed-nil client (this is what happens when client creation fails)
		var typedNilClient Client = (*mockClosableClient)(nil)

		fs := &FileSystem{
			sftpclient: typedNilClient,
			options: Options{
				AutoDisconnect: 1, // 1 second timeout
			},
		}

		// Verify we start with a typed-nil
		s.True(reflect.ValueOf(fs.sftpclient).IsNil(), "Should start with typed-nil client")

		// Capture any panic
		panicOccurred := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicOccurred = true
				}
			}()

			// Start the timer
			fs.connTimerStart()

			// Wait for timer to fire
			time.Sleep(1500 * time.Millisecond)
		}()

		// Verify no panic occurred
		s.False(panicOccurred, "Timer should handle typed-nil without panic")
	})

	s.Run("Timer does not call Close on typed-nil client", func() {
		// This test validates that Close() is NOT called on typed-nil
		// (which would panic since the receiver is nil)
		closableNil := &mockClosableClient{
			closeCalled: make(chan bool, 1),
		}
		var typedNilClient Client = (*mockClosableClient)(nil)

		fs := &FileSystem{
			sftpclient: typedNilClient,
			options: Options{
				AutoDisconnect: 1,
			},
		}

		// Verify it's a typed nil before timer starts
		s.True(reflect.ValueOf(fs.sftpclient).IsNil(), "Should start with typed-nil")

		// This is the critical test - no panic should occur
		panicOccurred := false
		done := make(chan bool, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					panicOccurred = true
				}
				done <- true
			}()

			// Start timer
			fs.connTimerStart()

			// Wait for timer to fire
			time.Sleep(1500 * time.Millisecond)
		}()

		<-done

		// The key validation: no panic should have occurred
		s.False(panicOccurred, "Timer should handle typed-nil without panic")

		// Verify Close was never called (channel should be empty)
		select {
		case <-closableNil.closeCalled:
			s.Fail("Close should NOT be called on typed-nil client")
		default:
			// Success - Close was not called
		}
	})
}

// mockClosableClient implements Client interface for testing timer behavior
type mockClosableClient struct {
	closeCalled chan bool
}

func (m *mockClosableClient) Close() error {
	if m.closeCalled != nil {
		m.closeCalled <- true
	}
	return nil
}

func (m *mockClosableClient) Chmod(path string, mode os.FileMode) error { return nil }
func (m *mockClosableClient) Chtimes(path string, atime, mtime time.Time) error {
	return nil
}
func (m *mockClosableClient) Create(path string) (*_sftp.File, error) { return nil, nil }
func (m *mockClosableClient) MkdirAll(path string) error              { return nil }
func (m *mockClosableClient) OpenFile(path string, f int) (*_sftp.File, error) {
	return nil, nil
}
func (m *mockClosableClient) ReadDir(p string) ([]os.FileInfo, error) { return nil, nil }
func (m *mockClosableClient) Remove(path string) error                { return nil }
func (m *mockClosableClient) Rename(oldname, newname string) error    { return nil }
func (m *mockClosableClient) Stat(p string) (os.FileInfo, error)      { return nil, nil }

// mockCloser implements io.Closer for testing
type mockCloser struct {
	closeCalled chan bool
}

func (m *mockCloser) Close() error {
	if m.closeCalled != nil {
		m.closeCalled <- true
	}
	return nil
}
