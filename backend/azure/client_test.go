package azure

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/v7/utils"
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
	props, err := client.Properties(t.Context(), mockServer.URL, "test.txt")
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
	err = client.Upload(t.Context(), f, strings.NewReader("Hello world!"), "text/plain")
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
	reader, err := client.Download(t.Context(), f)
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
	metadata := map[string]*string{"key": utils.Ptr("value")}
	err = client.SetMetadata(t.Context(), f, metadata)
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
	err = client.Copy(t.Context(), srcFile, tgtFile)
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
	list, err := client.List(t.Context(), l)
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
	err = client.Delete(t.Context(), f)
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
	err = client.DeleteAllVersions(t.Context(), f)
	require.NoError(t, err)
}
