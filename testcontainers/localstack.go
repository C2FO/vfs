package testcontainers

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"

	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/s3"
)

const (
	localStackPort   = "4566/tcp"
	localStackRegion = "dummy"
	localStackKey    = "dummy"
	localStackSecret = "dummy"
)

func registerLocalStack(t *testing.T) string {
	ctx := context.Background()
	is := require.New(t)

	ctr, err := localstack.Run(ctx, "localstack/localstack:latest", testcontainers.WithName("vfs-localstack"))
	testcontainers.CleanupContainer(t, ctr)
	is.NoError(err)

	ep, err := ctr.PortEndpoint(ctx, localStackPort, "http")
	is.NoError(err)

	cfg, err := config.LoadDefaultConfig(ctx)
	is.NoError(err)

	cli := awss3.NewFromConfig(cfg, func(opts *awss3.Options) {
		opts.Region = localStackRegion
		opts.UsePathStyle = true
		opts.BaseEndpoint = aws.String(ep)
		opts.Credentials = credentials.NewStaticCredentialsProvider(localStackKey, localStackSecret, "")
	})
	_, err = cli.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String("localstack")})
	is.NoError(err)

	backend.Register("s3://localstack/", s3.NewFileSystem(s3.WithClient(cli)))
	return "s3://localstack/"
}
