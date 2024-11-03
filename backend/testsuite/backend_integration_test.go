//go:build vfsintegration
// +build vfsintegration

package testsuite

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/azure"
	"github.com/c2fo/vfs/v6/backend/gs"
	"github.com/c2fo/vfs/v6/backend/sftp"
	"github.com/c2fo/vfs/v6/utils"
	"github.com/c2fo/vfs/v6/vfssimple"
)

type vfsTestSuite struct {
	suite.Suite
	testLocations map[string]vfs.Location
}

func buildExpectedURI(fs vfs.FileSystem, volume, path string) string {
	if fs.Name() == "azure" {
		azFs := fs.(*azure.FileSystem)
		return fmt.Sprintf("%s://%s/%s%s", fs.Scheme(), azFs.Host(), volume, path)
	}
	return fmt.Sprintf("%s://%s%s", fs.Scheme(), volume, path)
}

func (s *vfsTestSuite) SetupSuite() {
	locs := os.Getenv("VFS_INTEGRATION_LOCATIONS")
	s.testLocations = make(map[string]vfs.Location)
	for _, loc := range strings.Split(locs, ";") {
		l, err := vfssimple.NewLocation(loc)
		s.NoError(err)
		switch l.FileSystem().Scheme() {
		case "file":
			s.testLocations[l.FileSystem().Scheme()] = CopyOsLocation(l)
		case "s3":
			s.testLocations[l.FileSystem().Scheme()] = CopyS3Location(l)
		case "sftp":
			s.testLocations[l.FileSystem().Scheme()] = CopySFTPLocation(l)
		case "gs":
			s.testLocations[l.FileSystem().Scheme()] = CopyGSLocation(l)
		case "mem":
			s.testLocations[l.FileSystem().Scheme()] = CopyMemLocation(l)
		case "https":
			s.testLocations[l.FileSystem().Scheme()] = CopyAzureLocation(l)
		case "ftp":
			s.testLocations[l.FileSystem().Scheme()] = CopyFTPLocation(l)
		default:
			panic(fmt.Sprintf("unknown scheme: %s", l.FileSystem().Scheme()))
		}
	}

}

// Test File
func (s *vfsTestSuite) TestScheme() {
	for scheme, location := range s.testLocations {
		fmt.Printf("************** TESTING scheme: %s **************\n", scheme)
		s.FileSystem(location)
		s.Location(location)
		s.File(location)
	}
}

// Test FileSystem
func (s *vfsTestSuite) FileSystem(baseLoc vfs.Location) {
	fmt.Println("****** testing vfs.FileSystem ******")

	// setup FileSystem
	fs := baseLoc.FileSystem()
	// NewFile initializes a File on the specified volume at path 'absFilePath'.
	//
	//   * Accepts volume and an absolute file path.
	//   * Upon success, a vfs.File, representing the file's new path (location path + file relative path), will be returned.
	//   * On error, nil is returned for the file.
	//   * Note that not all file systems will have a "volume" and will therefore be "":
	//       file:///path/to/file has a volume of "" and name /path/to/file
	//     whereas
	//       s3://mybucket/path/to/file has a volume of "mybucket and name /path/to/file
	//     results in /tmp/dir1/newerdir/file.txt for the final vfs.File path.
	//   * The file may or may not already exist.
	filepaths := map[string]bool{
		"/path/to/file.txt":    true,
		"/path/./to/file.txt":  true,
		"/path/../to/file.txt": true,
		"path/to/file.txt":     false,
		"./path/to/file.txt":   false,
		"../path/to/":          false,
		"/path/to/":            false,
		"":                     false,
	}
	for name, validates := range filepaths {
		file, err := fs.NewFile(baseLoc.Volume(), name)
		if validates {
			s.NoError(err, "there should be no error")
			expected := buildExpectedURI(fs, baseLoc.Volume(), path.Clean(name))
			s.Equal(expected, file.URI(), "uri's should match")
		} else {
			s.Error(err, "should have validation error for scheme[%s] and name[%s]", fs.Scheme(), name)
		}
	}

	// NewLocation initializes a Location on the specified volume with the given path.
	//
	//   * Accepts volume and an absolute location path.
	//   * The file may or may not already exist. Note that on key-store file systems like S3 or GCS, paths never truly exist.
	//   * On error, nil is returned for the location.
	//
	// See NewFile for note on volume.
	locpaths := map[string]bool{
		"/path/to/":         true,
		"/path/./to/":       true,
		"/path/../to/":      true,
		"path/to/":          false,
		"./path/to/":        false,
		"../path/to/":       false,
		"/path/to/file.txt": false,
		"":                  false,
	}
	for name, validates := range locpaths {
		loc, err := fs.NewLocation(baseLoc.Volume(), name)
		if validates {
			s.NoError(err, "there should be no error")
			expected := buildExpectedURI(fs, baseLoc.Volume(), utils.EnsureTrailingSlash(path.Clean(name)))
			s.Equal(expected, loc.URI(), "uri's should match")

		} else {
			s.Error(err, "should have validation error for scheme[%s] and name[%s]", fs.Scheme(), name)
		}
	}
}

