/*
Package s3 implements [github.com/c2fo/vfs/v8.FileSystem] for AWS S3 using the AWS SDK for Go v2.

Import path:

	import "github.com/c2fo/vfs/v8/backend/s3"

Construct a filesystem with [NewFileSystem] and functional options such as [WithOptions] and [WithClient].

	fs := s3.NewFileSystem(s3.WithOptions(s3.Options{
	    Region: "us-west-2",
	}))

There is no global registration: callers keep the concrete [*FileSystem] or pass values as
[github.com/c2fo/vfs/v8.FileSystem] where needed.

# Object ACL

Canned ACL values can be set in [Options.ACL] and apply to writes, copies, and moves that use this file system.
See https://docs.aws.amazon.com/AmazonS3/latest/userguide/acl-overview.html#canned-acl

# Authentication

By default, credentials are resolved when [FileSystem.Client] is called, following the AWS SDK
default chain (environment variables, shared credentials file, IAM role for EC2/ECS, etc.):

  - AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN
  - Shared credentials file (often ~/.aws/credentials)
  - Instance/container role credentials

# Configuration options

[Options] includes region, static credentials, role ARN, endpoint (S3-compatible services), ACL,
SSE, retry and buffer sizes, and more. See the type documentation for the full list.

# Cross-account IAM role assumption

When [Options.RoleARN] is set, the backend can assume that role via STS (see README and options
for details). The caller identity must be allowed to call sts:AssumeRole, and the role trust
policy must allow the caller.

# AWS SDK

This package uses AWS SDK for Go v2: https://aws.github.io/aws-sdk-go-v2/docs/

# See also

  - [README](README.md) for behavior summary and integration tests
  - https://github.com/aws/aws-sdk-go-v2/tree/main/service/s3
*/
package s3
