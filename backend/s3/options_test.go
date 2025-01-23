package s3

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	client, err := getClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Empty(client.(*s3.Client).Options().Region, "config is empty")

	// options set
	opts = Options{
		AccessKeyID:     "mykey",
		SecretAccessKey: "mysecret",
		Region:          "some-region",
		ForcePathStyle:  true,
	}
	client, err = getClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("some-region", client.(*s3.Client).Options().Region, "region is set")
	o.Truef(client.(*s3.Client).Options().UsePathStyle, "region is set")

	// env var
	_ = os.Setenv("AWS_DEFAULT_REGION", "set-by-envvar")
	opts = Options{}
	client, err = getClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("set-by-envvar", client.(*s3.Client).Options().Region, "region is set by env var")

	// role ARN set
	opts = Options{
		AccessKeyID:     "mykey",
		SecretAccessKey: "mysecret",
		Region:          "some-region",
		RoleARN:         "arn:aws:iam::123456789012:role/my-role",
	}
	client, err = getClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("some-region", client.(*s3.Client).Options().Region, "region is set")
	o.NotNil(client.(*s3.Client).Options().Credentials, "credentials are set")
}

func TestOptions(t *testing.T) {
	suite.Run(t, new(optionsTestSuite))
}
