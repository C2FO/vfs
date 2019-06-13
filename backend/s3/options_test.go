package s3

import (
	"github.com/c2fo/vfs/v4"
	"os"
	"testing"
	"time"

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
	//no options
	opts := Options{}
	client, err := getClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("", *client.(*s3.S3).Config.Region, "config is empty")

	//options set
	opts = Options{
		AccessKeyID:     "mykey",
		SecretAccessKey: "mysecret",
		Region:          "some-region",
	}
	client, err = getClient(opts)
	o.NoError(err)
	o.NotNil(client, "client is set")
	o.Equal("some-region", *client.(*s3.S3).Config.Region, "region is set")

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

type Foo struct{}

func (*Foo) Close() error {
	panic("implement me")
}

func (*Foo) Read(p []byte) (n int, err error) {
	panic("implement me")
}

func (*Foo) Seek(offset int64, whence int) (int64, error) {
	panic("implement me")
}

func (*Foo) Write(p []byte) (n int, err error) {
	panic("implement me")
}

func (*Foo) String() string {
	panic("implement me")
}

func (*Foo) Exists() (bool, error) {
	panic("implement me")
}

func (*Foo) Location() vfs.Location {
	panic("implement me")
}

func (*Foo) CopyToLocation(location vfs.Location) (vfs.File, error) {
	panic("implement me")
}

func (*Foo) CopyToFile(vfs.File) error {
	panic("implement me")
}

func (*Foo) MoveToLocation(location vfs.Location) (vfs.File, error) {
	panic("implement me")
}

func (*Foo) MoveToFile(vfs.File) error {
	panic("implement me")
}

func (*Foo) Delete() error {
	panic("implement me")
}

func (*Foo) LastModified() (*time.Time, error) {
	panic("implement me")
}

func (*Foo) Size() (uint64, error) {
	panic("implement me")
}

func (*Foo) Path() string {
	panic("implement me")
}

func (*Foo) Name() string {
	panic("implement me")
}

func (*Foo) URI() string {
	panic("implement me")
}
