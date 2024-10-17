# Delete Options

---

Package delete consists of custom delete options

## DeleteAllVersions
Currently, we have delete.AllVersions option that can be used to remove all the versions of a file upon delete.
This is supported for all filesystems that have file versioning (E.g: S3, GS etc.)

### Usage

Delete file using file.delete():

```go
    import(
        "github.com/c2fo/vfs/v6/options"
        "github.com/c2fo/vfs/v6/options/delete"
    )
    
    func DeleteFile() error {
        file, err := fs.NewFile(bucketName, fileName)
        ...
        err = file.Delete(delete.WithAllVersions())
        ...
    }
```

OR

Delete file using location.delete():

```go
    import(
        "github.com/c2fo/vfs/v6/options"
        "github.com/c2fo/vfs/v6/options/delete"
    )
    
    func DeleteFileUsingLocation() error {
        err = location.DeleteFile("filename.txt", delete.WithAllVersions())
        ...
    }
```