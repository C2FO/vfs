package lockfile_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"
	"github.com/c2fo/vfs/v7/contrib/lockfile"
)

func TestNewLock(t *testing.T) {
	tests := []struct {
		name          string
		filePath      string
		opts          []lockfile.Option
		expectedError error
	}{
		{
			name:          "valid file path",
			filePath:      "/test.txt",
			opts:          nil,
			expectedError: nil,
		},
		{
			name:          "with TTL",
			filePath:      "/test.txt",
			opts:          []lockfile.Option{lockfile.WithTTL(5 * time.Second)},
			expectedError: nil,
		},
		{
			name:          "with owner ID",
			filePath:      "/test.txt",
			opts:          []lockfile.Option{lockfile.WithOwnerID("test-owner")},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := mem.NewFileSystem()
			f, err := fs.NewFile("", tt.filePath)
			require.NoError(t, err)

			lock, err := lockfile.NewLock(f, tt.opts...)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, lock)
		})
	}
}

func TestAcquireAndRelease(t *testing.T) {
	tests := []struct {
		name          string
		ttl           time.Duration
		shouldAcquire bool
		shouldRelease bool
	}{
		{
			name:          "acquire and release with no TTL",
			ttl:           0,
			shouldAcquire: true,
			shouldRelease: true,
		},
		{
			name:          "acquire and release with TTL",
			ttl:           5 * time.Second,
			shouldAcquire: true,
			shouldRelease: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := mem.NewFileSystem()
			f, err := fs.NewFile("", "/test.txt")
			require.NoError(t, err)

			lock, err := lockfile.NewLock(f, lockfile.WithTTL(tt.ttl))
			require.NoError(t, err)

			if tt.shouldAcquire {
				err = lock.Acquire()
				require.NoError(t, err)

				// Verify lock file exists
				exists, err := lock.LockFile().Exists()
				require.NoError(t, err)
				require.True(t, exists)
			}

			if tt.shouldRelease {
				err = lock.Release()
				require.NoError(t, err)

				// Verify lock file is gone
				exists, err := lock.LockFile().Exists()
				require.NoError(t, err)
				require.False(t, exists)
			}
		})
	}
}

func TestConcurrentLocks(t *testing.T) {
	fs := mem.NewFileSystem()
	f, err := fs.NewFile("", "/test.txt")
	require.NoError(t, err)

	// First lock
	lock1, err := lockfile.NewLock(f)
	require.NoError(t, err)
	err = lock1.Acquire()
	require.NoError(t, err)

	// Second lock attempt should fail
	lock2, err := lockfile.NewLock(f)
	require.NoError(t, err)
	err = lock2.Acquire()
	require.ErrorIs(t, err, lockfile.ErrLockAlreadyHeld)

	// Release first lock
	err = lock1.Release()
	require.NoError(t, err)

	// Second lock should now succeed
	err = lock2.Acquire()
	require.NoError(t, err)
	err = lock2.Release()
	require.NoError(t, err)
}

func TestMetadata(t *testing.T) {
	fs := mem.NewFileSystem()
	f, err := fs.NewFile("", "/test.txt")
	require.NoError(t, err)

	lock, err := lockfile.NewLock(f, lockfile.WithTTL(5*time.Second), lockfile.WithOwnerID("test-owner"))
	require.NoError(t, err)

	err = lock.Acquire()
	require.NoError(t, err)

	meta, err := lock.Metadata()
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, "test-owner", meta.OwnerID)
	require.Equal(t, 5*time.Second, meta.TTL)
	require.NotEmpty(t, meta.Hostname)
	require.NotZero(t, meta.PID)
	require.WithinDuration(t, time.Now(), meta.CreatedAt, time.Second)

	err = lock.Release()
	require.NoError(t, err)
}

func TestAge(t *testing.T) {
	fs := mem.NewFileSystem()
	f, err := fs.NewFile("", "/test.txt")
	require.NoError(t, err)

	lock, err := lockfile.NewLock(f)
	require.NoError(t, err)

	err = lock.Acquire()
	require.NoError(t, err)

	age, err := lock.Age()
	require.NoError(t, err)
	require.GreaterOrEqual(t, age, time.Duration(0))
	require.Less(t, age, time.Second) // Should be very recent

	err = lock.Release()
	require.NoError(t, err)
}

func TestStaleLock(t *testing.T) {
	fs := mem.NewFileSystem()
	f, err := fs.NewFile("", "/test.txt")
	require.NoError(t, err)

	// Create a lock with a very short TTL
	lock1, err := lockfile.NewLock(f, lockfile.WithTTL(100*time.Millisecond))
	require.NoError(t, err)

	err = lock1.Acquire()
	require.NoError(t, err)

	// Wait for lock to become stale
	time.Sleep(200 * time.Millisecond)

	// Create second lock with stale handler
	staleHandlerCalled := false
	lock2, err := lockfile.NewLock(f,
		lockfile.WithTTL(5*time.Second),
		lockfile.OnStale(func(meta lockfile.Metadata) error {
			staleHandlerCalled = true
			return nil
		}),
	)
	require.NoError(t, err)

	// Should be able to acquire the stale lock
	err = lock2.Acquire()
	require.NoError(t, err)
	require.True(t, staleHandlerCalled)

	err = lock2.Release()
	require.NoError(t, err)
}

func TestErrorCases(t *testing.T) {
	tests := []struct {
		name          string
		setup         func() (*lockfile.Lock, error)
		action        func(*lockfile.Lock) error
		expectedError error
	}{
		{
			name: "release without acquire",
			setup: func() (*lockfile.Lock, error) {
				fs := mem.NewFileSystem()
				f, err := fs.NewFile("", "/test.txt")
				if err != nil {
					return nil, err
				}
				return lockfile.NewLock(f)
			},
			action:        func(l *lockfile.Lock) error { return l.Release() },
			expectedError: nil, // Release is now idempotent
		},
		{
			name: "metadata without lock",
			setup: func() (*lockfile.Lock, error) {
				fs := mem.NewFileSystem()
				f, err := fs.NewFile("", "/test.txt")
				if err != nil {
					return nil, err
				}
				return lockfile.NewLock(f)
			},
			action: func(l *lockfile.Lock) error {
				_, err := l.Metadata()
				return err
			},
			expectedError: errors.New("file does not exist"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock, err := tt.setup()
			require.NoError(t, err)

			err = tt.action(lock)
			if tt.expectedError != nil {
				require.ErrorContains(t, err, tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWithLock(t *testing.T) {
	fs := mem.NewFileSystem()
	f, err := fs.NewFile("", "/test.txt")
	require.NoError(t, err)

	// Test successful lock acquisition and execution
	called := false
	err = lockfile.WithLock(f, func(f vfs.File) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, called)

	// Test concurrent access
	lock, err := lockfile.NewLock(f)
	require.NoError(t, err)
	err = lock.Acquire()
	require.NoError(t, err)

	err = lockfile.WithLock(f, func(f vfs.File) error {
		return nil
	})
	require.ErrorIs(t, err, lockfile.ErrLockAlreadyHeld)

	err = lock.Release()
	require.NoError(t, err)

	// Test error propagation
	expectedErr := errors.New("test error")
	err = lockfile.WithLock(f, func(f vfs.File) error {
		return expectedErr
	})
	require.ErrorIs(t, err, expectedErr)
}
