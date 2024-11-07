package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Options holds s3-specific options.  Currently only client options are used.
type Options struct {
	AccessKeyID                 string                `json:"accessKeyId,omitempty"`
	SecretAccessKey             string                `json:"secretAccessKey,omitempty"`
	SessionToken                string                `json:"sessionToken,omitempty"`
	Region                      string                `json:"region,omitempty"`
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

// getClient setup S3 client
func getClient(opt Options) (Client, error) {
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

		opts.Retryer = opt.Retry

		if opt.AccessKeyID != "" && opt.SecretAccessKey != "" {
			opts.Credentials = credentials.NewStaticCredentialsProvider(
				opt.AccessKeyID,
				opt.SecretAccessKey,
				opt.SessionToken,
			)
		}
	}), nil
}
