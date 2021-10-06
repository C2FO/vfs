package azure

import (
	"io"
	"net/http"

	"github.com/Azure/azure-storage-blob-go/azblob"

	"github.com/c2fo/vfs/v6"
)

// MockAzureClient is a mock implementation of azure.Client.
type MockAzureClient struct {
	PropertiesError  error
	PropertiesResult *BlobProperties
	ExpectedError    error
	ExpectedResult   interface{}
}

// Properties returns a PropertiesResult if it exists, otherwise it will return the value of PropertiesError
func (a *MockAzureClient) Properties(locationURI, filePath string) (*BlobProperties, error) {
	if a.PropertiesResult == nil {
		return nil, a.PropertiesError
	}
	return a.PropertiesResult, a.PropertiesError
}

// SetMetadata returns the value of ExpectedError
func (a *MockAzureClient) SetMetadata(file vfs.File, metadata map[string]string) error {
	return a.ExpectedError
}

// Upload returns the value of ExpectedError
func (a *MockAzureClient) Upload(file vfs.File, content io.ReadSeeker) error {
	return a.ExpectedError
}

// Download returns ExpectedResult if it exists, otherwise it returns ExpectedError
func (a *MockAzureClient) Download(file vfs.File) (io.ReadCloser, error) {
	if a.ExpectedResult != nil {
		return a.ExpectedResult.(io.ReadCloser), nil
	}
	return nil, a.ExpectedError
}

// Copy returns the value of ExpectedError
func (a *MockAzureClient) Copy(srcFile, tgtFile vfs.File) error {
	return a.ExpectedError
}

// List returns the value of ExpectedResult if it exists, otherwise it returns ExpectedError.
func (a *MockAzureClient) List(l vfs.Location) ([]string, error) {
	if a.ExpectedResult != nil {
		return a.ExpectedResult.([]string), nil
	}
	return nil, a.ExpectedError
}

// Delete returns the value of ExpectedError
func (a *MockAzureClient) Delete(file vfs.File) error {
	return a.ExpectedError
}

// MockStorageError is a mock for the azblob.StorageError interface
type MockStorageError struct {
	azblob.ResponseError
}

// ServiceCode always returns "BlobNotFound" to simulate the not found condition
func (mse MockStorageError) ServiceCode() azblob.ServiceCodeType {
	return "BlobNotFound"
}

// Response returns nil
func (mse MockStorageError) Response() *http.Response {
	return nil
}

// Timeout returns nil
func (mse MockStorageError) Timeout() bool {
	return false
}

// Temporary returns nil
func (mse MockStorageError) Temporary() bool {
	return false
}

// Error returns empty string
func (mse MockStorageError) Error() string {
	return ""
}