// Test Location
func (s *vfsTestSuite) Location(baseLoc vfs.Location) {
	fmt.Println("****** testing vfs.Location ******")

	srcLoc, err := baseLoc.NewLocation("locTestSrc/")
	s.NoError(err, "there should be no error")
	defer func() {
		// clean up srcLoc after test for OS
		if srcLoc.FileSystem().Scheme() == "file" {
			exists, err := srcLoc.Exists()
			s.Require().NoError(err)
			if exists {
				s.NoError(os.RemoveAll(srcLoc.Path()), "failed to clean up location test srcLoc")
			}
		}
	}()

	// NewLocation is an initializer for a new Location relative to the existing one.
	//
	// Given location:
	//     loc := fs.NewLocation(:s3://mybucket/some/path/to/")
	// calling:
	//     newLoc := loc.NewLocation("../../")
	// would return a new vfs.Location representing:
	//     s3://mybucket/some/
	//
	//   * Accepts a relative location path.
	locpaths := map[string]bool{
		"/path/to/":         false,
		"/path/./to/":       false,
		"/path/../to/":      false,
		"path/to/":          true,
		"./path/to/":        true,
		"../path/to/":       true,
		"/path/to/file.txt": false,
		"":                  false,
	}
	for name, validates := range locpaths {
		loc, err := srcLoc.NewLocation(name)
		if validates {
			s.NoError(err, "there should be no error")
			expected := buildExpectedURI(srcLoc.FileSystem(), baseLoc.Volume(), utils.EnsureTrailingSlash(path.Clean(path.Join(srcLoc.Path(), name))))
			s.Equal(expected, loc.URI(), "uri's should match")

		} else {
			s.Error(err, "should have validation error for scheme and name: %s : %s", srcLoc.FileSystem().Scheme(), name)
		}
	}

	// NewFile will instantiate a vfs.File instance at or relative to the current location's path.
	//
	//   * Accepts a relative file path.
	//   * In the case of an error, nil is returned for the file.
	//   * Resultant File path will be the shortest path name equivalent of combining the Location path and relative path, if any.
	//       ie, /tmp/dir1/ as location and relFilePath "newdir/./../newerdir/file.txt"
	//       results in /tmp/dir1/newerdir/file.txt for the final vfs.File path.
	//   * Upon success, a vfs.File, representing the file's new path (location path + file relative path), will be returned.
	//   * The file may or may not already exist.
	filepaths := map[string]bool{
		"/path/to/file.txt":    false,
		"/path/./to/file.txt":  false,
		"/path/../to/file.txt": false,
		"path/to/file.txt":     true,
		"./path/to/file.txt":   true,
		"../path/to/":          false,
		"../path/to/file.txt":  true,
		"/path/to/":            false,
		"":                     false,
	}
	for name, validates := range filepaths {
		file, err := srcLoc.NewFile(name)
		if validates {
			s.NoError(err, "there should be no error")
			expected := buildExpectedURI(srcLoc.FileSystem(), srcLoc.Volume(), path.Clean(path.Join(srcLoc.Path(), name)))
			s.Equal(expected, file.URI(), "uri's should match")
		} else {
			s.Error(err, "should have validation error for scheme and name: %s : +%s+", srcLoc.FileSystem().Scheme(), name)
		}
	}

	// ChangeDir updates the existing Location's path to the provided relative location path.

	// Given location:
	// 	   loc := fs.NewLocation("file:///some/path/to/")
	// calling:
	//     loc.ChangeDir("../../")
	// would update the current location instance to
	// file:///some/.
	//
	//   * ChangeDir accepts a relative location path.

	// setup test
	cdTestLoc, err := srcLoc.NewLocation("chdirTest/")
	s.NoError(err)

	s.Error(cdTestLoc.ChangeDir(""), "empty string should error")
	s.Error(cdTestLoc.ChangeDir("/home/"), "absolute path should error")
	s.Error(cdTestLoc.ChangeDir("file.txt"), "file should error")
	s.NoError(cdTestLoc.ChangeDir("l1dir1/./l2dir1/../l2dir2/"), "should be no error for relative path")

	// Path returns absolute location path, ie /some/path/to/.
	//	==== Path() string
	s.True(strings.HasSuffix(cdTestLoc.Path(), "locTestSrc/chdirTest/l1dir1/l2dir2/"), "should end with dot dirs resolved")
	s.True(strings.HasPrefix(cdTestLoc.Path(), "/"), "should start with slash (abs path)")

	// URI returns the fully qualified URI for the Location.  IE, s3://bucket/some/path/
	//
	// URI's for locations must always end with a separator character.
	s.True(strings.HasSuffix(cdTestLoc.URI(), "locTestSrc/chdirTest/l1dir1/l2dir2/"), "should end with dot dirs resolved")
	prefix := fmt.Sprintf("%s://", cdTestLoc.FileSystem().Scheme())
	s.True(strings.HasPrefix(cdTestLoc.URI(), prefix), "should start with schema and abs slash")

	/* Exists returns boolean if the location exists on the file system. Returns an error if any.

		   TODO: *************************************************************************************************************
			     note that Exists is not consistent among implementations. GCSs and S3 always return true if the bucket exist.
		         Fundamentally, why one wants to know if location exists is to know whether you're able to write there.  But
		         this feels unintuitive.
	         	 *************************************************************************************************************

		   Consider:

				// CREATE LOCATION INSTANCE
				loc, _ := vfssimple.NewLocation("scheme://vol/path/")

				// DO EXISTS CHECK ON LOCATION
		        if !loc.Exists() {
		            // CREATE LOCATION ON OS
				}

		        // CREATE FILE IN LOCATION AND DO WORK
		        myfile, _ := loc.NewFile("myfile.txt")
		        myfile.Write("write some text")
		        myfile.Close()


		    Now consider if the context is os/sftp OR gcs/s3/mem.

			==== Exists() (bool, error)
	*/
	exists, err := baseLoc.Exists()
	s.NoError(err)
	s.True(exists, "baseLoc location exists check")

	// setup list tests
	f1, err := srcLoc.NewFile("file1.txt")
	s.NoError(err)
	_, err = f1.Write([]byte("this is a test file"))
	s.NoError(err)
	s.NoError(f1.Close())

	f2, err := srcLoc.NewFile("file2.txt")
	s.NoError(err)
	s.NoError(f1.CopyToFile(f2))
	s.NoError(f1.Close())

	f3, err := srcLoc.NewFile("self.txt")
	s.NoError(err)
	s.NoError(f1.CopyToFile(f3))
	s.NoError(f1.Close())

	subLoc, err := srcLoc.NewLocation("somepath/")
	s.NoError(err)

	f4, err := subLoc.NewFile("that.txt")
	s.NoError(err)
	s.NoError(f1.CopyToFile(f4))
	s.NoError(f1.Close())

	// List returns a slice of strings representing the base names of the files found at the Location.
	//
	//   * All implementations are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location.
	//   * If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
	//     be checked first.
	//	====		List() ([]string, error)

	files, err := srcLoc.List()
	s.NoError(err)
	s.Len(files, 3, "list srcLoc location")

	files, err = subLoc.List()
	s.NoError(err)
	s.Len(files, 1, "list subLoc location")
	s.Equal("that.txt", files[0], "returned basename")

	files, err = cdTestLoc.List()
	s.NoError(err)
	s.Empty(files, "non-existent location")

	switch baseLoc.FileSystem().Scheme() {
	case "gs":
		fmt.Println("!!!!!! testing gs-specific List() tests !!!!!!")
		s.gsList(baseLoc)
	default:
	}

	// ListByPrefix returns a slice of strings representing the base names of the files found in Location whose filenames
	// match the given prefix.
	//
	//   * All implementations are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location.
	//   * "relative" prefixes are allowed, ie, ListByPrefix() from location "/some/path/" with prefix "to/somepattern"
	//     is the same as location "/some/path/to/" with prefix of "somepattern"
	//   * If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
	//     be checked first.
	//	====	ListByPrefix(prefix string) ([]string, error)

	files, err = srcLoc.ListByPrefix("file")
	s.NoError(err)
	s.Len(files, 2, "list srcLoc location matching prefix")

	files, err = srcLoc.ListByPrefix("s")
	s.NoError(err)
	s.Len(files, 1, "list srcLoc location")
	s.Equal("self.txt", files[0], "returned only file basename, not subdir matching prefix")

	files, err = srcLoc.ListByPrefix("somepath/t")
	s.NoError(err)
	s.Len(files, 1, "list 'somepath' location relative to srcLoc")
	s.Equal("that.txt", files[0], "returned only file basename, using relative prefix")

	files, err = cdTestLoc.List()
	s.NoError(err)
	s.Empty(files, "non-existent location")

	// ListByRegex returns a slice of strings representing the base names of the files found in the Location that matched the
	// given regular expression.
	//
	//   * All implementations are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location.
	//   * If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
	//     be checked first.
	//	====	ListByRegex(regex *regexp.Regexp) ([]string, error)

	files, err = srcLoc.ListByRegex(regexp.MustCompile("^f"))
	s.NoError(err)
	s.Len(files, 2, "list srcLoc location matching prefix")

	files, err = srcLoc.ListByRegex(regexp.MustCompile(`.txt$`))
	s.NoError(err)
	s.Len(files, 3, "list srcLoc location matching prefix")

	files, err = srcLoc.ListByRegex(regexp.MustCompile(`Z`))
	s.NoError(err)
	s.Empty(files, "list srcLoc location matching prefix")

	// DeleteFile deletes the file of the given name at the location.
	//
	// This is meant to be a short cut for instantiating a new file and calling delete on that, with all the necessary
	// error handling overhead.
	//
	// * Accepts relative file path.
	//
	//	====	DeleteFile(fileName string) error
	s.NoError(srcLoc.DeleteFile(f1.Name()), "deleteFile file1")
	s.NoError(srcLoc.DeleteFile(f2.Name()), "deleteFile file2")
	s.NoError(srcLoc.DeleteFile(f3.Name()), "deleteFile self.txt")
	s.NoError(srcLoc.DeleteFile("somepath/that.txt"), "deleted relative path")

	// should error if file doesn't exist
	s.Error(srcLoc.DeleteFile(f1.Path()), "deleteFile trying to delete a file already deleted")

}

