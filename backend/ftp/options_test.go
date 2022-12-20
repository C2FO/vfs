package ftp

import (
	"bytes"
	"context"
	"crypto/tls"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/utils"
)

type optionsSuite struct {
	suite.Suite
}

func TestOptions(t *testing.T) {
	suite.Run(t, new(optionsSuite))
}

func (s *optionsSuite) TestFetchUsername() {
	tests := []struct {
		description string
		authority   string
		options     Options
		envVar      *string
		expected    string
	}{
		{
			description: "check defaults",
			authority:   "host.com",
			expected:    "anonymous",
		},
		{
			description: "authority value expected",
			authority:   "bob@host.com",
			expected:    "bob",
		},
		{
			description: "env var is set but with empty value",
			authority:   "bob@host.com",
			expected:    "bob",
			envVar:      ptrString(""),
		},
		{
			description: "env var is set, value should override",
			authority:   "host.com",
			expected:    "bill",
			envVar:      ptrString("bill"),
		},
		{
			description: "option should override",
			authority:   "bob@host.com",
			expected:    "sam",
			envVar:      ptrString("bill"),
			options: Options{
				UserName: "sam",
			},
		},
	}

	for i := range tests {
		auth, err := utils.NewAuthority(tests[i].authority)
		s.NoError(err, tests[i].description)

		if tests[i].envVar != nil {
			err := os.Setenv(envUsername, *tests[i].envVar)
			s.NoError(err, tests[i].description)
		}

		username := fetchUsername(auth, tests[i].options)
		s.Equal(tests[i].expected, username, tests[i].description)
	}
}

func (s *optionsSuite) TestFetchPassword() {
	tests := []struct {
		description string
		options     Options
		envVar      *string
		expected    string
	}{
		{
			description: "check defaults",
			expected:    "anonymous",
		},
		{
			description: "env var is set but with empty value",
			expected:    "",
			envVar:      ptrString(""),
		},
		{
			description: "env var is set, value should override",
			expected:    "12abc3",
			envVar:      ptrString("12abc3"),
		},
		{
			description: "option should override",
			expected:    "xyz123",
			envVar:      ptrString("12abc3"),
			options: Options{
				Password: "xyz123",
			},
		},
	}

	for i := range tests {
		if tests[i].envVar != nil {
			err := os.Setenv(envPassword, *tests[i].envVar)
			s.NoError(err, tests[i].description)
		}

		password := fetchPassword(tests[i].options)
		s.Equal(tests[i].expected, password, tests[i].description)
	}
}

func (s *optionsSuite) TestFetchHostPortString() {
	tests := []struct {
		description string
		authority   string
		envVar      *string
		expected    string
	}{
		{
			description: "check defaults",
			authority:   "user@host.com",
			expected:    "host.com:21",
		},
		{
			description: "authority has port specified",
			authority:   "user@host.com:10000",
			expected:    "host.com:10000",
		},
	}

	for i := range tests {
		auth, err := utils.NewAuthority(tests[i].authority)
		s.NoError(err, tests[i].description)

		if tests[i].envVar != nil {
			err := os.Setenv(envPassword, *tests[i].envVar)
			s.NoError(err, tests[i].description)
		}

		hostPortString := fetchHostPortString(auth)
		s.Equal(tests[i].expected, hostPortString, tests[i].description)
	}
}

func (s *optionsSuite) TestIsDisableEPSV() {
	var trueVal = true
	var falseVal = false
	tests := []struct {
		description string
		options     Options
		envVar      *string
		expected    bool
	}{
		{
			description: "check defaults",
			expected:    false,
		},
		{
			description: "env var is set but empty",
			envVar:      ptrString(""),
			expected:    false,
		},
		{
			description: "env var is set and is a non-true value",
			envVar:      ptrString("not expected"),
			expected:    false,
		},
		{
			description: "env var is set and is a `false` value",
			envVar:      ptrString("false"),
			expected:    false,
		},
		{
			description: "env var is set and is '1' value",
			envVar:      ptrString("1"),
			expected:    true,
		},
		{
			description: "env var is set and is 'true'",
			envVar:      ptrString("true"),
			expected:    true,
		},
		{
			description: "Options is set to false'",
			options: Options{
				DisableEPSV: &falseVal,
			},
			expected: false,
		},
		{
			description: "Options is set to true'",
			options: Options{
				DisableEPSV: &trueVal,
			},
			expected: true,
		},
		{
			description: "env var is set true but Options is set to false'",
			envVar:      ptrString("true"),
			options: Options{
				DisableEPSV: &falseVal,
			},
			expected: false,
		},
		{
			description: "env var is set true but Options is set to false'",
			envVar:      ptrString("false"),
			options: Options{
				DisableEPSV: &trueVal,
			},
			expected: true,
		},
	}

	for i := range tests {
		if tests[i].envVar != nil {
			err := os.Setenv(envDisableEPSV, *tests[i].envVar)
			s.NoError(err, tests[i].description)
		}

		disabled := isDisableOption(tests[i].options)
		s.Equal(tests[i].expected, disabled, tests[i].description)
	}
}

