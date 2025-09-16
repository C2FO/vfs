package sftp

import (
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

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
	for i := 0; i < 3; i++ {
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

	for i := 0; i < numGoroutines; i++ {
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
		for i := 0; i < 5; i++ {
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
				for j := 0; j < 3; j++ {
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

	for i := 0; i < numOperations; i++ {
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
