package vfspoller

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/vfssimple"
)

type PollerTestSuite struct {
	suite.Suite
}

func (s *PollerTestSuite) TestNewPoller() {
	location := mocks.NewLocation(s.T())
	location.EXPECT().Exists().Return(true, nil)
	location2 := mocks.NewLocation(s.T())
	location2.EXPECT().Exists().Return(false, nil)
	location2.EXPECT().URI().Return("scheme:///location2")

	tests := []struct {
		name     string
		location vfs.Location
		wantErr  bool
	}{
		{
			name:     "Valid location",
			location: location,
			wantErr:  false,
		},
		{
			name:     "Nil location",
			location: nil,
			wantErr:  true,
		},
		{
			name:     "location does not exist",
			location: location2,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			_, err := NewPoller(tt.location)
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
		})
	}
}

func (s *PollerTestSuite) TestWithOptions() {
	location := mocks.NewLocation(s.T())
	location.EXPECT().Exists().Return(true, nil)

	// Test WithInterval
	poller, err := NewPoller(location, WithInterval(30*time.Second))
	s.NoError(err)
	s.Equal(30*time.Second, poller.interval)

	// Test WithMinAge
	poller, err = NewPoller(location, WithMinAge(5*time.Minute))
	s.NoError(err)
	s.Equal(5*time.Minute, poller.minAge)

	// Test WithMaxFiles
	poller, err = NewPoller(location, WithMaxFiles(5000))
	s.NoError(err)
	s.Equal(5000, poller.maxFiles)

	// Test WithCleanupAge
	poller, err = NewPoller(location, WithCleanupAge(12*time.Hour))
	s.NoError(err)
	s.Equal(12*time.Hour, poller.cleanupAge)

	// Test multiple options
	poller, err = NewPoller(location,
		WithInterval(45*time.Second),
		WithMinAge(2*time.Minute),
		WithMaxFiles(1000),
		WithCleanupAge(6*time.Hour))
	s.NoError(err)
	s.Equal(45*time.Second, poller.interval)
	s.Equal(2*time.Minute, poller.minAge)
	s.Equal(1000, poller.maxFiles)
	s.Equal(6*time.Hour, poller.cleanupAge)
}

func (s *PollerTestSuite) TestStart() {
	tests := []struct {
		name    string
		handler vfsevents.HandlerFunc
		wantErr bool
	}{
		{
			name:    "Valid handler",
			handler: func(event vfsevents.Event) {},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			location := mocks.NewLocation(s.T())
			location.EXPECT().Exists().Return(true, nil)
			poller, _ := NewPoller(location)
			ctx := context.Background()
			errFunc := func(err error) {
				s.NoError(err)
			}
			err := poller.Start(ctx, tt.handler, errFunc)
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
			s.NoError(poller.Stop())
		})
	}
}

func (s *PollerTestSuite) TestStop() {
	ctx := context.Background()
	handler := func(event vfsevents.Event) {}
	location := mocks.NewLocation(s.T())
	location.EXPECT().Exists().Return(true, nil)
	poller, _ := NewPoller(location)
	errFunc := func(err error) {
		s.NoError(err)
	}
	err := poller.Start(ctx, handler, errFunc)
	s.NoError(err)
	s.NoError(poller.Stop())
	// Ensure that the polling process stops correctly
	poller.mu.Lock()
	s.Nil(poller.cancel)
	poller.mu.Unlock()
}

