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
	authorityString       string
	host, user, pass, str string
	hasError              bool
	errMessage            string
	message               string
}

func (a *authoritySuite) TestEnsureTrailingSlash() {
	tests := []authorityTest{
		{
			authorityString: "",
			host:            "",
			user:            "",
			pass:            "",
			str:             "",
			hasError:        true,
			errMessage:      "authority string may not be empty",
			message:         "empty input",
		},
		{
			authorityString: "some.host.com",
			host:            "some.host.com",
			user:            "",
			pass:            "",
			str:             "some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "host-only",
		},
		{
			authorityString: "some.host.com:22",
			host:            "some.host.com:22",
			user:            "",
			pass:            "",
			str:             "some.host.com:22",
			hasError:        false,
			errMessage:      "",
			message:         "host-only (with port)",
		},
		{
			authorityString: "some.host.com:",
			host:            "some.host.com:",
			user:            "",
			pass:            "",
			str:             "some.host.com:",
			hasError:        false,
			errMessage:      "",
			message:         "host-only (colon, no port)",
		},
		{
			authorityString: "me@some.host.com:22",
			host:            "some.host.com:22",
			user:            "me",
			pass:            "",
			str:             "me@some.host.com:22",
			hasError:        false,
			errMessage:      "",
			message:         "user and host",
		},
		{
			authorityString: "me:secret@some.host.com:22",
			host:            "some.host.com:22",
			user:            "me",
			pass:            "secret",
			str:             "me@some.host.com:22",
			hasError:        false,
			errMessage:      "",
			message:         "user, pass, and host (pass shouldn't be shown in String()",
		},
		{
			authorityString: "me:@some.host.com",
			host:            "some.host.com",
			user:            "me",
			pass:            "",
			str:             "me@some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "host and user, colon but no pass",
		},
		{
			authorityString: ":asdf@some.host.com",
			host:            "some.host.com",
			user:            "",
			pass:            "asdf",
			str:             "some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "host and pass, no user",
		},
		{
			authorityString: "Bob2@some.host.com",
			host:            "some.host.com",
			user:            "Bob2",
			pass:            "",
			str:             "Bob2@some.host.com",
			hasError:        false,
			errMessage:      "",
			message:         "user has upper and numeric",
		},
		{
			authorityString: "#blah@some.host.com",
			host:            "",
			user:            "",
			pass:            "",
			str:             "",
			hasError:        true,
			errMessage:      "invalid userinfo",
			message:         "user has bad character",
		},
		{
			authorityString: "127.0.0.1",
			host:            "127.0.0.1",
			user:            "",
			pass:            "",
			str:             "127.0.0.1",
			hasError:        false,
			errMessage:      "",
			message:         "ipv4 host-only",
		},
		{
			authorityString: "127.0.0.1:22",
			host:            "127.0.0.1:22",
			user:            "",
			pass:            "",
			str:             "127.0.0.1:22",
			hasError:        false,
			errMessage:      "",
			message:         "ipv4 host with port",
		},
		{
			authorityString: "[0:0:0:0:0:0:0:1]",
			host:            "[0:0:0:0:0:0:0:1]",
			user:            "",
			pass:            "",
			str:             "[0:0:0:0:0:0:0:1]",
			hasError:        false,
			errMessage:      "",
			message:         "ipv6 host-only",
		},
		{
			authorityString: "[0:0:0:0:0:0:0:1",
			host:            "[0:0:0:0:0:0:0:1",
			user:            "",
			pass:            "",
			str:             "[0:0:0:0:0:0:0:1",
			hasError:        true,
			errMessage:      "missing ']' in host",
			message:         "ipv6 host-only malformed (missing bracket)",
		},
		{
			authorityString: "[:::::::1]",
			host:            "[:::::::1]",
			user:            "",
			pass:            "",
			str:             "[:::::::1]",
			hasError:        false,
			errMessage:      "",
			message:         "ipv6 compress host-only",
		},
		{
			authorityString: "[:::::::1]:3022",
			host:            "[:::::::1]:3022",
			user:            "",
			pass:            "",
			str:             "[:::::::1]:3022",
			hasError:        false,
			errMessage:      "",
			message:         "ipv6 compress host with port",
		},
		{
			authorityString: "[:::::::1]3022",
			host:            "[:::::::1]3022",
			user:            "",
			pass:            "",
			str:             "[:::::::1]3022",
			hasError:        true,
			errMessage:      "invalid port \"3022\" after host",
			message:         "ipv6 compress host with port, missing colon",
		},
		{
			authorityString: "[:::::::1]:asdf",
			host:            "[:::::::1]:asdf",
			user:            "",
			pass:            "",
			str:             "[:::::::1]:asdf",
			hasError:        true,
			errMessage:      "invalid port \":asdf\" after host",
			message:         "host with invalid port (non-numeric ",
		},
	}

	for _, t := range tests {
		actual, err := NewAuthority(t.authorityString)
		if t.hasError {
			a.EqualError(err, t.errMessage, t.message)
		} else {
			a.NoError(err, t.message)
			a.Equal(t.host, actual.Host, t.message)
			a.Equal(t.user, actual.User, t.message)
			a.Equal(t.pass, actual.Pass, t.message)
			a.Equal(t.str, actual.String(), t.message)
		}
	}
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(authoritySuite))
}
