package azure

import (
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"

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
func (a *MockAzureClient) SetMetadata(vfs.File, map[string]*string) error {
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

// DeleteAllVersions returns the value of ExpectedError
func (a *MockAzureClient) DeleteAllVersions(file vfs.File) error {
	return a.ExpectedError
}

var blobNotFoundErr = &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}
