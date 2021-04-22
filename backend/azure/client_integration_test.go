//+build vfsintegration

package azure

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/stretchr/testify/suite"
)

type ClientTestSuite struct {
	suite.Suite
	testContainerURL azblob.ContainerURL
	accountName      string
	accountKey       string
}

func (suite *ClientTestSuite) SetupSuite() {
	suite.accountName, suite.accountKey = os.Getenv("AZURE_STORAGE_ACCOUNT"), os.Getenv("AZURE_STORAGE_ACCESS_KEY")

	credential, err := azblob.NewSharedKeyCredential(suite.accountName, suite.accountKey)
	if err != nil {
		panic(err)
	}

	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	baseURL, err := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net", suite.accountName))
	suite.NoError(err)
	serviceURL := azblob.NewServiceURL(*baseURL, p)
	suite.testContainerURL = serviceURL.NewContainerURL("test-container")
	_, err = suite.testContainerURL.Create(context.Background(), azblob.Metadata{}, azblob.PublicAccessNone)
	suite.NoError(err)

	// The create function claims to be synchronous but for some reason it does not exist for a little bit so
	// we need to wait for it to be there.
	_, err = suite.testContainerURL.GetProperties(context.Background(), azblob.LeaseAccessConditions{})
	for {
		time.Sleep(2 * time.Second)
		if err == nil || err.(azblob.StorageError).ServiceCode() != "BlobNotFound" {
			break
		}
		_, err = suite.testContainerURL.GetProperties(context.Background(), azblob.LeaseAccessConditions{})
	}
}

func (suite *ClientTestSuite) TearDownSuite() {
	_, err := suite.testContainerURL.Delete(context.Background(), azblob.ContainerAccessConditions{})
	suite.NoError(err)
}

func (suite *ClientTestSuite) TestNewClient() {
	client, err := NewClient("", "&")
	suite.Error(err, "AccountKey must be composed of base 64 decodable characters")
	suite.Nil(client, "The call to NewClient resulted in an error so the returned client should be nil")

	client, err = NewClient(suite.accountName, suite.accountKey)
	suite.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")
	suite.NotNil(client, "No error is expected so the client should be non-nil")
	suite.NotNil(client.pipeline, "pipeline should be non-nil")
}

func (suite *ClientTestSuite) TestAllTheThings_FileWithNoPath() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/test.txt")
	suite.NoError(err)
	client, err := fs.Client()
	suite.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	// Create the new file
	err = client.Upload(f, strings.NewReader("Hello world!"))
	suite.NoError(err, "The file should be successfully uploaded to azure")

	// make sure it exists
	_, err = client.Properties(f.Location().URI(), f.Name())
	suite.NoError(err, "If the file exists no error should be returned")

	// download it
	reader, err := client.Download(f)
	suite.NoError(err)
	dlContent, err := ioutil.ReadAll(reader)
	err = reader.Close()
	suite.NoError(err)
	suite.NoError(err, "there should be no error reading the downloaded file")
	suite.Equal("Hello world!", string(dlContent))

	// copy it
	copyOf, err := fs.NewFile("test-container", "/copy_of_test.txt")
	err = client.Copy(f, copyOf)
	suite.NoError(err, "Copy should succeed so there should be no error")
	_, err = client.Properties(copyOf.Location().URI(), copyOf.Name())
	suite.NoError(err, "The copy should succeed so we should not get an error on the properties call")

	// list the location
	list, err := client.List(f.Location())
	suite.NoError(err)
	suite.Len(list, 2)
	suite.Equal("copy_of_test.txt", list[0])
	suite.Equal("test.txt", list[1])

	// delete it
	err = client.Delete(f)
	suite.NoError(err, "if the file was deleted no error should be returned")

	// make sure it got deleted
	_, err = client.Properties(f.Location().URI(), f.Name())
	suite.Error(err, "File should have been deleted so we should get an error")
}

