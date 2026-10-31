package s3

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/s3/mocks"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

type listEntriesTestSuite struct {
	suite.Suite
	client *sdkStyleClient
	fs     *FileSystem
}

func TestListEntriesTestSuite(t *testing.T) {
	suite.Run(t, new(listEntriesTestSuite))
}

func (s *listEntriesTestSuite) SetupTest() {
	s.client = newSDKStyleClient(s.T())
	s.fs = &FileSystem{client: s.client}
}

func (s *listEntriesTestSuite) location(path string) vfs.Location {
	s.T().Helper()
	loc, err := s.fs.NewLocation("bucket", path)
	s.Require().NoError(err)
	return loc
}

func (s *listEntriesTestSuite) keys(loc vfs.Location, opts ...options.ListOption) []string {
	s.T().Helper()
	entries, err := list.Collect(list.Entries(s.T().Context(), loc, opts...))
	s.Require().NoError(err)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func objects(keys ...string) []types.Object {
	objs := make([]types.Object, len(keys))
	for i, key := range keys {
		objs[i] = types.Object{Key: aws.String(key), Size: aws.Int64(int64(len(key))), LastModified: aws.Time(time.Unix(1_700_000_000, 0))}
	}
	return objs
}

func commonPrefixes(prefixes ...string) []types.CommonPrefix {
	out := make([]types.CommonPrefix, len(prefixes))
	for i, p := range prefixes {
		out[i] = types.CommonPrefix{Prefix: aws.String(p)}
	}
	return out
}

// expectList expects one ListObjectsV2 call with exactly the given request and answers it.
func (s *listEntriesTestSuite) expectList(in *s3.ListObjectsV2Input, out *s3.ListObjectsV2Output) {
	s.client.v2.EXPECT().ListObjectsV2(mock.Anything, in).Return(out, nil).Once()
}

type fakeAPIError struct{ code string }

func (e fakeAPIError) Error() string     { return e.code }
func (e fakeAPIError) ErrorCode() string { return e.code }

func (s *listEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location("/dir1/"))
}

func (s *listEntriesTestSuite) TestPagesOfAThousandKeysMakeOneRequestEach() {
	const total = 2500
	all := make([]string, total)
	for i := range all {
		all[i] = fmt.Sprintf("dir1/k%04d", i)
	}
	page := func(from, to int, truncated bool) *s3.ListObjectsV2Output {
		return &s3.ListObjectsV2Output{Contents: objects(all[from:to]...), IsTruncated: aws.Bool(truncated)}
	}
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/")}, page(0, 1000, true))
	s.expectList(&s3.ListObjectsV2Input{
		Bucket: aws.String("bucket"), Prefix: aws.String("dir1/"), StartAfter: aws.String("dir1/k0999"),
	}, page(1000, 2000, true))
	s.expectList(&s3.ListObjectsV2Input{
		Bucket: aws.String("bucket"), Prefix: aws.String("dir1/"), StartAfter: aws.String("dir1/k1999"),
	}, page(2000, total, false))

	keys := s.keys(s.location("/dir1/"), list.Recursive())

	s.Require().Len(keys, total)
	s.Equal("k0000", keys[0])
	s.Equal("k2499", keys[total-1])
	// The client is a strict mock: any HeadObject call, or a fourth list call, would have failed the test.
}

func (s *listEntriesTestSuite) TestNonRecursiveListingRollsUpAtTheDelimiter() {
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/"), Delimiter: aws.String("/")},
		&s3.ListObjectsV2Output{
			Contents:       objects("dir1/", "dir1/a.txt", "dir1/z.txt"),
			CommonPrefixes: commonPrefixes("dir1/logs/", "dir1/sub/"),
		})

	s.Equal([]string{"a.txt", "logs/", "sub/", "z.txt"}, s.keys(s.location("/dir1/")))
}

func (s *listEntriesTestSuite) TestResumesPastARolledUpDirectory() {
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/"), Delimiter: aws.String("/")},
		&s3.ListObjectsV2Output{
			Contents:       objects("dir1/a.txt"),
			CommonPrefixes: commonPrefixes("dir1/logs/"),
			IsTruncated:    aws.Bool(true),
		})
	s.expectList(&s3.ListObjectsV2Input{
		Bucket: aws.String("bucket"), Prefix: aws.String("dir1/"), Delimiter: aws.String("/"),
		StartAfter: aws.String("dir1/logs/\U0010FFFF"),
	}, &s3.ListObjectsV2Output{Contents: objects("dir1/z.txt")})

	s.Equal([]string{"a.txt", "logs/", "z.txt"}, s.keys(s.location("/dir1/")))
}

