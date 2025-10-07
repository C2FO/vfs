package sftp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/mitchellh/go-homedir"
	_sftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

const systemWideKnownHosts = "/etc/ssh/ssh_known_hosts"

// Options holds sftp-specific options.  Currently only client options are used.
type Options struct {
	Username           string              `json:"username,omitempty"`       // env var VFS_SFTP_USERNAME
	Password           string              `json:"password,omitempty"`       // env var VFS_SFTP_PASSWORD
	KeyFilePath        string              `json:"keyFilePath,omitempty"`    // env var VFS_SFTP_KEYFILE
	KeyPassphrase      string              `json:"keyPassphrase,omitempty"`  // env var VFS_SFTP_KEYFILE_PASSPHRASE
	KnownHostsFile     string              `json:"knownHostsFile,omitempty"` // env var VFS_SFTP_KNOWN_HOSTS_FILE
	KnownHostsString   string              `json:"knownHostsString,omitempty"`
	KeyExchanges       []string            `json:"keyExchanges,omitempty"`
	Ciphers            []string            `json:"ciphers,omitempty"`
	MACs               []string            `json:"macs,omitempty"`
	HostKeyAlgorithms  []string            `json:"hostKeyAlgorithms,omitempty"`
	AutoDisconnect     int                 `json:"autoDisconnect,omitempty"` // seconds before disconnecting. default: 10
	ConnectTimeout     int                 `json:"connectTimeout,omitempty"` // seconds before conn AND auth timeout. default: 30
	KnownHostsCallback ssh.HostKeyCallback // env var VFS_SFTP_INSECURE_KNOWN_HOSTS
	FileBufferSize     int                 `json:"fileBufferSize,omitempty"`  // Buffer Size In Bytes Used with utils.TouchCopyBuffered
	FilePermissions    *string             `json:"filePermissions,omitempty"` // Default File Permissions for new files
}

// GetFileMode converts the FilePermissions string to os.FileMode.
func (o *Options) GetFileMode() (*os.FileMode, error) {
	if o.FilePermissions == nil {
		return nil, nil
	}

	// Convert the string to an unsigned integer, interpreting it as an octal value
	parsed, err := strconv.ParseUint(*o.FilePermissions, 0, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid file mode: %w", err)
	}
	return utils.Ptr(os.FileMode(parsed)), nil
}

var defaultSSHConfig = &ssh.ClientConfig{
	HostKeyAlgorithms: []string{
		"rsa-sha2-256-cert-v01@openssh.com",
		"rsa-sha2-512-cert-v01@openssh.com",
		"ssh-rsa-cert-v01@openssh.com",
		"ecdsa-sha2-nistp256-cert-v01@openssh.com",
		"ecdsa-sha2-nistp384-cert-v01@openssh.com",
		"ecdsa-sha2-nistp521-cert-v01@openssh.com",
		"ssh-ed25519-cert-v01@openssh.com",
		"ssh-ed25519",
		"ecdsa-sha2-nistp256",
		"ecdsa-sha2-nistp384",
		"ecdsa-sha2-nistp521",
		"ssh-rsa",
		"rsa-sha2-256",
		"rsa-sha2-512",
		"sk-ssh-ed25519@openssh.com",
		"sk-ecdsa-sha2-nistp256@openssh.com",
	},
	Config: ssh.Config{
		KeyExchanges: []string{
			"curve25519-sha256",
			"curve25519-sha256@libssh.org",
			"ecdh-sha2-nistp256",
			"ecdh-sha2-nistp384",
			"ecdh-sha2-nistp521",
			"diffie-hellman-group-exchange-sha256",
			"diffie-hellman-group16-sha512",
			"diffie-hellman-group18-sha512",
			"diffie-hellman-group14-sha256",
			"diffie-hellman-group14-sha1",
		},
		Ciphers: []string{
			"aes128-gcm@openssh.com",
			"aes256-gcm@openssh.com",
			"chacha20-poly1305@openssh.com",
			"aes256-ctr",
			"aes192-ctr",
			"aes128-ctr",
			"aes128-cbc",
			"3des-cbc",
		},
		MACs: []string{
			"hmac-sha2-256-etm@openssh.com",
			"hmac-sha2-512-etm@openssh.com",
			"hmac-sha2-256",
			"hmac-sha2-512",
			"hmac-sha1",
			"hmac-sha1-96",
		},
	},
}

