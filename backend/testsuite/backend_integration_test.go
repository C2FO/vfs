// build vfsintegration

package testsuite

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/backend/gs"
	"github.com/c2fo/vfs/v5/backend/mem"
	_os "github.com/c2fo/vfs/v5/backend/os"
	"github.com/c2fo/vfs/v5/backend/s3"
	"github.com/c2fo/vfs/v5/utils"
	"github.com/c2fo/vfs/v5/vfssimple"
	"github.com/stretchr/testify/suite"
)

type vfsTestSuite struct {
	suite.Suite
	testLocations map[string]vfs.Location
}

func copyOsLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*_os.Location)
	ret := &cp

	// setup os location
	exists, err := ret.Exists()
	if err != nil {
		panic(err)
	}
	if !exists {
		err := os.Mkdir(ret.Path(), 0755)
		if err != nil {
			panic(err)
		}
	}

	return ret
}

func copyS3Location(loc vfs.Location) vfs.Location {
	cp := *loc.(*s3.Location)
	return &cp
}

func copyGSLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*gs.Location)
	return &cp
}

func copyMemLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*mem.Location)
	return &cp
}

func (s *vfsTestSuite) SetupSuite() {
	locs := os.Getenv("VFS_INTEGRATION_LOCATIONS")
	s.testLocations = make(map[string]vfs.Location)
	for _, loc := range strings.Split(locs, ";") {
		l, err := vfssimple.NewLocation(loc)
		s.NoError(err)
		switch l.FileSystem().Scheme() {
		case "file":
			s.testLocations[l.FileSystem().Scheme()] = copyOsLocation(l)
		case "s3":
			s.testLocations[l.FileSystem().Scheme()] = copyS3Location(l)
		case "gs":
			s.testLocations[l.FileSystem().Scheme()] = copyGSLocation(l)
		case "mem":
			s.testLocations[l.FileSystem().Scheme()] = copyMemLocation(l)
		default:
			panic(fmt.Sprintf("unknown scheme: %s", l.FileSystem().Scheme()))
		}
	}

}

//Test File
func (s *vfsTestSuite) TestScheme() {
	for scheme, location := range s.testLocations {
		fmt.Printf("************** TESTING scheme: %s **************\n", scheme)
		s.FileSystem(location)
		s.Location(location)
		s.File(location)
	}
}

//Test FileSystem
func (s *vfsTestSuite) FileSystem(baseLoc vfs.Location) {
	fmt.Println("****** testing vfs.FileSystem ******")

	//setup FileSystem
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
			expected := fmt.Sprintf("%s://%s%s", fs.Scheme(), baseLoc.Volume(), path.Clean(name))
			s.Equal(expected, file.URI(), "uri's should match")
		} else {
			s.Error(err, "should have validation error for scheme and name: %s : %s", fs.Scheme(), name)
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
			expected := fmt.Sprintf("%s://%s%s", fs.Scheme(), baseLoc.Volume(), utils.EnsureTrailingSlash(path.Clean(name)))
			s.Equal(expected, loc.URI(), "uri's should match")

		} else {
			s.Error(err, "should have validation error for scheme and name: %s : %s", fs.Scheme(), name)
		}
	}
}

