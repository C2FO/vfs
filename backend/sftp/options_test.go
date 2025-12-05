package sftp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

/**********************************
 ************TESTS*****************
 **********************************/

type optionsSuite struct {
	suite.Suite
	tmpdir    string
	keyFiles  keyFiles
	publicKey ssh.PublicKey
}

func (o *optionsSuite) SetupSuite() {
	o.tmpdir = o.T().TempDir()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	o.Require().NoError(err)
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	o.Require().NoError(err)
	o.publicKey = publicKey

	keyFiles, err := setupKeyFiles(o.tmpdir)
	o.Require().NoError(err)
	o.keyFiles = *keyFiles
}

func (o *optionsSuite) TestFoundFile() {
	// test file
	filename := filepath.Join(o.tmpdir, "some.key")
	f, err := os.Create(filename) //nolint:gosec
	o.Require().NoError(err, "create file for foundfile test")
	_, err = f.WriteString("blah")
	o.Require().NoError(err, "writing to file for foundfile test")
	o.Require().NoError(f.Close(), "closing file for foundfile test")
	defer func() { o.Require().NoError(os.Remove(filename), "clean up file for foundfile test") }()

	tests := []struct {
		file       string
		expected   bool
		hasError   bool
		errMessage string
		message    string
	}{
		{
			file:       filename,
			expected:   true,
			hasError:   false,
			errMessage: "",
			message:    "should find file",
		},
		{
			file:       filepath.Join(o.tmpdir, "nonexistent.key"),
			expected:   false,
			hasError:   false,
			errMessage: "",
			message:    "shouldn't find file",
		},
	}

	for _, t := range tests {
		o.Run(t.message, func() {
			actual, err := foundFile(t.file)
			if t.hasError {
				o.Require().EqualError(err, t.errMessage, t.message)
			} else {
				o.Require().NoError(err, t.message)
				o.Equal(t.expected, actual, t.message)
			}
		})
	}
}

func (o *optionsSuite) TestGetKeyFile() {
	tests := []struct {
		keyfile    string
		passphrase string
		hasError   bool
		err        error
		errMessage string
		message    string
	}{
		{
			keyfile:    o.keyFiles.SSHPrivateKey,
			passphrase: o.keyFiles.passphrase,
			hasError:   false,
			errMessage: "",
			message:    "key should parse with passphrase",
		},
		{
			keyfile:    o.keyFiles.SSHPrivateKeyNoPassphrase,
			passphrase: "",
			hasError:   false,
			errMessage: "",
			message:    "key should parse, no passphrase",
		},
		{
			keyfile:    "nonexistent.key",
			passphrase: "",
			hasError:   true,
			err:        os.ErrNotExist,
			message:    "file not found",
		},
		{
			keyfile:    o.keyFiles.SSHPrivateKey,
			passphrase: "",
			hasError:   true,
			errMessage: "ssh: this private key is passphrase protected",
			// error message changed from "ssh: cannot decode encrypted private keys" to
			// "ssh: this private key is passphrase protected"
			// in https://github.com/golang/crypto/commit/0a08dada0ff98d02f3864a23ae8d27cb8fba5303
			message: "missing passphrase",
		},
		{
			keyfile:    o.keyFiles.SSHPrivateKey,
			passphrase: "badpass",
			hasError:   true,
			errMessage: "x509: decryption password incorrect",
			message:    "bad passphrase",
		},
	}

	for _, t := range tests {
		o.Run(t.message, func() {
			_, err := getKeyFile(t.keyfile, t.passphrase)
			if t.hasError {
				if t.err != nil {
					o.Require().ErrorIs(err, t.err, t.message)
				} else {
					o.Require().EqualError(err, t.errMessage, t.message)
				}
			} else {
				o.Require().NoError(err, t.message)
			}
		})
	}
}

