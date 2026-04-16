package gs

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	"github.com/c2fo/vfs/v8/backend/testsuite"
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
		for idx := range fileCount {
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
			for idx := range dirCount {
				createObjects(fmt.Sprintf("%sd%d/", prefix, idx), level+1, levels-1)
			}
		}
	}
	createObjects("", 0, 2)

	lt.T().Logf("objectNames = %s", objectNames)
	lt.T().Logf("objectPrefixes = %s", objectPrefixes)
	lt.T().Logf("objectBaseNameSet = %v", objectBaseNameSet)

	objectBaseNames := make([]string, 0, len(objectBaseNameSet))
	for name := range objectBaseNameSet {
		objectBaseNames = append(objectBaseNames, name)
	}
	lt.Len(objectBaseNames, fileCount)

	server := fakestorage.NewServer(objects)
	defer server.Stop()

	fs := NewFileSystem(WithClient(server.Client()))
	ctx := context.Background()
	for _, objectPrefix := range objectPrefixes {
		lt.Run("list direct "+objectPrefix, func() {
			loc, err := fs.NewLocation(bucket, "/"+objectPrefix)
			lt.Require().NoError(err)
			lt.T().Logf("location URI: %q", loc.URI())

			files, err := testsuite.CollectList(ctx, loc)
			lt.Require().NoError(err)
			lt.ElementsMatch(objectBaseNames, files, "should find all files in the location")
		})
		lt.Run("list prefix "+objectPrefix, func() {
			loc, err := fs.NewLocation(bucket, "/")
			lt.Require().NoError(err)
			lt.T().Logf("location URI: %q", loc.URI())

			lt.Run("without slash", func() {
				files, err := testsuite.CollectList(ctx, loc, vfs.WithPrefix(objectPrefix))
				lt.Require().NoError(err)
				lt.ElementsMatch(objectBaseNames, files, "should find all files in the location")
			})
			lt.Run("with slash", func() {
				files, err := testsuite.CollectList(ctx, loc, vfs.WithPrefix(objectPrefix+"/"))
				lt.Require().NoError(err)
				lt.ElementsMatch(objectBaseNames, files, "should find all files in the location")
			})
			lt.Run("include object-level filename prefix f2", func() {
				files, err := testsuite.CollectList(ctx, loc, vfs.WithPrefix(objectPrefix+"/f2"))
				lt.Require().NoError(err)
				fileObjectBaseNames := []string{"f2.txt"}
				lt.ElementsMatch(fileObjectBaseNames, files, "should find all files in the location matching f2")
			})
		})
		lt.Run("list regex "+objectPrefix, func() {
			loc, err := fs.NewLocation(bucket, "/"+objectPrefix)
			lt.Require().NoError(err)
			lt.T().Logf("location URI: %q", loc.URI())

			files, err := testsuite.CollectList(ctx, loc, vfs.WithRegexp(regexp.MustCompile("^f[02].txt$")))
			lt.Require().NoError(err)
			lt.ElementsMatch([]string{"f0.txt", "f2.txt"}, files,
				"should find exactly two files f0.txt and f2.txt")
		})
	}
}

func (lt *locationTestSuite) TestPath() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem(WithClient(server.Client()))

	loc, err := fs.NewLocation("bucket", "/path/")
	lt.Require().NoError(err)
	lt.Equal("/path/", loc.Path(), "Path() should return the path on location.")

	loc2, err2 := fs.NewLocation("bucket", "/path/../newpath/")
	lt.Require().NoError(err2)
	lt.Equal("/newpath/", loc2.Path(), "Path() should return the path on location.")

	loc3, err3 := fs.NewLocation("bucket", "/path/./to/")
	lt.Require().NoError(err3)
	lt.Equal("/path/to/", loc3.Path(), "Path() should return the path on location.")
}