func (suite *ClientTestSuite) TestAllTheThings_FileWithPath() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/foo/bar/test.txt")
	suite.NoError(err)
	client, err := fs.Client()
	suite.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	// create a new file
	err = client.Upload(f, strings.NewReader("Hello world!"))
	suite.NoError(err, "The file should be successfully uploaded to azure")

	// check to see if it exists
	_, err = client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	suite.NoError(err, "If the file exists no error should be returned")

	// download it
	reader, err := client.Download(f)
	suite.NoError(err)
	dlContent, err := ioutil.ReadAll(reader)
	err = reader.Close()
	suite.NoError(err)
	suite.NoError(err, "there should be no error reading the downloaded file")
	suite.Equal("Hello world!", string(dlContent))

	// list the location
	list, err := client.List(f.Location())
	suite.NoError(err)
	suite.Len(list, 1)
	suite.Equal("foo/bar/test.txt", list[0])
}

func (suite *ClientTestSuite) TestProperties() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/foo/bar/test.txt")
	suite.NoError(err)
	client, err := fs.Client()
	suite.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	err = client.Upload(f, strings.NewReader("Hello world!"))
	suite.NoError(err, "The file should be successfully uploaded to azure so we shouldn't get an error")
	props, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	suite.NoError(err, "Tne file exists so we shouldn't get an error")
	suite.NotNil(props, "We should get a non-nil BlobProperties pointer back")
	suite.Greater(props.Size, uint64(0), "The size should be greater than zero")
	suite.NotNil(props.LastModified, "Should have a non-nil LastModified")
}

func (suite *ClientTestSuite) TestProperties_Location() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/foo/bar/test.txt")
	l, _ := fs.NewLocation("test-container", "/")
	client, _ := fs.Client()

	err = client.Upload(f, strings.NewReader("Hello world!"))
	suite.NoError(err, "The file should be successfully uploaded to azure so we shouldn't get an error")

	props, err := client.Properties(l.URI(), "")
	suite.NoError(err)
	suite.Nil(props, "no props returned when calling properties on a location")
}

func (suite *ClientTestSuite) TestProperties_NonExistentFile() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/nosuchfile.txt")
	suite.NoError(err)
	client, err := fs.Client()
	suite.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	_, err = client.Properties(f.Location().URI(), f.Path())
	suite.Error(err, "Tne file does not exist so we expect an error")
	suite.Equal(404, err.(azblob.ResponseError).Response().StatusCode)
}

func (suite *ClientTestSuite) TestDelete_NonExistentFile() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/nosuchfile.txt")
	suite.NoError(err)
	client, err := fs.Client()
	suite.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	err = client.Delete(f)
	suite.Error(err, "Tne file does not exist so we expect an error")
}

func (suite *ClientTestSuite) TestTouch_NonexistantContainer() {
	fs := NewFileSystem()
	f, err := fs.NewFile("nosuchcontainer", "/file.txt")
	suite.NoError(err)
	client, err := fs.Client()
	suite.NoError(err, "Env variables (AZURE_STORAGE_ACCOUNT, AZURE_STORAGE_ACCESS_KEY) should contain valid azure account credentials")

	err = client.Upload(f, strings.NewReader(""))
	suite.Error(err, "The container doesn't exist so we should get an error")
}

func (suite *ClientTestSuite) TestTouch_FileAlreadyExists() {
	fs := NewFileSystem()
	f, err := fs.NewFile("test-container", "/touch-test.txt")
	suite.NoError(err)
	client, err := fs.Client()
	suite.NoError(err)

	err = client.Upload(f, strings.NewReader("One fish, two fish, red fish, blue fish."))
	suite.NoError(err)
	originalProps, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	suite.NoError(err, "Should get properties back from azure with no error")

	err = f.Touch()
	suite.NoError(err, "Should not receive an error when touching an existing file")
	newProps, err := client.Properties(f.Location().(*Location).ContainerURL(), f.Path())
	suite.NoError(err)
	suite.NotNil(newProps, "New props should be non-nil")
	suite.True(newProps.LastModified.After(*originalProps.LastModified), "newProps.LastModified should be after originalProps.LastModified")
}

func TestAzureClient(t *testing.T) {
	suite.Run(t, new(ClientTestSuite))
}
