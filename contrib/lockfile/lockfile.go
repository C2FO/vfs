/*
Package lockfile provides an advisory locking mechanism for vfs.File objects
using companion `.lock` files.

⚠️ Advisory Locking Note:

This package implements **advisory locks**, which means:
- Locks are **not enforced by the OS, filesystem, or remote backend**
- All cooperating processes must **explicitly check for and honor** the lock
- It is still possible for non-cooperative processes to ignore the lock and access the file

Lock files are created using atomic file creation (write to <lockfile>.tmp -> MoveTo <lockfile>)
and include metadata such as timestamp, PID, hostname, and optional TTL.

This approach is portable and backend-agnostic, and it works on local filesystems,
cloud object storage (e.g., S3/GCS), SFTP, and any vfs backend that supports
basic file creation and deletion.

This is similar in spirit to common conventions used by package managers, editors,
and distributed systems where mandatory locking is unavailable or unreliable.
*/
package lockfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/c2fo/vfs/v7"
)

var (
	ErrLockAlreadyHeld = errors.New("lockfile: lock already held")
	ErrLockNotHeld     = errors.New("lockfile: no lock held")
)

// Metadata contains information about the current lock holder.
type Metadata struct {
	CreatedAt time.Time     `json:"createdAt"`
	TTL       time.Duration `json:"ttl"`
	Hostname  string        `json:"hostname"`
	PID       int           `json:"pid"`
	OwnerID   string        `json:"ownerId,omitempty"`
}

// StaleHandler is a function that is called when a lock is stale.
// It is called with the lock metadata and should return an error if the lock should not be acquired.
// If the error is nil, the lock will be acquired.
type StaleHandler func(meta Metadata) error

// Lock provides a portable advisory locking mechanism for files.
// It uses a separate lock file with metadata to implement the lock.
// The lock is advisory, meaning it only works if all processes using the file
// respect the lock.
type Lock struct {
	// lockedFile is the file being protected by the lock
	lockedFile vfs.File
	// lockFile is the file containing the lock metadata
	lockFile vfs.File
	// metadata contains information about the current lock holder
	metadata Metadata
	// ttl is the time-to-live for the lock. If 0, the lock never expires.
	ttl time.Duration
	// ownerID is an optional identifier for the lock owner
	ownerID string
	// onStale is called when attempting to acquire a stale lock
	onStale StaleHandler
	// metadataRead caches the last read metadata
	metadataRead *Metadata
}

// Option is a function that configures a Lock.
type Option func(*Lock)

// WithTTL sets the time-to-live for the lock.
// If the lock is not acquired within this time, it will be considered stale.
// If the TTL is 0, the lock never expires.
func WithTTL(ttl time.Duration) Option {
	return func(l *Lock) {
		l.ttl = ttl
	}
}

// WithOwnerID sets the owner ID for the lock.
// This is an optional identifier for the lock owner.
func WithOwnerID(owner string) Option {
	return func(l *Lock) {
		l.ownerID = owner
	}
}

// OnStale sets the function to call when the lock is stale.
// The function is called with the lock metadata and should return an error if the lock should not be acquired.
// If the error is nil, the lock will be acquired.
func OnStale(hook StaleHandler) Option {
	return func(l *Lock) {
		l.onStale = hook
	}
}

// NewLock creates a new Lock instance for the given file.
// The lock file will be created at the same location as the target file
// with a ".lock" extension.
func NewLock(f vfs.File, opts ...Option) (*Lock, error) {
	lockFile, err := f.Location().NewFile(f.Name() + ".lock")
	if err != nil {
		return nil, fmt.Errorf("lockfile: error creating lock file: %w", err)
	}

	lock := &Lock{
		lockedFile: f,
		lockFile:   lockFile,
		ttl:        0,
	}

	for _, opt := range opts {
		opt(lock)
	}

	return lock, nil
}

