package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

/*
   URI parlance (see https://www.rfc-editor.org/rfc/rfc3986.html#section-3.2):

       foo://example.com:8042/over/there?name=ferret#nose
       \_/   \______________/\_________/ \_________/ \__/
        |           |            |            |        |
     scheme     authority       path        query   fragment

   Where:
     authority   = [ userinfo "@" ] host [ ":" port ]
     host        = IP-literal / IPv4address / reg-name
     port        = *DIGIT
*/

// ------------------------------------ host    :port
var portRegex = regexp.MustCompile(`(.*?)(?::(\d*))?$`)

// Authority represents host, port and userinfo (user/pass) in a URI
type Authority struct {
	userinfo UserInfo
	host     string
	port     uint16
}

// UserInfo represents user/pass portion of a URI
type UserInfo struct {
	username, password string
}

// Username returns the username of a URI UserInfo.  May be an empty string.
func (u UserInfo) Username() string {
	return u.username
}

// Password returns the password of a URI UserInfo.  May be an empty string.
func (u UserInfo) Password() string {
	return u.password
}

// String() returns a string representation of authority.  It does not include password per
// https://tools.ietf.org/html/rfc3986#section-3.2.1
//
//	Applications should not render as clear text any data after the first colon (":") character found within a userinfo
//	subcomponent unless the data after the colon is the empty string (indicating no password).
func (a Authority) String() string {
	authority := a.HostPortStr()
	if a.userinfo.Username() != "" {
		authority = fmt.Sprintf("%s@%s", a.userinfo.Username(), authority)
	}
	return authority
}

func (a Authority) UserInfo() UserInfo {
	return a.userinfo
}

func (a Authority) Host() string {
	return a.host
}

func (a Authority) Port() uint16 {
	return a.port
}

func (a Authority) HostPortStr() string {
	if a.Port() != 0 {
		return fmt.Sprintf("%s:%d", a.Host(), a.Port())
	}
	return a.Host()
}

// NewAuthority initializes Authority struct by parsing authority string.
func NewAuthority(authority string) (Authority, error) {
	if authority == "" {
		return Authority{}, errors.New("authority string may not be empty")
	}
	u, p, h, err := parseAuthority(authority)
	if err != nil {
		return Authority{}, err
	}

	matches := portRegex.FindAllStringSubmatch(h, -1)
	if len(matches) == 0 {
		return Authority{}, errors.New("could not determine port")
	}
	var port uint16
	if matches[0][2] != "" {
		val, err := strconv.ParseUint(matches[0][2], 10, 16)
		if err != nil {
			return Authority{}, err
		}
		port = uint16(val)
	}

	return Authority{
		userinfo: UserInfo{
			username: u,
			password: p,
		},
		host: matches[0][1],
		port: port,
	}, nil
}

/*
	NOTE: Below was mostly taken line-for-line from the "url" package (https://github.com/golang/go/blob/master/src/net/url/url.go),
	minus unencoding and some unused split logic.  Unfortunately none of it was exposed in a way that could be used for parsing authority.

		Copyright (c) 2009 The Go Authors. All rights reserved.

		Redistribution and use in source and binary forms, with or without
		modification, are permitted provided that the following conditions are
		met:

		   * Redistributions of source code must retain the above copyright
		notice, this list of conditions and the following disclaimer.
		   * Redistributions in binary form must reproduce the above
		copyright notice, this list of conditions and the following disclaimer
		in the documentation and/or other materials provided with the
		distribution.
		   * Neither the name of Google Inc. nor the names of its
		contributors may be used to endorse or promote products derived from
		this software without specific prior written permission.

		THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
		"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
		LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
		A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
		OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
		SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
		LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
		DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
		THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
		(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
		OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

func parseAuthority(authority string) (username, password, host string, err error) {
	i := strings.LastIndex(authority, "@")
	if i < 0 {
		host, err = parseHost(authority)
	} else {
		host, err = parseHost(authority[i+1:])
	}
	if err != nil {
		return "", "", "", err
	}
	if i < 0 {
		return "", "", host, nil
	}
	userinfo := authority[:i]
	if !validUserinfo(userinfo) {
		return "", "", host, errors.New("invalid userinfo")
	}
	if !strings.Contains(userinfo, ":") {
		username = userinfo
	} else {
		username, password = split(userinfo, ":")
	}

	return
}

func split(s, c string) (string, string) {
	i := strings.Index(s, c)
	return s[:i], s[i+len(c):]
}

func validUserinfo(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '-', '.', '_', ':', '~', '!', '$', '&', '\'',
			'(', ')', '*', '+', ',', ';', '=', '%', '@':
			continue
		default:
			return false
		}
	}
	return true
}

func parseHost(host string) (string, error) {
	if strings.HasPrefix(host, "[") {
		// Parse an IP-Literal in RFC 3986 and RFC 6874.
		// E.g., "[fe80::1]", "[fe80::1%25en0]", "[fe80::1]:80".
		i := strings.LastIndex(host, "]")
		if i < 0 {
			return "", errors.New("missing ']' in host")
		}
		colonPort := host[i+1:]
		if !validOptionalPort(colonPort) {
			return "", fmt.Errorf("invalid port %q after host", colonPort)
		}
	}

	return host, nil
}

func validOptionalPort(port string) bool {
	if port == "" {
		return true
	}
	if port[0] != ':' {
		return false
	}
	for _, b := range port[1:] {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}
