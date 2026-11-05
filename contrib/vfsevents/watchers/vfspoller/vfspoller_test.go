package vfspoller

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"iter"
	"log"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/vfsevents"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/options"
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
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

func (s *PollerTestSuite) TestWithOptions() {
	location := mocks.NewLocation(s.T())
	location.EXPECT().Exists().Return(true, nil)

	// Test WithInterval
	poller, err := NewPoller(location, WithInterval(30*time.Second))
	s.Require().NoError(err)
	s.Equal(30*time.Second, poller.interval)

	// Test WithMinAge
	poller, err = NewPoller(location, WithMinAge(5*time.Minute))
	s.Require().NoError(err)
	s.Equal(5*time.Minute, poller.minAge)

	// Test WithMaxFiles
	poller, err = NewPoller(location, WithMaxFiles(5000))
	s.Require().NoError(err)
	s.Equal(5000, poller.maxFiles)

	// Test WithCleanupAge
	poller, err = NewPoller(location, WithCleanupAge(12*time.Hour))
	s.Require().NoError(err)
	s.Equal(12*time.Hour, poller.cleanupAge)

	// Test multiple options
	poller, err = NewPoller(location,
		WithInterval(45*time.Second),
		WithMinAge(2*time.Minute),
		WithMaxFiles(1000),
		WithCleanupAge(6*time.Hour))
	s.Require().NoError(err)
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
			errFunc := func(err error) {
				s.Require().NoError(err)
			}
			err := poller.Start(s.T().Context(), tt.handler, errFunc)
			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
			s.Require().NoError(poller.Stop())
		})
	}
}