func (s *PollerTestSuite) TestPoll() {
	hourAgo := time.Now().UTC().Add(-time.Hour)
	tests := []struct {
		name       string
		setupMocks func(*mocks.Location)
		wantErr    bool
		opts       []Option
		fileCache  map[string]*FileInfo
	}{
		{
			name: "Valid poll",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return([]string{"file1", "file2"}, nil)
				file := mocks.NewFile(s.T())
				file.EXPECT().URI().Return("scheme:///file1")
				file.EXPECT().LastModified().Return(&hourAgo, nil)
				file.EXPECT().Size().Return(uint64(100), nil)
				loc.EXPECT().NewFile("file1").Return(file, nil)
				file2 := mocks.NewFile(s.T())
				file2.EXPECT().URI().Return("scheme:///file2")
				file2.EXPECT().LastModified().Return(&hourAgo, nil)
				file2.EXPECT().Size().Return(uint64(200), nil)
				loc.EXPECT().NewFile("file2").Return(file2, nil)
			},
			wantErr: false,
		},
		{
			name: "List error",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return(nil, fmt.Errorf("list error"))
			},
			wantErr: true,
		},
		{
			name: "NewFile error",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return([]string{"file1"}, nil)
				loc.EXPECT().NewFile("file1").Return(nil, fmt.Errorf("new file error"))
			},
			wantErr: true,
		},
		{
			name: "File too new (minAge > 0)",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return([]string{"file1"}, nil)
				file := mocks.NewFile(s.T())
				file.EXPECT().URI().Return("scheme:///file1")
				timeNow := time.Now().UTC()
				file.EXPECT().LastModified().Return(&timeNow, nil)
				file.EXPECT().Size().Return(uint64(100), nil)
				loc.EXPECT().NewFile("file1").Return(file, nil)
			},
			wantErr: false,
			opts:    []Option{WithMinAge(2 * time.Minute)},
		},
		{
			name: "Detect deleted files",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return([]string{"file1"}, nil)
				file := mocks.NewFile(s.T())
				file.EXPECT().URI().Return("scheme:///file1")
				file.EXPECT().LastModified().Return(&hourAgo, nil)
				file.EXPECT().Size().Return(uint64(100), nil)
				loc.EXPECT().NewFile("file1").Return(file, nil)
			},
			wantErr: false,
			fileCache: map[string]*FileInfo{
				"scheme:///someotherfile": {
					URI:          "scheme:///someotherfile",
					LastModified: hourAgo,
					Size:         uint64(150),
					FirstSeen:    time.Now().Add(-time.Hour),
				},
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc := mocks.NewLocation(s.T())
			loc.EXPECT().Exists().Return(true, nil)
			poller, _ := NewPoller(loc, tt.opts...)
			if tt.fileCache != nil {
				poller.fileCache = tt.fileCache
			}
			tt.setupMocks(loc)

			// Create config and status for new poll signature
			config := &vfsevents.StartConfig{
				RetryConfig: vfsevents.RetryConfig{
					Enabled: false, // Disable retry for basic tests
				},
			}
			status := &vfsevents.WatcherStatus{}

			err := poller.poll(func(event vfsevents.Event) {}, config, status)
			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}
		})
	}
}

