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
// Example:
// ```
//
//	type TakeBackupDeleteOption{}
//	func (o TakeBackupDeleteOption) DeleteOptionName() string {
//		return "take backup"
//	}
//	func (o TakeBackupDeleteOption) BackupLocation() string {
//		return o.backupLocation
//	}
//
// ```
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
