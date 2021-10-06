package sftp

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v6/utils"
)

/**********************************
 ************TESTS*****************
 **********************************/

type optionsSuite struct {
	suite.Suite
	tmpdir   string
	keyFiles keyFiles
}

func (o *optionsSuite) SetupSuite() {
	dir, err := ioutil.TempDir("", "sftp_options_test")
	o.NoError(err, "setting up sftp_options_test temp dir")
	o.tmpdir = dir

	keyFiles, err := setupKeyFiles(o.tmpdir)
	if !o.NoError(err) {
		panic("couldn't setup key files")
	}
	o.keyFiles = *keyFiles
}

func (o *optionsSuite) TearDownSuite() {
	o.NoError(os.RemoveAll(o.tmpdir), "cleaning up after test")
}

type foundFileTest struct {
	file       string
	expected   bool
	hasError   bool
	errMessage string
	message    string
}

func (o *optionsSuite) TestFoundFile() {
	// test file
	filename := filepath.Join(o.tmpdir, "some.key")
	f, err := os.Create(filename)
	o.NoError(err, "create file for foundfile test")
	_, err = f.Write([]byte("blah"))
	o.NoError(err, "writing to file for foundfile test")
	o.NoError(f.Close(), "closing file for foundfile test")
	defer func() { o.NoError(os.Remove(filename), "clean up file for foundfile test") }()

	tests := []foundFileTest{
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
		actual, err := foundFile(t.file)
		if t.hasError {
			o.EqualError(err, t.errMessage, t.message)
		} else {
			o.NoError(err, t.message)
			o.Equal(t.expected, actual, t.message)
		}
	}
}

type getFileTest struct {
	keyfile    string
	passphrase string
	hasError   bool
	errMessage string
	message    string
}

func (o *optionsSuite) TestGetKeyFile() {

	tests := []getFileTest{
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
			errMessage: "open nonexistent.key: no such file or directory",
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
		_, err := getKeyFile(t.keyfile, t.passphrase)
		if t.hasError {
			o.EqualError(err, t.errMessage, t.message)
		} else {
			o.NoError(err, t.message)
		}
	}
}

type hostkeyTest struct {
	options    Options
	envVars    map[string]string
	hasError   bool
	errMessage string
	message    string
}

func (o *optionsSuite) TestGetHostKeyCallback() {

	knwonHosts := filepath.Join(o.tmpdir, "known_hosts")
	f, err := os.Create(knwonHosts)
	o.NoError(err, "create file for getHostKeyCallback test")
	_, err = f.Write([]byte("127.0.0.1 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBMkEmvHLSa43yoLA8QBqTfwgXgNCfd0DKs20NlBVbMoo21+Bs0fUpemyy6U0nnGHiOJVhiL7lNG/lB1fF1ymouM=")) //nolint:lll // long line
	o.NoError(err, "writing to file for getHostKeyCallback test")
	o.NoError(f.Close(), "closing file for getHostKeyCallback test")
	defer func() { o.NoError(os.Remove(knwonHosts), "clean up file for getHostKeyCallback test") }()

	tests := []hostkeyTest{
		{
			options: Options{
				KnownHostsCallback: ssh.InsecureIgnoreHostKey(),
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
				KnownHostsFile: knwonHosts,
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
				"VFS_SFTP_KNOWN_HOSTS_FILE": knwonHosts,
			},
			hasError:   false,
			errMessage: "",
			message:    "Env fallthrough KnownHostsFile",
		},
		{ // TODO:  this may be a bad test if a user/system-wide known_hosts file isn't found
			hasError:   false,
			errMessage: "",
			message:    "default fallthrough KnownHostsFile",
		},
	} // #nosec - InsecureIgnoreHostKey only used for testing

	for _, t := range tests { // nolint:gocritic // rangeValCopy
		// setup env vars, if any
		tmpMap := make(map[string]string)
		for k, v := range t.envVars {
			tmpMap[k] = os.Getenv(k)
			o.NoError(os.Setenv(k, v))
		}

		// apply test
		_, err := getHostKeyCallback(t.options)
		if t.hasError {
			o.EqualError(err, t.errMessage, t.message)
		} else {
			o.NoError(err, t.message)
		}

		// return env vars to original value
		for k, v := range tmpMap {
			o.NoError(os.Setenv(k, v))
		}
	}
}

type authTest struct {
	options     Options
	envVars     map[string]string
	returnCount int
	hasError    bool
	errMessage  string
	message     string
}

