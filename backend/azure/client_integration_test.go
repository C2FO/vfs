//go:build vfsintegration
// +build vfsintegration

package azure

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/stretchr/testify/suite"
)

type ClientIntegrationTestSuite struct {
	suite.Suite
	testContainerURL *container.Client
	accountName      string
	accountKey       string
}

func (s *ClientIntegrationTestSuite) SetupSuite() {
	s.accountName, s.accountKey = os.Getenv("VFS_AZURE_STORAGE_ACCOUNT"), os.Getenv("VFS_AZURE_STORAGE_ACCESS_KEY")

	credential, err := azblob.NewSharedKeyCredential(s.accountName, s.accountKey)
	if err != nil {
		panic(err)
	}

	cli, err := container.NewClientWithSharedKeyCredential(fmt.Sprintf("https://%s.blob.core.windows.net", s.accountName), credential, nil)
	s.NoError(err)
	s.testContainerURL = cli

	_, err = s.testContainerURL.Create(context.Background(), nil)
	s.NoError(err)

	// The create function claims to be synchronous but for some reason it does not exist for a little bit so
	// we need to wait for it to be there.
	_, err = s.testContainerURL.GetProperties(context.Background(), nil)
	for {
		time.Sleep(2 * time.Second)
		if err == nil || !bloberror.HasCode(err, bloberror.BlobNotFound) {
			break
		}
		_, err = s.testContainerURL.GetProperties(context.Background(), nil)
	}
}

func (s *ClientIntegrationTestSuite) TearDownSuite() {
	_, err := s.testContainerURL.Delete(context.Background(), nil)
	s.NoError(err)
}

func (s *ClientIntegrationTestSuite) TestAllTheThings_FileWithNoPath() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/test.txt")
	s.NoError(err)
	client, err := fs.Client()
	s.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	// Create the new file
	err = client.Upload(f, strings.NewReader("Hello world!"))
	s.NoError(err, "The file should be successfully uploaded to azure")

	// make sure it exists
	_, err = client.Properties(f.Location().URI(), f.Name())
	s.NoError(err, "If the file exists no error should be returned")

	// download it
	reader, err := client.Download(f)
	s.NoError(err)
	dlContent, err := io.ReadAll(reader)
	s.NoError(err)
	err = reader.Close()
	s.NoError(err, "there should be no error reading the downloaded file")
	s.Equal("Hello world!", string(dlContent))

	// copy it
	copyOf, err := fs.NewFile("test-container", "/copy_of_test.txt")
	s.NoError(err)
	err = client.Copy(f, copyOf)
	s.NoError(err, "Copy should succeed so there should be no error")
	_, err = client.Properties(copyOf.Location().URI(), copyOf.Name())
	s.NoError(err, "The copy should succeed so we should not get an error on the properties call")

	// list the location
	list, err := client.List(f.Location())
	s.NoError(err)
	s.Len(list, 2)
	s.Equal("copy_of_test.txt", list[0])
	s.Equal("test.txt", list[1])

	// delete it
	err = client.Delete(f)
	s.NoError(err, "if the file was deleted no error should be returned")

	// make sure it got deleted
	_, err = client.Properties(f.Location().URI(), f.Name())
	s.Error(err, "File should have been deleted so we should get an error")
}

func (s *ClientIntegrationTestSuite) TestAllTheThings_FileWithPath() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/foo/bar/test.txt")
	s.NoError(err)
	client, err := fs.Client()
	s.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	// create a new file
	err = client.Upload(f, strings.NewReader("Hello world!"))
	s.NoError(err, "The file should be successfully uploaded to azure")

	// check to see if it exists
	_, err = client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	s.NoError(err, "If the file exists no error should be returned")

	// download it
	reader, err := client.Download(f)
	s.NoError(err)
	dlContent, err := io.ReadAll(reader)
	s.NoError(err)
	err = reader.Close()
	s.NoError(err)
	s.NoError(err, "there should be no error reading the downloaded file")
	s.Equal("Hello world!", string(dlContent))

	// list the location
	list, err := client.List(f.Location())
	s.NoError(err)
	s.Len(list, 1)
	s.Equal("foo/bar/test.txt", list[0])
}

