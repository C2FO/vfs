package gs

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/utils"

	"github.com/fsouza/fake-gcs-server/fakestorage"
)

type locationTestSuite struct {
	suite.Suite
}

type Objects []fakestorage.Object

func (lt *locationTestSuite) TestList() {
	bucket := "fake-bucket"
	fileCount := 3
	dirCount := 3
	objectNames := []string{}
	objectPrefixes := []string{}
	objects := Objects{}
	objectBaseNameSet := map[string]struct{}{}
	var createObjects func(prefix string, level int, levels int)
	createObjects = func(prefix string, level int, levels int) {
		objectPrefixes = append(objectPrefixes, prefix)
		for idx := 0; idx < fileCount; idx++ {
			objectBaseName := fmt.Sprintf("f%d.txt", idx)
			objectName := fmt.Sprintf("%s%s", prefix, objectBaseName)
			objectNames = append(objectNames, objectName)
			objectBaseNameSet[objectBaseName] = struct{}{}
			objects = append(objects, fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName:      bucket,
					Name:            objectName,
					ContentType:     "text/plain",
					ContentEncoding: "utf8",
				},
				Content: []byte(objectName + " content"),
			})
		}
		if levels > 0 {
			for idx := 0; idx < dirCount; idx++ {
				createObjects(fmt.Sprintf("%sd%d/", prefix, idx), level+1, levels-1)
			}
		}
	}
	createObjects("", 0, 2)

	lt.T().Logf("objectNames = %s", objectNames)
	lt.T().Logf("objectPrefixes = %s", objectPrefixes)
	lt.T().Logf("objectBaseNameSet = %v", objectBaseNameSet)

	objectBaseNames := []string{}
	for name := range objectBaseNameSet {
		objectBaseNames = append(objectBaseNames, name)
	}
	lt.Len(objectBaseNames, fileCount)

	server := fakestorage.NewServer(objects)
	defer server.Stop()

	fs := NewFileSystem().WithClient(server.Client())
	for _, objectPrefix := range objectPrefixes {
		lt.Run("list direct "+objectPrefix, func() {
			loc, err := fs.NewLocation(bucket, "/"+objectPrefix)
			lt.Require().NoError(err)
			lt.T().Logf("location URI: %q", loc.URI())

			files, err := loc.List()
			lt.Require().NoError(err)
			lt.ElementsMatch(objectBaseNames, files, "should find all files in the location")
		})
		lt.Run("list prefix "+objectPrefix, func() {
			loc, err := fs.NewLocation(bucket, "/")
			lt.Require().NoError(err)
			lt.T().Logf("location URI: %q", loc.URI())

			lt.Run("without slash", func() {
				files, err := loc.ListByPrefix(objectPrefix)
				lt.Require().NoError(err)
				lt.ElementsMatch(objectBaseNames, files, "should find all files in the location")
			})
			lt.Run("with slash", func() {
				files, err := loc.ListByPrefix(objectPrefix + "/")
				lt.Require().NoError(err)
				lt.ElementsMatch(objectBaseNames, files, "should find all files in the location")
			})
			lt.Run("include object-level filename prefix f2", func() {
				files, err := loc.ListByPrefix(objectPrefix + "/f2")
				lt.Require().NoError(err)
				fileObjectBaseNames := []string{"f2.txt"}
				lt.ElementsMatch(fileObjectBaseNames, files, "should find all files in the location matching f2")
			})
		})
		lt.Run("list regex "+objectPrefix, func() {
			loc, err := fs.NewLocation(bucket, "/"+objectPrefix)
			lt.Require().NoError(err)
			lt.T().Logf("location URI: %q", loc.URI())

			files, err := loc.ListByRegex(regexp.MustCompile("^f[02].txt$"))
			lt.Require().NoError(err)
			lt.ElementsMatch([]string{"f0.txt", "f2.txt"}, files,
				"should find exactly two files f0.txt and f2.txt")
		})
	}
}

func (lt *locationTestSuite) TestVolume() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	bucket := "c2fo-vfs-a"
	loc, err := fs.NewLocation(bucket, "/")
	lt.NoError(err)
	lt.Equal(bucket, loc.Volume(), "Volume() should return the bucket name on location.")
}

func (lt *locationTestSuite) TestPath() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	loc, err := fs.NewLocation("bucket", "/path/")
	lt.NoError(err)
	lt.Equal("/path/", loc.Path(), "Path() should return the path on location.")

	loc2, err2 := fs.NewLocation("bucket", "/path/../newpath/")
	lt.NoError(err2)
	lt.Equal("/newpath/", loc2.Path(), "Path() should return the path on location.")

	loc3, err3 := fs.NewLocation("bucket", "/path/./to/")
	lt.NoError(err3)
	lt.Equal("/path/to/", loc3.Path(), "Path() should return the path on location.")
}