func (o *optionsSuite) TestGetAuthMethods() {

	tests := []authTest{
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
			message:     "explicit Options password",
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
			errMessage:  "open nonexistent.key: no such file or directory",
			message:     "env var keyfile returns error for file not found",
		},
	}

	for _, t := range tests { // nolint:gocritic // rangeValCopy
		// setup env vars, if any
		tmpMap := make(map[string]string)
		for k, v := range t.envVars {
			tmpMap[k] = os.Getenv(k)
			o.NoError(os.Setenv(k, v))
		}

		// apply test
		auth, err := getAuthMethods(t.options)
		if t.hasError {
			o.EqualError(err, t.errMessage, t.message)
		} else {
			o.NoError(err, t.message)
			o.Equal(t.returnCount, len(auth), "auth count")
		}

		// return env vars to original value
		for k, v := range tmpMap {
			o.NoError(os.Setenv(k, v))
		}
	}
}

type getClientTest struct {
	options   Options
	authority utils.Authority
	hasError  bool
	errRegex  string
	message   string
}

func (o *optionsSuite) TestGetClient() {

	tests := []getClientTest{
		{
			authority: utils.Authority{
				User: "someuse",
				Host: "badhost",
			},
			options: Options{
				Password:           "somepassword",
				KnownHostsCallback: ssh.InsecureIgnoreHostKey(),
			},
			hasError: true,
			errRegex: "(?:no such host|Temporary failure in name resolution)",
			message:  "getclient - bad host",
		},
		{
			authority: utils.Authority{
				User: "someuser",
				Host: "badhost",
			},
			options: Options{
				KeyFilePath:        "nonexistent.key",
				KnownHostsCallback: ssh.InsecureIgnoreHostKey(),
			},
			hasError: true,
			errRegex: "open nonexistent.key: no such file or directory",
			message:  "getclient - bad auth key",
		},
		{
			authority: utils.Authority{
				User: "someuser",
				Host: "badhost",
			},
			options: Options{
				Password:         "somepassword",
				KnownHostsString: "badstring",
			},
			hasError: true,
			errRegex: "ssh: no key found",
			message:  "getclient - bad known hosts",
		},
	} // #nosec - InsecureIgnoreHostKey only used for testing

	for _, t := range tests { // nolint:gocritic // rangeValCopy
		// apply test
		_, err := getClient(t.authority, t.options)
		if t.hasError {
			if o.Error(err, "error found") {
				re := regexp.MustCompile(t.errRegex)
				o.Regexp(re, err.Error(), "error matches")
			}
		} else {
			o.NoError(err, t.message)
		}
	}
}

func (o *optionsSuite) TestMarshalOptions() {
	// address bug #49 where json struct tag was misnamed
	pw := "secret1234"
	kh := "/path/to/known_hosts"

	opts := map[string]interface{}{
		"password":    pw,
		"keyFilePath": kh,
	}

	raw, err := json.Marshal(opts)
	o.Nil(err)
	optStruct := &Options{}
	err = json.Unmarshal(raw, optStruct)
	o.Nil(err)

	o.Equal(kh, optStruct.KeyFilePath, "KeyFilePath check")
	o.Equal(pw, optStruct.Password, "Password check")
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(optionsSuite))
}

type keyFiles struct {
	SSHPrivateKey             string
	SSHPubkey                 string
	passphrase                string
	SSHPrivateKeyNoPassphrase string
	SSHPubkeyNoPassphrase     string
}

func setupKeyFiles(tmpdir string) (*keyFiles, error) {
	kf := &keyFiles{}

	// setup ssh dir
	dir := path.Join(tmpdir, "ssh_keys")
	err := os.Mkdir(dir, 0700)
	if err != nil {
		return nil, err
	}

	kf.SSHPrivateKey = path.Join(dir, "gotest.key")
	err = writeFileString(kf.SSHPrivateKey, keyWithPassphgrase)
	if err != nil {
		return nil, err
	}

	kf.passphrase = SSHpassphrase

	kf.SSHPubkey = path.Join(dir, "gotest.key.pub")
	err = writeFileString(kf.SSHPubkey, keyWithPassphrasePubkey)
	if err != nil {
		return nil, err
	}

	kf.SSHPrivateKeyNoPassphrase = path.Join(dir, "gotest-nopassphrase.key")
	err = writeFileString(kf.SSHPrivateKeyNoPassphrase, keyWithoutPassphgrase)
	if err != nil {
		return nil, err
	}

	kf.SSHPubkeyNoPassphrase = path.Join(dir, "gotest-nopassphrase.key.pub")
	err = writeFileString(kf.SSHPubkeyNoPassphrase, keyWithoutPassphrasePubkey)
	if err != nil {
		return nil, err
	}

	return kf, nil
}

