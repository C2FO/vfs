# options

---


Package options provides a means of creating custom options that can be used with operations that are performed on components of filesystem. 

## DeleteOption
Currently, we define DeleteOption interface that can be used to implement custom options that can be used for delete operation. One such implementation is the [delete.AllVersions](./delete_options.md#DeleteAllVersions) option.

## Development

### Create new DeleteOption
To create your own DeleteOption, you must create a type that implements the DeleteOption interface:

```go
    type MyTakeBackupDeleteOption {
	    backupLocation string
    }

    func (o MyTakeBackupDeleteOption) DeleteOptionName() string {
        return "take-backup"
    }

    func (o MyTakeBackupDeleteOption) BackupLocation() string {
        return o.backupLocation
    }
```

Now, in each implementation of file.Delete(... options.DeleteOptions), implement the behaviour for this new option according to your need:

```go
    func (f *File) Delete(opts ...options.DeleteOption) error {
        for _, o := range opts {
            switch o.(type) {
            case delete.AllVersions:
                allVersions = true
			case delete.MyTakeBackupDeleteOption:
                // do something to take backup	
            default:
            }
        }
        ...
        ...
    }
```