func (o *optionsSuite) TestGetHostKeyCallback() {
	knownHosts := filepath.Join(o.tmpdir, "known_hosts")
	f, err := os.Create(knownHosts) //nolint:gosec
	o.Require().NoError(err, "create file for getHostKeyCallback test")
	_, err = f.WriteString("127.0.0.1 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBMkEmvHLSa43yoLA8QBqTfwgXgNCfd0DKs20NlBVbMoo21+Bs0fUpemyy6U0nnGHiOJVhiL7lNG/lB1fF1ymouM=") //nolint:lll // long line
	o.Require().NoError(err, "writing to file for getHostKeyCallback test")
	o.Require().NoError(f.Close(), "closing file for getHostKeyCallback test")
	defer func() { o.Require().NoError(os.Remove(knownHosts), "clean up file for getHostKeyCallback test") }()

	tests := []struct {
		options    Options
		envVars    map[string]string
		hasError   bool
		errMessage string
		message    string
	}{
		{
			options: Options{
				KnownHostsCallback: ssh.FixedHostKey(o.publicKey),
			},
			hasError:   false,
			errMessage: "",
			message:    "explicit Options callback",
		},
		{
			options: Options{
				KnownHostsString: "127.0.0.1 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBMkEmvHLSa43yoLA8QBqTfwgXgNCfd0DKs20NlBVbMoo21+Bs0fUpemyy6U0nnGHiOJVhiL7lNG/lB1fF1ymouM=", //nolint:lll // long line
			},
			hasError:   false,
			errMessage: "",
			message:    "Options KnownHostsString",
		},
		{
			options: Options{
				KnownHostsString: "blah",
			},
			hasError:   true,
			errMessage: "ssh: no key found",
			message:    "Options KnownHostsString, malformed",
		},
		{
			options: Options{
				KnownHostsFile: knownHosts,
			},
			hasError:   false,
			errMessage: "",
			message:    "Options KnownHostsFile",
		},
		{
			options: Options{
				KnownHostsFile: "nonexistent.key",
			},
			envVars: map[string]string{
				"VFS_SFTP_INSECURE_KNOWN_HOSTS": "true",
			},
			hasError:   false,
			errMessage: "",
			message:    "insecure known hosts",
		},
		{
			options: Options{},
			envVars: map[string]string{
				"VFS_SFTP_KNOWN_HOSTS_FILE": knownHosts,
			},
			hasError:   false,
			errMessage: "",
			message:    "Env fallthrough KnownHostsFile",
		},
		{ // TODO: this may be a bad test if a user/system-wide known_hosts file isn't found
			hasError:   false,
			errMessage: "",
			message:    "default fallthrough KnownHostsFile",
		},
	} // #nosec - InsecureIgnoreHostKey only used for testing

	for _, t := range tests { //nolint:gocritic // rangeValCopy
		o.Run(t.message, func() {
			// setup env vars, if any
			for k, v := range t.envVars {
				o.T().Setenv(k, v)
			}

			// apply test
			_, err := getHostKeyCallback(t.options)
			if t.hasError {
				o.Require().EqualError(err, t.errMessage, t.message)
			} else {
				o.Require().NoError(err, t.message)
			}
		})
	}
}