func writeFileString(p, contents string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}

	_, err = f.Write([]byte(contents))
	if err != nil {
		return err
	}

	return f.Close()
}

const SSHpassphrase = "secret"

const keyWithPassphgrase = `-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: AES-128-CBC,9A807E7480C8204C896DCA201D6F4F74

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
-----END RSA PRIVATE KEY-----` // #nosec - these are generated specifically for testing

const keyWithPassphrasePubkey = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDDNP1Ox5CSGHGt7kxNTcMUEGXypeWyNLcpJ6PTpbrLCWAG66gvmLzuOqCJgTR3oPP9XeO57MyRdjqRxsvQwLTK2wTj5cZoFtbtnNReqd9c90Ei2UFxqrHSfkimuWIzvELNh80kjVCgFLNhU1xen8BtN5FYMX+RVJGXCAuqf34ACUsj+PZPliGpSWUEtdO0UTB0dYPCG+zWVexZAyGTlvE7r0DQa9rYR/4OSdNbJvsvtSeaEwnhgJR4tLx3w6/rUONJxbJEmZ5ggMSxqSyTNhuPvSzrqSgntOqRltHDGgJv6oVflsN1grSzNPGXOa842wCZmqxii1IX8baX4/RYWNkyIY1fjPZFunLxtiwr9o02rN4rwSG1CIg6SChrD4LQFdtoRq2sXQvoEvqU+1DogllYKXQDeTi2E93MZ7boKWpAg4W/8rjRzCq49NPV2dHYDRn5hVbytCm+J/iAWg76rRUI98kFrSxx3oKF1HZrtYkSBVI3SqPFuf235C5d+gxR4HqrZbMqPVrsqSR8WecHYRqpJbF4QJDXYf/R8oSfkO8LqPuZiynlQBDczLCoLPkPEa0WSaTbPHYgF/rZwowR0uggSeIRqcdqNjG6yBHRoLZBrax5sIz2M3bOb7/uSZzMgrqe9HZ5+WrvgqRVHCj3NuHcqbjegZ0bfdQPMbPyZw882w==` // nolint:gosec,lll // these are generated specifically for testing

const keyWithoutPassphgrase = `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----` // #nosec - these are generated specifically for testing

const keyWithoutPassphrasePubkey = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDREGo3hNkWxVOjMml8lm8bjIpwqG+daIZ8/gO+T9RxTJrQ243WoA9qn9hl9zabTa6QMXFiIYY+RIut+MQO4+kLIgAPxGFedbKi/aZdLvTiKX5MJGrKvnz1V0I/b85+TpacCn0rdkYAa0b1zJy9DVwFF54KQ4r5KmO/o1Flz69aY9PV6Kf7Gnj7JardgQjxGx7n01PlQgAh7AWLImhR5RfJw7r1q2ecuVkg3qoe4aKn3KEXafP6YWYepfKSRKtJ23NIAFgzKVs/Ub/uG2/DilJeZQe9tlREB22us2X+5bM7jw/oy1tcIG9/L9SNMaVRVhP7gZPKWtqiXmj8mvjOP09LxYHu9PoYIC0MN6hCnxrGbD+Z+dPYpk4NiDThauSdKp6dOTH3ZRMhgshQIONLLTv2q+FACRKjBVSUNeqKyksFv0vcZuU0HBTjAbTRbzdx6A9Cle71E4nACw4qDA6nL9p3YYHq2oqCNMH4kq93y6IEO4kKIpQlM1xtTL9OMSkIhC2qT9QLU1vUeIDxWg0b0ikXrFCpk3wJqB/H4kIUOYWyTzmEAz4rLFR+IuvGfPE7fUEJiwcl/EMAbkr3FsEODcbf0+RAkA0r9WKAqmWKRHXozg1JwEbRMHOMFiCrKuTxm6n66vHoZykRJPUeVyyW0R9RXByzqqjZijE8i82X/JRq6Q==` // nolint:gosec,lll // these are generated specifically for testing
