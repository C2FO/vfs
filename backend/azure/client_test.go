package azure

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/backend/azure/types"
)

func TestNewClient(t *testing.T) {
	options := &Options{
		AccountName: "testaccount",
		AccountKey:  "dGVzdGtleQ==", // "testkey" base64 encoded
	}

	client, err := NewClient(options)
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, "https://testaccount.blob.core.windows.net", client.serviceURL.String())
	assert.NotNil(t, client.credential)
}

func TestDefaultClient_Properties(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "11")
			w.Header().Set("Last-Modified", time.Now().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer mockServer.Close()

	// Configure the DefaultClient to use the mock server
	client := &DefaultClient{
		serviceURL: mustParseURL(mockServer.URL),
		credential: nil, // No credential needed for mock server
	}

	// Test the Properties method
	props, err := client.Properties(mockServer.URL, "test.txt")
	require.NoError(t, err)
	require.NotNil(t, props)
	require.NotNil(t, props.Size)
	assert.Equal(t, int64(11), *props.Size)
	assert.NotNil(t, props.LastModified)
}

func TestDefaultClient_Upload(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer mockServer.Close()

	// Configure the DefaultClient to use the mock server
	client := &DefaultClient{
		serviceURL: mustParseURL(mockServer.URL),
		credential: nil, // No credential needed for mock server
	}

	// Create a mock file
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/test.txt")
	require.NoError(t, err)

	// Test the Upload method
	err = client.Upload(f, strings.NewReader("Hello world!"), "text/plain")
	require.NoError(t, err)
}

func TestDefaultClient_Download(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Hello world!"))
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer mockServer.Close()

	// Configure the DefaultClient to use the mock server
	client := &DefaultClient{
		serviceURL: mustParseURL(mockServer.URL),
		credential: nil, // No credential needed for mock server
	}

	// Create a mock file
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/test.txt")
	require.NoError(t, err)

	// Test the Download method
	reader, err := client.Download(f)
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "Hello world!", string(content))
}

func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}

func TestDefaultClient_SetMetadata(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer mockServer.Close()

	// Configure the DefaultClient to use the mock server
	client := &DefaultClient{
		serviceURL: mustParseURL(mockServer.URL),
		credential: nil, // No credential needed for mock server
	}

	// Create a mock file
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/test.txt")
	require.NoError(t, err)

	// Test the SetMetadata method
	metadata := map[string]*string{"key": new("value")}
	err = client.SetMetadata(f, metadata)
	require.NoError(t, err)
}

func TestDefaultClient_Copy(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.Header().Set("x-ms-copy-id", "some-copy-id")
			w.Header().Set("x-ms-copy-status", string(blob.CopyStatusTypeSuccess))
			w.WriteHeader(http.StatusAccepted)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer mockServer.Close()

	// Configure the DefaultClient to use the mock server
	client := &DefaultClient{
		serviceURL: mustParseURL(mockServer.URL),
		credential: nil, // No credential needed for mock server
	}

	// Create mock files
	fs := NewFileSystem()
	srcFile, err := fs.NewFile("test-container", "/src.txt")
	require.NoError(t, err)
	tgtFile, err := fs.NewFile("test-container", "/tgt.txt")
	require.NoError(t, err)

	// Test the Copy method
	err = client.Copy(srcFile, tgtFile)
	require.NoError(t, err)
}

