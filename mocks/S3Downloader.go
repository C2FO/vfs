package mocks

import (
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type S3MockDownloader struct {
	SideEffect       error
	Downloaded       int64
	IsCalled         bool
	ExpectedContents string
}

func (m S3MockDownloader) Download(w io.WriterAt, input *s3.GetObjectInput, options ...func(downloader *s3manager.Downloader)) (n int64, err error) {
	m.IsCalled = true
	written, err := w.WriteAt([]byte(m.ExpectedContents), 0)
	if err != nil {
		return 0, err
	}
	return int64(written), m.SideEffect
}

func (m S3MockDownloader) DownloadWithContext(ctx aws.Context, w io.WriterAt, input *s3.GetObjectInput, options ...func(*s3manager.Downloader)) (n int64, err error) {
	return m.Downloaded, m.SideEffect
}

func (m S3MockDownloader) DownloadWithIterator(ctx aws.Context, iter s3manager.BatchDownloadIterator, opts ...func(*s3manager.Downloader)) error {
	return m.SideEffect
}
