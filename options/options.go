/*
Package options provides a set of interfaces and functions to define and apply options for file systems.

Options are grouped by the operation they apply to:

  - NewFileSystemOption configures a backend's FileSystem through its NewFileSystem function.
    Each backend provides its own options, such as s3.WithOptions or gs.WithRetryer.
  - NewFileOption configures FileSystem.NewFile and Location.NewFile.
    See the options/newfile package, for example newfile.WithContentType.
  - DeleteOption configures File.Delete and Location.DeleteFile.
    See the options/delete package, for example delete.WithAllVersions.

# Creating a Custom Option

To create your own option, define a type that implements the relevant interface.  For example, a DeleteOption:

	type TakeBackupDeleteOption struct {
	    backupLocation string
	}

	func (o TakeBackupDeleteOption) DeleteOptionName() string {
	    return "take-backup"
	}

	func (o TakeBackupDeleteOption) BackupLocation() string {
	    return o.backupLocation
	}

Then, in each backend implementation that should support it, handle the option in Delete:

	func (f *File) Delete(opts ...options.DeleteOption) error {
	    for _, o := range opts {
	        switch o := o.(type) {
	        case delete.AllVersions:
	            // delete all versions
	        case TakeBackupDeleteOption:
	            // copy the file to o.BackupLocation() before deleting
	        default:
	        }
	    }
	    ...
	}
*/
package options

// Option interface contains function that should be implemented by any custom option.
type Option[T any] interface {
	Apply(*T)
}

// ApplyOptions is a helper to apply multiple options in a single call.
func ApplyOptions[T any](fs *T, opts ...NewFileSystemOption[T]) {
	for _, opt := range opts {
		opt.Apply(fs)
	}
}

// DeleteOption interface contains function that should be implemented by any custom option to qualify as a delete option.
// See the package documentation for an example.
type DeleteOption interface {
	DeleteOptionName() string
}

// NewFileOption interface contains function that should be implemented by any custom option to qualify as a new file option.
type NewFileOption interface {
	NewFileOptionName() string
}

// NewFileSystemOption interface contains function that should be implemented by any custom option to qualify as a new
// file system option.
type NewFileSystemOption[T any] interface {
	Option[T]
	NewFileSystemOptionName() string
}
