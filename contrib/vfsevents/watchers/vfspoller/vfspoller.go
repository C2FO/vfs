// Package vfspoller implements a polling watcher for vfs.Locations.
package vfspoller

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"sort"
	"sync"
	"time"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
)

// FileInfo stores cached file metadata so a poll can tell whether a file changed
type FileInfo struct {
	URI          string
	LastModified time.Time
	Size         uint64
	FirstSeen    time.Time
}

// Poller implements the Watcher interface by polling a vfs.Location.
//
// Performance Considerations:
//   - Suitable for small to medium directories (< 1000 files)
//   - For large directories, consider using cloud-native event watchers (S3Events, GCSEvents)
//   - Memory usage grows with number of files but includes periodic cleanup
//   - Each poll lists the location with list.Entries. A backend with native listing reports every file's size and
//     modification time in that listing, so a poll makes no per-file requests; other backends are listed by name and
//     a file's metadata is requested only when it is needed
type Poller struct {
	location    vfs.Location
	interval    time.Duration
	minAge      time.Duration
	maxFiles    int           // Maximum files to track (prevents memory issues)
	cleanupAge  time.Duration // Age after which to remove files from cache
	cancel      context.CancelFunc
	mu          sync.Mutex
	fileCache   map[string]*FileInfo // Cached file metadata
	wg          sync.WaitGroup
	lastCleanup time.Time
}

// Option is a functional option for configuring a Poller.
type Option func(*Poller)

// WithInterval sets the polling interval. Default is 1 minute.
func WithInterval(interval time.Duration) Option {
	return func(p *Poller) {
		p.interval = interval
	}
}

// WithMinAge sets the minimum age of files to consider. This is useful for ignoring files that are still being written on some
// filesystems, such as OS, SFTP, and FTP.
func WithMinAge(age time.Duration) Option {
	return func(p *Poller) {
		p.minAge = age
	}
}

// WithMaxFiles sets the maximum number of files to track in memory.
// When exceeded, oldest files are removed from cache. Default is 10000.
func WithMaxFiles(maxFiles int) Option {
	return func(p *Poller) {
		p.maxFiles = maxFiles
	}
}

// WithCleanupAge sets how long to keep deleted files in cache before cleanup.
// This prevents memory leaks from deleted files. Default is 24 hours.
func WithCleanupAge(age time.Duration) Option {
	return func(p *Poller) {
		p.cleanupAge = age
	}
}

// NewPoller initializes an Poller with a vfs.Location.
func NewPoller(location vfs.Location, opts ...Option) (*Poller, error) {
	// validate location
	if location == nil {
		return nil, errors.New("location cannot be nil")
	}
	exists, err := location.Exists()
	if err != nil {
		return nil, fmt.Errorf("failed to check if location exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("location does not exist: %s", location.URI())
	}

	p := &Poller{
		location:    location,
		interval:    time.Minute,    // Default to 1 minute
		minAge:      0,              // Default to no minimum age
		maxFiles:    10000,          // Default max files to track
		cleanupAge:  24 * time.Hour, // Default cleanup age
		fileCache:   make(map[string]*FileInfo),
		lastCleanup: time.Now(),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p, nil
}

// Start begins polling the vfs.Location for file changes. It triggers the handler on events. The polling process can be
// stopped by calling Stop.
func (p *Poller) Start(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	errHandler vfsevents.ErrorHandlerFunc,
	opts ...vfsevents.StartOption) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		return errors.New("poller is already running")
	}

	// Process start options
	config := &vfsevents.StartConfig{}
	for _, opt := range opts {
		opt(config)
	}

	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	p.wg.Go(func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		// Create a wrapped handler that applies filtering if configured
		wrappedHandler := handler
		if config.EventFilter != nil {
			wrappedHandler = func(event vfsevents.Event) {
				if config.EventFilter(event) {
					handler(event)
				}
			}
		}

		// Status tracking
		status := vfsevents.WatcherStatus{
			Running:   true,
			StartTime: time.Now(),
		}

		// Send initial status if callback is configured
		if config.StatusCallback != nil {
			config.StatusCallback(status)
		}

		for {
			select {
			case <-ctx.Done():
				status.Running = false
				if config.StatusCallback != nil {
					config.StatusCallback(status)
				}
				return
			case <-ticker.C:
				if err := p.poll(ctx, wrappedHandler, config, &status); err != nil {
					if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
						// Stop canceled the listing; that is a shutdown, not a failure to report.
						continue
					}
					status.LastError = err
					if config.StatusCallback != nil {
						config.StatusCallback(status)
					}
					errHandler(err)
				} else {
					status.EventsProcessed++
					status.LastEventTime = time.Now()
					if config.StatusCallback != nil {
						config.StatusCallback(status)
					}
				}
			}
		}
	})

	return nil
}