// Test File
func (s *vfsTestSuite) File(baseLoc vfs.Location) {
	fmt.Println("****** testing vfs.File ******")
	srcLoc, err := baseLoc.NewLocation("fileTestSrc/")
	s.NoError(err)
	defer func() {
		// clean up srcLoc after test for OS
		if srcLoc.FileSystem().Scheme() == "file" {
			exists, err := srcLoc.Exists()
			s.Require().NoError(err)
			if exists {
				s.NoError(os.RemoveAll(srcLoc.Path()), "failed to clean up file test srcLoc")
			}
		}
	}()

	// setup srcFile
	srcFile, err := srcLoc.NewFile("srcFile.txt")
	s.NoError(err)

	/*
		Location returns the vfs.Location for the File.

		Location() Location
	*/

	/*
		io.Writer
	*/
	sz, err := srcFile.Write([]byte("this is a test\n"))
	s.NoError(err)
	s.EqualValues(15, sz)
	sz, err = srcFile.Write([]byte("and more text"))
	s.NoError(err)
	s.EqualValues(13, sz)

	/*
		io.Closer
	*/
	err = srcFile.Close()
	s.NoError(err)

	/*
		Exists returns boolean if the file exists on the file system.  Also returns an error if any.

		Exists() (bool, error)
	*/
	exists, err := srcFile.Exists()
	s.NoError(err)
	s.True(exists, "file exists")

	/*
		Name returns the base name of the file path.  For file:///some/path/to/file.txt, it would return file.txt

		Name() string
	*/
	s.Equal("srcFile.txt", srcFile.Name(), "name test")

	/*
		Path returns absolute path (with leading slash) including filename, ie /some/path/to/file.txt

		Path() string
	*/
	s.Equal(path.Join(baseLoc.Path(), "fileTestSrc/srcFile.txt"), srcFile.Path(), "path test")

	/*
		URI returns the fully qualified URI for the File.  IE, s3://bucket/some/path/to/file.txt

		URI() string
	*/
	s.Equal(baseLoc.URI()+"fileTestSrc/srcFile.txt", srcFile.URI(), "uri test")

	/*
		String() must be implemented to satisfy the stringer interface.  This ends up simply calling URI().

		fmt.Stringer
	*/
	s.Equal(baseLoc.URI()+"fileTestSrc/srcFile.txt", srcFile.String(), "string(er) explicit test")
	s.Equal(baseLoc.URI()+"fileTestSrc/srcFile.txt", fmt.Sprintf("%s", srcFile), "string(er) implicit test")

	/*
		Size returns the size of the file in bytes.

		Size() (uint64, error)
	*/
	b, err := srcFile.Size()
	s.NoError(err)
	s.EqualValues(28, b)

	/*
		LastModified returns the timestamp the file was last modified (as *time.Time).

		LastModified() (*time.Time, error)
	*/
	t, err := srcFile.LastModified()
	s.NoError(err)
	s.IsType((*time.Time)(nil), t, "last modified returned *time.Time")

	/*
		Exists returns boolean if the file exists on the file system.  Also returns an error if any.

		Exists() (bool, error)
	*/
	exists, err = srcFile.Exists()
	s.NoError(err)
	s.True(exists, "file exists")

	/*
		io.Reader and io.Seeker
	*/
	str, err := io.ReadAll(srcFile)
	s.NoError(err)
	s.Equal("this is a test\nand more text", string(str), "read was successful")

	offset, err := srcFile.Seek(3, 0)
	s.NoError(err)
	s.EqualValues(3, offset, "seek was successful")

	str, err = io.ReadAll(srcFile)
	s.NoError(err)
	s.Equal("s is a test\nand more text", string(str), "read after seek")
	err = srcFile.Close()
	s.NoError(err)

	for _, testLoc := range s.testLocations {
		// setup dstLoc
		dstLoc, err := testLoc.NewLocation("dstLoc/")
		s.NoError(err)
		fmt.Printf("** location %s **\n", dstLoc)
		defer func() {
			// clean up dstLoc after test for OS
			if dstLoc.FileSystem().Scheme() == "file" {
				exists, err := dstLoc.Exists()
				s.Require().NoError(err)
				if exists {
					s.NoError(os.RemoveAll(dstLoc.Path()), "failed to clean up file test dstLoc")
				}
			}
		}()

		// CopyToLocation will copy the current file to the provided location.
		//
		//   * Upon success, a vfs.File, representing the file at the new location, will be returned.
		//   * In the case of an error, nil is returned for the file.
		//   * CopyToLocation should use native functions when possible within the same scheme.
		//   * If the file already exists at the location, the contents will be overwritten with the current file's contents.
		_, err = srcFile.Seek(0, 0)
		s.NoError(err)
		dst, err := srcFile.CopyToLocation(dstLoc)
		s.NoError(err)
		exists, err := dst.Exists()
		s.NoError(err)
		s.True(exists, "dst file should now exist")
		exists, err = srcFile.Exists()
		s.NoError(err)
		s.True(exists, "src file should still exist")

		// CopyToFile will copy the current file to the provided file instance.
		//
		//   * In the case of an error, nil is returned for the file.
		//   * CopyToLocation should use native functions when possible within the same scheme.
		//   * If the file already exists, the contents will be overwritten with the current file's contents.

		// setup dstFile
		dstFile1, err := dstLoc.NewFile("dstFile1.txt")
		s.NoError(err)
		exists, err = dstFile1.Exists()
		s.NoError(err)
		s.False(exists, "dstFile1 file should not yet exist")
		_, err = srcFile.Seek(0, 0)
		s.NoError(err)
		err = srcFile.CopyToFile(dstFile1)
		s.NoError(err)
		exists, err = dstFile1.Exists()
		s.NoError(err)
		s.True(exists, "dstFile1 file should now exist")
		exists, err = srcFile.Exists()
		s.NoError(err)
		s.True(exists, "src file should still exist")

		/*
			io.Copy
		*/
		// create a local copy from srcFile with io.Copy
		copyFile1, err := srcLoc.NewFile("copyFile1.txt")
		s.NoError(err)
		// should not exist
		exists, err = copyFile1.Exists()
		s.NoError(err)
		s.False(exists, "copyFile1 should not yet exist locally")
		// do copy
		// skip this test for ftp files
		buffer := make([]byte, utils.TouchCopyMinBufferSize)

		if srcLoc.FileSystem().Scheme() != "ftp" {
			_, err = srcFile.Seek(0, 0)
			s.NoError(err)
			b1, err := io.CopyBuffer(copyFile1, srcFile, buffer)
			s.NoError(err)
			s.EqualValues(28, b1)
			err = copyFile1.Close()
			s.NoError(err)

			// should now exist
			exists, err = copyFile1.Exists()
			s.NoError(err)
			s.True(exists, "%s should now exist locally", copyFile1)
			err = copyFile1.Close()
			s.NoError(err)
		} else {
			// else still have to ensure copyFile1 exists for later tests
			err = copyFile1.Touch()
			s.NoError(err)
		}

		// create another local copy from srcFile with io.Copy
		copyFile2, err := srcLoc.NewFile("copyFile2.txt")
		s.NoError(err)
		// should not exist
		exists, err = copyFile2.Exists()
		s.NoError(err)
		s.False(exists, "copyFile2 should not yet exist locally")
		// do copy
		// skip this test for ftp files
		if srcLoc.FileSystem().Scheme() != "ftp" {
			_, err = srcFile.Seek(0, 0)
			s.NoError(err)
			buffer = make([]byte, utils.TouchCopyMinBufferSize)
			b2, err := io.CopyBuffer(copyFile2, srcFile, buffer)
			s.NoError(err)
			s.EqualValues(28, b2)

			err = copyFile2.Close()
			s.NoError(err)
			// should now exist
			exists, err = copyFile2.Exists()
			s.NoError(err)
			s.True(exists, "copyFile2 should now exist locally")
			err = copyFile2.Close()
			s.NoError(err)
		} else {
			// else still have to ensure copyFile1 exists for later tests
			err = copyFile2.Touch()
			s.NoError(err)
		}

		// MoveToLocation will move the current file to the provided location.
		//
		//   * If the file already exists at the location, the contents will be overwritten with the current file's contents.
		//   * If the location does not exist, an attempt will be made to create it.
		//   * Upon success, a vfs.File, representing the file at the new location, will be returned.
		//   * In the case of an error, nil is returned for the file.
		//   * When moving within the same Scheme, native move/rename should be used where possible.
		//   * If the file already exists, the contents will be overwritten with the current file's contents.
		fileForNew, err := srcLoc.NewFile("fileForNew.txt")
		s.NoError(err)

		// skip this test for ftp files
		if srcLoc.FileSystem().Scheme() != "ftp" {
			_, err = srcFile.Seek(0, 0)
			s.NoError(err)
			buffer = make([]byte, utils.TouchCopyMinBufferSize)
			_, err = io.CopyBuffer(fileForNew, srcFile, buffer)
			s.NoError(err)
			err = fileForNew.Close()
			s.NoError(err)

			newLoc, err := dstLoc.NewLocation("doesnotexist/")
			s.NoError(err)
			dstCopyNew, err := fileForNew.MoveToLocation(newLoc)
			s.NoError(err)
			exists, err = dstCopyNew.Exists()
			s.NoError(err)
			s.True(exists)
			s.NoError(dstCopyNew.Delete()) // clean up file
		}

		dstCopy1, err := copyFile1.MoveToLocation(dstLoc)
		s.NoError(err)
		// destination file should now exist
		exists, err = dstCopy1.Exists()
		s.NoError(err)
		s.True(exists, "dstCopy1 file should now exist")
		// local copy should no longer exist
		exists, err = copyFile1.Exists()
		s.NoError(err)
		s.False(exists, "copyFile1 should no longer exist locally")

		// MoveToFile will move the current file to the provided file instance.
		//
		//   * If the file already exists, the contents will be overwritten with the current file's contents.
		//   * The current instance of the file will be removed.
		dstCopy2, err := dstLoc.NewFile("dstFile2.txt")
		s.NoError(err)
		// destination file should not exist
		exists, err = dstCopy2.Exists()
		s.NoError(err)
		s.False(exists, "dstCopy2 file should not yet exist")
		// do move file
		err = copyFile2.MoveToFile(dstCopy2)
		s.NoError(err)
		// local copy should no longer exist
		exists, err = copyFile2.Exists()
		s.NoError(err)
		s.False(exists, "copyFile2 should no longer exist locally")
		// destination file should now exist
		exists, err = dstCopy2.Exists()
		s.NoError(err)
		s.True(exists, "dstCopy2 file should now exist")

		// clean up files
		err = dst.Delete()
		s.NoError(err)
		err = dstFile1.Delete()
		s.NoError(err)
		err = dstCopy1.Delete()
		s.NoError(err)
		err = dstCopy2.Delete()
		s.NoError(err)

		// ensure that MoveToFile() works for files with spaces
		type moveSpaceTest struct {
			Path, Filename string
		}
		tests := []moveSpaceTest{
			{Path: "file/", Filename: "has space.txt"},
			{Path: "file/", Filename: "has%20encodedSpace.txt"},
			{Path: "path has/", Filename: "space.txt"},
			{Path: "path%20has/", Filename: "encodedSpace.txt"},
		}

		for i, test := range tests {
			s.Run(fmt.Sprintf("%d", i), func() {
				// setup src
				srcSpaces, err := srcLoc.NewFile(path.Join(test.Path, test.Filename))
				s.NoError(err)
				b, err := srcSpaces.Write([]byte("something"))
				s.NoError(err)
				s.Equal(9, b, "byte count is correct")
				err = srcSpaces.Close()
				s.NoError(err)

				testDestLoc, err := dstLoc.NewLocation(test.Path)
				s.NoError(err)

				dstSpaces, err := srcSpaces.MoveToLocation(testDestLoc)
				s.NoError(err)
				exists, err := dstSpaces.Exists()
				s.NoError(err)
				s.True(exists, "dstSpaces should now exist")
				exists, err = srcSpaces.Exists()
				s.NoError(err)
				s.False(exists, "srcSpaces should no longer exist")
				s.True(
					strings.HasSuffix(dstSpaces.URI(), path.Join(test.Path, test.Filename)),
					"destination file %s ends with source string for %s", dstSpaces.URI(), path.Join(test.Path, test.Filename),
				)

				newSrcSpaces, err := dstSpaces.MoveToLocation(srcSpaces.Location())
				s.NoError(err)
				exists, err = newSrcSpaces.Exists()
				s.NoError(err)
				s.True(exists, "newSrcSpaces should now exist")
				exists, err = dstSpaces.Exists()
				s.NoError(err)
				s.False(exists, "dstSpaces should no longer exist")
				hasSuffix := strings.HasSuffix(newSrcSpaces.URI(), path.Join(test.Path, test.Filename))
				s.True(hasSuffix, "destination file %s ends with source string for %s", dstSpaces.URI(), path.Join(test.Path, test.Filename))

				err = newSrcSpaces.Delete()
				s.NoError(err)
				exists, err = newSrcSpaces.Exists()
				s.NoError(err)
				s.False(exists, "newSrcSpaces should now exist")
			})
		}
	}

	// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
	// Returns error if unable to touch File.

	touchedFile, err := srcLoc.NewFile("touch.txt")
	s.NoError(err)
	defer func() { _ = touchedFile.Delete() }()
	exists, err = touchedFile.Exists()
	s.NoError(err)
	s.False(exists, "%s shouldn't yet exist", touchedFile)

	err = touchedFile.Touch()
	s.NoError(err)
	exists, err = touchedFile.Exists()
	s.NoError(err)
	s.True(exists, "%s now exists", touchedFile)

	size, err := touchedFile.Size()
	s.NoError(err)
	s.Zero(size, "%s should be empty", touchedFile)

	// capture last modified
	modified, err := touchedFile.LastModified()
	s.NoError(err)
	modifiedDeRef := *modified
	// wait for eventual consistency
	time.Sleep(1 * time.Second)
	err = touchedFile.Touch()
	s.NoError(err)
	newModified, err := touchedFile.LastModified()

	s.NoError(err)
	s.True(newModified.UnixNano() > modifiedDeRef.UnixNano(), "touch updated modified date for %s", touchedFile)

	/*
		Delete unlinks the File on the file system.

		Delete() error
	*/
	err = srcFile.Delete()
	s.NoError(err)
	exists, err = srcFile.Exists()
	s.NoError(err)
	s.False(exists, "file no longer exists")

	// The following blocks test that an error is thrown when these operations are called on a non-existent file
	srcFile, err = srcLoc.NewFile("thisFileDoesNotExist")
	s.NoError(err, "unexpected error creating file")

	exists, err = srcFile.Exists()
	s.NoError(err)
	s.False(exists, "file should not exist")

	size, err = srcFile.Size()
	s.Error(err, "expected error because file does not exist")
	s.Zero(size)

	_, err = srcFile.LastModified()
	s.Error(err, "expected error because file does not exist")

	seeked, err := srcFile.Seek(-1, 2)
	s.Error(err, "expected error because file does not exist")
	s.Zero(seeked)

	_, err = srcFile.Read(make([]byte, 1))
	s.Error(err, "expected error because file does not exist")

	// end existence tests

}

