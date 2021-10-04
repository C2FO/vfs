/*
Package s3 AWS S3 VFS implementation.

Usage

Rely on github.com/c2fo/vfs/v6/backend

  import(
      "github.com/c2fo/vfs/v6/backend"
      "github.com/c2fo/vfs/v6/backend/s3"
  )

  func UseFs() error {
      fs := backend.Backend(s3.Scheme)
      ...
  }

Or call directly:

  import "github.com/c2fo/vfs/v6/backend/s3"

  func DoSomething() {
      fs := s3.NewFileSystem()
      ...
  }

s3 can be augmented with the following implementation-specific methods.  Backend returns vfs.FileSystem interface so it
would have to be cast as s3.FileSystem to use the following:

  func DoSomething() {

      ...

      // cast if fs was created using backend.Backend().  Not necessary if created directly from s3.NewFileSystem().
      fs = fs.(s3.FileSystem)

      // to pass in client options
      fs = fs.WithOptions(
          s3.Options{
              AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
              SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
              Region:          "us-west-2",
              ACL: 			   "bucket-owner-full-control",
          },
      )

      // to pass specific client, for instance a mock client
      s3apiMock := &mocks.S3API{}
      s3apiMock.On("GetObject", mock.AnythingOfType("*s3.GetObjectInput")).
          Return(&s3.GetObjectOutput{
              Body: nopCloser{bytes.NewBufferString("Hello world!")},
              }, nil)
      fs = fs.WithClient(s3apiMock)
  }

Object ACL

Canned ACL's can be passed in as an Option.  This string will be applied to all writes, moves, and copies.
See https://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html#canned-acl for values.

Authentication

Authentication, by default, occurs automatically when Client() is called. It looks for credentials in the following places,
preferring the first location found:

  1. StaticProvider - set of credentials which are set programmatically, and will never expire.
  2. EnvProvider - credentials from the environment variables of the
     running process. Environment credentials never expire.
     Environment variables used:

  	* Access Key ID:     AWS_ACCESS_KEY_ID or AWS_ACCESS_KEY
  	* Secret Access Key: AWS_SECRET_ACCESS_KEY or AWS_SECRET_KEY

  3. SharedCredentialsProvider - looks for "AWS_SHARED_CREDENTIALS_FILE" env variable. If the
     env value is empty will default to current user's home directory.

  	* Linux/OSX: "$HOME/.aws/credentials"
  	* Windows:   "%USERPROFILE%\.aws\credentials"

  4. RemoteCredProvider - default remote endpoints such as EC2 or ECS IAM Roles
  5. EC2RoleProvider - credentials from the EC2 service, and keeps track if those credentials are expired

See the following for more auth info: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-envvars.html
and https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html

See Also

See: https://github.com/aws/aws-sdk-go/tree/master/service/s3
*/
package s3
