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
	"github.com/testcontainers/testcontainers-go/modules/minio"

	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/s3"
)

const minioRegion = "dummy"

func registerMinio(t *testing.T) string {
	ctx := context.Background()
	is := require.New(t)

	ctr, err := minio.Run(ctx, "minio/minio:latest", testcontainers.WithName("vfs-minio"))
	testcontainers.CleanupContainer(t, ctr)
	is.NoError(err)

	ep, err := ctr.ConnectionString(ctx)
	is.NoError(err)

	cfg, err := config.LoadDefaultConfig(ctx)
	is.NoError(err)

	cli := awss3.NewFromConfig(cfg, func(opts *awss3.Options) {
		opts.Region = minioRegion
		opts.UsePathStyle = true
		opts.BaseEndpoint = aws.String("http://" + ep)
		opts.Credentials = credentials.NewStaticCredentialsProvider(ctr.Username, ctr.Password, "")
	})
	_, err = cli.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String("miniobucket")})
	is.NoError(err)

	backend.Register("s3://miniobucket/", s3.NewFileSystem(s3.WithClient(cli), s3.WithOptions(s3.Options{DisableServerSideEncryption: true})))
	return "s3://miniobucket/"
}
