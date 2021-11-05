package sftp

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/mitchellh/go-homedir"
	_sftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/utils"
)

const systemWideKnownHosts = "/etc/ssh/ssh_known_hosts"

// Options holds sftp-specific options.  Currently only client options are used.
type Options struct {
	Password           string              `json:"password,omitempty"`       // env var VFS_SFTP_PASSWORD
	KeyFilePath        string              `json:"keyFilePath,omitempty"`    // env var VFS_SFTP_KEYFILE
	KeyPassphrase      string              `json:"keyPassphrase,omitempty"`  // env var VFS_SFTP_KEYFILE_PASSPHRASE
	KnownHostsFile     string              `json:"knownHostsFile,omitempty"` // env var VFS_SFTP_KNOWN_HOSTS_FILE
	KnownHostsString   string              `json:"knownHostsString,omitempty"`
	KeyExchanges       []string            `json:"keyExchanges,omitempty"`
	AutoDisconnect     int                 `json:"autoDisconnect,omitempty"` // seconds before disconnecting. default: 10
	KnownHostsCallback ssh.HostKeyCallback // env var VFS_SFTP_INSECURE_KNOWN_HOSTS
	Retry              vfs.Retry
	MaxRetries         int
	FileBufferSize     int // Buffer Size In Bytes Used with utils.TouchCopyBuffered
}

// Note that as of 1.12, OPENSSH private key format is not supported when encrypt (with passphrase).
// See https://github.com/golang/go/issues/18692
// To force creation of PEM format(instead of OPENSSH format), use ssh-keygen -m PEM

func getClient(authority utils.Authority, opts Options) (Client, io.Closer, error) {

	// setup Authentication
	authMethods, err := getAuthMethods(opts)
	if err != nil {
		return nil, nil, err
	}

	// get callback for handling known_hosts man-in-the-middle checks
	hostKeyCallback, err := getHostKeyCallback(opts)
	if err != nil {
		return nil, nil, err
	}
	// To avoid ssh: handshake failed: ssh: no common algorithm for key exchange;
	// client offered: [curve25519-sha256@libssh.org ecdh-sha2-nistp256 ecdh-sha2-nistp384 ecdh-sha2-	nistp521 diffie-hellman-group14-sha1],
	// server offered: [diffie-hellman-group-exchange-sha256 ]
	// Now receive KeyExchange algorithm as an option
	sshConfig := ssh.Config{KeyExchanges: opts.KeyExchanges}

	// Define the Client Config
	config := &ssh.ClientConfig{
		User:            authority.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Config:          sshConfig,
	}
	// default to port 22
	host := authority.Host
	if !strings.Contains(host, ":") {
		host = fmt.Sprintf("%s%s", host, ":22")
	}

	// TODO begin timeout until session is created
	sshConn, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return nil, nil, err
	}

	sftpClient, err := _sftp.NewClient(sshConn)
	if err != nil {
		return nil, nil, err
	}

	return sftpClient, sshConn, nil
}

// getHostKeyCallback gets host key callback for all known_hosts files
func getHostKeyCallback(opts Options) (ssh.HostKeyCallback, error) {
	var knownHostsFiles []string
	switch {

	// use explicit callback in Options
	case opts.KnownHostsCallback != nil:
		return opts.KnownHostsCallback, nil

	case opts.KnownHostsString != "":
		hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(opts.KnownHostsString))
		if err != nil {
			return nil, err
		}
		return ssh.FixedHostKey(hostKey), nil

	// use env var known_hosts file path, ie, /home/bob/.ssh/known_hosts
	case opts.KnownHostsFile != "":
		// check first to prevent auto-vivification of file
		found, err := foundFile(opts.KnownHostsFile)
		if err != nil {
			return nil, err
		}
		if found {
			knownHostsFiles = append(knownHostsFiles, opts.KnownHostsFile)
			break
		}
		// use env var if explicit file wasn't found wasn't found
		fallthrough

	// use env var known_hosts file path, ie, /home/bob/.ssh/known_hosts
	case os.Getenv("VFS_SFTP_KNOWN_HOSTS_FILE") != "":
		// check first to prevent auto-vivification of file
		found, err := foundFile(os.Getenv("VFS_SFTP_KNOWN_HOSTS_FILE"))
		if err != nil {
			return nil, err
		}
		if found {
			knownHostsFiles = append(knownHostsFiles, os.Getenv("VFS_SFTP_KNOWN_HOSTS_FILE"))
			break
		}
		// use default if env var file wasn't found
		fallthrough

	// use env var known_hosts file path, ie, /home/bob/.ssh/known_hosts
	case os.Getenv("VFS_SFTP_INSECURE_KNOWN_HOSTS") != "":
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // this is only use if a uer specifically call it (testing)

	// use user/system-wide known_hosts paths (as defined by OpenSSH https://man.openbsd.org/ssh)
	default:
		var err error
		knownHostsFiles, err = findHomeSystemKnownHosts(knownHostsFiles)
		if err != nil {
			return nil, err
		}
	}

	// get host key callback for all known_hosts files
	return knownhosts.New(knownHostsFiles...)
}

func findHomeSystemKnownHosts(knownHostsFiles []string) ([]string, error) {
	// add ~/.ssh/known_hosts
	home, err := homedir.Dir()
	if err != nil {
		return nil, err
	}
	homeKnonwHostsPath := utils.EnsureLeadingSlash(path.Join(home, ".ssh/known_hosts"))

	// check file existence first to prevent auto-vivification of file
	found, err := foundFile(homeKnonwHostsPath)
	if err != nil && err != os.ErrNotExist {
		return nil, err
	}
	if found {
		knownHostsFiles = append(knownHostsFiles, homeKnonwHostsPath)
	}

	// add /etc/ssh/.ssh/known_hosts for unix-like systems.  SSH doesn't exist natively on Windows and each
	// implementation has a different location for known_hosts. Better to specify in KnownHostsFile for Windows
	if runtime.GOOS != "windows" {
		// check file existence first to prevent auto-vivification of file
		found, err := foundFile(systemWideKnownHosts)
		if err != nil && err != os.ErrNotExist {
			return nil, err
		}
		if found {
			knownHostsFiles = append(knownHostsFiles, systemWideKnownHosts)
		}
	}
	return knownHostsFiles, nil
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

	// explicitly set password from opts, then from env if any
	pw := os.Getenv("VFS_SFTP_PASSWORD")
	if opts.Password != "" {
		pw = opts.Password
	}
	if pw != "" {
		auth = append(auth, ssh.Password(pw))
	}

	// setup key-based auth from env, if any
	keyfile := os.Getenv("VFS_SFTP_KEYFILE")
	if opts.KeyFilePath != "" {
		keyfile = opts.KeyFilePath
	}
	if keyfile != "" {
		// gather passphrase, if any
		passphrase := os.Getenv("VFS_SFTP_KEYFILE_PASSPHRASE")
		if opts.KeyPassphrase != "" {
			passphrase = opts.KeyPassphrase
		}

		// setup keyfile
		secretKey, err := getKeyFile(keyfile, passphrase)
		if err != nil {
			return []ssh.AuthMethod{}, err
		}
		auth = append(auth, ssh.PublicKeys(secretKey))
	}

	return auth, nil
}

func getKeyFile(file, passphrase string) (key ssh.Signer, err error) {

	buf, err := ioutil.ReadFile(file) //nolint:gosec
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