// gs-specific test cases
func (s *vfsTestSuite) gsList(baseLoc vfs.Location) {
	/*
			test description:
				When a persistent "folder" is created through the UI, it simply creates a zero length object
		        with a trailing "/". The UI or gsutil knows to interpret these objects as folders but they are
		        still just objects.  List(), in its current state, should ignore these objects.

			If we create the following objects:
			    gs://bucket/some/path/to/myfolder/         -- Note that object base name is "myfolder/"
		        gs://bucket/some/path/to/myfolder/file.txt

		    List() from location "gs://bucket/some/path/to/myfolder/" should only return object name "file.txt";
			"myfolder/" should be ignored.
	*/

	/*
		first create persistent "folder"
	*/

	// getting client since VFS doesn't allow a File ending with a slash
	client, err := baseLoc.FileSystem().(*gs.FileSystem).Client()
	s.NoError(err)

	objHandle := client.
		Bucket("enterprise-test").
		Object(utils.RemoveLeadingSlash(baseLoc.Path() + "myfolder/"))

	ctx := context.Background()

	// write zero length object
	writer := objHandle.NewWriter(ctx)
	_, err = writer.Write([]byte(""))
	s.NoError(err)
	s.NoError(writer.Close())

	/*
		next create a file inside the "folder"
	*/

	f, err := baseLoc.NewFile("myfolder/file.txt")
	s.NoError(err)

	_, err = f.Write([]byte("some text"))
	s.NoError(err)
	s.NoError(f.Close())

	/*
		finally list "folder" should only return file.txt
	*/

	files, err := f.Location().List()
	s.NoError(err)
	s.Len(len(files), 1, "check file count found")
	s.Equal("file.txt", files[0], "file.txt was found")

	// CLEAN UP
	s.NoError(f.Delete(), "clean up file.txt")
	s.NoError(objHandle.Delete(ctx))
}

