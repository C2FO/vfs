package options

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
