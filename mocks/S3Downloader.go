package mocks

import (
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// S3MockDownloader is a mock implementation of the s3.Downloader interface for the S3 backend.
type S3MockDownloader struct {
	SideEffect       error
	Downloaded       int64
	IsCalled         bool
	ExpectedContents string
}

// Download mocks the s3manager.Download function
func (m S3MockDownloader) Download(w io.WriterAt, _ *s3.GetObjectInput, _ ...func(downloader *s3manager.Downloader)) (n int64, err error) {
	m.IsCalled = true
	written, err := w.WriteAt([]byte(m.ExpectedContents), 0)
	if err != nil {
		return 0, err
	}
	return int64(written), m.SideEffect
}

// DownloadWithContext mocks the s3manager.DownloadWithContext function
func (m S3MockDownloader) DownloadWithContext(_ aws.Context, _ io.WriterAt, _ *s3.GetObjectInput, _ ...func(*s3manager.Downloader)) (n int64, err error) {
	return m.Downloaded, m.SideEffect
}

// DownloadWithIterator mocks the s3manager.DownloadWithIterator function
func (m S3MockDownloader) DownloadWithIterator(_ aws.Context, _ s3manager.BatchDownloadIterator, _ ...func(*s3manager.Downloader)) error {
	return m.SideEffect
}