func (s *listEntriesTestSuite) TestBucketRootHasNoPrefix() {
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Delimiter: aws.String("/")},
		&s3.ListObjectsV2Output{Contents: objects("a.txt"), CommonPrefixes: commonPrefixes("logs/")})

	s.Equal([]string{"a.txt", "logs/"}, s.keys(s.location("/")))
}

func (s *listEntriesTestSuite) TestDirectoriesAreSynthesizedFromKeys() {
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/")},
		&s3.ListObjectsV2Output{Contents: objects("dir1/a/b/c.txt", "dir1/folder/")})

	s.Equal([]string{"a/", "a/b/", "a/b/c.txt", "folder/"}, s.keys(s.location("/dir1/"), list.Recursive()))
}

func (s *listEntriesTestSuite) TestPrefixAndCursorAreSentToS3() {
	s.expectList(&s3.ListObjectsV2Input{
		Bucket: aws.String("bucket"), Prefix: aws.String("dir1/logs/o"), Delimiter: aws.String("/"),
		StartAfter: aws.String("dir1/logs/old/"),
	}, &s3.ListObjectsV2Output{Contents: objects("dir1/logs/other.log")})

	s.Equal([]string{"logs/other.log"}, s.keys(s.location("/dir1/"), list.Prefix("logs/o"), list.StartAfter("logs/old/")))
}

func (s *listEntriesTestSuite) TestEntriesCarryTheListedMetadata() {
	listed := objects("dir1/a.txt")
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/")},
		&s3.ListObjectsV2Output{Contents: listed})

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/"), list.Recursive()))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	e := entries[0]
	s.True(e.HasMetadata())
	s.Equal(uint64(len("dir1/a.txt")), e.Size())
	s.True(listed[0].LastModified.Equal(e.ModTime()))
	s.Equal(listed[0], e.Sys())
	info, err := e.Info()
	s.Require().NoError(err, "listed metadata needs no HeadObject request")
	s.Equal(int64(len("dir1/a.txt")), info.Size())
	s.Equal("/dir1/a.txt", e.Path())
}

func (s *listEntriesTestSuite) TestStoppingEarlyStopsTheRequests() {
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/")},
		&s3.ListObjectsV2Output{Contents: objects("dir1/a.txt", "dir1/b.txt"), IsTruncated: aws.Bool(true)})

	for _, err := range s.location("/dir1/").(vfs.EntryLister).ListEntries(s.T().Context(), list.Recursive()) {
		s.Require().NoError(err)
		break
	}
}

func (s *listEntriesTestSuite) TestAccessDeniedMatchesErrPermission() {
	s.client.v2.EXPECT().ListObjectsV2(mock.Anything, mock.Anything).Return(nil, fakeAPIError{code: "AccessDenied"}).Once()

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/")))

	s.Require().ErrorIs(err, iofs.ErrPermission)
	var pe *iofs.PathError
	s.Require().ErrorAs(err, &pe)
	s.Equal("list", pe.Op)
}

func (s *listEntriesTestSuite) TestMissingBucketIsAnError() {
	s.client.v2.EXPECT().ListObjectsV2(mock.Anything, mock.Anything).Return(nil, &types.NoSuchBucket{}).Once()

	_, err := list.Collect(list.Entries(s.T().Context(), s.location("/dir1/")))

	var noBucket *types.NoSuchBucket
	s.Require().ErrorAs(err, &noBucket)
	s.NotErrorIs(err, iofs.ErrNotExist)
}

func (s *listEntriesTestSuite) TestOtherErrorsEndTheListing() {
	boom := errors.New("boom")
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/")},
		&s3.ListObjectsV2Output{Contents: objects("dir1/a.txt"), IsTruncated: aws.Bool(true)})
	s.client.v2.EXPECT().ListObjectsV2(mock.Anything, mock.MatchedBy(func(in *s3.ListObjectsV2Input) bool {
		return in.StartAfter != nil
	})).Return(nil, boom).Once()

	var keys []string
	var errs []error
	for e, err := range list.Entries(s.T().Context(), s.location("/dir1/"), list.Recursive()) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		keys = append(keys, e.RelPath())
	}

	s.Equal([]string{"a.txt"}, keys)
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], boom)
}

func (s *listEntriesTestSuite) TestInvalidOptionsMakeNoRequest() {
	var errs []error
	for _, err := range s.location("/dir1/").(vfs.EntryLister).ListEntries(s.T().Context(), list.FilesOnly(), list.DirsOnly()) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], list.ErrInvalidOption)
}

func (s *listEntriesTestSuite) TestCanceledContextMakesNoRequest() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	var errs []error
	for _, err := range s.location("/dir1/").(vfs.EntryLister).ListEntries(ctx) {
		errs = append(errs, err)
	}

	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], context.Canceled)
}

