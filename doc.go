/*
Package vfs provides a pluggable, extensible, and opinionated set of filesystem
functionality for Go across a number of filesystem types such as os, S3, and GCS.

Philosophy

When building our platform, initially we wrote a library that was something to the effect of
  if config.DISK == "S3" {
	  // do some s3 filesystem operation
  } else if config.DISK == "mock" {
      // fake something
  } else {
      // do some native os.xxx operation
  }

Not only was ugly but because the behaviors of each "filesystem" were different and we had to constantly alter the
file locations and pass a bucket string (even if the fs didn't know what a bucket was).

We found a handful of third-party libraries that were interesting but none of them had everything we needed/wanted. Of
particular inspiration was https://github.com/spf13/afero in its composition of the super-powerful stdlib io.* interfaces.
Unfortunately, it didn't support Google Cloud Storage and there was still a lot of passing around of strings and structs.
Few, if any, of the vfs-like libraries provided interfaces to easily and confidently create new filesystem backends.

What we needed/wanted was the following(and more):
  * self-contained set of structs that could be passed around like a file/dir handle
  * the struct would represent an existing or nonexistent file/dir
  * provide common (and only common) functionality across all filesystem so that after initialization, we don't care
    what the underlying filesystem is and can therefore write our code agnostically/portably
  * use io.* interfaces such as io.Reader and io.Writer without needing to call a separate function
  * extensibility to easily add other needed filesystems like Microsoft Azure Cloud File Storage or SFTP
  * prefer native atomic functions when possible (ie S3 to S3 moving would use the native move api call rather than
    copy-delete)
  * a uniform way of addressing files regardless of filesystem.  This is why we use complete URI's in vfssimple
  * fmt.Stringer interface so that the file struct passed to a log message (or other Stringer use) would show the URI
  * mockable filesystem
  * pluggability so that third-party implemenations of our interfaces could be used

Install

Go install:
  go get -u github.com/c2fo/vfs

Usage

We provide vfssimple as basic way of initializing filesystem backends (see each implementations's docs about authentication).
vfssimple pulls in every c2fo/vfs backend.  If you need to reduce the backend requirements (and app memory footprint) or
add a third party backend, you'll need to implement your own "factory".  See backend doc for more info.

You can then use those filesystems to initialize locations which you'll be referencing frequently, or initialize files directly


  osFile, err := vfssimple.NewFile("file:///path/to/file.txt")
  s3File, err := vfssimple.NewFile("s3://bucket/prefix/file.txt")

  osLocation, err := vfssimple.NewLocation("file:///tmp/")
  s3Location, err := vfssimple.NewLocation("s3://bucket/")

  osTmpFile, err := osLocation.NewFile("anotherFile.txt") // file at /tmp/anotherFile.txt


With a number of files and locations between s3 and the local filesystem you can perform a number of actions without any consideration for the system's api or
implementation details.

  osFileExists, err := osFile.Exists() // true, nil
  s3FileExists, err := s3File.Exists() // false, nil
  err = osFile.CopyToFile(s3File) // nil
  s3FileExists, err = s3File.Exists() // true, nil

  movedOsFile, err := osFile.MoveToLocation(osLocation)
  osFileExists, err = osFile.Exists() // false, nil (move actions delete the original file)
  movedOsFileExists, err := movedOsFile.Exists() // true, nil

  s3FileUri := s3File.URI() // s3://bucket/prefix/file.txt
  s3FileName := s3File.Name() // file.txt
  s3FilePath := s3File.Path() // /prefix/file.txt

File's io.* interfaces may be used directly:

  reader := strings.NewReader("Clear is better than clever")
  gsFile, err := vfssimple.NewFile("gs://somebucket/path/to/file.txt")

  byteCount, err := io.Copy(gsFile, reader)
  err := gsFile.Close()

Third-party Backends

* none so far

Feel free to send a pull request if you want to add your backend to the list.

Ideas

Things to add:
  * Add SFTP backend
  * Add Azure storage backend
  * Add in-memory backend
  * Provide better List() functionality with more abstracted filering and paging (iterator?) Retrun File structs vs URIs?
  * Add better/any context.Context() support

Contributors

Brought to you by the Enterprise Pipeline team at C2FO:

* John Judd - john.judd@c2fo.com
* Jason Coble - [@jasonkcoble](https://twitter.com/jasonkcoble) - jason@c2fo.com
* Chris Roush – chris.roush@c2fo.com

https://github.com/c2fo/

Contributing

  1. Fork it (<https://github.com/c2fo/vfs/fork>)
  2. Create your feature branch (`git checkout -b feature/fooBar`)
  3. Commit your changes (`git commit -am 'Add some fooBar'`)
  4. Push to the branch (`git push origin feature/fooBar`)
  5. Create a new Pull Request

License

Distributed under the MIT license. See `http://github.com/c2fo/vfs/License.md for more information.

Definitions

* absolute path - A path is said to be absolute if it provides the entire context need to find a file, including the
filesystem root. An absolute path must begin with a slash and may include . and .. directories.

* relative path - A relative path is a way to locate a dir or file relative to another directory. A relative path may not
begin with a slash but may include . and .. directories.

* file path - A file path ends with a filename and therefore may not end with a slash.

* location path - A location/dir path must end with a slash.

* URI - A Uniform Resource Identifier (URI) is a string of characters that unambiguously identifies a particular resource.
To guarantee uniformity, all URIs follow a predefined set of syntax rules, but also maintain extensibility through
a separately defined hierarchical naming scheme (e.g. http://).
*/
package vfs
