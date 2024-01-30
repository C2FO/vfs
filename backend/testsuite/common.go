package testsuite

import (
	"os"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/azure"
	"github.com/c2fo/vfs/v6/backend/ftp"
	"github.com/c2fo/vfs/v6/backend/gs"
	"github.com/c2fo/vfs/v6/backend/mem"
	_os "github.com/c2fo/vfs/v6/backend/os"
	"github.com/c2fo/vfs/v6/backend/s3"
	"github.com/c2fo/vfs/v6/backend/sftp"
)

func CopyOsLocation(loc vfs.Location) vfs.Location {
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

func CopyMemLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*mem.Location)
	return &cp
}

func CopyS3Location(loc vfs.Location) vfs.Location {
	cp := *loc.(*s3.Location)
	return &cp
}

func CopySFTPLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*sftp.Location)
	return &cp
}

func CopyFTPLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*ftp.Location)
	return &cp
}

func CopyGSLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*gs.Location)
	return &cp
}

func CopyAzureLocation(loc vfs.Location) vfs.Location {
	cp := *loc.(*azure.Location)
	return &cp
}
