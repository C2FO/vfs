package sftp

import (
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v5"
)

// Options holds sftp-specific options.  Currently only client options are used.
type Options struct {
	clientCofnig ssh.ClientConfig
	Retry        vfs.Retry
}