func TestDefaultClient_List(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
			<EnumerationResults>
				<ServiceEndpoint>https://mockserver</ServiceEndpoint>
				<ContainerName>test-container</ContainerName>
				<Prefix></Prefix>
				<Marker></Marker>
				<MaxResults>5000</MaxResults>
				<Delimiter>/</Delimiter>
				<Blobs>
					<Blob>
						<Name>file1.txt</Name>
					</Blob>
					<Blob>
						<Name>file2.txt</Name>
					</Blob>
				</Blobs>
				<NextMarker></NextMarker>
			</EnumerationResults>`))
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer mockServer.Close()

	// Configure the DefaultClient to use the mock server
	client := &DefaultClient{
		serviceURL: mustParseURL(mockServer.URL),
		credential: nil, // No credential needed for mock server
	}

	// Create a mock location
	fs := NewFileSystem()
	l, err := fs.NewLocation("test-container", "/")
	require.NoError(t, err)

	// Test the List method
	list, err := client.List(l)
	require.NoError(t, err)
	assert.Equal(t, []string{"file1.txt", "file2.txt"}, list)
}

func TestDefaultClient_Delete(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusAccepted)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer mockServer.Close()

	// Configure the DefaultClient to use the mock server
	client := &DefaultClient{
		serviceURL: mustParseURL(mockServer.URL),
		credential: nil, // No credential needed for mock server
	}

	// Create a mock file
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/test.txt")
	require.NoError(t, err)

	// Test the Delete method
	err = client.Delete(f)
	require.NoError(t, err)
}

func TestDefaultClient_DeleteAllVersions(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusAccepted)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
        <EnumerationResults>
            <Blobs>
                <Blob>
                    <VersionId>1</VersionId>
                </Blob>
                <Blob>
                    <VersionId>2</VersionId>
                </Blob>
            </Blobs>
        </EnumerationResults>`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer mockServer.Close()

	// Configure the DefaultClient to use the mock server
	client := &DefaultClient{
		serviceURL: mustParseURL(mockServer.URL),
		credential: nil, // No credential needed for mock server
	}

	// Create a mock file
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/test.txt")
	require.NoError(t, err)

	// Test the DeleteAllVersions method
	err = client.DeleteAllVersions(f)
	require.NoError(t, err)
}

func TestDefaultClient_UploadStream_SinglePut(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		contentType string
	}{
		{name: "small payload with content type", payload: "Hello stream!", contentType: "text/plain"},
		{name: "empty payload without content type", payload: "", contentType: ""},
		{
			name:        "2 MiB payload stays a single Put Blob under default block size",
			payload:     strings.Repeat("m", 2*1024*1024),
			contentType: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody []byte
			var gotContentType string
			var putCalls int

			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut && r.URL.Query().Get("comp") == "" {
					var err error
					gotBody, err = io.ReadAll(r.Body)
					assert.NoError(t, err)
					gotContentType = r.Header.Get("x-ms-blob-content-type")
					putCalls++
					w.WriteHeader(http.StatusCreated)
					return
				}
				w.WriteHeader(http.StatusMethodNotAllowed)
			}))
			defer mockServer.Close()

			client := &DefaultClient{serviceURL: mustParseURL(mockServer.URL)}
			var streamer types.BlobStreamer = client

			fs := NewFileSystem()
			f, err := fs.NewFile("test-container", "/stream.txt")
			require.NoError(t, err)

			err = streamer.UploadStream(t.Context(), f, strings.NewReader(tt.payload), tt.contentType)
			require.NoError(t, err)
			assert.Equal(t, 1, putCalls)
			assert.Equal(t, tt.payload, string(gotBody))
			assert.Equal(t, tt.contentType, gotContentType)
		})
	}
}