func (s *optionsSuite) TestFetchTLSConfig() {
	cfg := &tls.Config{
		MinVersion:             tls.VersionTLS12,
		InsecureSkipVerify:     false,
		ClientSessionCache:     tls.NewLRUClientSessionCache(0),
		ServerName:             "host.com",
		SessionTicketsDisabled: true,
	}

	tests := []struct {
		description string
		authority   string
		options     Options
		expected    *tls.Config
	}{
		{
			description: "check defaults",
			authority:   "user@host.com",
			expected: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, //nolint:gosec
				ClientSessionCache: tls.NewLRUClientSessionCache(0),
				ServerName:         "host.com",
			},
		},
		{
			description: "authority has port specified",
			authority:   "user@host.com:10000",
			options: Options{
				UserName:  "blah",
				Password:  "xyz",
				TLSConfig: cfg,
			},
			expected: cfg,
		},
	}

	for i := range tests {
		auth, err := utils.NewAuthority(tests[i].authority)
		s.NoError(err, tests[i].description)

		tlsCfg := fetchTLSConfig(auth, tests[i].options)
		s.Equal(tests[i].expected, tlsCfg, tests[i].description)
	}
}

func (s *optionsSuite) TestFetchProtocol() {
	tests := []struct {
		description string
		options     Options
		envVar      *string
		expected    string
	}{
		{
			description: "check defaults",
			expected:    protocolFTP,
		},
		{
			description: "env var is set but empty",
			envVar:      ptrString(""),
			expected:    "",
		},
		{
			description: "env var is set to ftps",
			envVar:      ptrString("FTPS"),
			expected:    protocolFTPS,
		},
		{
			description: "env var is set to ftpes",
			envVar:      ptrString("FTPES"),
			expected:    protocolFTPES,
		},
		{
			description: "env var is set to garbage",
			envVar:      ptrString("blah"),
			expected:    "blah",
		},
		{
			description: "options set to garbage",
			options: Options{
				Protocol: protocolFTPS,
			},
			expected: protocolFTPS,
		},
		{
			description: "options set to FTPES - overriding FTPS",
			envVar:      ptrString("FTPS"),
			options: Options{
				Protocol: protocolFTPES,
			},
			expected: protocolFTPES,
		},
	}

	for i := range tests {
		s.NoError(os.Unsetenv(envProtocol))
		if tests[i].envVar != nil {
			err := os.Setenv(envProtocol, *tests[i].envVar)
			s.NoError(err, tests[i].description)
		}

		protocol := fetchProtocol(tests[i].options)
		s.Equal(tests[i].expected, protocol, tests[i].description)
	}
}

func (s *optionsSuite) TestFetchDialOptions() {
	tests := []struct {
		description string
		authority   string
		options     Options
		envVar      *string
		expected    int
	}{
		{
			description: "check defaults",
			authority:   "user@host.com",
			expected:    2,
		},
		{
			description: "protocol env var is set to FTPS",
			authority:   "user@host.com",
			envVar:      ptrString(protocolFTPS),
			expected:    3,
		},
		{
			description: "protocol env var is set to FTPES",
			authority:   "user@host.com",
			envVar:      ptrString(protocolFTPES),
			expected:    3,
		},
		{
			description: "protocol is set to empty",
			authority:   "user@host.com",
			envVar:      ptrString(""),
			expected:    2,
		},
		{
			description: "protocol Options is set to FTPS",
			authority:   "user@host.com",
			options: Options{
				Protocol: protocolFTPS,
			},
			expected: 3,
		},
		{
			description: "protocol Options is set to garbage value",
			authority:   "user@host.com",
			options: Options{
				Protocol: "blah",
			},
			expected: 2,
		},
		{
			description: "debug writer is set",
			authority:   "user@host.com",
			options: Options{
				DebugWriter: bytes.NewBuffer([]byte{}),
			},
			expected: 3,
		},
		{
			description: "dial timeout is set",
			authority:   "user@host.com",
			options: Options{
				DialTimeout: 1 * time.Minute,
			},
			expected: 3,
		},
		{
			description: "all options set ",
			authority:   "user@host.com",
			options: Options{
				DebugWriter: bytes.NewBuffer([]byte{}),
				DialTimeout: 1 * time.Minute,
				Protocol:    protocolFTPS,
			},
			expected: 5,
		},
	}

	for i := range tests {
		if tests[i].envVar != nil {
			err := os.Setenv(envProtocol, *tests[i].envVar)
			s.NoError(err, tests[i].description)
		}

		auth, err := utils.NewAuthority(tests[i].authority)
		s.NoError(err, tests[i].description)

		dialOpts := fetchDialOptions(context.Background(), auth, tests[i].options)
		s.Len(dialOpts, tests[i].expected, tests[i].description)
	}
}

func ptrString(str string) *string {
	return &str
}