func (s *PollerTestSuite) TestPollWithRetry() {
	tests := []struct {
		name           string
		setupMocks     func(*mocks.Location)
		retryConfig    vfsevents.RetryConfig
		wantErr        bool
		expectedStatus func(*vfsevents.WatcherStatus) bool
	}{
		{
			name: "Success on first attempt",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return([]string{"file1"}, nil).Once()
				file := mocks.NewFile(s.T())
				file.EXPECT().URI().Return("scheme:///file1")
				file.EXPECT().LastModified().Return(func() *time.Time { t := time.Now().UTC().Add(-time.Hour); return &t }(), nil)
				file.EXPECT().Size().Return(uint64(100), nil)
				loc.EXPECT().NewFile("file1").Return(file, nil)
			},
			retryConfig: vfsevents.RetryConfig{
				Enabled:        true,
				MaxRetries:     2,
				InitialBackoff: 10 * time.Millisecond,
				MaxBackoff:     100 * time.Millisecond,
				BackoffFactor:  2.0,
			},
			wantErr: false,
			expectedStatus: func(status *vfsevents.WatcherStatus) bool {
				return status.RetryAttempts == 0 && status.ConsecutiveErrors == 0
			},
		},
		{
			name: "Retry disabled - fail immediately",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return(nil, fmt.Errorf("connection timeout")).Once()
			},
			retryConfig: vfsevents.RetryConfig{
				Enabled: false,
			},
			wantErr: true,
			expectedStatus: func(status *vfsevents.WatcherStatus) bool {
				return status.RetryAttempts == 0
			},
		},
		{
			name: "Retryable error with success on second attempt",
			setupMocks: func(loc *mocks.Location) {
				// First call fails with retryable error
				loc.EXPECT().List().Return(nil, fmt.Errorf("connection timeout")).Once()
				// Second call succeeds
				loc.EXPECT().List().Return([]string{"file1"}, nil).Once()
				file := mocks.NewFile(s.T())
				file.EXPECT().URI().Return("scheme:///file1")
				file.EXPECT().LastModified().Return(func() *time.Time { t := time.Now().UTC().Add(-time.Hour); return &t }(), nil)
				file.EXPECT().Size().Return(uint64(100), nil)
				loc.EXPECT().NewFile("file1").Return(file, nil)
			},
			retryConfig: vfsevents.RetryConfig{
				Enabled:         true,
				MaxRetries:      2,
				InitialBackoff:  10 * time.Millisecond,
				MaxBackoff:      100 * time.Millisecond,
				BackoffFactor:   2.0,
				RetryableErrors: []string{"timeout", "connection"},
			},
			wantErr: false,
			expectedStatus: func(status *vfsevents.WatcherStatus) bool {
				return status.RetryAttempts == 1 && status.ConsecutiveErrors == 0
			},
		},
		{
			name: "Non-retryable error - fail immediately",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return(nil, fmt.Errorf("permission denied")).Once()
			},
			retryConfig: vfsevents.RetryConfig{
				Enabled:         true,
				MaxRetries:      2,
				InitialBackoff:  10 * time.Millisecond,
				MaxBackoff:      100 * time.Millisecond,
				BackoffFactor:   2.0,
				RetryableErrors: []string{"timeout", "connection"},
			},
			wantErr: true,
			expectedStatus: func(status *vfsevents.WatcherStatus) bool {
				return status.RetryAttempts == 0
			},
		},
		{
			name: "Max retries exceeded",
			setupMocks: func(loc *mocks.Location) {
				// All attempts fail with retryable error
				loc.EXPECT().List().Return(nil, fmt.Errorf("connection timeout")).Times(3) // MaxRetries=2 means 3 total attempts
			},
			retryConfig: vfsevents.RetryConfig{
				Enabled:         true,
				MaxRetries:      2,
				InitialBackoff:  10 * time.Millisecond,
				MaxBackoff:      100 * time.Millisecond,
				BackoffFactor:   2.0,
				RetryableErrors: []string{"timeout", "connection"},
			},
			wantErr: true,
			expectedStatus: func(status *vfsevents.WatcherStatus) bool {
				return status.RetryAttempts == 3 && status.ConsecutiveErrors == 3
			},
		},
		{
			name: "Custom retryable error patterns",
			setupMocks: func(loc *mocks.Location) {
				// First call fails with custom retryable error
				loc.EXPECT().List().Return(nil, fmt.Errorf("S3 SlowDown")).Once()
				// Second call succeeds
				loc.EXPECT().List().Return([]string{}, nil).Once()
			},
			retryConfig: vfsevents.RetryConfig{
				Enabled:         true,
				MaxRetries:      2,
				InitialBackoff:  10 * time.Millisecond,
				MaxBackoff:      100 * time.Millisecond,
				BackoffFactor:   2.0,
				RetryableErrors: []string{"SlowDown", "ServiceUnavailable"},
			},
			wantErr: false,
			expectedStatus: func(status *vfsevents.WatcherStatus) bool {
				return status.RetryAttempts == 1 && status.ConsecutiveErrors == 0
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc := mocks.NewLocation(s.T())
			// Setup Exists() mock before creating poller
			loc.EXPECT().Exists().Return(true, nil)

			poller, _ := NewPoller(loc)
			tt.setupMocks(loc)

			config := &vfsevents.StartConfig{
				RetryConfig: tt.retryConfig,
			}
			status := &vfsevents.WatcherStatus{}

			start := time.Now()
			err := poller.poll(func(event vfsevents.Event) {}, config, status)
			elapsed := time.Since(start)

			if tt.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
			}

			if tt.expectedStatus != nil {
				s.True(tt.expectedStatus(status), "Status validation failed: RetryAttempts=%d, ConsecutiveErrors=%d",
					status.RetryAttempts, status.ConsecutiveErrors)
			}

			// Verify retry timing for retry scenarios
			if tt.retryConfig.Enabled && status.RetryAttempts > 0 {
				expectedMinTime := time.Duration(status.RetryAttempts) * tt.retryConfig.InitialBackoff / 2 // Account for jitter
				s.GreaterOrEqual(elapsed, expectedMinTime, "Retry backoff timing too short")
			}
		})
	}
}