//Test Location
func (s *vfsTestSuite) Location(baseLoc vfs.Location) {
	fmt.Println("****** testing vfs.Location ******")

	srcLoc, err := baseLoc.NewLocation("locTestSrc/")
	s.NoError(err, "there should be no error")
	defer func() {
		//clean up srcLoc after test for OS
		if srcLoc.FileSystem().Scheme() == "file" {
			exists, err := srcLoc.Exists()
			if err != nil {
				panic(err)
			}
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
			expected := fmt.Sprintf("%s://%s%s", srcLoc.FileSystem().Scheme(), baseLoc.Volume(), utils.EnsureTrailingSlash(path.Clean(path.Join(srcLoc.Path(), name))))
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
			expected := fmt.Sprintf("%s://%s%s", srcLoc.FileSystem().Scheme(), srcLoc.Volume(), path.Clean(path.Join(srcLoc.Path(), name)))
			actual := file.URI()
			s.Equal(expected, actual, `uri's should match for fs.NewFile("%s", "%s")`, baseLoc.Volume(), name)
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

	//setup test
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
	prefix := fmt.Sprintf("%s://%s%s", cdTestLoc.FileSystem().Scheme(), cdTestLoc.Volume(), "/")
	s.True(strings.HasPrefix(cdTestLoc.URI(), prefix), "should start with schema and abs slash")

	/* Exists returns boolean if the location exists on the file system. Returns an error if any.

		   TODO: *************************************************************************************************************
			     note that Exists is not consistent among implementations. GCSs and S3 always return true if the bucket exist.
		         Fundamentally, why one wants to know if location exists is to know whether you're able to write there.  But
		         this feels unintuitve.
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
	s.True(exists, "srcLoc location doesn't exist")

	//setup list tests
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
	s.Equal(3, len(files), "list srcLoc location")

	files, err = subLoc.List()
	s.NoError(err)
	s.Equal(1, len(files), "list subLoc location")
	s.Equal("that.txt", files[0], "returned basename")

	files, err = cdTestLoc.List()
	s.NoError(err)
	s.Equal(0, len(files), "non-existent location")

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
	s.Equal(2, len(files), "list srcLoc location matching prefix")

	files, err = srcLoc.ListByPrefix("s")
	s.NoError(err)
	s.Equal(1, len(files), "list srcLoc location")
	s.Equal("self.txt", files[0], "returned only file basename, not subdir matching prefix")

	files, err = srcLoc.ListByPrefix("somepath/t")
	s.NoError(err)
	s.Equal(1, len(files), "list 'somepath' location relative to srcLoc")
	s.Equal("that.txt", files[0], "returned only file basename, using relative prefix")

	files, err = cdTestLoc.List()
	s.NoError(err)
	s.Equal(0, len(files), "non-existent location")

	// ListByRegex returns a slice of strings representing the base names of the files found in the Location that matched the
	// given regular expression.
	//
	//   * All implementations are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location.
	//   * If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
	//     be checked first.
	//	====	ListByRegex(regex *regexp.Regexp) ([]string, error)

	files, err = srcLoc.ListByRegex(regexp.MustCompile("^f"))
	s.NoError(err)
	s.Equal(2, len(files), "list srcLoc location matching prefix")

	files, err = srcLoc.ListByRegex(regexp.MustCompile(`.txt$`))
	s.NoError(err)
	s.Equal(3, len(files), "list srcLoc location matching prefix")

	files, err = srcLoc.ListByRegex(regexp.MustCompile(`Z`))
	s.NoError(err)
	s.Equal(0, len(files), "list srcLoc location matching prefix")

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

	//should error if file doesn't exist
	s.Error(srcLoc.DeleteFile(f1.Path()), "deleteFile trying to delete a file already deleted")

}

//Test File
func (s *vfsTestSuite) File(baseLoc vfs.Location) {
	fmt.Println("****** testing vfs.File ******")
	srcLoc, err := baseLoc.NewLocation("fileTestSrc/")
	s.NoError(err)
	defer func() {
		//clean up srcLoc after test for OS
		if srcLoc.FileSystem().Scheme() == "file" {
			exists, err := srcLoc.Exists()
			if err != nil {
				panic(err)
			}
			if exists {
				s.NoError(os.RemoveAll(srcLoc.Path()), "failed to clean up file test srcLoc")
			}
		}
	}()

	//setup srcFile
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
	str, err := ioutil.ReadAll(srcFile)
	s.NoError(err)
	s.Equal("this is a test\nand more text", string(str), "read was successful")

	offset, err := srcFile.Seek(3, 0)
	s.NoError(err)
	s.EqualValues(3, offset, "seek was successful")

	str, err = ioutil.ReadAll(srcFile)
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
			//clean up dstLoc after test for OS
			if dstLoc.FileSystem().Scheme() == "file" {
				exists, err := dstLoc.Exists()
				if err != nil {
					panic(err)
				}
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
		//   * CopyToLocation should use native functions when possible withen the same scheme.
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
		_, err = srcFile.Seek(0, 0)
		s.NoError(err)
		b1, err := io.Copy(copyFile1, srcFile)
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

		// create another local copy from srcFile with io.Copy
		copyFile2, err := srcLoc.NewFile("copyFile2.txt")
		s.NoError(err)
		// should not exist
		exists, err = copyFile2.Exists()
		s.NoError(err)
		s.False(exists, "copyFile2 should not yet exist locally")
		// do copy
		_, err = srcFile.Seek(0, 0)
		s.NoError(err)
		b2, err := io.Copy(copyFile2, srcFile)
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

		// MoveToLocation will move the current file to the provided location.
		//
		//   * If the file already exists at the location, the contents will be overwritten with the current file's contents.
		//   * If the location does not exist, an attempt will be made to create it.
		//   * Upon success, a vfs.File, representing the file at the new location, will be returned.
		//   * In the case of an error, nil is returned for the file.
		//   * When moving within the same Scheme, native move/rename should be used where possible.
		//   * If the file already exists, the contents will be overwritten with the current file's contents.
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
	}

	// Test empty reader used to io.Copy

	// TODO: ***************************************************************************************************
	//  it has yet to be determined what behavior should occur
	//  likely we will not expect io.Copy to produce a file since that seems to be the general expected behavior
	//  however, does that mean then that we enforce that it should NOT create a file, like it does in OS.
	//  Also, Should we ensure that an empty write() doesn't create a file on close()?
	//  ********************************************************************************************************

	/*
		setup reader and vfs.File target
		emptyIOReader, err := srcLoc.NewFile("srcEmptyfile.txt")
		s.NoError(err)
		_, err = emptyIOReader.Write([]byte(""))
		s.NoError(err)
		s.NoError(emptyIOReader.Close())

		emptyTargetFile, err := srcLoc.NewFile("emptyfile.txt")
		s.NoError(err)

		fmt.Println(emptyTargetFile)
		// use io.Copy
		byteCount, err := io.Copy(emptyTargetFile, emptyIOReader)
		s.NoError(err, "should have no error copying emtpy file")
		s.Equal(int64(0), byteCount, "should copy zero bytes")

		//does target exist?
		exists, err = emptyTargetFile.Exists()
		s.NoError(err)
		s.True(exists, "new empty file should exist")

		//does size match?
		size, err := emptyTargetFile.Size()
		s.NoError(err)
		s.Equal(uint64(0), size, "new empty file should have size of 0")

		//clean up
		err = emptyTargetFile.Delete()
		s.NoError(err)
	*/

	/*
		Delete unlinks the File on the file system.

		Delete() error
	*/
	err = srcFile.Delete()
	s.NoError(err)
	exists, err = srcFile.Exists()
	s.NoError(err)
	s.False(exists, "file no longer exists")
}

func TestVFS(t *testing.T) {
	suite.Run(t, new(vfsTestSuite))
}
