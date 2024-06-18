package utils

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

/*
   URI parlance (see https://www.rfc-editor.org/rfc/rfc3986.html#section-3.2):

       foo://example.com:8042/over/there?name=ferret#nose
       \_/   \______________/\_________/ \_________/ \__/
        |           |            |            |        |
     scheme     authority       path        query   fragment

   Where:
     authority   = [ userinfo "@" ] host [ ":" port ]
     userinfo    = *( unreserved / pct-encoded / sub-delims / ":" )
     host        = IP-literal / IPv4address / reg-name
     port        = *DIGIT
     reg-name    = *( unreserved / pct-encoded / sub-delims )
     unreserved  = ALPHA / DIGIT / "-" / "." / "_" / "~"
     sub-delims  = "!" / "$" / "&" / "'" / "(" / ")" / "*" / "+" / "," / ";" / "="
     pct-encoded = "%" HEXDIG HEXDIG
*/

// Authority represents host, port and userinfo (user/pass) in a URI
type Authority struct {
	host string
	port uint16
	url  *url.URL
}

// UserInfo represents user/pass portion of a URI
type UserInfo struct {
	url *url.URL
}

// Username returns the username of a URI UserInfo.  May be an empty string.
func (u UserInfo) Username() string {
	return u.url.User.Username()
}

// Password returns the password of a URI UserInfo.  May be an empty string.
func (u UserInfo) Password() string {
	p, _ := u.url.User.Password()
	return p
}

// String() returns a string representation of authority.  It does not include password per
// https://tools.ietf.org/html/rfc3986#section-3.2.1
//
//	Applications should not render as clear text any data after the first colon (":") character found within a userinfo
//	subcomponent unless the data after the colon is the empty string (indicating no password).
func (a Authority) String() string {
	authority := a.HostPortStr()
	if a.UserInfo().Username() != "" {
		authority = fmt.Sprintf("%s@%s", a.UserInfo().Username(), authority)
	}
	return authority
}

// UserInfo returns the userinfo section of authority.  userinfo is username and password(deprecated).
func (a Authority) UserInfo() UserInfo {
	return UserInfo{
		url: a.url,
	}
}

// Host returns the host portion of an authority
func (a Authority) Host() string {
	return a.url.Hostname()
}

// Port returns the port portion of an authority
func (a Authority) Port() uint16 {
	return a.port
}

// HostPortStr returns a concatenated string of host and port from authority, separated by a colon, ie "host.com:1234"
func (a Authority) HostPortStr() string {
	if a.Port() != 0 {
		return fmt.Sprintf("%s:%d", a.Host(), a.Port())
	}
	return a.Host()
}

var schemeRE = regexp.MustCompile("^[A-Za-z][A-Za-z0-9+.-]*://")

// NewAuthority initializes Authority struct by parsing authority string.
func NewAuthority(authority string) (Authority, error) {
	if authority == "" {
		return Authority{}, errors.New("authority string may not be empty")
	}

	var err error
	matched := schemeRE.MatchString(authority)
	if !matched {
		authority = "scheme://" + authority
	}

	u, err := url.Parse(authority)
	if err != nil {
		return Authority{}, err
	}

	host, portStr := splitHostPort(u.Host)
	var port uint16
	if portStr != "" {
		val, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return Authority{}, err
		}
		port = uint16(val)
	}

	return Authority{
		host: host,
		port: port,
		url:  u,
	}, nil
}

// splitHostPort separates host and port. If the port is not valid, it returns
// the entire input as host, and it doesn't check the validity of the host.
// Unlike net.SplitHostPort, but per RFC 3986, it requires ports to be numeric.
func splitHostPort(hostPort string) (host, port string) {
	host = hostPort

	colon := strings.LastIndexByte(host, ':')
	if colon != -1 && validOptionalPort(host[colon:]) {
		host, port = host[:colon], host[colon+1:]
	}

	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}

	return
}

// validOptionalPort reports whether port is either an empty string
// or matches /^:\d*$/
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

// EncodeUserInfo takes an unencoded URI authority userinfo string and encodes it
func EncodeUserInfo(rawUserInfo string) string {
	parts := strings.SplitN(rawUserInfo, ":", 2)
	encodedParts := make([]string, len(parts))
	for i, part := range parts {
		encoded := url.QueryEscape(part)
		decoded := strings.NewReplacer(
			"%21", "!", "%24", "$", "%26", "&", "%27", "'",
			"%28", "(", "%29", ")", "%2A", "*", "%2B", "+",
			"%2C", ",", "%3B", ";", "%3D", "=",
		).Replace(encoded)
		encodedParts[i] = decoded
	}
	return strings.Join(encodedParts, ":")
}

// EncodeAuthority takes an unencoded URI authority string and encodes it
func EncodeAuthority(rawAuthority string) string {
	var userInfo, hostPort string

	// Split the authority into user info and hostPort
	atIndex := strings.LastIndex(rawAuthority, "@")
	if atIndex != -1 {
		userInfo = rawAuthority[:atIndex]
		hostPort = rawAuthority[atIndex+1:]
	} else {
		hostPort = rawAuthority
	}

	// Encode userInfo if present
	if userInfo != "" {
		userInfo = EncodeUserInfo(userInfo)
	}

	// Split host and port
	var host, port string
	hostPortSplit := strings.SplitN(hostPort, ":", 2)
	if len(hostPortSplit) > 0 {
		host = hostPortSplit[0]
	}
	if len(hostPortSplit) > 1 {
		port = hostPortSplit[1]
	}

	// Encode host and port
	encodedHost := url.QueryEscape(host)
	var encodedPort string
	if port != "" {
		encodedPort = url.QueryEscape(port)
	}

	// Reconstruct the encoded authority string
	var encodedAuthority string
	if userInfo != "" {
		encodedAuthority = userInfo + "@"
	}
	encodedAuthority += encodedHost
	if encodedPort != "" {
		encodedAuthority += ":" + encodedPort
	}

	return encodedAuthority
}
