package sftp

import (
	"io/ioutil"
	"os"
	"sync"

	"github.com/google/uuid"
	_sftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/utils"
)

// Options holds sftp-specific options.  Currently only client options are used.
type Options struct {
	KeyFilePath   string `json:"keyFilePath,omitempty"`   // env var VFS_SFTP_KEYFILE
	KeyPassphrase string `json:"KeyPassphrase,omitempty"` // env var VFS_SFTP_KEYFILE_PASSPHRASE
	Password      string `json:"accessKeyId,omitempty"`   // env var VFS_SFTP_PASSWORD
	Retry         vfs.Retry
	MaxRetries    int
}

func getClient(authority utils.Authority, opts Options) (*_sftp.Client, error) {

	// setup Authentication
	authMethods, err := getAuthMethods(opts)
	if err != nil {
		return nil, err
	}

	// Define the Client Config
	config := &ssh.ClientConfig{
		User:            authority.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //todo consider making this configurable
	}

	//TODO begin timeout until session is created
	sshClient, err := ssh.Dial("tcp", authority.Host, config)
	if err != nil {
		return nil, err
	}

	client, err := _sftp.NewClient(sshClient)
	if err != nil {
		return nil, err
	}

	return client, nil
}

type sshConn struct {
	conn     *ssh.Client
	sessions map[string]*sftpSession
	sync.RWMutex
}

func (s *sshConn) NewSession() (*sftpSession, error) {
	u := uuid.New().String()

	client, err := _sftp.NewClient(s.conn)
	if err != nil {
		return nil, err
	}

	return &sftpSession{
		Client: client,
		s:      s,
		uuid:   u,
	}, nil
}

func (s *sshConn) RemoveSession(uuid string) error {
	s.Lock()
	defer s.Unlock()
	if sess, ok := s.sessions[uuid]; ok {
		err := sess.Close()
		if err != nil {

		}
	}
	return nil
}

// sftp session wraps sftp.Client so we can
type sftpSession struct {
	*_sftp.Client
	s    *sshConn
	uuid string
}

func (sf *sftpSession) Close() error {
	err := sf.Client.Close()
	if err != nil {
		return err
	}
	return sf.s.RemoveSession(sf.uuid)
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
	//TODO: ssh.ParsePrivateKeyWithPassphrase() expects pemBytes.  DOes this work if not PEM?

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