// poll checks for new, modified, or deleted files with improved performance and memory management.
func (p *Poller) poll(
	ctx context.Context,
	handler vfsevents.HandlerFunc,
	config *vfsevents.StartConfig,
	status *vfsevents.WatcherStatus,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var entries []vfs.Entry
	var err error

	// Retry the listing if retry is enabled
	if config.RetryConfig.Enabled {
		var lastErr error
		for attempt := range config.RetryConfig.MaxRetries + 1 {
			entries, err = p.listFiles(ctx)
			if err == nil {
				// Success - reset consecutive error count
				status.ConsecutiveErrors = 0
				break
			}

			// A listing that ended because ctx did will end the same way on every retry.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("listing stopped: %w", ctxErr)
			}

			// Check if error is retryable
			if !vfsevents.IsRetryableError(err, config.RetryConfig) {
				return fmt.Errorf("non-retryable error listing files: %w", err)
			}

			lastErr = err

			// Update status with retry information
			status.RetryAttempts++
			status.ConsecutiveErrors++
			status.LastError = err
			status.LastRetryTime = time.Now()

			// Last attempt - don't retry
			if attempt == config.RetryConfig.MaxRetries {
				break
			}

			// Calculate backoff delay
			backoff := vfsevents.CalculateBackoff(attempt, config.RetryConfig)

			// Send status update if callback is configured
			if config.StatusCallback != nil {
				config.StatusCallback(*status)
			}

			// Wait before retry, unless the poller is stopped first
			select {
			case <-ctx.Done():
				return fmt.Errorf("waiting to retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		// If we still have an error after all retries, return it
		if err != nil {
			return fmt.Errorf("max retries (%d) exceeded listing files: %w", config.RetryConfig.MaxRetries, lastErr)
		}
	} else {
		// No retry - single attempt
		entries, err = p.listFiles(ctx)
		if err != nil {
			return fmt.Errorf("error listing files: %w", err)
		}
	}

	currentFiles := make(map[string]struct{})
	if err := p.processCurrentFiles(entries, currentFiles, handler); err != nil {
		return err
	}

	p.detectDeletedFiles(currentFiles, handler)
	p.performMaintenanceTasks()

	return nil
}

// listFiles lists the files directly in the location. A location that does not exist lists nothing, so when files that
// were being tracked are gone the location itself is checked: its removal is an error, not the deletion of every file.
func (p *Poller) listFiles(ctx context.Context) ([]vfs.Entry, error) {
	var entries []vfs.Entry
	for e, err := range list.Entries(ctx, p.location, list.FilesOnly()) {
		if errors.Is(err, iofs.ErrInvalid) {
			// A name that cannot be a file of this location, which the listing reports and then carries on past, must
			// not stop every poll; the file cannot be opened through the location either way.
			continue
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	if len(entries) == 0 && len(p.fileCache) > 0 {
		exists, err := p.location.Exists()
		if err != nil {
			return nil, fmt.Errorf("failed to check if location exists: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("location no longer exists: %s", p.location.URI())
		}
	}

	return entries, nil
}

// processCurrentFiles processes all current files and detects new/modified files
func (p *Poller) processCurrentFiles(entries []vfs.Entry, currentFiles map[string]struct{}, handler vfsevents.HandlerFunc) error {
	for _, e := range entries {
		f, err := e.File()
		if err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		currentFiles[f.URI()] = struct{}{}

		if err := p.processFile(e, f, handler); err != nil {
			return err
		}
	}
	return nil
}

// processFile processes a single file and handles new/modified file detection
func (p *Poller) processFile(e vfs.Entry, f vfs.File, handler vfsevents.HandlerFunc) error {
	fileMetadata, err := p.getFileMetadata(e, f)
	if err != nil {
		return err
	}

	if p.shouldSkipFile(fileMetadata) {
		return nil
	}

	return p.handleFileEvent(f, fileMetadata, handler)
}

// getFileMetadata returns the file's metadata, taking it from the listing when the backend reported it, which costs
// nothing, and otherwise requesting it only when it is needed.
func (p *Poller) getFileMetadata(e vfs.Entry, f vfs.File) (*fileMetadata, error) {
	metadata := &fileMetadata{uri: f.URI()}

	// A symlink is listed with its own size and time, but the file's methods report its target's, which is what changes.
	if e.HasMetadata() && e.Type()&iofs.ModeSymlink == 0 {
		lastModified := e.ModTime()
		metadata.lastModified = &lastModified
		metadata.size = e.Size()
		return metadata, nil
	}

	// Only call LastModified() if we need it (for minAge check or modification detection)
	if p.minAge > 0 || p.fileCache[f.URI()] != nil {
		lastModified, err := f.LastModified()
		if err != nil {
			return nil, fmt.Errorf("error getting last modified time: %w", err)
		}
		metadata.lastModified = lastModified

		fileSize, err := f.Size()
		if err != nil {
			return nil, fmt.Errorf("error getting file size: %w", err)
		}
		metadata.size = fileSize
	}

	return metadata, nil
}

// fileMetadata holds file metadata for processing
type fileMetadata struct {
	uri          string
	lastModified *time.Time
	size         uint64
}

// shouldSkipFile determines if a file should be skipped due to minAge
func (p *Poller) shouldSkipFile(metadata *fileMetadata) bool {
	if p.minAge > 0 && metadata.lastModified != nil {
		return time.Since(*metadata.lastModified) < p.minAge
	}
	return false
}

// handleFileEvent handles new file creation and modification events
func (p *Poller) handleFileEvent(f vfs.File, metadata *fileMetadata, handler vfsevents.HandlerFunc) error {
	cachedInfo, exists := p.fileCache[metadata.uri]

	if !exists {
		return p.handleNewFile(f, metadata, handler)
	}

	return p.handleExistingFile(metadata, cachedInfo, handler)
}

// handleNewFile handles new file detection and caching
func (p *Poller) handleNewFile(f vfs.File, metadata *fileMetadata, handler vfsevents.HandlerFunc) error {
	// Ensure we have metadata for new files
	if metadata.lastModified == nil {
		lastModified, err := f.LastModified()
		if err != nil {
			return fmt.Errorf("error getting last modified time: %w", err)
		}
		metadata.lastModified = lastModified

		fileSize, err := f.Size()
		if err != nil {
			return fmt.Errorf("error getting file size: %w", err)
		}
		metadata.size = fileSize
	}

	p.fileCache[metadata.uri] = &FileInfo{
		URI:          metadata.uri,
		LastModified: *metadata.lastModified,
		Size:         metadata.size,
		FirstSeen:    time.Now(),
	}

	handler(vfsevents.Event{
		URI:       metadata.uri,
		Type:      vfsevents.EventCreated,
		Timestamp: time.Now().Unix(),
	})

	return nil
}

// handleExistingFile handles modification detection for existing files
func (p *Poller) handleExistingFile(metadata *fileMetadata, cachedInfo *FileInfo, handler vfsevents.HandlerFunc) error {
	if metadata.lastModified != nil && p.isFileModified(metadata, cachedInfo) {
		cachedInfo.LastModified = *metadata.lastModified
		cachedInfo.Size = metadata.size

		handler(vfsevents.Event{
			URI:       metadata.uri,
			Type:      vfsevents.EventModified,
			Timestamp: time.Now().Unix(),
		})
	}

	return nil
}

// isFileModified checks if a file has been modified
func (p *Poller) isFileModified(metadata *fileMetadata, cachedInfo *FileInfo) bool {
	if metadata.lastModified == nil {
		return false
	}
	return !cachedInfo.LastModified.Equal(*metadata.lastModified) || metadata.size != cachedInfo.Size
}

// detectDeletedFiles detects and handles deleted files
func (p *Poller) detectDeletedFiles(currentFiles map[string]struct{}, handler vfsevents.HandlerFunc) {
	for uri := range p.fileCache {
		if _, exists := currentFiles[uri]; !exists {
			delete(p.fileCache, uri)
			handler(vfsevents.Event{
				URI:       uri,
				Type:      vfsevents.EventDeleted,
				Timestamp: time.Now().Unix(),
			})
		}
	}
}

// performMaintenanceTasks handles cleanup and cache size enforcement
func (p *Poller) performMaintenanceTasks() {
	// Periodic cleanup to prevent memory leaks
	if time.Since(p.lastCleanup) > p.cleanupAge/2 { // Cleanup more frequently than the age threshold
		p.performCleanup()
	}

	// Enforce maximum file limit
	if len(p.fileCache) > p.maxFiles {
		p.enforceMaxFiles()
	}
}

// performCleanup removes old entries from the cache to prevent memory leaks
func (p *Poller) performCleanup() {
	p.lastCleanup = time.Now()
	cutoff := time.Now().Add(-p.cleanupAge)

	for uri, fileInfo := range p.fileCache {
		if fileInfo.FirstSeen.Before(cutoff) {
			delete(p.fileCache, uri)
		}
	}
}

// enforceMaxFiles removes the oldest files when the cache exceeds maxFiles
func (p *Poller) enforceMaxFiles() {
	if len(p.fileCache) <= p.maxFiles {
		return
	}

	// Collect all URIs with their FirstSeen times
	type fileEntry struct {
		uri       string
		firstSeen time.Time
	}

	entries := make([]fileEntry, 0, len(p.fileCache))
	for uri, fileInfo := range p.fileCache {
		entries = append(entries, fileEntry{uri: uri, firstSeen: fileInfo.FirstSeen})
	}

	// Sort by FirstSeen (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].firstSeen.Before(entries[j].firstSeen)
	})

	// Remove the oldest files to get back to maxFiles
	numToRemove := len(entries) - p.maxFiles
	for i := range numToRemove {
		delete(p.fileCache, entries[i].uri)
	}
}

// Stop stops the polling process.
func (p *Poller) Stop(opts ...vfsevents.StopOption) error {
	// Process stop options
	config := &vfsevents.StopConfig{
		Timeout: 30 * time.Second, // Default timeout
		Force:   false,
	}
	for _, opt := range opts {
		opt(config)
	}

	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}

	// Handle graceful shutdown with timeout
	if config.Force {
		// Force immediate shutdown - don't wait for goroutines
		return nil
	}

	// Wait for the polling goroutine to exit with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(config.Timeout):
		return fmt.Errorf("timeout waiting for poller to stop after %v", config.Timeout)
	}
}