func (o *optionsSuite) TestGetAuthMethods() {
	tests := []struct {
		options     Options
		envVars     map[string]string
		returnCount int
		hasError    bool
		errMessage  string
		err         error
		message     string
	}{
		{
			options: Options{
				Password: "somepassword",
			},
			returnCount: 1,
			hasError:    false,
			errMessage:  "",
			message:     "explicit Options password",
		},
		{
			envVars: map[string]string{
				"VFS_SFTP_PASSWORD": "somepassword",
			},
			returnCount: 1,
			hasError:    false,
			errMessage:  "",
			message:     "env var password",
		},
		{
			envVars: map[string]string{
				"VFS_SFTP_KEYFILE": o.keyFiles.SSHPrivateKeyNoPassphrase,
			},
			returnCount: 1,
			hasError:    false,
			errMessage:  "",
			message:     "env var keyfile - no password",
		},
		{
			envVars: map[string]string{
				"VFS_SFTP_KEYFILE":            o.keyFiles.SSHPrivateKey,
				"VFS_SFTP_KEYFILE_PASSPHRASE": o.keyFiles.passphrase,
			},
			returnCount: 1,
			hasError:    false,
			errMessage:  "",
			message:     "env var keyfile - with passphrase",
		},
		{
			// behavior was fixed in https://github.com/golang/crypto/commit/0a08dada0ff98d02f3864a23ae8d27cb8fba5303
			// such that sending a passphrase with an unencrypted keyfile now throws error.  This test added
			// to reflect this case.  Test above was altered to reflect what it was testing (env var passphrase).
			envVars: map[string]string{
				"VFS_SFTP_KEYFILE":            o.keyFiles.SSHPrivateKeyNoPassphrase,
				"VFS_SFTP_KEYFILE_PASSPHRASE": o.keyFiles.passphrase,
			},
			returnCount: 1,
			hasError:    true,
			errMessage:  "ssh: not an encrypted key",
			message:     "unencrypted keyfile - with passphrase",
		},
		{
			options: Options{
				KeyFilePath: o.keyFiles.SSHPrivateKeyNoPassphrase,
			},
			returnCount: 1,
			hasError:    false,
			errMessage:  "",
			message:     "explicit Options keypath - no passphrase",
		},
		{
			options: Options{
				KeyFilePath:   o.keyFiles.SSHPrivateKey,
				KeyPassphrase: o.keyFiles.passphrase,
			},
			returnCount: 1,
			hasError:    false,
			errMessage:  "",
			message:     "explicit Options keypath - no passphrase",
		},
		{
			envVars: map[string]string{
				"VFS_SFTP_KEYFILE": o.keyFiles.SSHPrivateKeyNoPassphrase, // overridden by explicit options value
			},
			options: Options{
				KeyFilePath:   o.keyFiles.SSHPrivateKey,
				KeyPassphrase: o.keyFiles.passphrase,
				Password:      "somepassword",
				KeyExchanges:  []string{"diffie-hellman-group-exchange-sha256"},
			},
			returnCount: 2,
			hasError:    false,
			errMessage:  "",
			message:     "multiple auths",
		},
		{
			options: Options{
				Password:     "somepassword",
				KeyExchanges: []string{"diffie-hellman-group-exchange-sha256", "ecdh-sha2-nistp256"},
			},
			returnCount: 1,
			hasError:    false,
			errMessage:  "",
			message:     "multiple key exchange algorithms",
		},
		{
			envVars: map[string]string{
				"VFS_SFTP_KEYFILE": "nonexistent.key",
			},
			returnCount: 1,
			hasError:    true,
			err:         os.ErrNotExist,
			message:     "env var keyfile returns error for file not found",
		},
	}

	for _, t := range tests { //nolint:gocritic // rangeValCopy
		o.Run(t.message, func() {
			// setup env vars, if any
			for k, v := range t.envVars {
				o.T().Setenv(k, v)
			}

			// apply test
			auth, err := getAuthMethods(t.options)
			if t.hasError {
				if t.err != nil {
					o.Require().ErrorIs(err, t.err, t.message)
				} else {
					o.Require().EqualError(err, t.errMessage, t.message)
				}
			} else {
				o.Require().NoError(err, t.message)
				o.Len(auth, t.returnCount, "auth count")
			}
		})
	}
}