func (s *PollerTestSuite) TestStop() {
	handler := func(event vfsevents.Event) {}
	location := mocks.NewLocation(s.T())
	location.EXPECT().Exists().Return(true, nil)
	poller, _ := NewPoller(location)
	errFunc := func(err error) {
		s.Require().NoError(err)
	}
	err := poller.Start(s.T().Context(), handler, errFunc)
	s.Require().NoError(err)
	s.Require().NoError(poller.Stop())
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
				loc.EXPECT().List().Return(nil, errors.New("list error"))
			},
			wantErr: true,
		},
		{
			name: "NewFile error",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return([]string{"file1"}, nil)
				loc.EXPECT().NewFile("file1").Return(nil, errors.New("new file error"))
			},
			wantErr: true,
		},
		{
			name: "File too new (minAge > 0)",
			setupMocks: func(loc *mocks.Location) {
				loc.EXPECT().List().Return([]string{"file1"}, nil)
				file := mocks.NewFile(s.T())
				file.EXPECT().URI().Return("scheme:///file1")
				file.EXPECT().LastModified().Return(new(time.Now().UTC()), nil)
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
			loc.EXPECT().Exists().Return(true, nil).Once()
			loc.EXPECT().Path().Return("/dir/").Maybe()
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

			err := poller.poll(s.T().Context(), func(event vfsevents.Event) {}, config, status)
			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
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
				loc.EXPECT().List().Return(nil, errors.New("connection timeout")).Once()
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
				loc.EXPECT().List().Return(nil, errors.New("connection timeout")).Once()
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
				loc.EXPECT().List().Return(nil, errors.New("permission denied")).Once()
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
				loc.EXPECT().List().Return(nil, errors.New("connection timeout")).Times(3) // MaxRetries=2 means 3 total attempts
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
				loc.EXPECT().List().Return(nil, errors.New("S3 SlowDown")).Once()
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
			loc.EXPECT().Exists().Return(true, nil).Once()
			loc.EXPECT().Path().Return("/dir/").Maybe()

			poller, _ := NewPoller(loc)
			tt.setupMocks(loc)

			config := &vfsevents.StartConfig{
				RetryConfig: tt.retryConfig,
			}
			status := &vfsevents.WatcherStatus{}

			start := time.Now()
			err := poller.poll(s.T().Context(), func(event vfsevents.Event) {}, config, status)
			elapsed := time.Since(start)

			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
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
				s.Containsf(poller.fileCache, uri, "Expected URI %s to remain in cache", uri)
			}

			// Verify removed entries
			for _, uri := range tt.expectedRemove {
				s.NotContainsf(poller.fileCache, uri, "Expected URI %s to be removed from cache", uri)
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
			s.Len(poller.fileCache, tt.expectedSize, "Cache size should match expected")

			// If enforcement was needed, verify oldest files were removed
			if len(tt.initialCache) > tt.maxFiles && tt.maxFiles > 0 {
				// Find the newest files that should remain
				type fileEntry struct {
					uri       string
					firstSeen time.Time
				}
				entries := make([]fileEntry, 0, len(tt.initialCache))
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
					s.Containsf(poller.fileCache, entry.uri, "Expected newest file %s to remain", entry.uri)
				}

				// Verify oldest files were removed
				oldestEntries := entries[:len(entries)-tt.maxFiles]
				for _, entry := range oldestEntries {
					s.NotContainsf(poller.fileCache, entry.uri, "Expected oldest file %s to be removed", entry.uri)
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
			s.Require().NoError(err)

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

// nativeLocation is a Location whose listing is native, so list.Entries uses ListEntries instead of List.
type nativeLocation struct {
	*mocks.Location
	*mocks.EntryLister
}

// newNativeLocation returns a poller over a native listing. The listed file mocks have no LastModified or Size
// expectations, so any per-file metadata request fails the test.
func (s *PollerTestSuite) newNativeLocation(opts ...Option) (*Poller, *mocks.EntryLister, *mocks.Location) {
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().Exists().Return(true, nil)
	loc.EXPECT().Path().Return("/dir/").Maybe()
	lister := mocks.NewEntryLister(s.T())

	poller, err := NewPoller(nativeLocation{loc, lister}, opts...)
	s.Require().NoError(err)

	loc.EXPECT().NewFile(mock.Anything).RunAndReturn(func(name string, _ ...options.NewFileOption) (vfs.File, error) {
		f := mocks.NewFile(s.T())
		f.EXPECT().URI().Return("scheme:///dir/" + name)
		return f, nil
	}).Maybe()
	return poller, lister, loc
}

func (s *PollerTestSuite) listed(loc vfs.Location, name string, size uint64, modified time.Time) vfs.Entry {
	e, err := vfs.NewFileEntry(loc, name, vfs.WithEntryMetadata(size, modified))
	s.Require().NoError(err)
	return e
}

func entrySeq(entries ...vfs.Entry) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		for _, e := range entries {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func (s *PollerTestSuite) TestNativeListingNeedsNoPerFileRequests() {
	poller, lister, loc := s.newNativeLocation()
	hourAgo := time.Now().UTC().Add(-time.Hour)
	polls := [][]vfs.Entry{
		{s.listed(loc, "a.txt", 1, hourAgo), s.listed(loc, "b.txt", 2, hourAgo)},
		{s.listed(loc, "a.txt", 1, hourAgo), s.listed(loc, "b.txt", 2, hourAgo.Add(time.Minute))},
		{s.listed(loc, "b.txt", 2, hourAgo.Add(time.Minute))},
	}
	for _, entries := range polls {
		lister.EXPECT().ListEntries(mock.Anything, mock.Anything).Return(entrySeq(entries...)).Once()
	}

	var events []string
	handler := func(e vfsevents.Event) { events = append(events, fmt.Sprintf("%s %s", e.Type, e.URI)) }
	status := &vfsevents.WatcherStatus{}
	for range polls {
		s.Require().NoError(poller.poll(s.T().Context(), handler, &vfsevents.StartConfig{}, status))
	}

	s.Equal([]string{
		"Created scheme:///dir/a.txt", "Created scheme:///dir/b.txt",
		"Modified scheme:///dir/b.txt",
		"Deleted scheme:///dir/a.txt",
	}, events)
}

func (s *PollerTestSuite) TestNativeListingSizeChangeIsAModification() {
	poller, lister, loc := s.newNativeLocation()
	modified := time.Now().UTC().Add(-time.Hour)
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).Return(entrySeq(s.listed(loc, "a.txt", 1, modified))).Once()
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).Return(entrySeq(s.listed(loc, "a.txt", 5, modified))).Once()

	var types []vfsevents.EventType
	handler := func(e vfsevents.Event) { types = append(types, e.Type) }
	for range 2 {
		s.Require().NoError(poller.poll(s.T().Context(), handler, &vfsevents.StartConfig{}, &vfsevents.WatcherStatus{}))
	}

	s.Equal([]vfsevents.EventType{vfsevents.EventCreated, vfsevents.EventModified}, types)
}

func (s *PollerTestSuite) TestNativeListingHonorsMinAge() {
	poller, lister, loc := s.newNativeLocation(WithMinAge(time.Hour))
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).Return(entrySeq(
		s.listed(loc, "fresh.txt", 1, time.Now().UTC()),
		s.listed(loc, "old.txt", 1, time.Now().UTC().Add(-2*time.Hour)),
	)).Once()

	var uris []string
	handler := func(e vfsevents.Event) { uris = append(uris, e.URI) }
	s.Require().NoError(poller.poll(s.T().Context(), handler, &vfsevents.StartConfig{}, &vfsevents.WatcherStatus{}))

	s.Equal([]string{"scheme:///dir/old.txt"}, uris)
}

func (s *PollerTestSuite) TestListsFilesOnly() {
	poller, lister, _ := s.newNativeLocation()
	lister.EXPECT().ListEntries(mock.Anything, mock.MatchedBy(func(opts []options.ListOption) bool {
		q, err := list.ParseQuery(opts)
		return err == nil && q.Kind == list.KindFiles && !q.Recursive
	})).Return(entrySeq()).Once()

	s.Require().NoError(poller.poll(s.T().Context(), func(vfsevents.Event) {}, &vfsevents.StartConfig{}, &vfsevents.WatcherStatus{}))
}

func (s *PollerTestSuite) TestEmptyListingOfARemovedLocationIsAnError() {
	tests := []struct {
		name        string
		exists      bool
		existsErr   error
		wantErr     string
		wantDeleted bool
	}{
		{name: "Location was removed", exists: false, wantErr: "location no longer exists"},
		{name: "Location cannot be checked", existsErr: errors.New("boom"), wantErr: "failed to check if location exists"},
		{name: "Files were deleted from a location that remains", exists: true, wantDeleted: true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc := mocks.NewLocation(s.T())
			loc.EXPECT().Exists().Return(true, nil).Once()
			loc.EXPECT().Path().Return("/dir/").Maybe()
			loc.EXPECT().URI().Return("scheme:///dir/").Maybe()
			poller, err := NewPoller(loc)
			s.Require().NoError(err)
			poller.fileCache["scheme:///dir/a.txt"] = &FileInfo{URI: "scheme:///dir/a.txt"}

			loc.EXPECT().List().Return([]string{}, nil).Once()
			loc.EXPECT().Exists().Return(tt.exists, tt.existsErr).Once()

			var deleted int
			err = poller.poll(s.T().Context(), func(e vfsevents.Event) {
				if e.Type == vfsevents.EventDeleted {
					deleted++
				}
			}, &vfsevents.StartConfig{}, &vfsevents.WatcherStatus{})

			if tt.wantErr != "" {
				s.Require().ErrorContains(err, tt.wantErr)
				s.Zero(deleted, "no file is reported deleted when the location is gone")
				return
			}
			s.Require().NoError(err)
			s.Equal(1, deleted)
		})
	}
}

