package s3

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

type optionsTestSuite struct {
	suite.Suite
}

func (o *optionsTestSuite) SetupTest() {
	os.Clearenv()
}

func (o *optionsTestSuite) TestGetClient() {
	// no options
	opts := Options{}
	client, err := GetClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Empty(client.Options().AppID, "config is empty")

	// options set
	opts = Options{
		AccessKeyID:     "mykey",
		SecretAccessKey: "mysecret",
		Region:          "some-region",
		ForcePathStyle:  true,
	}
	client, err = GetClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("some-region", client.Options().Region, "region is set")
	o.Truef(client.Options().UsePathStyle, "region is set")

	// env var
	_ = os.Setenv("AWS_DEFAULT_REGION", "set-by-envvar")
	opts = Options{}
	client, err = GetClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("set-by-envvar", client.Options().Region, "region is set by env var")

	// role ARN set
	opts = Options{
		AccessKeyID:     "mykey",
		SecretAccessKey: "mysecret",
		Region:          "some-region",
		RoleARN:         "arn:aws:iam::123456789012:role/my-role",
	}
	client, err = GetClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("some-region", client.Options().Region, "region is set")
	o.NotNil(client.Options().Credentials, "credentials are set")
}

func TestOptions(t *testing.T) {
	suite.Run(t, new(optionsTestSuite))
}