func (s *ClientIntegrationTestSuite) TestDeleteAllVersions() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/test.txt")
	s.NoError(err)
	client, err := fs.Client()
	s.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	// Create the new file
	err = client.Upload(f, strings.NewReader("Hello!"))
	s.NoError(err, "The file should be successfully uploaded to azure")

	// Recreate the file
	err = client.Upload(f, strings.NewReader("Hello world!"))
	s.NoError(err, "The file should be successfully uploaded to azure")

	// make sure it exists
	_, err = client.Properties(f.Location().URI(), f.Name())
	s.NoError(err, "If the file exists no error should be returned")

	// delete it
	err = client.DeleteAllVersions(f)
	s.NoError(err, "if the file versions were deleted no error should be returned")

	// make sure the file doesn't exist
	exists, err := f.Exists()
	s.NoError(err, "no error should be returned on exists check")
	s.False(exists)
}

func (s *ClientIntegrationTestSuite) TestProperties() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/foo/bar/test.txt")
	s.NoError(err)
	client, err := fs.Client()
	s.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	err = client.Upload(f, strings.NewReader("Hello world!"))
	s.NoError(err, "The file should be successfully uploaded to azure so we shouldn't get an error")
	props, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	s.NoError(err, "The file exists so we shouldn't get an error")
	s.NotNil(props, "We should get a non-nil BlobProperties pointer back")
	s.Greater(props.Size, uint64(0), "The size should be greater than zero")
	s.NotNil(props.LastModified, "Should have a non-nil LastModified")
}

func (s *ClientIntegrationTestSuite) TestProperties_Location() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/foo/bar/test.txt")
	s.NoError(err)
	l, _ := fs.NewLocation("test-container", "/")
	client, _ := fs.Client()

	err = client.Upload(f, strings.NewReader("Hello world!"))
	s.NoError(err, "The file should be successfully uploaded to azure so we shouldn't get an error")

	props, err := client.Properties(l.URI(), "")
	s.NoError(err)
	s.Nil(props, "no props returned when calling properties on a location")
}

func (s *ClientIntegrationTestSuite) TestProperties_NonExistentFile() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/nosuchfile.txt")
	s.NoError(err)
	client, err := fs.Client()
	s.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	_, err = client.Properties(f.Location().URI(), f.Path())
	s.Error(err, "The file does not exist so we expect an error")
	s.Equal(404, err.(*azcore.ResponseError).StatusCode)
}

func (s *ClientIntegrationTestSuite) TestDelete_NonExistentFile() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/nosuchfile.txt")
	s.NoError(err)
	client, err := fs.Client()
	s.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	err = client.Delete(f)
	s.Error(err, "The file does not exist so we expect an error")
}

func (s *ClientIntegrationTestSuite) TestTouch_NonexistentContainer() {
	fs := NewFileSystem()
	f, err := fs.NewFile("nosuchcontainer", "/file.txt")
	s.NoError(err)
	client, err := fs.Client()
	s.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	err = client.Upload(f, strings.NewReader(""))
	s.Error(err, "The container doesn't exist so we should get an error")
}

func (s *ClientIntegrationTestSuite) TestTouch_FileAlreadyExists() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/touch-test.txt")
	s.NoError(err)
	client, err := fs.Client()
	s.NoError(err)

	err = client.Upload(f, strings.NewReader("One fish, two fish, red fish, blue fish."))
	s.NoError(err)
	originalProps, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	s.NoError(err, "Should get properties back from azure with no error")

	err = f.Touch()
	s.NoError(err, "Should not receive an error when touching an existing file")
	newProps, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	s.NoError(err)
	s.NotNil(newProps, "New props should be non-nil")
	s.True(newProps.LastModified.After(*originalProps.LastModified), "newProps.LastModified should be after originalProps.LastModified")
}

func TestAzureClient(t *testing.T) {
	suite.Run(t, new(ClientIntegrationTestSuite))
}
