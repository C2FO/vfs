package s3

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/backend/s3/mocks"
)

type clientTestSuite struct {
	suite.Suite
}

// sdkStyleClient provides both list operations, matching the shape of the AWS SDK's *s3.Client,
// which is what a client has to look like to be used without adaptation. It composes the two
// generated mocks because neither alone satisfies both Client and backendClient.
type sdkStyleClient struct {
	*mocks.Client
	v2 *mocks.BackendClient
}

func newSDKStyleClient(t *testing.T) *sdkStyleClient {
	t.Helper()
	return &sdkStyleClient{Client: mocks.NewClient(t), v2: mocks.NewBackendClient(t)}
}

func (c *sdkStyleClient) ListObjectsV2(
	ctx context.Context,
	in *s3.ListObjectsV2Input,
	opts ...func(*s3.Options),
) (*s3.ListObjectsV2Output, error) {
	return c.v2.ListObjectsV2(ctx, in, opts...)
}

func (cs *clientTestSuite) TestAsBackendClient() {
	cs.Run("client already implementing ListObjectsV2 is not wrapped", func() {
		client := newSDKStyleClient(cs.T())
		cs.Same(client, asBackendClient(client))
	})

	cs.Run("client without ListObjectsV2 is wrapped", func() {
		client := mocks.NewClient(cs.T())
		adapted := asBackendClient(client)
		legacy, ok := adapted.(*legacyClient)
		cs.Require().True(ok, "expected the client to be wrapped in a legacyClient")
		cs.Same(client, legacy.Client)
	})
}

func (cs *clientTestSuite) TestLegacyClientListObjectsV2() {
	bucket := "bucket"
	prefix := "dir1/"
	delimiter := "/"
	token := "token"
	startAfter := "dir1/file0.txt"
	nextMarker := "dir1/file2.txt"
	contents := convertKeysToS3Objects([]string{"dir1/file1.txt", "dir1/file2.txt"})

	tests := []struct {
		name                     string
		input                    *s3.ListObjectsV2Input
		output                   *s3.ListObjectsOutput
		clientError              error
		expectedInput            *s3.ListObjectsInput
		expectedNextContinuation *string
		expectedKeyCount         int32
		expectedError            string
	}{
		{
			name: "first page maps prefix and delimiter",
			input: &s3.ListObjectsV2Input{
				Bucket:    aws.String(bucket),
				Prefix:    aws.String(prefix),
				Delimiter: aws.String(delimiter),
			},
			output: &s3.ListObjectsOutput{
				Contents:    contents,
				IsTruncated: aws.Bool(false),
			},
			expectedInput: &s3.ListObjectsInput{
				Bucket:    aws.String(bucket),
				Prefix:    aws.String(prefix),
				Delimiter: aws.String(delimiter),
			},
			expectedKeyCount: 2,
		},
		{
			name: "continuation token becomes marker",
			input: &s3.ListObjectsV2Input{
				Bucket:            aws.String(bucket),
				ContinuationToken: aws.String(token),
			},
			output: &s3.ListObjectsOutput{IsTruncated: aws.Bool(false)},
			expectedInput: &s3.ListObjectsInput{
				Bucket: aws.String(bucket),
				Marker: aws.String(token),
			},
		},
		{
			name: "start after becomes marker when no continuation token",
			input: &s3.ListObjectsV2Input{
				Bucket:     aws.String(bucket),
				StartAfter: aws.String(startAfter),
			},
			output: &s3.ListObjectsOutput{IsTruncated: aws.Bool(false)},
			expectedInput: &s3.ListObjectsInput{
				Bucket: aws.String(bucket),
				Marker: aws.String(startAfter),
			},
		},
		{
			name: "continuation token wins over start after",
			input: &s3.ListObjectsV2Input{
				Bucket:            aws.String(bucket),
				ContinuationToken: aws.String(token),
				StartAfter:        aws.String(startAfter),
			},
			output: &s3.ListObjectsOutput{IsTruncated: aws.Bool(false)},
			expectedInput: &s3.ListObjectsInput{
				Bucket: aws.String(bucket),
				Marker: aws.String(token),
			},
		},
		{
			name:  "truncated page uses next marker",
			input: &s3.ListObjectsV2Input{Bucket: aws.String(bucket)},
			output: &s3.ListObjectsOutput{
				Contents:    contents,
				IsTruncated: aws.Bool(true),
				NextMarker:  aws.String(nextMarker),
			},
			expectedInput:            &s3.ListObjectsInput{Bucket: aws.String(bucket)},
			expectedNextContinuation: aws.String(nextMarker),
			expectedKeyCount:         2,
		},
		{
			name:  "truncated page without next marker falls back to last key",
			input: &s3.ListObjectsV2Input{Bucket: aws.String(bucket)},
			output: &s3.ListObjectsOutput{
				Contents:    contents,
				IsTruncated: aws.Bool(true),
			},
			expectedInput:            &s3.ListObjectsInput{Bucket: aws.String(bucket)},
			expectedNextContinuation: aws.String("dir1/file2.txt"),
			expectedKeyCount:         2,
		},
		{
			name:  "truncated empty page yields no continuation token",
			input: &s3.ListObjectsV2Input{Bucket: aws.String(bucket)},
			output: &s3.ListObjectsOutput{
				IsTruncated: aws.Bool(true),
			},
			expectedInput: &s3.ListObjectsInput{Bucket: aws.String(bucket)},
		},
		{
			name:          "nil IsTruncated is treated as not truncated",
			input:         &s3.ListObjectsV2Input{Bucket: aws.String(bucket)},
			output:        &s3.ListObjectsOutput{Contents: contents},
			expectedInput: &s3.ListObjectsInput{Bucket: aws.String(bucket)},
			// KeyCount is still reported even though the response omitted IsTruncated.
			expectedKeyCount: 2,
		},
		{
			name:          "nil input is tolerated",
			input:         nil,
			output:        &s3.ListObjectsOutput{IsTruncated: aws.Bool(false)},
			expectedInput: &s3.ListObjectsInput{},
		},
		{
			name:          "error is returned unwrapped",
			input:         &s3.ListObjectsV2Input{Bucket: aws.String(bucket)},
			clientError:   errors.New("some error"),
			expectedInput: &s3.ListObjectsInput{Bucket: aws.String(bucket)},
			expectedError: "some error",
		},
	}

	for _, tt := range tests {
		cs.Run(tt.name, func() {
			client := mocks.NewClient(cs.T())
			client.EXPECT().
				ListObjects(matchContext, tt.expectedInput).
				Return(tt.output, tt.clientError).
				Once()

			out, err := (&legacyClient{Client: client}).ListObjectsV2(cs.T().Context(), tt.input)

			if tt.expectedError != "" {
				cs.Require().Error(err)
				cs.Contains(err.Error(), tt.expectedError)
				cs.Nil(out)
				return
			}

			cs.Require().NoError(err)
			cs.Equal(tt.expectedNextContinuation, out.NextContinuationToken)
			cs.Equal(tt.expectedKeyCount, aws.ToInt32(out.KeyCount))
			cs.Equal(tt.output.Contents, out.Contents)
			cs.Equal(tt.output.IsTruncated, out.IsTruncated)
		})
	}
}