func (s *PollerTestSuite) TestCanceledContextStopsTheListing() {
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().Exists().Return(true, nil).Once()
	loc.EXPECT().Path().Return("/dir/").Maybe()
	poller, err := NewPoller(loc)
	s.Require().NoError(err)

	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	err = poller.poll(ctx, func(vfsevents.Event) {}, &vfsevents.StartConfig{}, &vfsevents.WatcherStatus{})
	s.Require().ErrorIs(err, context.Canceled)
}

func (s *PollerTestSuite) TestNameThatCannotBeAFileIsSkipped() {
	poller, lister, loc := s.newNativeLocation()
	bad := &iofs.PathError{Op: "list", Path: "a/b", Err: iofs.ErrInvalid}
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).Return(
		func(yield func(vfs.Entry, error) bool) {
			if !yield(vfs.Entry{}, bad) {
				return
			}
			yield(s.listed(loc, "ok.txt", 1, time.Now().UTC().Add(-time.Hour)), nil)
		}).Once()

	var uris []string
	handler := func(e vfsevents.Event) { uris = append(uris, e.URI) }
	s.Require().NoError(poller.poll(s.T().Context(), handler, &vfsevents.StartConfig{}, &vfsevents.WatcherStatus{}))

	s.Equal([]string{"scheme:///dir/ok.txt"}, uris)
}

func (s *PollerTestSuite) TestStoppingDuringAListingIsNotReportedAsAnError() {
	poller, lister, _ := s.newNativeLocation(WithInterval(time.Millisecond))
	listing := make(chan struct{})
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, _ ...options.ListOption) iter.Seq2[vfs.Entry, error] {
			return func(yield func(vfs.Entry, error) bool) {
				close(listing)
				<-ctx.Done()
				yield(vfs.Entry{}, ctx.Err())
			}
		}).Once()

	errs := make(chan error, 1)
	s.Require().NoError(poller.Start(s.T().Context(), func(vfsevents.Event) {}, func(err error) { errs <- err }))
	<-listing
	s.Require().NoError(poller.Stop())

	select {
	case err := <-errs:
		s.Fail("a canceled listing was reported", "%v", err)
	default:
	}
}