func (o *optionsSuite) TestGetClient() {
	auth, err := authority.NewAuthority("someuser@badhost")
	o.Require().NoError(err)

	authNoUser, err := authority.NewAuthority("badhost")
	o.Require().NoError(err)

	// Set environment variable for testing
	o.T().Setenv("VFS_SFTP_USERNAME", "envuser")

	tests := []struct {
		options   Options
		authority authority.Authority
		hasError  bool
		err       error
		errRegex  string
		message   string
	}{
		{
			authority: auth,
			options: Options{
				Password:           "somepassword",
				KnownHostsCallback: ssh.FixedHostKey(o.publicKey),
			},
			hasError: true,
			errRegex: ".*",
			message:  "getclient - bad host",
		},
		{
			authority: auth,
			options: Options{
				KeyFilePath:        "nonexistent.key",
				KnownHostsCallback: ssh.FixedHostKey(o.publicKey),
			},
			hasError: true,
			err:      os.ErrNotExist,
			message:  "getclient - bad auth key",
		},
		{
			authority: auth,
			options: Options{
				Password:         "somepassword",
				KnownHostsString: "badstring",
			},
			hasError: true,
			errRegex: "ssh: no key found",
			message:  "getclient - bad known hosts",
		},
		{
			authority: authNoUser,
			options: Options{
				Username:           "customuser",
				Password:           "somepassword",
				KnownHostsCallback: ssh.FixedHostKey(o.publicKey),
			},
			hasError: true,
			errRegex: ".*",
			message:  "getclient - username from options",
		},
		{
			authority: authNoUser,
			options: Options{
				Password:           "somepassword",
				KnownHostsCallback: ssh.FixedHostKey(o.publicKey),
			},
			hasError: true,
			errRegex: ".*",
			message:  "getclient - username from environment",
		},
	} // #nosec - InsecureIgnoreHostKey only used for testing

	for _, t := range tests { //nolint:gocritic // rangeValCopy
		o.Run(t.message, func() {
			_, _, err := GetClient(t.authority, t.options)
			if t.hasError {
				o.Require().Error(err)
				if t.err != nil {
					o.Require().ErrorIs(err, t.err, t.message)
				} else {
					re := regexp.MustCompile(t.errRegex)
					o.Regexp(re, err.Error(), "error matches")
				}
			} else {
				o.Require().NoError(err, t.message)
			}
		})
	}
}

func (o *optionsSuite) TestMarshalOptions() {
	// address bug #49 where json struct tag was misnamed
	pw := "secret1234"
	kh := "/path/to/known_hosts"

	opts := map[string]any{
		"password":    pw,
		"keyFilePath": kh,
	}

	raw, err := json.Marshal(opts)
	o.Require().NoError(err)
	optStruct := &Options{}
	err = json.Unmarshal(raw, optStruct)
	o.Require().NoError(err)

	o.Equal(kh, optStruct.KeyFilePath, "KeyFilePath check")
	o.Equal(pw, optStruct.Password, "Password check")
}

func (o *optionsSuite) TestGetSSHConfig() {
	tests := []struct {
		name   string
		opts   Options
		expect *ssh.ClientConfig
	}{
		{
			name:   "DefaultConfig",
			opts:   Options{},
			expect: defaultSSHConfig,
		},
		{
			name: "CustomHostKeyAlgorithms",
			opts: Options{
				HostKeyAlgorithms: []string{"ssh-rsa", "ecdsa-sha2-nistp256"},
			},
			expect: &ssh.ClientConfig{
				HostKeyAlgorithms: []string{"ssh-rsa", "ecdsa-sha2-nistp256"},
				Config:            defaultSSHConfig.Config,
			},
		},
		{
			name: "CustomCiphers",
			opts: Options{
				Ciphers: []string{"aes128-ctr", "aes192-ctr", "aes256-ctr"},
			},
			expect: &ssh.ClientConfig{
				HostKeyAlgorithms: defaultSSHConfig.HostKeyAlgorithms,
				Config: ssh.Config{
					Ciphers:      []string{"aes128-ctr", "aes192-ctr", "aes256-ctr"},
					MACs:         defaultSSHConfig.MACs,
					KeyExchanges: defaultSSHConfig.KeyExchanges,
				},
			},
		},
		{
			name: "CustomMACs",
			opts: Options{
				MACs: []string{""},
			},
			expect: &ssh.ClientConfig{
				HostKeyAlgorithms: defaultSSHConfig.HostKeyAlgorithms,
				Config: ssh.Config{
					Ciphers:      defaultSSHConfig.Ciphers,
					MACs:         []string{""},
					KeyExchanges: defaultSSHConfig.KeyExchanges,
				},
			},
		},
		{
			name: "CustomKeyExchanges",
			opts: Options{
				KeyExchanges: []string{"diffie-hellman-group-exchange-sha256", "ecdh-sha2-nistp256"},
			},
			expect: &ssh.ClientConfig{
				HostKeyAlgorithms: defaultSSHConfig.HostKeyAlgorithms,
				Config: ssh.Config{
					Ciphers:      defaultSSHConfig.Ciphers,
					MACs:         defaultSSHConfig.MACs,
					KeyExchanges: []string{"diffie-hellman-group-exchange-sha256", "ecdh-sha2-nistp256"},
				},
			},
		},
	}

	for _, tc := range tests { //nolint:gocritic // rangeValCopy
		o.Run(tc.name, func() {
			result := getSSHConfig(tc.opts)
			o.Equal(tc.expect, result)
		})
	}
}