func sftpRemoveAll(location *sftp.Location) error {

	// get sftp client from FileSystem
	client, err := location.FileSystem().(*sftp.FileSystem).Client(location.Authority)
	if err != nil {
		return err
	}

	// recursively remove directory
	return recursiveSFTPRemove(location.Path(), client)
}

func recursiveSFTPRemove(absPath string, client sftp.Client) error {

	// we can return early if we can just remove it
	err := client.Remove(absPath)
	// if we succeeded or it didn't exist, just return
	if err == nil || os.IsNotExist(err) {
		// success
		return nil
	}

	// handle error unless it was directory which we'll assume we couldn't delete because it isn't empty
	if !strings.HasSuffix(absPath, "/") {
		// not a directory (file's should have already been deleted) so return err
		return err
	}

	// Remove child objects in directory
	children, err := client.ReadDir(absPath)
	if err != nil {
		return err
	}

	var rErr error
	for _, child := range children {
		childName := child.Name()
		// TODO: what about symlinks to directories? we're not recursing into them, which I think is right
		//      if we need to, we'd do:
		//          if child.Mode() & ModeSymLink != 0 {
		// 	          do something
		// 	        }
		if child.IsDir() {
			childName = utils.EnsureTrailingSlash(childName)
		}
		err := recursiveSFTPRemove(absPath+childName, client)
		if err != nil {
			rErr = err
		}
	}
	if rErr != nil {
		return rErr
	}

	// try to remove the object again
	return client.Remove(absPath)
}

func TestVFS(t *testing.T) {
	suite.Run(t, new(vfsTestSuite))
}