func (lt *locationTestSuite) TestNewFile() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem(WithClient(server.Client()))

	loc, err := fs.NewLocation("bucket", "/some/path/to/")
	lt.Require().NoError(err)
	lt.Equal("/some/path/to/", loc.Path(), "Path() should return the path on location.")

	newfile, _ := loc.NewFile("a/file.txt")
	lt.Equal("/some/path/to/a/file.txt", newfile.Path(), "NewFile relative path works")

	newrelfile, _ := loc.NewFile("../../where/file.txt")
	lt.Equal("/some/where/file.txt", newrelfile.Path(), "Newfile relative dot path works")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewFile("/path/to/file.txt")
	lt.Require().ErrorIs(err, errLocationRequired, "errors returned by NewFile")

	// test empty path error
	_, err = loc.NewFile("")
	lt.Require().ErrorIs(err, errPathRequired, "errors returned by NewFile")

	// test validation error
	_, err = loc.NewFile("/absolute/path/to/file.txt")
	lt.Require().ErrorIs(err, utils.ErrBadRelFilePath, "errors returned by NewLocation")

	// new tests for location update
	lt.Run("new file with relative path updates location", func() {
		newFile, err := loc.NewFile("../newfile.txt")
		lt.Require().NoError(err)
		lt.Equal("/some/path/newfile.txt", newFile.Path(), "NewFile with relative path should update location correctly")
		lt.Equal("/some/path/", newFile.Location().Path(), "NewFile with relative path should update location correctly")
	})

	lt.Run("new file with relative path to root", func() {
		newFile, err := loc.NewFile("../../../../newrootfile.txt")
		lt.Require().NoError(err)
		lt.Equal("/newrootfile.txt", newFile.Path(), "NewFile with relative path to root should update location correctly")
		lt.Equal("/", newFile.Location().Path(), "NewFile with relative path to root should update location correctly")
	})
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
	fs := NewFileSystem(WithClient(server.Client()))
	loc, err := fs.NewLocation(bucket, "/")
	lt.Require().NoError(err)
	exists, err := loc.Exists()
	lt.Require().NoError(err, "No error expected from Exists")
	lt.True(exists, "Call to Exists expected to return true.")
}

func (lt *locationTestSuite) TestExists_false() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem(WithClient(server.Client()))
	bucket := "foo"
	loc, err := fs.NewLocation(bucket, "/")
	lt.Require().NoError(err)
	exists, err := loc.Exists()
	lt.Require().NoError(err, "No error expected from Exists")
	lt.False(exists, "Call to Exists expected to return true.")
}

func (lt *locationTestSuite) TestNewLocation() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem(WithClient(server.Client()))

	loc, err := fs.NewLocation("bucket", "/old/")
	lt.Require().NoError(err)
	newLoc, err := loc.NewLocation("new/path/")
	lt.Require().NoError(err, "No error from successful call to NewLocation")
	lt.Equal("/old/new/path/", newLoc.Path(), "New location should have correct path set")
	lt.Equal("/old/", loc.Path(), "Ensure original path is unchanged.")

	newRelLoc, err := newLoc.NewLocation("../../some/path/")
	lt.Require().NoError(err)
	lt.Equal("/old/some/path/", newRelLoc.Path(), "NewLocation works with rel dot paths")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewLocation("/path/to/")
	lt.Require().ErrorIs(err, errLocationRequired, "errors returned by NewLocation")

	// test empty path error
	_, err = loc.NewLocation("")
	lt.Require().ErrorIs(err, errPathRequired, "errors returned by NewLocation")

	// test validation error
	_, err = loc.NewLocation("/absolute/path/to/")
	lt.Require().ErrorIs(err, utils.ErrBadRelLocationPath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestStringURI() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem(WithClient(server.Client()))
	auth, err := authority.NewAuthority("mybucket")
	lt.Require().NoError(err)
	loc := &Location{fileSystem: fs, prefix: "some/path/to/location", authority: auth}
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
	fs := NewFileSystem(WithClient(server.Client()))

	loc, err := fs.NewLocation(bucket, "/old/")
	lt.Require().NoError(err)

	lt.Run("delete existing", func() {
		err = loc.DeleteFile("filename.txt")
		lt.Require().NoError(err, "Successful delete should not return an error.")
	})

	lt.Run("delete non-existing", func() {
		err = loc.DeleteFile("filename.txt")
		lt.Require().Error(err, "Delete of non existing file should fail")
	})
}

func TestLocation(t *testing.T) {
	suite.Run(t, new(locationTestSuite))
}