func (o *optionsSuite) TestGetFileMode() {
	tests := []struct {
		name            string
		filePermissions *string
		expectedMode    *os.FileMode
		expectError     bool
	}{
		{
			name:            "NilFilePermissions",
			filePermissions: nil,
			expectedMode:    nil,
			expectError:     false,
		},
		{
			name:            "ValidOctalString",
			filePermissions: utils.Ptr("0755"),
			expectedMode:    utils.Ptr(os.FileMode(0755)),
			expectError:     false,
		},
		{
			name:            "InvalidString",
			filePermissions: utils.Ptr("invalid"),
			expectedMode:    nil,
			expectError:     true,
		},
		{
			name:            "EmptyString",
			filePermissions: utils.Ptr(""),
			expectedMode:    nil,
			expectError:     true,
		},
		{
			name:            "ValidDecimalString",
			filePermissions: utils.Ptr("493"), // 0755 in decimal
			expectedMode:    utils.Ptr(os.FileMode(0755)),
			expectError:     false,
		},
	}

	for _, tt := range tests {
		o.Run(tt.name, func() {
			opts := &Options{
				FilePermissions: tt.filePermissions,
			}
			mode, err := opts.GetFileMode()
			if tt.expectError {
				o.Require().Error(err)
			} else {
				o.Require().NoError(err)
				o.Equal(tt.expectedMode, mode)
			}
		})
	}
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(optionsSuite))
}

type keyFiles struct {
	SSHPrivateKey             string
	passphrase                string
	SSHPrivateKeyNoPassphrase string
}

func setupKeyFiles(tmpdir string) (*keyFiles, error) {
	kf := &keyFiles{}

	// setup ssh dir
	dir := path.Join(tmpdir, "ssh_keys")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		return nil, err
	}

	kf.passphrase = "fake secret"

	keyWithPassphrase, err := generatePrivateKey([]byte(kf.passphrase))
	if err != nil {
		return nil, err
	}

	kf.SSHPrivateKey = path.Join(dir, "gotest.key")
	err = writeFile(kf.SSHPrivateKey, keyWithPassphrase)
	if err != nil {
		return nil, err
	}

	keyNoPassphrase, err := generatePrivateKey(nil)
	if err != nil {
		return nil, err
	}
	kf.SSHPrivateKeyNoPassphrase = path.Join(dir, "gotest-nopassphrase.key")
	err = writeFile(kf.SSHPrivateKeyNoPassphrase, keyNoPassphrase)
	if err != nil {
		return nil, err
	}

	return kf, nil
}

func writeFile(p string, contents []byte) error {
	f, err := os.Create(p) //nolint:gosec
	if err != nil {
		return err
	}

	_, err = f.Write(contents)
	if err != nil {
		return err
	}

	return f.Close()
}

func generatePrivateKey(passphrase []byte) ([]byte, error) {
	// generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	// validate private key
	if err := privateKey.Validate(); err != nil {
		return nil, err
	}

	// setup pem block
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if len(passphrase) > 0 {
		pemBlock, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, "", passphrase)
		if err != nil {
			return nil, err
		}
	}

	// encode private key to PEM
	return pem.EncodeToMemory(pemBlock), nil
}