func (s *PollerTestSuite) TestPerformCleanup() {
	tests := []struct {
		name           string
		cleanupAge     time.Duration
		initialCache   map[string]*FileInfo
		expectedRemain []string
		expectedRemove []string
	}{
		{
			name:       "Remove old entries",
			cleanupAge: 1 * time.Hour,
			initialCache: map[string]*FileInfo{
				"old1": {
					URI:       "old1",
					FirstSeen: time.Now().Add(-2 * time.Hour), // Older than cleanup age
				},
				"old2": {
					URI:       "old2",
					FirstSeen: time.Now().Add(-90 * time.Minute), // Older than cleanup age
				},
				"recent": {
					URI:       "recent",
					FirstSeen: time.Now().Add(-30 * time.Minute), // Newer than cleanup age
				},
			},
			expectedRemain: []string{"recent"},
			expectedRemove: []string{"old1", "old2"},
		},
		{
			name:       "No entries to remove",
			cleanupAge: 1 * time.Hour,
			initialCache: map[string]*FileInfo{
				"recent1": {
					URI:       "recent1",
					FirstSeen: time.Now().Add(-30 * time.Minute),
				},
				"recent2": {
					URI:       "recent2",
					FirstSeen: time.Now().Add(-45 * time.Minute),
				},
			},
			expectedRemain: []string{"recent1", "recent2"},
			expectedRemove: []string{},
		},
		{
			name:           "Empty cache",
			cleanupAge:     1 * time.Hour,
			initialCache:   map[string]*FileInfo{},
			expectedRemain: []string{},
			expectedRemove: []string{},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc := mocks.NewLocation(s.T())
			loc.EXPECT().Exists().Return(true, nil)
			poller, _ := NewPoller(loc, WithCleanupAge(tt.cleanupAge))

			// Set up initial cache
			poller.fileCache = tt.initialCache

			// Perform cleanup
			poller.performCleanup()

			// Verify remaining entries
			for _, uri := range tt.expectedRemain {
				s.Contains(poller.fileCache, uri, "Expected URI %s to remain in cache", uri)
			}

			// Verify removed entries
			for _, uri := range tt.expectedRemove {
				s.NotContains(poller.fileCache, uri, "Expected URI %s to be removed from cache", uri)
			}

			// Verify lastCleanup was updated
			s.WithinDuration(time.Now(), poller.lastCleanup, time.Second, "lastCleanup should be updated")
		})
	}
}

