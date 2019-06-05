package testsuite

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend/gs"
	_os "github.com/c2fo/vfs/v4/backend/os"
	"github.com/c2fo/vfs/v4/backend/s3"
	"github.com/c2fo/vfs/v4/utils"
	"github.com/c2fo/vfs/v4/vfssimple"
	"github.com/stretchr/testify/suite"
)

type vfsTestSuite struct {
	suite.Suite
	testLocations map[string]vfs.Location
}

func copyOsLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*_os.Location)
	return &cp
}

func copyS3Location(loc vfs.Location) vfs.Location {
	cp := *loc.(*s3.Location)
	return &cp
}

func copyGSLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*gs.Location)
	return &cp
}

func (s *vfsTestSuite) SetupSuite() {
	locs := os.Getenv("VFS_INTEGRATION_LOCATIONS")
	s.testLocations = make(map[string]vfs.Location, 0)
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
	//setup filesystem
	fs := baseLoc.FileSystem()
	// NewFile initializes a File on the specified volume at path 'name'. On error, nil is returned for the file.
	//
	//   * path is expected to always be absolute and therefore must begin with a separator character and may not be an
	//   empty string.  As a path to a file, 'name' may not end with a trailing separator character.
	//   * The file may or may not already exist.
	//   * Upon success, a vfs.File, representing the file's new path (location path + file relative path), will be returned.
	//   * On error, nil is returned for the file.
	//   * fileName param must be a an absolute path to a file and therefore may not start or end with a separator characters.
	//     This is not to be confused with vfs.Locations' NewFile() which requires a path relative the current location.
	//   * Note that not all filesystems will have a "volume" and will therefore be "":
	//       file:///path/to/file has a volume of "" and name /path/to/file
	//       whereas
	//       s3://mybucket/path/to/file has a volume of "mybucket and name /path/to/file
	//       results in /tmp/dir1/newerdir/file.txt for the final vfs.File path.
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
			expected := fmt.Sprintf("%s://%s%s", fs.Scheme(), baseLoc.Volume(), filepath.Clean(name))
			s.Equal(expected, file.URI(), "uri's should match")
		} else {
			s.Error(err, "should have validation error for scheme and name: %s : %s", fs.Scheme(), name)
		}
	}

	// NewLocation initializes a Location on the specified volume with the given path. On error, nil is returned
	// for the location
	//
	//   * The path may or may not already exist.  Note that on keystore filesystems like S3 or GCS, paths never truly exist.
	//   * path is expected to always be absolute and therefore must begin and end with a separator character. This is not to
	//        be confused with vfs.Locations' NewLocation() which requires a path relative to the current location.
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
			s.Equal(fmt.Sprintf("%s://%s%s", fs.Scheme(), baseLoc.Volume(), utils.AddTrailingSlash(filepath.Clean(name))), loc.URI(), "uri's should match")

		} else {
			s.Error(err, "should have validation error for scheme and name: %s : %s", fs.Scheme(), name)
		}
	}
}

