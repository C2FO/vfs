package azure

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/stretchr/testify/suite"
)

type OptionsTestSuite struct {
	suite.Suite
}

func (s *OptionsTestSuite) TearDownTest() {
	s.NoError(os.Unsetenv("VFS_AZURE_TENANT_ID"))
	s.NoError(os.Unsetenv("VFS_AZURE_CLIENT_ID"))
	s.NoError(os.Unsetenv("VFS_AZURE_CLIENT_SECRET"))
	s.NoError(os.Unsetenv("VFS_AZURE_STORAGE_ACCOUNT"))
	s.NoError(os.Unsetenv("VFS_AZURE_STORAGE_ACCESS_KEY"))
}

func (s *OptionsTestSuite) TestNewOptions() {
	o := NewOptions()
	s.NotNil(o, "when NewOptions returns an error we expect to get a nill options struct")
}

func (s *OptionsTestSuite) TestCredentials_ServiceAccount() {
	options := Options{
		AccountName:            "foo",
		TenantID:               "foo",
		ClientID:               "foo",
		ClientSecret:           "foo",
		tokenCredentialFactory: MockTokenCredentialFactory,
	}

	credential, err := options.Credential()
	s.NoError(err, "service account vars are present so no error")
	s.NotNil(credential, "expect a non-nil credential when service account vars are set")
	s.Implements((*azcore.TokenCredential)(nil), credential, "credentials type should be TokenCredential")
}

func (s *OptionsTestSuite) TestCredentials_StorageAccount() {
	options := Options{
		AccountName:            "foo",
		AccountKey:             base64.StdEncoding.EncodeToString([]byte("bar")),
		tokenCredentialFactory: MockTokenCredentialFactory,
	}

	credential, err := options.Credential()
	s.NoError(err, "service account vars are present so no error")
	s.NotNil(credential, "expect a non-nil credential when service account vars are set")
	s.IsType((*azblob.SharedKeyCredential)(nil), credential, "credentials type should be SharedKeyCredential")
}

func (s *OptionsTestSuite) TestCredentials_Anon() {
	options := Options{
		AccountName:            "foo",
		tokenCredentialFactory: MockTokenCredentialFactory,
	}

	credential, err := options.Credential()
	s.NoError(err, "anon vars are present so no error")
	s.Nil(credential, "when no env vars are set we should get a nil credential")
}

func TestOptions(t *testing.T) {
	suite.Run(t, new(OptionsTestSuite))
}