func (s *PollerTestSuite) TestEnforceMaxFiles() {
	tests := []struct {
		name         string
		maxFiles     int
		initialCache map[string]*FileInfo
		expectedSize int
	}{
		{
			name:     "Enforce limit - remove oldest",
			maxFiles: 2,
			initialCache: map[string]*FileInfo{
				"oldest": {
					URI:       "oldest",
					FirstSeen: time.Now().Add(-3 * time.Hour),
				},
				"middle": {
					URI:       "middle",
					FirstSeen: time.Now().Add(-2 * time.Hour),
				},
				"newest": {
					URI:       "newest",
					FirstSeen: time.Now().Add(-1 * time.Hour),
				},
			},
			expectedSize: 2,
		},
		{
			name:     "No enforcement needed",
			maxFiles: 5,
			initialCache: map[string]*FileInfo{
				"file1": {
					URI:       "file1",
					FirstSeen: time.Now().Add(-1 * time.Hour),
				},
				"file2": {
					URI:       "file2",
					FirstSeen: time.Now().Add(-2 * time.Hour),
				},
			},
			expectedSize: 2,
		},
		{
			name:         "Empty cache",
			maxFiles:     10,
			initialCache: map[string]*FileInfo{},
			expectedSize: 0,
		},
		{
			name:     "Remove multiple oldest files",
			maxFiles: 1,
			initialCache: map[string]*FileInfo{
				"file1": {
					URI:       "file1",
					FirstSeen: time.Now().Add(-4 * time.Hour),
				},
				"file2": {
					URI:       "file2",
					FirstSeen: time.Now().Add(-3 * time.Hour),
				},
				"file3": {
					URI:       "file3",
					FirstSeen: time.Now().Add(-2 * time.Hour),
				},
				"newest": {
					URI:       "newest",
					FirstSeen: time.Now().Add(-1 * time.Hour),
				},
			},
			expectedSize: 1,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc := mocks.NewLocation(s.T())
			loc.EXPECT().Exists().Return(true, nil)
			poller, _ := NewPoller(loc, WithMaxFiles(tt.maxFiles))

			// Set up initial cache
			poller.fileCache = tt.initialCache

			// Enforce max files
			poller.enforceMaxFiles()

			// Verify cache size
			s.Equal(tt.expectedSize, len(poller.fileCache), "Cache size should match expected")

			// If enforcement was needed, verify oldest files were removed
			if len(tt.initialCache) > tt.maxFiles && tt.maxFiles > 0 {
				// Find the newest files that should remain
				type fileEntry struct {
					uri       string
					firstSeen time.Time
				}
				var entries []fileEntry
				for uri, info := range tt.initialCache {
					entries = append(entries, fileEntry{uri: uri, firstSeen: info.FirstSeen})
				}

				// Sort by FirstSeen (oldest first)
				sort.Slice(entries, func(i, j int) bool {
					return entries[i].firstSeen.Before(entries[j].firstSeen)
				})

				// Verify the newest files remain
				newestEntries := entries[len(entries)-tt.maxFiles:]
				for _, entry := range newestEntries {
					s.Contains(poller.fileCache, entry.uri, "Expected newest file %s to remain", entry.uri)
				}

				// Verify oldest files were removed
				oldestEntries := entries[:len(entries)-tt.maxFiles]
				for _, entry := range oldestEntries {
					s.NotContains(poller.fileCache, entry.uri, "Expected oldest file %s to be removed", entry.uri)
				}
			}
		})
	}
}

