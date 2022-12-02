package utils

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

/**********************************
 ************TESTS*****************
 **********************************/

type authoritySuite struct {
	suite.Suite
}

type authorityTest struct {
	authorityString                    string
	host, user, pass, str, hostPortStr string
	port                               uint16
	hasError                           bool
	errMessage                         string
	message                            string
}

func (a *authoritySuite) TestAuthority() {
	tests := []authorityTest{
		{
			authorityString: "",
			host:            "",
			port:            0,
			user:            "",
			pass:            "",
			str:             "",
			hostPortStr:     "",
			hasError:        true,
			errMessage:      "authority string may not be empty",
			message:         "empty input",
		},
		{
			authorityString: "some.host.com",
			host:            "some.host.com",
			port:            0,
			user:            "",
			pass:            "",
			str:             "some.host.com",
			hostPortStr:     "some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "host-only",
		},
		{
			authorityString: "some.host.com:22",
			host:            "some.host.com",
			port:            22,
			user:            "",
			pass:            "",
			str:             "some.host.com:22",
			hostPortStr:     "some.host.com:22",
			hasError:        false,
			errMessage:      "",
			message:         "host-only (with port)",
		},
		{
			authorityString: "some.host.com:",
			host:            "some.host.com",
			port:            0,
			user:            "",
			pass:            "",
			str:             "some.host.com",
			hostPortStr:     "some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "host-only (colon, no port)",
		},
		{
			authorityString: "me@some.host.com:22",
			host:            "some.host.com",
			port:            22,
			user:            "me",
			pass:            "",
			str:             "me@some.host.com:22",
			hostPortStr:     "some.host.com:22",
			hasError:        false,
			errMessage:      "",
			message:         "user and host",
		},
		{
			authorityString: "me:secret@some.host.com:22",
			host:            "some.host.com",
			port:            22,
			user:            "me",
			pass:            "secret",
			str:             "me@some.host.com:22",
			hostPortStr:     "some.host.com:22",
			hasError:        false,
			errMessage:      "",
			message:         "user, pass, and host (pass shouldn't be shown in String()",
		},
		{
			authorityString: "me:@some.host.com",
			host:            "some.host.com",
			port:            0,
			user:            "me",
			pass:            "",
			str:             "me@some.host.com",
			hostPortStr:     "some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "host and user, colon but no pass",
		},
		{
			authorityString: ":asdf@some.host.com",
			host:            "some.host.com",
			port:            0,
			user:            "",
			pass:            "asdf",
			str:             "some.host.com",
			hostPortStr:     "some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "host and pass, no user",
		},
		{
			authorityString: "Bob2@some.host.com",
			host:            "some.host.com",
			port:            0,
			user:            "Bob2",
			pass:            "",
			str:             "Bob2@some.host.com",
			hostPortStr:     "some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "user has upper and numeric",
		},
		{
			authorityString: "#blah@some.host.com",
			host:            "",
			port:            0,
			user:            "",
			pass:            "",
			str:             "",
			hostPortStr:     "",
			hasError:        true,
			errMessage:      "invalid userinfo",
			message:         "user has bad character",
		},
		{
			authorityString: "127.0.0.1",
			host:            "127.0.0.1",
			port:            0,
			user:            "",
			pass:            "",
			str:             "127.0.0.1",
			hostPortStr:     "127.0.0.1",
			hasError:        false,
			errMessage:      "",
			message:         "ipv4 host-only",
		},
		{
			authorityString: "127.0.0.1:22",
			host:            "127.0.0.1",
			port:            22,
			user:            "",
			pass:            "",
			str:             "127.0.0.1:22",
			hostPortStr:     "127.0.0.1:22",
			hasError:        false,
			errMessage:      "",
			message:         "ipv4 host with port",
		},
		{
			authorityString: "[0:0:0:0:0:0:0:1]",
			host:            "[0:0:0:0:0:0:0:1]",
			port:            0,
			user:            "",
			pass:            "",
			str:             "[0:0:0:0:0:0:0:1]",
			hostPortStr:     "[0:0:0:0:0:0:0:1]",
			hasError:        false,
			errMessage:      "",
			message:         "ipv6 host-only",
		},
		{
			authorityString: "[0:0:0:0:0:0:0:1",
			host:            "[0:0:0:0:0:0:0:1",
			port:            0,
			user:            "",
			pass:            "",
			str:             "[0:0:0:0:0:0:0:1",
			hostPortStr:     "[0:0:0:0:0:0:0:1",
			hasError:        true,
			errMessage:      "missing ']' in host",
			message:         "ipv6 host-only malformed (missing bracket)",
		},
		{
			authorityString: "[:::::::1]",
			host:            "[:::::::1]",
			port:            0,
			user:            "",
			pass:            "",
			str:             "[:::::::1]",
			hostPortStr:     "[:::::::1]",
			hasError:        false,
			errMessage:      "",
			message:         "ipv6 compress host-only",
		},
		{
			authorityString: "[:::::::1]:3022",
			host:            "[:::::::1]",
			port:            3022,
			user:            "",
			pass:            "",
			str:             "[:::::::1]:3022",
			hostPortStr:     "[:::::::1]:3022",
			hasError:        false,
			errMessage:      "",
			message:         "ipv6 compress host with port",
		},
		{
			authorityString: "[:::::::1]3022",
			host:            "[:::::::1]3022",
			port:            3022,
			user:            "",
			pass:            "",
			str:             "[:::::::1]3022",
			hostPortStr:     "[:::::::1]3022",
			hasError:        true,
			errMessage:      "invalid port \"3022\" after host",
			message:         "ipv6 compress host with port, missing colon",
		},
		{
			authorityString: "[:::::::1]:asdf",
			host:            "[:::::::1]",
			port:            0,
			user:            "",
			pass:            "",
			str:             "[:::::::1]:asdf",
			hostPortStr:     "[:::::::1]:asdf",
			hasError:        true,
			errMessage:      "invalid port \":asdf\" after host",
			message:         "host with invalid port (non-numeric)",
		},
	}

	for _, t := range tests {
		actual, err := NewAuthority(t.authorityString)
		if t.hasError {
			a.EqualError(err, t.errMessage, t.message)
		} else {
			a.NoError(err, t.message)
			a.Equal(t.host, actual.Host(), t.message)
			a.Equal(int(t.port), int(actual.Port()), t.message)
			a.Equal(t.user, actual.UserInfo().Username(), t.message)
			a.Equal(t.pass, actual.UserInfo().Password(), t.message)
			a.Equal(t.str, actual.String(), t.message)
		}
	}
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(authoritySuite))
}
