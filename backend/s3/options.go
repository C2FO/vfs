package s3

import (
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

// Options holds s3-specific options.  Currently only client options are used.
type Options struct {
	AccessKeyID           string `json:"accessKeyId,omitempty"`
	SecretAccessKey       string `json:"secretAccessKey,omitempty"`
	SessionToken          string `json:"sessionToken,omitempty"`
	Region                string `json:"region,omitempty"`
	Endpoint              string `json:"endpoint,omitempty"`
	ACL                   string `json:"acl,omitempty"`
	Retry                 request.Retryer
	MaxRetries            int
	FileBufferSize        int   // Buffer size in bytes used with utils.TouchCopyBuffered
	DownloadPartitionSize int64 // Partition size in bytes used to multipart download large files using S3 Downloader
}

// getClient setup S3 client
func getClient(opt Options) (s3iface.S3API, error) {

	// setup default config
	awsConfig := defaults.Config()

	// setup region using opt or env
	if opt.Region != "" {
		awsConfig.WithRegion(opt.Region)
	} else if val, ok := os.LookupEnv("AWS_DEFAULT_REGION"); ok {
		awsConfig.WithRegion(val)
	}

	// use specific endpoint, otherwise, will use aws "default endpoint resolver" based on region
	awsConfig.WithEndpoint(opt.Endpoint)

	if opt.Retry != nil {
		awsConfig.Retryer = opt.Retry
	}

	// set up credential provider chain
	credentialProviders, err := initCredentialProviderChain(opt)
	if err != nil {
		return nil, err
	}
	awsConfig.WithCredentials(
		credentials.NewChainCredentials(credentialProviders),
	)

	// create new session with config
	s, err := session.NewSessionWithOptions(
		session.Options{
			Config: *awsConfig,
		},
	)
	if err != nil {
		return nil, err
	}

	// return client instance
	return s3.New(s), nil
}

// initCredentialProviderChain returns an array of credential providers that will be used, in order, to attempt authentication
func initCredentialProviderChain(opt Options) ([]credentials.Provider, error) {
	p := make([]credentials.Provider, 0)

	// A StaticProvider is a set of credentials which are set programmatically,
	// and will never expire.
	if opt.AccessKeyID != "" && opt.SecretAccessKey != "" {
		// Make the auth
		v := credentials.Value{
			AccessKeyID:     opt.AccessKeyID,
			SecretAccessKey: opt.SecretAccessKey,
			SessionToken:    opt.SessionToken,
		}
		p = append(p, &credentials.StaticProvider{Value: v})
	}

	// A EnvProvider retrieves credentials from the environment variables of the
	// running process. Environment credentials never expire.
	//
	// Environment variables used:
	//
	// * Access Key ID:     AWS_ACCESS_KEY_ID or AWS_ACCESS_KEY
	//
	// * Secret Access Key: AWS_SECRET_ACCESS_KEY or AWS_SECRET_KEY
	p = append(p, &credentials.EnvProvider{}) // nolint:gocritic // appendCombine

	// Path to the shared credentials file.
	//
	// SharedCredentialsProvider will look for "AWS_SHARED_CREDENTIALS_FILE" env variable. If the
	// env value is empty will default to current user's home directory.
	// Linux/OSX: "$HOME/.aws/credentials"
	// Windows:   "%USERPROFILE%\.aws\credentials"
	p = append(p, &credentials.SharedCredentialsProvider{})

	lowTimeoutClient := &http.Client{Timeout: 1 * time.Second} // low timeout to ec2 metadata service

	// RemoteCredProvider for default remote endpoints such as EC2 or ECS IAM Roles
	def := defaults.Get()
	def.Config.HTTPClient = lowTimeoutClient
	p = append(p, defaults.RemoteCredProvider(*def.Config, def.Handlers))

	// EC2RoleProvider retrieves credentials from the EC2 service, and keeps track if those credentials are expired
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}
	p = append(p, &ec2rolecreds.EC2RoleProvider{
		Client: ec2metadata.New(sess, &aws.Config{
			HTTPClient: lowTimeoutClient,
		}),
		ExpiryWindow: 3,
	})

	return p, nil
}