func (s *PollerTestSuite) TestHandleExistingFile() {
	hourAgo := time.Now().UTC().Add(-time.Hour)
	now := time.Now().UTC()

	tests := []struct {
		name              string
		metadata          *fileMetadata
		cachedInfo        *FileInfo
		expectEvent       bool
		expectedType      vfsevents.EventType
		expectCacheUpdate bool
	}{
		{
			name: "File modified - time and size changed",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &now,
				size:         200,
			},
			cachedInfo: &FileInfo{
				URI:          "test://file1",
				LastModified: hourAgo,
				Size:         100,
			},
			expectEvent:       true,
			expectedType:      vfsevents.EventModified,
			expectCacheUpdate: true,
		},
		{
			name: "File modified - only time changed",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &now,
				size:         100,
			},
			cachedInfo: &FileInfo{
				URI:          "test://file1",
				LastModified: hourAgo,
				Size:         100,
			},
			expectEvent:       true,
			expectedType:      vfsevents.EventModified,
			expectCacheUpdate: true,
		},
		{
			name: "File modified - only size changed",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &hourAgo,
				size:         200,
			},
			cachedInfo: &FileInfo{
				URI:          "test://file1",
				LastModified: hourAgo,
				Size:         100,
			},
			expectEvent:       true,
			expectedType:      vfsevents.EventModified,
			expectCacheUpdate: true,
		},
		{
			name: "File not modified - same time and size",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &hourAgo,
				size:         100,
			},
			cachedInfo: &FileInfo{
				URI:          "test://file1",
				LastModified: hourAgo,
				Size:         100,
			},
			expectEvent:       false,
			expectCacheUpdate: false,
		},
		{
			name: "No lastModified in metadata",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: nil,
				size:         100,
			},
			cachedInfo: &FileInfo{
				URI:          "test://file1",
				LastModified: hourAgo,
				Size:         100,
			},
			expectEvent:       false,
			expectCacheUpdate: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc := mocks.NewLocation(s.T())
			loc.EXPECT().Exists().Return(true, nil)
			poller, _ := NewPoller(loc)

			var receivedEvent *vfsevents.Event
			handler := func(event vfsevents.Event) {
				receivedEvent = &event
			}

			// Store original cache values
			originalLastModified := tt.cachedInfo.LastModified
			originalSize := tt.cachedInfo.Size

			err := poller.handleExistingFile(tt.metadata, tt.cachedInfo, handler)
			s.NoError(err)

			if tt.expectEvent {
				s.NotNil(receivedEvent, "Expected event to be generated")
				s.Equal(tt.expectedType, receivedEvent.Type, "Event type should match")
				s.Equal(tt.metadata.uri, receivedEvent.URI, "Event URI should match")
			} else {
				s.Nil(receivedEvent, "Expected no event to be generated")
			}

			if tt.expectCacheUpdate {
				if tt.metadata.lastModified != nil {
					s.Equal(*tt.metadata.lastModified, tt.cachedInfo.LastModified, "Cache LastModified should be updated")
				}
				s.Equal(tt.metadata.size, tt.cachedInfo.Size, "Cache Size should be updated")
			} else {
				s.Equal(originalLastModified, tt.cachedInfo.LastModified, "Cache LastModified should not change")
				s.Equal(originalSize, tt.cachedInfo.Size, "Cache Size should not change")
			}
		})
	}
}

func (s *PollerTestSuite) TestIsFileModified() {
	hourAgo := time.Now().UTC().Add(-time.Hour)
	now := time.Now().UTC()

	tests := []struct {
		name       string
		metadata   *fileMetadata
		cachedInfo *FileInfo
		expected   bool
	}{
		{
			name: "Modified - time changed",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &now,
				size:         100,
			},
			cachedInfo: &FileInfo{
				LastModified: hourAgo,
				Size:         100,
			},
			expected: true,
		},
		{
			name: "Modified - size changed",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &hourAgo,
				size:         200,
			},
			cachedInfo: &FileInfo{
				LastModified: hourAgo,
				Size:         100,
			},
			expected: true,
		},
		{
			name: "Modified - both time and size changed",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &now,
				size:         200,
			},
			cachedInfo: &FileInfo{
				LastModified: hourAgo,
				Size:         100,
			},
			expected: true,
		},
		{
			name: "Not modified - same time and size",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &hourAgo,
				size:         100,
			},
			cachedInfo: &FileInfo{
				LastModified: hourAgo,
				Size:         100,
			},
			expected: false,
		},
		{
			name: "Not modified - no lastModified in metadata",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: nil,
				size:         100,
			},
			cachedInfo: &FileInfo{
				LastModified: hourAgo,
				Size:         100,
			},
			expected: false,
		},
		{
			name: "Not modified - exact same time (nanosecond precision)",
			metadata: &fileMetadata{
				uri:          "test://file1",
				lastModified: &hourAgo,
				size:         100,
			},
			cachedInfo: &FileInfo{
				LastModified: hourAgo,
				Size:         100,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc := mocks.NewLocation(s.T())
			loc.EXPECT().Exists().Return(true, nil)
			poller, _ := NewPoller(loc)

			result := poller.isFileModified(tt.metadata, tt.cachedInfo)
			s.Equal(tt.expected, result, "isFileModified result should match expected")
		})
	}
}