// TestLegacyClientListObjectsV2KeyCount verifies that KeyCount counts common prefixes as well as
// keys, which is what S3 (and minio) report for a delimited listing.
func (cs *clientTestSuite) TestLegacyClientListObjectsV2KeyCount() {
	client := mocks.NewClient(cs.T())
	client.EXPECT().
		ListObjects(matchContext, &s3.ListObjectsInput{Bucket: aws.String("bucket")}).
		Return(&s3.ListObjectsOutput{
			Contents: convertKeysToS3Objects([]string{"top1.txt", "top2.txt"}),
			CommonPrefixes: []types.CommonPrefix{
				{Prefix: aws.String("a/")},
				{Prefix: aws.String("b/")},
				{Prefix: aws.String("c/")},
			},
			IsTruncated: aws.Bool(false),
		}, nil).
		Once()

	out, err := (&legacyClient{Client: client}).
		ListObjectsV2(cs.T().Context(), &s3.ListObjectsV2Input{Bucket: aws.String("bucket")})
	cs.Require().NoError(err)
	cs.Equal(int32(5), aws.ToInt32(out.KeyCount), "KeyCount should count both keys and prefixes")
}

// TestLegacyClientListObjectsV2ForwardsOptions verifies per-request option functions reach the
// underlying operation rather than being dropped by the adapter.
func (cs *clientTestSuite) TestLegacyClientListObjectsV2ForwardsOptions() {
	client := mocks.NewClient(cs.T())

	called := false
	opt := func(*s3.Options) { called = true }

	client.EXPECT().
		ListObjects(matchContext, &s3.ListObjectsInput{Bucket: aws.String("bucket")}, mock.Anything).
		RunAndReturn(func(_ context.Context, _ *s3.ListObjectsInput, opts ...func(*s3.Options)) (*s3.ListObjectsOutput, error) {
			cs.Require().Len(opts, 1, "the option function should be forwarded")
			opts[0](&s3.Options{})
			return &s3.ListObjectsOutput{IsTruncated: aws.Bool(false)}, nil
		}).
		Once()

	_, err := (&legacyClient{Client: client}).
		ListObjectsV2(cs.T().Context(), &s3.ListObjectsV2Input{Bucket: aws.String("bucket")}, opt)
	cs.Require().NoError(err)
	cs.True(called, "the forwarded option should be the one that was passed in")
}

// TestLegacyClientListObjectsV2Passthrough covers the response fields that carry across unchanged.
func (cs *clientTestSuite) TestLegacyClientListObjectsV2Passthrough() {
	client := mocks.NewClient(cs.T())
	commonPrefixes := []types.CommonPrefix{{Prefix: aws.String("dir1/sub/")}}
	client.EXPECT().
		ListObjects(matchContext, &s3.ListObjectsInput{Bucket: aws.String("bucket")}).
		Return(&s3.ListObjectsOutput{
			CommonPrefixes: commonPrefixes,
			Delimiter:      aws.String("/"),
			EncodingType:   types.EncodingTypeUrl,
			IsTruncated:    aws.Bool(false),
			MaxKeys:        aws.Int32(1000),
			Name:           aws.String("bucket"),
			Prefix:         aws.String("dir1/"),
			RequestCharged: types.RequestChargedRequester,
		}, nil).
		Once()

	in := &s3.ListObjectsV2Input{Bucket: aws.String("bucket")}

	out, err := (&legacyClient{Client: client}).ListObjectsV2(cs.T().Context(), in)
	cs.Require().NoError(err)

	cs.Equal(commonPrefixes, out.CommonPrefixes)
	cs.Equal("/", aws.ToString(out.Delimiter))
	cs.Equal(types.EncodingTypeUrl, out.EncodingType)
	cs.Equal(int32(1000), aws.ToInt32(out.MaxKeys))
	cs.Equal("bucket", aws.ToString(out.Name))
	cs.Equal("dir1/", aws.ToString(out.Prefix))
	cs.Equal(types.RequestChargedRequester, out.RequestCharged)
}

func TestClient(t *testing.T) {
	suite.Run(t, new(clientTestSuite))
}
