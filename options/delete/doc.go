/*
Package delete consists of custom delete options

Currently, we have DeleteAllVersions option that can be used to remove all the versions of files upon delete.
This is supported for all filesystems that have file versioning (E.g: S3, GS etc.)

Usage

Delete file using file.delete():

  import(
      "github.com/c2fo/vfs/v6/options"
      "github.com/c2fo/vfs/v6/options/delete"
  )

  func DeleteFile() error {
      file, err := fs.NewFile(bucketName, "/"+objectName)
	  ...
      err = file.Delete(delete.WithDeleteAllVersions())
      ...
  }

OR

Delete file using location.delete():

	import(
      	"github.com/c2fo/vfs/v6/options"
      	"github.com/c2fo/vfs/v6/options/delete"
  	)

  	func DeleteFileUsingLocation() error {
      	err = location.DeleteFile("filename.txt", delete.WithDeleteAllVersions())
      	...
  	}
*/
package delete