func TestPollerTestSuite(t *testing.T) {
	suite.Run(t, new(PollerTestSuite))
}

// Example demonstrates basic usage of VFS Poller for monitoring file changes
func Example() {
	// Create a VFS location - works with any VFS backend
	location, err := vfssimple.NewLocation("file:///tmp/watch/")
	if err != nil {
		log.Printf("Failed to create VFS location: %v", err)
		return
	}

	// Create a poller watcher with custom configuration
	watcher, err := NewPoller(location,
		WithInterval(10*time.Second), // Poll every 10 seconds
		WithMinAge(2*time.Second),    // Ignore files newer than 2 seconds
	)
	if err != nil {
		log.Printf("Failed to create poller: %v", err)
		return
	}

	// Define event handler
	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("File Event: %s | %s\n", event.Type.String(), event.URI)

		// Access the file through VFS if needed
		if event.Type == vfsevents.EventCreated {
			file, err := location.NewFile(event.URI)
			if err == nil {
				// Read file content or perform other operations
				if size, err := file.Size(); err == nil {
					fmt.Printf("File size: %d bytes\n", size)
				}
			}
		}
	}

	// Define error handler
	errorHandler := func(err error) {
		fmt.Printf("Watcher error: %v\n", err)
	}

	// Start watching with context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = watcher.Start(ctx, eventHandler, errorHandler)
	if err != nil {
		log.Printf("Failed to start watcher: %v", err)
		return
	}

	// Stop watching
	err = watcher.Stop(vfsevents.WithTimeout(10 * time.Second))
	if err != nil {
		log.Printf("Failed to stop watcher: %v", err)
		return
	}
}

// ExampleNewPoller_withRetryLogic demonstrates VFS Poller with retry configuration
func ExampleNewPoller_withRetryLogic() {
	location, err := vfssimple.NewLocation("s3://my-bucket/path/")
	if err != nil {
		log.Printf("Failed to create VFS location: %v", err)
		return
	}

	// Create poller with retry logic for transient failures
	watcher, err := NewPoller(location,
		WithInterval(30*time.Second),
		WithMaxFiles(1000),           // Limit memory usage
		WithCleanupAge(24*time.Hour), // Clean up old entries
	)
	if err != nil {
		log.Printf("Failed to create poller: %v", err)
		return
	}

	eventHandler := func(event vfsevents.Event) {
		fmt.Printf("Event: %s on %s\n", event.Type.String(), event.URI)
	}

	errorHandler := func(err error) {
		fmt.Printf("Error: %v\n", err)
	}

	ctx := context.Background()

	// Start with retry configuration
	err = watcher.Start(ctx, eventHandler, errorHandler,
		vfsevents.WithRetryConfig(vfsevents.RetryConfig{
			Enabled:        true,
			MaxRetries:     3,
			InitialBackoff: time.Second,
			MaxBackoff:     30 * time.Second,
			BackoffFactor:  2.0,
		}),
		vfsevents.WithEventFilter(func(e vfsevents.Event) bool {
			// Only process .txt files
			return strings.HasSuffix(e.URI, ".txt")
		}),
	)
	if err != nil {
		log.Printf("Failed to start watcher: %v", err)
		return
	}

	// Stop with timeout
	err = watcher.Stop(vfsevents.WithTimeout(10 * time.Second))
	if err != nil {
		log.Printf("Failed to stop watcher: %v", err)
		return
	}
}
