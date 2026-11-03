package dropbox

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf16"
)

// apiArgHeader carries the JSON argument of upload and download requests.
const apiArgHeader = "Dropbox-API-Arg"

// headerSafeTransport authenticates requests and makes the Dropbox-API-Arg header ASCII, as Dropbox requires: every
// character above 0x7e is written as a \uXXXX JSON escape, with a surrogate pair above U+FFFF. The SDK puts the
// marshaled argument in the header unescaped, so an upload or download of a path with non-ASCII characters fails
// without this.
type headerSafeTransport struct {
	token string
	base  http.RoundTripper
}

// newHTTPClient returns the client every request goes through: it authenticates with token, and escapes the argument
// header. base carries the requests and defaults to http.DefaultTransport.
func newHTTPClient(token string, base http.RoundTripper) *http.Client {
	return &http.Client{Transport: headerSafeTransport{token: token, base: base}}
}

// RoundTrip implements http.RoundTripper. It does not modify req.
func (t headerSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.Header.Set("Authorization", "Bearer "+t.token)
	if arg := out.Header.Get(apiArgHeader); arg != "" {
		out.Header.Set(apiArgHeader, headerSafeJSON(arg))
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(out)
}

// headerSafeJSON escapes every character of a JSON document that is not printable ASCII, which JSON allows only
// inside strings.
func headerSafeJSON(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r < 0x7f:
			b.WriteRune(r)
		case r <= 0xFFFF:
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&b, "\\u%04x\\u%04x", hi, lo)
		}
	}
	return b.String()
}
