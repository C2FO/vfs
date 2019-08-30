package sftp

import (
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/mitchellh/go-homedir"
	_sftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/utils"
)

const systemWideKnownHosts = "/etc/ssh/ssh_known_hosts"

// Options holds sftp-specific options.  Currently only client options are used.
type Options struct {
	Password          string `json:"accessKeyId,omitempty"`   // env var VFS_SFTP_PASSWORD
	KeyFilePath       string `json:"keyFilePath,omitempty"`   // env var VFS_SFTP_KEYFILE
	KeyPassphrase     string `json:"KeyPassphrase,omitempty"` // env var VFS_SFTP_KEYFILE_PASSPHRASE
	KnownHostString   string // env var VFS_SFTP_KNOWN_HOST_STRING
	KnownHostsFile    string // env var VFS_SFTP_KNOWN_HOSTS_FILE
	KnonwHostCallback ssh.HostKeyCallback
	Retry             vfs.Retry
	MaxRetries        int
}

func getClient(authority utils.Authority, opts Options) (*_sftp.Client, error) {

	// setup Authentication
	authMethods, err := getAuthMethods(opts)
	if err != nil {
		return nil, err
	}

	// get callback for handling known_hosts man-in-the-middle checks
	hostKeyCallback, err := getHostKeyCallback(opts)
	if err != nil {
		return nil, err
	}

	// Define the Client Config
	config := &ssh.ClientConfig{
		User:            authority.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
	}

	//default to port 22
	host := authority.Host
	if !strings.Contains(host, ":") {
		host = host + ":22"
	}

	//TODO begin timeout until session is created
	sshClient, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return nil, err
	}

	client, err := _sftp.NewClient(sshClient)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// getHostKeyCallback gets host key callback for all known_hosts files
func getHostKeyCallback(opts Options) (ssh.HostKeyCallback, error) {
	var knownHostsFiles []string
	switch {

	// use explicit callback in Options
	case opts.KnonwHostCallback != nil:
		return opts.KnonwHostCallback, nil

	case opts.KnownHostString != "":
		hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(opts.KnownHostString))
		if err != nil {
			return nil, err
		}
		return ssh.FixedHostKey(hostKey), nil

	// use env var known_hosts file path, ie, /home/bob/.ssh/known_hosts
	case opts.KnownHostsFile != "":
		//check first to prevent auto-vivification of file
		found, err := foundFile(opts.KnownHostsFile)
		if err != nil {
			return nil, err
		}
		if found {
			knownHostsFiles = append(knownHostsFiles, os.Getenv("VFS_SFTP_KNOWN_HOSTS"))
			break
		}
		// use default if env var file wasn't found
		fallthrough

	// use env var known_hosts key string, ie, "AAAAB3Nz...cTqGvaDhgtAhw=="
	case os.Getenv("VFS_SFTP_KNOWN_HOSTS_STRING") != "":
		hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(os.Getenv("VFS_SFTP_KNOWN_HOSTS_STRING")))
		if err != nil {
			return nil, err
		}
		return ssh.FixedHostKey(hostKey), nil

	// use env var known_hosts file path, ie, /home/bob/.ssh/known_hosts
	case os.Getenv("VFS_SFTP_KNOWN_HOSTS_FILE") != "":
		//check first to prevent auto-vivification of file
		found, err := foundFile(os.Getenv("VFS_SFTP_KNOWN_HOSTS"))
		if err != nil {
			return nil, err
		}
		if found {
			knownHostsFiles = append(knownHostsFiles, os.Getenv("VFS_SFTP_KNOWN_HOSTS"))
			break
		}
		// use default if env var file wasn't found
		fallthrough

	// use user/system-wide known_hosts paths (as defined by OpenSSH https://man.openbsd.org/ssh)
	default:
		// add ~/.ssh/known_hosts
		home, err := homedir.Dir()
		if err != nil {
			return nil, err
		}
		homeKnonwHostsPath := utils.EnsureLeadingSlash(path.Join(home, ".ssh/known_hosts"))

		//check file existence first to prevent auto-vivification of file
		found, err := foundFile(homeKnonwHostsPath)
		if err != nil {
			return nil, err
		}
		if found {
			knownHostsFiles = append(knownHostsFiles, homeKnonwHostsPath)
		}

		// add /etc/ssh/.ssh/known_hosts for unix-like systems.  SSH doesn't exist natively on Windows and each
		// implemenation has a different location for known_hosts. Better to specify in
		if runtime.GOOS != "windows" {
			//check file existence first to prevent auto-vivification of file
			found, err := foundFile(systemWideKnownHosts)
			if err != nil {
				return nil, err
			}
			if found {
				knownHostsFiles = append(knownHostsFiles, systemWideKnownHosts)
			}
		}
	}

	// get host key callback for all known_hosts files
	cb, err := knownhosts.New(knownHostsFiles...)
	if err != nil {
		return nil, err
	}

	return cb, nil
}

func foundFile(file string) (bool, error) {
	if _, err := os.Stat(file); err != nil {
		if os.IsNotExist(err) {
			// file does not exist
			return false, nil
		}
		// other error
		return false, err
	}
	return true, nil
}

func getAuthMethods(opts Options) ([]ssh.AuthMethod, error) {

	auth := make([]ssh.AuthMethod, 0)

	// setup key-based auth from env, if any
	keyfile := os.Getenv("VFS_SFTP_KEYFILE")
	if keyfile != "" {
		secretKey, err := getKeyFile(keyfile, os.Getenv("VFS_SFTP_KEYFILE_PASSPHRASE"))
		if err != nil {
			return []ssh.AuthMethod{}, err
		}
		auth = append(auth, ssh.AuthMethod(ssh.PublicKeys(secretKey)))
	}

	// setup key-based auth from opts, if any
	if opts.KeyFilePath != "" {
		secretKey, err := getKeyFile(opts.KeyFilePath, opts.KeyPassphrase)
		if err != nil {
			return []ssh.AuthMethod{}, err
		}
		auth = append(auth, ssh.AuthMethod(ssh.PublicKeys(secretKey)))
	}

	// setup password, env password overrides opts password
	password := opts.Password
	if pass, found := os.LookupEnv("VFS_SFTP_PASSWORD"); found {
		password = pass
	}
	if password != "" {
		auth = append(auth, ssh.AuthMethod(ssh.Password(password)))
	}

	return auth, nil
}

func getKeyFile(file, passphrase string) (key ssh.Signer, err error) {

	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return
	}
	if passphrase != "" {
		key, err = ssh.ParsePrivateKeyWithPassphrase(buf, []byte(passphrase))
		if err != nil {
			return
		}
	} else {
		key, err = ssh.ParsePrivateKey(buf)
		if err != nil {
			return
		}
	}
	return
}
