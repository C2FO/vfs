/*
Package vfssimple provides a basic and easy to use set of functions to any supported backend filesystem by using full URI's:
  * Local OS:             file:///some/path/to/file.txt
  * Amazon S3:            s3://mybucket/path/to/file.txt
  * Google Cloud Storage: gs://mybucket/path/to/file.txt

Usage

Just import vfssimple.

  package main

  import(
	"github.com/c2fo/vfs/vfssimple"
  )

  ...

  func DoSomething() error {
    myLocalDir, err := vfssimple.NewLocation("file:///tmp/")
    if err != nil {
        return err
    }

    myS3File, err := vfssimple.NewFile("s3://mybucket/some/path/to/key.txt")
    if err != nil {
        return err
    }

    localFile, err := myS3File.MoveToLocation(myLocalDir)
    if err != nil {
        return err
    }

  }

Authentication and Options

vfssimple is largely an example of how to initialize a set of backend filesystems.  It only provides a default
initialization of the individual file systems.  See backend docs for specific authentication info for each backend but
generally speaking, most backends can use Environment variables to set credentials or client options.

File systems can only use one set of options. If you would like to configure more than one file system of the same type/schema with separate credentials,
you can register and map file system options to locations or individual objects. The vfssimple library will automatically try to
resolve the provided URI in NewFile() or NewLocation() to the registered file system.

  package main

  import(
	"github.com/c2fo/vfs/vfssimple"
	"github.com/c2fo/vfs/backend/s3"
  )

  ...

  func DoSomething() error {
	bucketAuth := s3.NewFileSystem().WithOptions(s3.Options{
		AccessKeyID:     "key1",
		SecretAccessKey: "secret1,
		Region:          "us-west-2",
	})

	fileAuth := s3.NewFileSystem().WithOptions(s3.Options{
		AccessKeyID:     "key2",
		SecretAccessKey: "secret2,
		Region:          "us-west-2",
	})

	backend.Register("s3://bucket1/, bucket1)
	backend.Register("s3://bucket2/file.txt, fileAuth)

    secureFile, _ := vfssimple.NewFile("s3://bucket2/file.txt")
	publicLocation, _ := vfssimple.NewLocation("s3://bucket1/")

	secureFile.CopyToLocation(publicLocation)
  }

To do more, especially if you need to pass in specific vfs.Option's via WithOption() or perhaps a mock client for testing via
WithClient() or something else, you'd need to implement your own factory.  See github.com/c2fo/vfs/backend for more information.
*/
package vfssimple
