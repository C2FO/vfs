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
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

// Options holds s3-specific options.  Currently only client options are used.
type Options struct {
	AccessKeyID     string `json:"accessKeyId,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	SessionToken    string `json:"sessionToken,omitempty"`
	Region          string `json:"region,omitempty"`
	//ACL                  string `json:"acl,omitempty"`
	//ServerSideEncryption string `json:"serverSideEncryption,omitempty"`
	//StorageClass         string `json:"storageClass,omitempty"`
	//UploadConcurrency    int    `json:"uploadConcurrency"`
}

func getClient(opt Options) (s3iface.S3API, error) {

	p := make([]credentials.Provider, 0)

	if opt.AccessKeyID != "" && opt.SecretAccessKey != "" {
		// Make the auth
		v := credentials.Value{
			AccessKeyID:     opt.AccessKeyID,
			SecretAccessKey: opt.SecretAccessKey,
			SessionToken:    opt.SessionToken,
		}
		// A StaticProvider is a set of credentials which are set programmatically,
		// and will never expire.
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
	p = append(p, &credentials.EnvProvider{})

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
	p = append(p, &ec2rolecreds.EC2RoleProvider{
		Client: ec2metadata.New(session.New(), &aws.Config{
			HTTPClient: lowTimeoutClient,
		}),
		ExpiryWindow: 3,
	})

	awsConfig := aws.Config{Logger: aws.NewDefaultLogger()}
	if opt.Region != "" {
		awsConfig = *awsConfig.WithRegion(opt.Region)
	} else if val, ok := os.LookupEnv("AWS_DEFAULT_REGION"); ok {
		awsConfig = *awsConfig.WithRegion(val)
	}

	awsConfig = *awsConfig.WithCredentials(credentials.NewChainCredentials(p))

	s, err := session.NewSessionWithOptions(session.Options{
		Config: awsConfig,
	})
	if err != nil {
		return nil, err
	}
	return s3.New(s), nil
}