func (lt *locationTestSuite) TestNewFile() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	loc, err := fs.NewLocation("bucket", "/some/path/to/")
	lt.NoError(err)
	lt.Equal("/some/path/to/", loc.Path(), "Path() should return the path on location.")

	newfile, _ := loc.NewFile("a/file.txt")
	lt.Equal("/some/path/to/a/file.txt", newfile.Path(), "NewFile relative path works")

	newrelfile, _ := loc.NewFile("../../where/file.txt")
	lt.Equal("/some/where/file.txt", newrelfile.Path(), "Newfile relative dot path works")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewFile("/path/to/file.txt")
	lt.EqualError(err, "non-nil gs.Location pointer is required", "errors returned by NewFile")

	// test empty path error
	_, err = loc.NewFile("")
	lt.EqualError(err, "non-empty string filePath is required", "errors returned by NewFile")

	// test validation error
	_, err = loc.NewFile("/absolute/path/to/file.txt")
	lt.EqualError(err, utils.ErrBadRelFilePath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestExists_true() {
	bucket := "foo"
	server := fakestorage.NewServer(Objects{
		fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName:      bucket,
				Name:            "file.txt",
				ContentType:     "text/plain",
				ContentEncoding: "utf8",
			},
			Content: []byte("content"),
		}})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())
	loc, err := fs.NewLocation(bucket, "/")
	lt.NoError(err)
	exists, err := loc.Exists()
	lt.NoError(err, "No error expected from Exists")
	lt.True(exists, "Call to Exists expected to return true.")
}

func (lt *locationTestSuite) TestExists_false() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())
	bucket := "foo"
	loc, err := fs.NewLocation(bucket, "/")
	lt.NoError(err)
	exists, err := loc.Exists()
	lt.NoError(err, "No error expected from Exists")
	lt.False(exists, "Call to Exists expected to return true.")
}

func (lt *locationTestSuite) TestChangeDir() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	// test nil Location
	var nilLoc *Location
	err := nilLoc.ChangeDir("path/to/")
	lt.EqualErrorf(err, "non-nil gs.Location pointer is required", "error expected for nil location")

	loc := &Location{fileSystem: fs, prefix: "/", bucket: "bucket"}

	err1 := loc.ChangeDir("../")
	lt.NoError(err1, "no error expected")
	lt.Equal("/", loc.Path())

	err2 := loc.ChangeDir("hello/")
	lt.NoError(err2, "no error expected")
	lt.Equal("/hello/", loc.Path())

	err3 := loc.ChangeDir("../.././../")
	lt.NoError(err3, "no error expected")
	lt.Equal("/", loc.Path())

	err4 := loc.ChangeDir("here/is/a/path/")
	lt.NoError(err4, "no error expected")
	lt.Equal("/here/is/a/path/", loc.Path())

	err5 := loc.ChangeDir("../")
	lt.NoError(err5, "no error expected")
	lt.Equal("/here/is/a/", loc.Path())
}

func (lt *locationTestSuite) TestNewLocation() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	loc, err := fs.NewLocation("bucket", "/old/")
	lt.NoError(err)
	newLoc, err := loc.NewLocation("new/path/")
	lt.NoError(err, "No error from successful call to NewLocation")
	lt.Equal("/old/new/path/", newLoc.Path(), "New location should have correct path set")
	lt.Equal("/old/", loc.Path(), "Ensure original path is unchanged.")

	newRelLoc, err := newLoc.NewLocation("../../some/path/")
	lt.NoError(err)
	lt.Equal("/old/some/path/", newRelLoc.Path(), "NewLocation works with rel dot paths")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewLocation("/path/to/")
	lt.EqualError(err, "non-nil gs.Location pointer is required", "errors returned by NewLocation")

	// test empty path error
	_, err = loc.NewLocation("")
	lt.EqualError(err, "non-empty string relativePath is required", "errors returned by NewLocation")

	// test validation error
	_, err = loc.NewLocation("/absolute/path/to/")
	lt.EqualError(err, utils.ErrBadRelLocationPath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestStringURI() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	loc := &Location{fileSystem: fs, prefix: "some/path/to/location", bucket: "mybucket"}
	lt.Equal("gs://mybucket/some/path/to/location/", loc.String(), "uri is returned")
}

func (lt *locationTestSuite) TestDeleteFile() {
	bucket := "bucket"
	server := fakestorage.NewServer(Objects{
		fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName:      bucket,
				Name:            "old/filename.txt",
				ContentType:     "text/plain",
				ContentEncoding: "utf8",
			},
			Content: []byte("content"),
		}})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	loc, err := fs.NewLocation(bucket, "/old/")
	lt.NoError(err)

	lt.Run("delete existing", func() {
		err = loc.DeleteFile("filename.txt")
		lt.NoError(err, "Successful delete should not return an error.")
	})

	lt.Run("delete non-existing", func() {
		err = loc.DeleteFile("filename.txt")
		lt.Error(err, "Delete of non existing file should fail")
	})
}

func TestLocation(t *testing.T) {
	suite.Run(t, new(locationTestSuite))

}