// GetClient returns a new sftp client and underlying ssh client(io.Closer) using the given authority and options.
func GetClient(a authority.Authority, opts Options) (sftpclient *_sftp.Client, sshclient *ssh.Client, err error) {
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

	// Define the Client Config
	config := getSSHConfig(opts)
	// override with env var, if any
	if _, ok := os.LookupEnv("VFS_SFTP_USERNAME"); ok {
		config.User = os.Getenv("VFS_SFTP_USERNAME")
	}
	// Use username from Options if available, otherwise from authority
	if opts.Username != "" {
		config.User = opts.Username
	} else {
		config.User = a.UserInfo().Username()
	}
	config.Auth = authMethods
	config.HostKeyCallback = hostKeyCallback

	// Set connection timeout (default 30 seconds)
	// This timeout covers BOTH TCP connection AND SSH authentication
	connectTimeout := 30 * time.Second
	if opts.ConnectTimeout > 0 {
		connectTimeout = time.Duration(opts.ConnectTimeout) * time.Second
	}

	// default to port 22
	host := fmt.Sprintf("%s:%d", a.Host(), a.Port())
	if a.Port() == 0 {
		host = fmt.Sprintf("%s:%d", host, 22)
	}

	// Create context with timeout to cover full connection + auth
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	// Use custom dialer with timeout context
	// This ensures timeout covers both TCP connection AND SSH authentication
	var dialer net.Dialer
	netConn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to %s: %w", host, err)
	}

	// Set deadline on the connection to enforce timeout during SSH handshake/auth
	// This is critical for servers that hang during authentication
	deadline, _ := ctx.Deadline()
	if err := netConn.SetDeadline(deadline); err != nil {
		_ = netConn.Close()
		return nil, nil, fmt.Errorf("failed to set connection deadline: %w", err)
	}

	// Perform SSH handshake - will timeout if it exceeds the deadline set above
	sshClientConn, chans, reqs, err := ssh.NewClientConn(netConn, host, config)
	if err != nil {
		_ = netConn.Close()
		return nil, nil, fmt.Errorf("SSH handshake failed: %w", err)
	}

	// Clear deadline after successful connection - operations should not be time-limited
	if err := netConn.SetDeadline(time.Time{}); err != nil {
		_ = netConn.Close()
		return nil, nil, fmt.Errorf("failed to clear connection deadline: %w", err)
	}

	// Create SSH client from the connection
	sshConn := ssh.NewClient(sshClientConn, chans, reqs)

	sftpClient, err := _sftp.NewClient(sshConn)
	if err != nil {
		return nil, nil, err
	}

	return sftpClient, sshConn, nil
}

// getSSHConfig gets ssh config from Options
func getSSHConfig(opts Options) *ssh.ClientConfig {
	// copy default config
	config := *defaultSSHConfig

	// override default config with any user-defined config
	if opts.HostKeyAlgorithms != nil {
		config.HostKeyAlgorithms = opts.HostKeyAlgorithms
	}
	if opts.Ciphers != nil {
		config.Ciphers = opts.Ciphers
	}
	if opts.KeyExchanges != nil {
		config.KeyExchanges = opts.KeyExchanges
	}
	if opts.MACs != nil {
		config.MACs = opts.MACs
	}

	return &config
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
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // this is only used if a user specifically calls it (testing)

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
	homeKnownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	if runtime.GOOS != "windows" {
		homeKnownHostsPath = utils.EnsureLeadingSlash(homeKnownHostsPath)
	}

	// check file existence first to prevent auto-vivification of file
	found, err := foundFile(homeKnownHostsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if found {
		knownHostsFiles = append(knownHostsFiles, homeKnownHostsPath)
	}

	// add /etc/ssh/.ssh/known_hosts for unix-like systems.  SSH doesn't exist natively on Windows and each
	// implementation has a different location for known_hosts. Better to specify in KnownHostsFile for Windows
	if runtime.GOOS != "windows" {
		// check file existence first to prevent auto-vivification of file
		found, err := foundFile(systemWideKnownHosts)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
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
	buf, err := os.ReadFile(file) //nolint:gosec
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