func TestDefaultClient_UploadStream_MultiBlockAndAbort(t *testing.T) {
	type blockListXML struct {
		Latest []string `xml:"Latest"`
	}

	t.Run("multi-block payload stages blocks and commits block list", func(t *testing.T) {
		var mu sync.Mutex
		staged := make(map[string][]byte)
		var committed []byte
		var committedContentType string

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			switch r.URL.Query().Get("comp") {
			case "block":
				b, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				mu.Lock()
				staged[r.URL.Query().Get("blockid")] = b
				mu.Unlock()
				w.WriteHeader(http.StatusCreated)
			case "blocklist":
				raw, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				var bl blockListXML
				assert.NoError(t, xml.Unmarshal(raw, &bl))
				mu.Lock()
				var buf bytes.Buffer
				for _, id := range bl.Latest {
					buf.Write(staged[id])
				}
				committed = buf.Bytes()
				committedContentType = r.Header.Get("x-ms-blob-content-type")
				mu.Unlock()
				w.WriteHeader(http.StatusCreated)
			default:
				w.WriteHeader(http.StatusBadRequest)
			}
		}))
		defer mockServer.Close()

		client := &DefaultClient{serviceURL: mustParseURL(mockServer.URL), uploadStreamBlockSize: 1024 * 1024}
		fs := NewFileSystem()
		f, err := fs.NewFile("test-container", "/large.bin")
		require.NoError(t, err)

		// 1.5 MiB forces UploadStream (1 MiB block size) to use StageBlock + CommitBlockList.
		payload := bytes.Repeat([]byte("abcd"), (3*1024*1024)/8)
		err = client.UploadStream(t.Context(), f, bytes.NewReader(payload), "application/octet-stream")
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()
		assert.GreaterOrEqual(t, len(staged), 2)
		assert.Equal(t, payload, committed)
		assert.Equal(t, "application/octet-stream", committedContentType)
	})

	t.Run("abandoned multi-block stream never commits", func(t *testing.T) {
		var mu sync.Mutex
		var stageCalls, commitCalls, putCalls int

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			mu.Lock()
			switch r.URL.Query().Get("comp") {
			case "block":
				stageCalls++
			case "blocklist":
				commitCalls++
			default:
				putCalls++
			}
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		}))
		defer mockServer.Close()

		client := &DefaultClient{serviceURL: mustParseURL(mockServer.URL), uploadStreamBlockSize: 1024 * 1024}
		fs := NewFileSystem()
		f, err := fs.NewFile("test-container", "/aborted.bin")
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		pr, pw := io.Pipe()
		done := make(chan error, 1)
		go func() {
			done <- client.UploadStream(ctx, f, pr, "application/octet-stream")
		}()

		// Write > 1 MiB so at least one block is staged, then cancel and close with error.
		_, err = pw.Write(bytes.Repeat([]byte("x"), (1024+256)*1024))
		require.NoError(t, err)
		cancel()
		boom := errors.New("stream aborted")
		_ = pw.CloseWithError(boom)

		err = <-done
		require.Error(t, err)

		mu.Lock()
		defer mu.Unlock()
		assert.GreaterOrEqual(t, stageCalls, 1)
		assert.Zero(t, commitCalls, "CommitBlockList must never be called when UploadStream is abandoned")
		assert.Zero(t, putCalls, "Put Blob must never be called when UploadStream is abandoned")
	})
}

func TestDefaultClient_DownloadRange(t *testing.T) {
	const fullBody = "Hello world!"

	tests := []struct {
		name           string
		offset         int64
		expectedRange  string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "zero offset reads entire blob",
			offset:         0,
			expectedRange:  "",
			expectedStatus: http.StatusOK,
			expectedBody:   fullBody,
		},
		{
			name:           "positive offset sends byte range",
			offset:         6,
			expectedRange:  "bytes=6-",
			expectedStatus: http.StatusPartialContent,
			expectedBody:   "world!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRange string
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				gotRange = r.Header.Get("x-ms-range")
				if gotRange == "" {
					gotRange = r.Header.Get("Range")
				}
				w.WriteHeader(tt.expectedStatus)
				_, _ = w.Write([]byte(tt.expectedBody))
			}))
			defer mockServer.Close()

			client := &DefaultClient{serviceURL: mustParseURL(mockServer.URL)}
			fs := NewFileSystem()
			f, err := fs.NewFile("test-container", "/test.txt")
			require.NoError(t, err)

			rc, err := client.DownloadRange(t.Context(), f, tt.offset)
			require.NoError(t, err)
			defer func() { _ = rc.Close() }()

			got, err := io.ReadAll(rc)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedRange, gotRange)
			assert.Equal(t, tt.expectedBody, string(got))
		})
	}
}