// Acquire attempts to acquire the lock.
// If the lock is already held and not stale, returns ErrLockAlreadyHeld.
// If the lock is stale, it will be acquired after calling the OnStale handler if set.
// The operation is atomic using a temporary file to ensure consistency.
func (l *Lock) Acquire() error {
	exists, err := l.lockFile.Exists()
	if err != nil {
		return fmt.Errorf("lockfile: error checking lock existence: %w", err)
	}
	if exists {
		stale, meta, err := l.IsStale()
		if err != nil {
			return fmt.Errorf("lockfile: error checking staleness: %w", err)
		}
		if !stale {
			return ErrLockAlreadyHeld
		}
		if l.onStale != nil {
			if err := l.onStale(*meta); err != nil {
				return fmt.Errorf("lockfile: stale handler blocked lock steal: %w", err)
			}
		}
		if err := l.lockFile.Delete(); err != nil {
			return fmt.Errorf("lockfile: error deleting stale lock: %w", err)
		}
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	l.metadata = Metadata{
		CreatedAt: time.Now().UTC(),
		TTL:       l.ttl,
		PID:       os.Getpid(),
		Hostname:  hostname,
		OwnerID:   l.ownerID,
	}

	data, err := json.MarshalIndent(l.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("lockfile: error marshaling metadata: %w", err)
	}

	// Use a temporary file for atomic write
	tmp, err := l.lockFile.Location().NewFile(l.lockFile.Name() + ".tmp")
	if err != nil {
		return fmt.Errorf("lockfile: error creating temp file: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("lockfile: error writing lock data: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("lockfile: error closing temp file: %w", err)
	}

	return tmp.MoveToFile(l.lockFile)
}

// Release releases the lock if it is currently held.
// If the lock file does not exist, it is treated as a successful release.
func (l *Lock) Release() error {
	if l.lockFile == nil {
		return nil
	}
	exists, err := l.lockFile.Exists()
	if err != nil {
		return fmt.Errorf("lockfile: error checking lock existence: %w", err)
	}
	if !exists {
		return nil
	}
	return l.lockFile.Delete()
}

// Age returns the duration since the lock was created.
// Returns an error if the lock metadata cannot be read.
func (l *Lock) Age() (time.Duration, error) {
	meta, err := l.Metadata()
	if err != nil {
		return 0, fmt.Errorf("lockfile: error getting lock age: %w", err)
	}
	return time.Since(meta.CreatedAt), nil
}

// Metadata returns the current lock metadata.
// The result is cached for subsequent calls.
func (l *Lock) Metadata() (*Metadata, error) {
	if l.metadataRead != nil {
		return l.metadataRead, nil
	}

	defer func() { _ = l.lockFile.Close() }()

	var meta Metadata
	if err := json.NewDecoder(l.lockFile).Decode(&meta); err != nil {
		return nil, fmt.Errorf("lockfile: error decoding metadata: %w", err)
	}
	l.metadataRead = &meta
	return &meta, nil
}

// IsStale checks if the current lock is stale based on its TTL.
// Returns true if the lock is stale, along with the lock metadata.
// A lock with zero TTL is never considered stale.
func (l *Lock) IsStale() (bool, *Metadata, error) {
	meta, err := l.Metadata()
	if err != nil {
		return false, nil, fmt.Errorf("lockfile: error checking lock staleness: %w", err)
	}
	// If TTL is zero, the lock never expires
	if meta.TTL == 0 {
		return false, meta, nil
	}
	expiry := meta.CreatedAt.Add(meta.TTL)
	return time.Now().After(expiry), meta, nil
}

// LockFile returns the underlying lock file.
// This is exposed for testing purposes only.
func (l *Lock) LockFile() vfs.File {
	return l.lockFile
}

// WithLock provides a convenient way to acquire a lock, execute a function, and release the lock.
// The lock is automatically released when the function returns, even if it panics.
// This is useful for scoped locking where you want to ensure the lock is always released.
func WithLock(f vfs.File, fn func(vfs.File) error, opts ...Option) error {
	lock, err := NewLock(f, opts...)
	if err != nil {
		return err
	}
	if err := lock.Acquire(); err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	return fn(f)
}
