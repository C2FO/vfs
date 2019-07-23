package sftp

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/user"
	"path"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/backend"
	"github.com/c2fo/vfs/v5/utils"
)

// Scheme defines the filesystem type.
const Scheme = "sftp"
const name = "Secure File Transfer Protocol"

// FileSystem implements vfs.Filesystem for the SFTP filesystem.
type FileSystem struct {
	options    vfs.Options
	client     *sftp.Client
	sftpclient *sftp.Client
}

// Retry will return the default no-op retrier. The SFTP client provides its own retryer interface, and is available
// to override via the sftp.FileSystem Options type.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the SFTP implementation of vfs.File.
func (fs *FileSystem) NewFile(authority string, filePath string) (vfs.File, error) {
	if fs == nil {
		return nil, errors.New("non-nil sftp.fileSystem pointer is required")
	}
	if filePath == "" {
		return nil, errors.New("non-empty string for path is required")
	}
	if err := utils.ValidateAbsoluteFilePath(filePath); err != nil {
		return nil, err
	}

	auth, err := utils.NewAuthority(authority)
	if err != nil {
		return nil, err
	}

	return &File{
		fileSystem: fs,
		Authority:  auth,
		path:       path.Clean(filePath),
	}, nil
}

// NewLocation function returns the SFTP implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authority string, locPath string) (vfs.Location, error) {
	if fs == nil {
		return nil, errors.New("non-nil sftp.fileSystem pointer is required")
	}
	if err := utils.ValidateAbsoluteLocationPath(locPath); err != nil {
		return nil, err
	}

	auth, err := utils.NewAuthority(authority)
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		path:       utils.EnsureTrailingSlash(path.Clean(locPath)),
		Authority:  auth,
	}, nil
}

// Name returns "Secure File Transfer Protocol"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme return "sftp" as the initial part of a file URI ie: sftp://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Client returns the underlying sftp client, creating it, if necessary
// See Overview for authentication resolution
func (fs *FileSystem) Client(authority utils.Authority) (*sftp.Client, error) {
	if fs.client == nil {
		if fs.options == nil {
			fs.options = Options{}
		}

		if _, ok := fs.options.(Options); ok {
			var err error
			fs.client, err = fs.getClient(authority)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("unable to create client, vfs.Options must be an sftp.Options")
		}
	}
	return fs.client, nil
}

// WithOptions sets options for client and returns the filesystem (chainable)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {

	// only set options if vfs.Options is sftp.Options
	if opts, ok := opts.(Options); ok {
		fs.options = opts
		//we set client to nil to ensure that a new client is created using the new context when Client() is called
		fs.client = nil
	}
	return fs
}

// WithClient passes in an sftp client and returns the filesystem (chainable)
func (fs *FileSystem) WithClient(client interface{}) *FileSystem {
	switch client.(type) {
	case *ssh.Client:
		fs.client = client.(*sftp.Client)
		fs.options = nil
	}
	return fs
}

// NewFileSystem initializer for fileSystem struct.
func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

func init() {
	//registers a default Filesystem
	backend.Register(Scheme, NewFileSystem())
}

func (fs *FileSystem) getClient(authority utils.Authority) (*sftp.Client, error) {
	if fs.sftpclient != nil {

		// Now in the main function DO:
		secretKey, err := getKeyFile()
		if err != nil {
			panic(err)
		}
		// Define the Client Config as :
		config := &ssh.ClientConfig{
			User: authority.User,
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(secretKey),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			BannerCallback:  ssh.BannerDisplayStderr(),
		}
		//TODO begin timeout until session is created
		sshClient, err := ssh.Dial("tcp", authority.Host, config)
		if err != nil {
			return nil, err
		}

		client, err := sftp.NewClient(sshClient)
		if err != nil {
			return nil, err
		}

		fs.sftpclient = client
	}

	return fs.sftpclient, nil
}

func getKeyFile() (key ssh.Signer, err error) {
	usr, _ := user.Current()
	file := usr.HomeDir + "/.ssh/id_rsa"
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return
	}
	key, err = ssh.ParsePrivateKeyWithPassphrase(buf, []byte("AlephBet6"))
	if err != nil {
		return
	}
	return
}
