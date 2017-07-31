package vfssimple

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/c2fo/vfs"
	"github.com/c2fo/vfs/gs"
	"github.com/c2fo/vfs/os"
	_s3 "github.com/c2fo/vfs/s3"
)

var (
	fileSystems map[string]vfs.FileSystem
)

func InitializeLocalFileSystem() {
	fileSystems[os.Scheme] = vfs.FileSystem(os.FileSystem{})
}

func InitializeGSFileSystem() error {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}
	fileSystems[gs.Scheme] = vfs.FileSystem(gs.NewFileSystem(ctx, client))
	return nil
}

// InitializeS3FileSystem will handle the bare minimum requirements for setting up an s3.FileSystem
// by setting up an s3 client with the accessKeyId and secreteAccessKey (both required), and an optional
// session token. It is required before making calls to vfs.NewLocation or vfs.NewFile with s3 URIs
// to have set this up ahead of time. If you require more in depth configuration of the s3 Client you
// may set one up yourself and pass the resulting s3iface.S3API to vfs.SetS3Client which will also
// fulfil this requirement.
func InitializeS3FileSystem(accessKeyId, secretAccessKey, token string) error {
	if accessKeyId == "" {
		return errors.New("accessKeyId argument of InitializeS3FileSystem cannot be an empty string.")
	}
	if secretAccessKey == "" {
		return errors.New("secretAccessKey argument of InitializeS3FileSystem cannot be an empty string.")
	}
	auth := credentials.NewStaticCredentials(accessKeyId, secretAccessKey, token)
	awsConfig := aws.NewConfig().WithCredentials(auth)
	awsSession, err := session.NewSession(awsConfig)
	if err != nil {
		return err
	}

	SetS3Client(s3.New(awsSession))
	return nil
}

// SetS3Client configures an s3.FileSystem with the client passed to it. This will be used by vfs when
// calling vfs.NewLocation or vfs.NewFile with an s3 URI. If you don't want to bother configuring the
// client manually vfs.InitializeS3FileSystem() will handle the client set up with the minimum
// required arguments (an access key id and secret access key.)
func SetS3Client(client s3iface.S3API) {
	fileSystems[_s3.Scheme] = vfs.FileSystem(_s3.FileSystem{client})
}

// NewLocation is a convenience function that allows for instantiating a location based on a uri string.
// "file://", "s3://", and "gs://" are supported, assuming they have been configured ahead of time.
func NewLocation(uri string) (vfs.Location, error) {
	u, err := parseSupportedURI(uri)
	if err != nil {
		return nil, err
	}

	return fileSystems[u.Scheme].NewLocation(u.Host, u.Path)
}

// NewFile is a convenience function that allows for instantiating a file based on a uri string. Any
// supported file system is supported, though some may require prior configuration. See the docs for
// specific requirements of each.
func NewFile(uri string) (vfs.File, error) {
	u, err := parseSupportedURI(uri)
	if err != nil {
		return nil, err
	}

	return fileSystems[u.Scheme].NewFile(u.Host, u.Path)
}

func parseSupportedURI(uri string) (*url.URL, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case gs.Scheme:
		if _, ok := fileSystems[gs.Scheme]; ok {
			return u, nil
		} else {
			return nil, fmt.Errorf("gs is a supported scheme but must be initialized. Call vfs.InitializeGSFileSystem() first.")
		}
		return u, nil
	case os.Scheme:
		if _, ok := fileSystems[os.Scheme]; ok {
			return u, nil
		} else {
			return nil, fmt.Errorf("file is a supported scheme but must be initialized. Call vfs.InitializeLocalFileSystem() first.")
		}
	case _s3.Scheme:
		if _, ok := fileSystems[_s3.Scheme]; ok {
			return u, nil
		} else {
			return nil, fmt.Errorf("s3 is a supported scheme but must be intialized. Call vfs.InitializeS3FileSystem(accessKeyId, secretAccessKey, token string), or vfs.SetS3Client(client s3iface.S3API) first.")
		}
	default:
		return nil, fmt.Errorf("scheme [%s] is not supported.", u.Scheme)
	}
}
