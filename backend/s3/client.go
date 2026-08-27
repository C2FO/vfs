package s3

import (
	"context"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client interface {
	manager.DownloadAPIClient
	manager.UploadAPIClient
	CopyObject(ctx context.Context, in *s3.CopyObjectInput, opts ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	DeleteObject(ctx context.Context, in *s3.DeleteObjectInput, opts ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadBucket(ctx context.Context, in *s3.HeadBucketInput, opts ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	HeadObject(ctx context.Context, in *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	ListObjects(ctx context.Context, in *s3.ListObjectsInput, opts ...func(*s3.Options)) (*s3.ListObjectsOutput, error)
	ListObjectVersions(ctx context.Context, in *s3.ListObjectVersionsInput, opts ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
}

// backendClient is the client contract the backend uses internally. It mirrors Client except that
// it takes the ListObjectsV2 operation in place of the superseded ListObjects, so backend code
// cannot reach for the older operation. Its methods are spelled out rather than embedding Client
// so that the internal contract carries no dependency on the deprecated feature/s3/manager package.
//
// Keeping this separate from Client lets the exported interface stay unchanged for callers that
// implement it themselves; those that predate ListObjectsV2 are adapted by legacyClient.
type backendClient interface {
	AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
	CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	ListObjectVersions(context.Context, *s3.ListObjectVersionsInput, ...func(*s3.Options)) (*s3.ListObjectVersionsOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error)
}

// isNilClient reports whether c is nil, including a typed nil such as (*s3.Client)(nil) passed
// through an interface parameter. A plain `c == nil` comparison misses that case: the interface
// still carries a concrete type, so it compares unequal to the untyped nil literal even though
// calling a method on it panics.
func isNilClient(c Client) bool {
	if c == nil {
		return true
	}
	v := reflect.ValueOf(c)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// asBackendClient returns c as a backendClient, wrapping it only when it doesn't already provide
// ListObjectsV2. The AWS SDK's *s3.Client does, so the default path is never wrapped.
func asBackendClient(c Client) backendClient {
	if bc, ok := c.(backendClient); ok {
		return bc
	}
	return &legacyClient{Client: c}
}

// legacyClient adapts a Client that predates ListObjectsV2 by emulating it with the v1 ListObjects
// operation.
type legacyClient struct {
	Client
}

// ListObjectsV2 emulates the v2 listing operation using v1 ListObjects.
//
// The two operations differ in how a caller resumes a listing: v1 has a single Marker, while v2
// splits that into ContinuationToken (subsequent pages) and StartAfter (first page only).
func (c *legacyClient) ListObjectsV2(
	ctx context.Context,
	in *s3.ListObjectsV2Input,
	opts ...func(*s3.Options),
) (*s3.ListObjectsV2Output, error) {
	if in == nil {
		in = &s3.ListObjectsV2Input{}
	}

	marker := in.ContinuationToken
	if marker == nil {
		marker = in.StartAfter
	}

	out, err := c.ListObjects(ctx, &s3.ListObjectsInput{
		Bucket:                   in.Bucket,
		Delimiter:                in.Delimiter,
		EncodingType:             in.EncodingType,
		ExpectedBucketOwner:      in.ExpectedBucketOwner,
		Marker:                   marker,
		MaxKeys:                  in.MaxKeys,
		OptionalObjectAttributes: in.OptionalObjectAttributes,
		Prefix:                   in.Prefix,
		RequestPayer:             in.RequestPayer,
	}, opts...)
	if err != nil {
		return nil, err
	}
	// A well-behaved client never returns (nil, nil), but nothing in the Client interface
	// forbids it, and every field access below would panic on a nil out. Treat it the same as
	// an empty page rather than trust the contract.
	if out == nil {
		out = &s3.ListObjectsOutput{}
	}

	// KeyCount counts common prefixes alongside keys, so both are summed. S3 never returns more
	// than MaxKeys entries, which is capped at 1000, so the conversion cannot overflow.
	keyCount := int32(len(out.Contents) + len(out.CommonPrefixes))

	v2Out := &s3.ListObjectsV2Output{
		CommonPrefixes:    out.CommonPrefixes,
		Contents:          out.Contents,
		ContinuationToken: in.ContinuationToken,
		Delimiter:         out.Delimiter,
		EncodingType:      out.EncodingType,
		IsTruncated:       out.IsTruncated,
		KeyCount:          aws.Int32(keyCount),
		MaxKeys:           out.MaxKeys,
		Name:              out.Name,
		Prefix:            out.Prefix,
		RequestCharged:    out.RequestCharged,
		StartAfter:        in.StartAfter,
		ResultMetadata:    out.ResultMetadata,
	}

	// S3 only populates NextMarker when the request set a delimiter. Without one, the caller is
	// expected to resume from the last key of the current page. The last key is a sound resumption
	// point precisely because no delimiter means no common prefixes to account for.
	if aws.ToBool(out.IsTruncated) {
		switch {
		case out.NextMarker != nil:
			v2Out.NextContinuationToken = out.NextMarker
		case len(out.Contents) > 0:
			v2Out.NextContinuationToken = out.Contents[len(out.Contents)-1].Key
		}
	}

	return v2Out, nil
}