//Test Location
func (s *vfsTestSuite) Location(baseLoc vfs.Location) {
	/*
		//LOCATION
		l := f.Location()
		//URI, String, Path
		s.Equal(rootTestLoc+"this/", l.URI(), "Location is correct")
		s.Equal(rootTestLoc+"this/", l.String(), "Stringer is correct")
		s.Equal(testPath+"this/", l.Path(), "Path is correct")

		//Exists
		locExists, lexerr := l.Exists()
		s.NoError(lexerr)
		s.True(locExists, "location exists")

		// List()
		files, lerr := l.List()
		s.NoError(lerr)
		s.Equal(3, len(files), "found all 3 files")

		// ListByPrefix()
		subdirFile, subdirFileErr := l.NewFile("file3/some.txt")
		s.NoError(subdirFileErr)
		_, subdirFileWriteErr := subdirFile.Write([]byte("this is a test"))
		s.NoError(subdirFileWriteErr)
		subdirFileCloseErr := subdirFile.Close()
		s.NoError(subdirFileCloseErr)
		file4, fileErr4 := l.NewFile("file4.txt")
		s.NoError(fileErr4)
		_, file4WriteErr := file4.Write([]byte("this is a test"))
		s.NoError(file4WriteErr)
		file4CloseErr := file4.Close()
		s.NoError(file4CloseErr)
		files, lerr = l.ListByPrefix("file")
		s.NoError(lerr)
		s.Equal(3, len(files), "found both files")
		s.Equal("[file.txt file2.txt file4.txt]", fmt.Sprintf("%+v", files), "only files found starting with file")

		// ListByRegex
		myRe, ReErr := regexp.Compile("[.]csv$")
		s.NoError(ReErr)
		files, lerr = l.ListByRegex(myRe)
		s.NoError(lerr)
		s.Equal(1, len(files), "found 1 file")

		// NewLocation
		newloc, err := l.NewLocation("subdir/")
		s.NoError(err)
		s.Equal(rootTestLoc+"this/subdir/", newloc.URI())

		relfile, _ := newloc.NewFile("../bam/this.txt")
		s.Equal(testPath+"this/bam/this.txt", relfile.Path(), "relative dot path works")

		// ChangeDir
		err = l.ChangeDir("../")
		s.NoError(err)
		s.Equal(rootTestLoc, l.URI())
		// change dir back
		err = l.ChangeDir("this/")
		s.NoError(err)
		s.Equal(rootTestLoc+"this/", l.URI())
	*/

	// List returns a slice of strings representing the base names of the files found at the Location. All implementations
	// are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location. If the user
	// cares about the distinction between an empty location and a non-existent one, Location.Exists() should be checked
	// first.
	//
	//	====		List() ([]string, error)

	// ListByPrefix returns a slice of strings representing the base names of the files found in Location whose
	// filenames match the given prefix. An empty slice will be returned even for locations that don't exist.
	//
	//	====	ListByPrefix(prefix string) ([]string, error)

	// ListByRegex returns a slice of strings representing the base names of the files found in Location that
	// matched the given regular expression. An empty slice will be returned even for locations that don't exist.
	//
	//	====	ListByRegex(regex *regexp.Regexp) ([]string, error)

	// Path returns absolute path to the Location with leading and trailing slashes, ie /some/path/to/
	//	==== Path() string

	//TODO:  NOT yet the current wording depending on whether we change the exists check (from checking bucket exists)
	//
	// Exists returns boolean if the location exists on the file system. Also returns an error if any.
	//
	//	For some keystore filesystems, a prefix/folder can't exist if there isn't an object with a key that contains that
	//	"location", so checking if a given "location" exists is effectively checking if keys with a particular prefix
	//	"directory" exist.
	//
	//	==== Exists() (bool, error)

	// NewLocation is an initializer for a new Location relative to the existing one.
	//
	// * "relativePath" parameter may use dot (. and ..) paths and may not begin with a separator character but must
	// end with a separator character.
	//
	// For location:
	//     file:///some/path/to/
	// calling:
	//     NewLocation("../../")
	// would return a new vfs.Location representing:
	//     file:///some/
	//	====	NewLocation(relativePath string) (Location, error)

	// ChangeDir updates the existing Location's path to the provided relative path. For instance, for location:
	// file:///some/path/to/, calling ChangeDir("../../") update the location instance to file:///some/.
	//
	// relativePath may use dot (. and ..) paths and may not begin with a separator character but must end with
	// a separator character.
	//   ie., path/to/location, path/to/location/, ./path/to/location, and ./path/to/location/ are all effectively equal.
	//
	//	====	ChangeDir(relativePath string) error

	// NewFile will instantiate a vfs.File instance at or relative to the current location's path.
	//
	//   * fileName param may use dot (. and ..) paths and may not begin or end with a separator character.
	//   * Resultant File path will be the shortest path name equivalent of combining the Location path and relative path, if any.
	//       ie, /tmp/dir1/ as location and fileName "newdir/./../newerdir/file.txt"
	//       results in /tmp/dir1/newerdir/file.txt for the final vfs.File path.
	//   * The file may or may not already exist.
	//   * Upon success, a vfs.File, representing the file's new path (location path + file relative path), will be returned.
	//   * In the case of an error, nil is returned for the file.
	//
	//	====	NewFile(fileName string) (File, error)

	// DeleteFile deletes the file of the given name at the location. This is meant to be a short cut for
	// instantiating a new file and calling delete on that, with all the necessary error handling overhead.
	//
	// fileName may be a relative path to a file but, as a file, may not end with a separator charactier
	//   ie., path/to/file.txt, ../../other/path/to/file.text are acceptable but path/to/file.txt/ is not
	//
	//	====	DeleteFile(fileName string) error

	// URI returns the fully qualified URI for the Location.  IE, s3://bucket/some/path/
	//
	// URI's for locations must always end with a separator character.
}

//Test File
func (s *vfsTestSuite) File(baseLoc vfs.Location) {
	srcLoc, err := baseLoc.NewLocation("fileTestSrc/")
	s.NoError(err)

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
	s.Equal(baseLoc.URI()+"fileTestSrc/srcFile.txt", srcFile.URI(), "string(er) test")

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
		fmt.Printf("************ location %s *************\n", dstLoc)

		/*
			CopyToLocation will copy the current file to the provided location. If the file already exists at the location,
			the contents will be overwritten with the current file's contents. In the case of an error, nil is returned
			for the file.

			CopyToLocation(location Location) (File, error)
		*/
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

		/*
			CopyToFile will copy the current file to the provided file instance. If the file already exists,
			the contents will be overwritten with the current file's contents. In the case of an error, nil is returned
			for the file.

			CopyToFile(File) error
		*/
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

		/*
			MoveToLocation will move the current file to the provided location.

			* If the file already exists at the location, the contents will be overwritten with the current file's contents.
			* If the location does not exist, an attempt will be made to create it.
			* Upon success, a vfs.File, representing the file at the new location, will be returned.
			* In the case of an error, nil is returned for the file.
			* When moving within the same Scheme, native move/rename should be used where possible

			MoveToLocation(location Location) (File, error)
		*/
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

		/*
			MoveToFile will move the current file to the provided file instance. If a file with the current file's name already exists,
			the contents will be overwritten with the current file's contents. The current instance of the file will be removed.

			MoveToFile(File) error
		*/
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

	/*
		Delete unlinks the File on the filesystem.

		Delete() error
	*/
	err = srcFile.Delete()
	s.NoError(err)
	exists, err = srcFile.Exists()
	s.NoError(err)
	s.False(exists, "file no longer exists")
}

//Test File
func (s *vfsTestSuite) oldFile(testLocation vfs.Location) {

}

func TestVFS(t *testing.T) {
	suite.Run(t, new(vfsTestSuite))
}
