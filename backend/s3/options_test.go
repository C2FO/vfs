package s3

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
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
	o.Empty(*client.(*s3.S3).Config.Region, "config is empty")

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
	o.Equal("some-region", *client.(*s3.S3).Config.Region, "region is set")
	o.Truef(*client.(*s3.S3).Config.S3ForcePathStyle, "region is set")

	// env var
	_ = os.Setenv("AWS_DEFAULT_REGION", "set-by-envvar")
	opts = Options{}
	client, err = getClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("set-by-envvar", *client.(*s3.S3).Config.Region, "region is set by env var")
}

func TestOptions(t *testing.T) {
	suite.Run(t, new(optionsTestSuite))
}