func (s *PollerTestSuite) TestSymlinkMetadataComesFromItsTarget() {
	loc := mocks.NewLocation(s.T())
	loc.EXPECT().Exists().Return(true, nil)
	loc.EXPECT().Path().Return("/dir/").Maybe()
	lister := mocks.NewEntryLister(s.T())
	poller, err := NewPoller(nativeLocation{loc, lister}, WithMinAge(time.Hour))
	s.Require().NoError(err)
	now := time.Now().UTC()
	targetOld := now.Add(-2 * time.Hour)

	link := func(modified time.Time) vfs.Entry {
		e, err := vfs.NewFileEntry(loc, "link", vfs.WithEntryMetadata(1, modified), vfs.WithEntryMode(iofs.ModeSymlink))
		s.Require().NoError(err)
		return e
	}
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).Return(entrySeq(link(now))).Once()
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).Return(entrySeq(link(now))).Once()

	// the link itself is new, but its target is old enough, then the target is edited
	targets := []time.Time{targetOld, now.Add(-90 * time.Minute)}
	sizes := []uint64{10, 20}
	var call int
	loc.EXPECT().NewFile("link").RunAndReturn(func(string, ...options.NewFileOption) (vfs.File, error) {
		f := mocks.NewFile(s.T())
		f.EXPECT().URI().Return("scheme:///dir/link")
		f.EXPECT().LastModified().Return(&targets[call], nil)
		f.EXPECT().Size().Return(sizes[call], nil)
		call++
		return f, nil
	}).Times(2)

	var types []vfsevents.EventType
	handler := func(e vfsevents.Event) { types = append(types, e.Type) }
	for range 2 {
		s.Require().NoError(poller.poll(s.T().Context(), handler, &vfsevents.StartConfig{}, &vfsevents.WatcherStatus{}))
	}

	s.Equal([]vfsevents.EventType{vfsevents.EventCreated, vfsevents.EventModified}, types)
}

func (s *PollerTestSuite) TestExpiredContextIsNotRetried() {
	poller, lister, _ := s.newNativeLocation()
	ctx, cancel := context.WithCancel(s.T().Context())
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, ...options.ListOption) iter.Seq2[vfs.Entry, error] {
			return func(yield func(vfs.Entry, error) bool) {
				cancel()
				yield(vfs.Entry{}, context.DeadlineExceeded)
			}
		}).Once() // a retry would call ListEntries a second time and fail the test

	config := &vfsevents.StartConfig{RetryConfig: vfsevents.RetryConfig{
		Enabled: true, MaxRetries: 3, InitialBackoff: time.Hour, MaxBackoff: time.Hour, BackoffFactor: 2,
	}}
	start := time.Now()
	err := poller.poll(ctx, func(vfsevents.Event) {}, config, &vfsevents.WatcherStatus{})

	s.Require().ErrorIs(err, context.Canceled)
	s.Less(time.Since(start), time.Minute, "no backoff is waited out")
}

func (s *PollerTestSuite) TestRetryBackoffEndsWhenTheContextDoes() {
	poller, lister, _ := s.newNativeLocation()
	ctx, cancel := context.WithCancel(s.T().Context())
	lister.EXPECT().ListEntries(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, ...options.ListOption) iter.Seq2[vfs.Entry, error] {
			return func(yield func(vfs.Entry, error) bool) {
				time.AfterFunc(50*time.Millisecond, cancel) // while the poller waits to retry
				yield(vfs.Entry{}, errors.New("connection timeout"))
			}
		}).Once()

	config := &vfsevents.StartConfig{RetryConfig: vfsevents.RetryConfig{
		Enabled: true, MaxRetries: 3, InitialBackoff: time.Hour, MaxBackoff: time.Hour, BackoffFactor: 2,
		RetryableErrors: []string{"timeout"},
	}}
	start := time.Now()
	err := poller.poll(ctx, func(vfsevents.Event) {}, config, &vfsevents.WatcherStatus{})

	s.Require().ErrorIs(err, context.Canceled)
	s.Less(time.Since(start), time.Minute)
}
