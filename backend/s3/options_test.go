package s3

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/suite"
)

type optionsTestSuite struct {
	suite.Suite
}

func (o *optionsTestSuite) SetupTest() {
	os.Clearenv()
}

func (o *optionsTestSuite) TestGetClient() {
	tests := []struct {
		name     string
		opts     Options
		envVar   map[string]string
		expected func(*optionsTestSuite, *s3.Client, error)
	}{
		{
			name: "no options",
			opts: Options{},
			expected: func(o *optionsTestSuite, client *s3.Client, err error) {
				o.Require().NoError(err)
				o.NotNil(client, "client is set")
				o.Empty(client.Options().AppID, "config is empty")
			},
		},
		{
			name: "options set",
			opts: Options{
				AccessKeyID:     "mykey",
				SecretAccessKey: "mysecret",
				Region:          "some-region",
				ForcePathStyle:  true,
				Endpoint:        "http://localhost:9000",
				Retry:           retry.AddWithMaxAttempts(retry.NewStandard(), 5),
				MaxRetries:      5,
			},
			expected: func(o *optionsTestSuite, client *s3.Client, err error) {
				o.Require().NoError(err)
				o.NotNil(client, "client is set")
				o.Equal("some-region", client.Options().Region, "region is set")
				o.Truef(client.Options().UsePathStyle, "path style is set")
				o.Equal("http://localhost:9000", *client.Options().BaseEndpoint, "endpoint is set")
				o.Equal(5, client.Options().RetryMaxAttempts, "max retries is set")
			},
		},
		{
			name: "env var",
			envVar: map[string]string{
				"AWS_DEFAULT_REGION": "set-by-envvar",
			},
			opts: Options{},
			expected: func(o *optionsTestSuite, client *s3.Client, err error) {
				o.Require().NoError(err)
				o.NotNil(client, "client is set")
				o.Equal("set-by-envvar", client.Options().Region, "region is set by env var")
			},
		},
		{
			name: "role ARN set",
			opts: Options{
				AccessKeyID:     "",
				SecretAccessKey: "",
				Region:          "some-region",
				RoleARN:         "arn:aws:iam::123456789012:role/my-role",
			},
			expected: func(o *optionsTestSuite, client *s3.Client, err error) {
				o.Require().NoError(err)
				o.NotNil(client, "client is set")
				o.Equal("some-region", client.Options().Region, "region is set")
				o.NotNil(client.Options().Credentials, "credentials are set")
			},
		},
	}

	for _, tt := range tests { //nolint:gocritic //rangeValCopy
		o.Run(tt.name, func() {
			for k, v := range tt.envVar {
				o.T().Setenv(k, v)
			}
			client, err := GetClient(tt.opts)
			tt.expected(o, client, err)
			for k := range tt.envVar {
				_ = os.Unsetenv(k)
			}
		})
	}
}

func (o *optionsTestSuite) TestStringToACL() {
	tests := []struct {
		input    string
		expected types.ObjectCannedACL
	}{
		{"private", types.ObjectCannedACLPrivate},
		{"public-read", types.ObjectCannedACLPublicRead},
		{"public-read-write", types.ObjectCannedACLPublicReadWrite},
		{"authenticated-read", types.ObjectCannedACLAuthenticatedRead},
		{"aws-exec-read", types.ObjectCannedACLAwsExecRead},
		{"bucket-owner-read", types.ObjectCannedACLBucketOwnerRead},
		{"bucket-owner-full-control", types.ObjectCannedACLBucketOwnerFullControl},
		{"unknown", types.ObjectCannedACLPrivate},
	}

	for _, tt := range tests {
		o.Equal(tt.expected, StringToACL(tt.input))
	}
}

func TestOptions(t *testing.T) {
	suite.Run(t, new(optionsTestSuite))
}