func (s *listEntriesTestSuite) TestClientErrorIsReported() {
	fileSystem := &FileSystem{clientErr: errors.New("no credentials")}
	loc, err := fileSystem.NewLocation("bucket", "/dir1/")
	s.Require().NoError(err)

	_, err = list.Collect(list.Entries(s.T().Context(), loc))

	s.Require().ErrorContains(err, "no credentials")
}

func (s *listEntriesTestSuite) TestLegacyClientsResumeWithAMarker() {
	legacy := mocks.NewClient(s.T())
	loc, err := (&FileSystem{client: legacy}).NewLocation("bucket", "/dir1/")
	s.Require().NoError(err)
	legacy.EXPECT().ListObjects(mock.Anything, &s3.ListObjectsInput{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/")}).
		Return(&s3.ListObjectsOutput{Contents: objects("dir1/a.txt"), IsTruncated: aws.Bool(true)}, nil).Once()
	legacy.EXPECT().ListObjects(mock.Anything, &s3.ListObjectsInput{
		Bucket: aws.String("bucket"), Prefix: aws.String("dir1/"), Marker: aws.String("dir1/a.txt"),
	}).Return(&s3.ListObjectsOutput{Contents: objects("dir1/b.txt")}, nil).Once()

	s.Equal([]string{"a.txt", "b.txt"}, s.keys(loc, list.Recursive()))
}

func (s *listEntriesTestSuite) TestNilKeysAndPrefixesAreSkipped() {
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/")},
		&s3.ListObjectsV2Output{
			Contents:       []types.Object{{}, {Key: aws.String("dir1/a.txt")}},
			CommonPrefixes: []types.CommonPrefix{{}},
		})

	s.Equal([]string{"a.txt"}, s.keys(s.location("/dir1/"), list.Recursive()))
}

func (s *listEntriesTestSuite) TestNilResponseIsAnEmptyListing() {
	s.client.v2.EXPECT().ListObjectsV2(mock.Anything, mock.Anything).Return(nil, nil).Once()

	s.Empty(s.keys(s.location("/dir1/"), list.Recursive()))
}

// S3 rolls the directory a cursor sits in or under up again on a delimited listing, since keys after the cursor still
// share its prefix. The listing must drop that directory rather than yield it a second time.
func (s *listEntriesTestSuite) TestCursorInsideADirectoryDoesNotRepeatIt() {
	tests := []struct {
		name   string
		cursor string
	}{
		{name: "cursor on the directory itself", cursor: "logs/"},
		{name: "cursor deep inside the directory", cursor: "logs/old/2024.log"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.v2.EXPECT().ListObjectsV2(mock.Anything, &s3.ListObjectsV2Input{
				Bucket: aws.String("bucket"), Prefix: aws.String("dir1/"), Delimiter: aws.String("/"),
				StartAfter: aws.String("dir1/" + tt.cursor),
			}).Return(&s3.ListObjectsV2Output{
				Contents:       objects("dir1/z.txt"),
				CommonPrefixes: commonPrefixes("dir1/logs/"),
			}, nil).Once()

			s.Equal([]string{"z.txt"}, s.keys(s.location("/dir1/"), list.StartAfter(tt.cursor)))
		})
	}
}

func (s *listEntriesTestSuite) TestContextReachesTheClient() {
	type ctxKey struct{}
	ctx := context.WithValue(s.T().Context(), ctxKey{}, "marker")
	s.client.v2.EXPECT().ListObjectsV2(mock.MatchedBy(func(c context.Context) bool {
		return c.Value(ctxKey{}) == "marker"
	}), mock.Anything).Return(&s3.ListObjectsV2Output{}, nil).Once()

	entries, err := list.Collect(list.Entries(ctx, s.location("/dir1/")))

	s.Require().NoError(err)
	s.Empty(entries)
}

// TestOutOfOrderPagesEndTheListing covers a server that does not return keys in byte order, as S3 directory buckets
// do: a page that starts before the end of the previous one must be an error, not a silently wrong listing.
func (s *listEntriesTestSuite) TestOutOfOrderPagesEndTheListing() {
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/")},
		&s3.ListObjectsV2Output{Contents: objects("dir1/b.txt"), IsTruncated: aws.Bool(true)})
	s.expectList(&s3.ListObjectsV2Input{Bucket: aws.String("bucket"), Prefix: aws.String("dir1/"), StartAfter: aws.String("dir1/b.txt")},
		&s3.ListObjectsV2Output{Contents: objects("dir1/a.txt")})

	var keys []string
	var errs []error
	for e, err := range list.Entries(s.T().Context(), s.location("/dir1/"), list.Recursive()) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		keys = append(keys, e.RelPath())
	}

	s.Equal([]string{"b.txt"}, keys)
	s.Require().Len(errs, 1)
	s.Require().ErrorContains(errs[0], "byte order")
}
