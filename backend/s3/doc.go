package s3

/*
Package s3 - AWS S3 VFS implementation using AWS SDK for Go v2.

# Usage

Rely on github.com/c2fo/vfs/v7/backend

	import(
	    "github.com/c2fo/vfs/v7/backend"
	    "github.com/c2fo/vfs/v7/backend/s3"
	)

	func UseFs() error {
	    fs := backend.Backend(s3.Scheme)
	    ...
	}

Or call directly:

	import "github.com/c2fo/vfs/v7/backend/s3"

	func DoSomething() {
	    fs := s3.NewFileSystem()
	    ...
	}

s3 can be augmented with the following implementation-specific methods.  Backend returns vfs.FileSystem interface so it
would have to be cast as s3.FileSystem to use the following:

	func DoSomething() {
	    ...

	    // to pass in client options
	    fs = s3.NewFileSystem(
	        s3.WithOptions(
	            s3.Options{
	                AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
	                SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	                SessionToken:    "AQoD..." // Optional for temporary credentials
	                Region:          "us-west-2",
	                RoleARN:         "arn:aws:iam::123456789012:role/MyRole",
	                Endpoint:        "https://s3.us-west-2.amazonaws.com",
	                ACL:             "bucket-owner-full-control",
	                ForcePathStyle:  false,
	                MaxRetries:      3,
	            },
	        ),
	    )

	    // to pass specific client, for instance a mock client
	    s3MockClient := mocks.NewClient(t)
	    s3MockClient.EXPECT().
	        GetObject(matchContext, mock.IsType((*s3.GetObjectInput)(nil))).
	        Return(&s3.GetObjectOutput{
	            Body: nopCloser{bytes.NewBufferString("Hello world!")},
	            }, nil)

	    fs = s3.NewFileSystem(s3.WithClient(s3MockClient))
	}

# Object ACL

Canned ACL's can be passed in as an Option. This string will be applied to all writes, moves, and copies.
See https://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html#canned-acl for values.

# Authentication

Authentication, by default, occurs automatically when Client() is called. It looks for credentials in the following places,
preferring the first location found:

 1. StaticProvider - set of credentials which are set programmatically, and will never expire.

 2. EnvProvider - credentials from the environment variables of the
    running process. Environment credentials never expire.
    Environment variables used:

    * Access Key ID:     AWS_ACCESS_KEY_ID or AWS_ACCESS_KEY
    * Secret Access Key: AWS_SECRET_ACCESS_KEY or AWS_SECRET_KEY
    * Session Token:     AWS_SESSION_TOKEN (optional)

 3. SharedCredentialsProvider - looks for "AWS_SHARED_CREDENTIALS_FILE" env variable. If the
    env value is empty will default to current user's home directory.

    * Linux/OSX: "$HOME/.aws/credentials"
    * Windows:   "%USERPROFILE%\.aws\credentials"

 4. RemoteCredProvider - default remote endpoints such as EC2 or ECS IAM Roles

 5. EC2RoleProvider - credentials from the EC2 service, and keeps track if those credentials are expired

# Configuration Options

Additional configuration options available through s3.Options:

- AccessKeyID: AWS access key ID
- SecretAccessKey: AWS secret access key
- SessionToken: AWS session token (required for temporary credentials)
- Region: AWS region (e.g., "us-west-2")
- RoleARN: IAM role ARN for cross-account access (can be combined with static credentials)
- Endpoint: Custom S3 endpoint (useful for testing with S3-compatible storage)
- ACL: Canned ACL for objects (e.g., "private", "public-read")
- ForcePathStyle: Use path-style addressing (required for some S3-compatible services)
- DisableServerSideEncryption: Disable server-side encryption
- AllowLogOutputChecksumValidationSkipped: Enable AWS SDK v2 log output checksum validation
- Retry: Custom retry configuration
- MaxRetries: Maximum number of retry attempts
- FileBufferSize: Buffer size in bytes for file operations
- DownloadPartitionSize: Partition size in bytes for multipart downloads
- UploadPartitionSize: Partition size in bytes for multipart uploads

# Cross-Account Access with IAM Role Assumption

The S3 backend supports assuming an IAM role for cross-account access. This is useful when a service
account in one AWS organization needs to access S3 buckets in a different organization's account.

There are two ways to use role assumption:

1. With explicit static credentials (recommended for cross-org access):

	opts := s3.Options{
	    AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",      // Service account credentials
	    SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	    Region:          "us-west-2",
	    RoleARN:         "arn:aws:iam::EXTERNAL_ACCOUNT:role/cross-account-role",
	}
	client, err := s3.GetClient(opts)

This creates an STS client using the static credentials, then assumes the specified role.

2. With default credential chain (for same-org or instance profile scenarios):

	opts := s3.Options{
	    Region:  "us-west-2",
	    RoleARN: "arn:aws:iam::123456789012:role/MyRole",
	}
	client, err := s3.GetClient(opts)

This uses credentials from environment variables, shared credentials file, or instance profile
to assume the specified role.

Prerequisites:
  - The source IAM user/role must have sts:AssumeRole permission for the target role
  - The target role's trust policy must allow assumption from the source identity

# AWS SDK

This package uses AWS SDK for Go v2. For more information, see:
https://aws.github.io/aws-sdk-go-v2/docs/

# See Also

See: https://github.com/aws/aws-sdk-go-v2/tree/main/service/s3
*/
