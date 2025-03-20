package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Options holds s3-specific options.  Currently only client options are used.
type Options struct {
	AccessKeyID                 string                `json:"accessKeyId,omitempty"`
	SecretAccessKey             string                `json:"secretAccessKey,omitempty"`
	SessionToken                string                `json:"sessionToken,omitempty"`
	Region                      string                `json:"region,omitempty"`
	RoleARN                     string                `json:"roleARN,omitempty"`
	Endpoint                    string                `json:"endpoint,omitempty"`
	ACL                         types.ObjectCannedACL `json:"acl,omitempty"`
	ForcePathStyle              bool                  `json:"forcePathStyle,omitempty"`
	DisableServerSideEncryption bool                  `json:"disableServerSideEncryption,omitempty"`
	Retry                       aws.Retryer
	MaxRetries                  int
	FileBufferSize              int   // Buffer size in bytes used with utils.TouchCopyBuffered
	DownloadPartitionSize       int64 // Partition size in bytes used to multipart download of large files using manager.Downloader
	UploadPartitionSize         int64 // Partition size in bytes used to multipart upload of large files using manager.Uploader
}

// GetClient setup S3 client
func GetClient(opt Options) (*s3.Client, error) {
	// setup default config
	awsConfig, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}

	// return client instance
	return s3.NewFromConfig(awsConfig, func(opts *s3.Options) {
		if opt.Region != "" {
			opts.Region = opt.Region
		}

		// set filepath for minio users
		opts.UsePathStyle = opt.ForcePathStyle

		// use specific endpoint, otherwise, will use aws "default endpoint resolver" based on region
		if opt.Endpoint != "" {
			opts.BaseEndpoint = aws.String(opt.Endpoint)
		}

		if opt.Retry != nil {
			opts.Retryer = opt.Retry
		}

		if opt.MaxRetries > 0 {
			opts.RetryMaxAttempts = opt.MaxRetries
		}

		if opt.AccessKeyID != "" && opt.SecretAccessKey != "" {
			opts.Credentials = credentials.NewStaticCredentialsProvider(
				opt.AccessKeyID,
				opt.SecretAccessKey,
				opt.SessionToken,
			)
		} else if opt.RoleARN != "" {
			opts.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(awsConfig), opt.RoleARN))
		}
	}), nil
}

// StringToACL converts a string to an ObjectCannedACL
// see https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3/types#ObjectCannedACL
func StringToACL(acl string) types.ObjectCannedACL {
	switch acl {
	case "private":
		return types.ObjectCannedACLPrivate
	case "public-read":
		return types.ObjectCannedACLPublicRead
	case "public-read-write":
		return types.ObjectCannedACLPublicReadWrite
	case "authenticated-read":
		return types.ObjectCannedACLAuthenticatedRead
	case "aws-exec-read":
		return types.ObjectCannedACLAwsExecRead
	case "bucket-owner-read":
		return types.ObjectCannedACLBucketOwnerRead
	case "bucket-owner-full-control":
		return types.ObjectCannedACLBucketOwnerFullControl
	default:
		return types.ObjectCannedACLPrivate
	}
}
