package ftp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/ftp/types"
	"github.com/c2fo/vfs/v6/utils"
)

type Options struct {
	UserName    string // env var VFS_FTP_USERNAME
	Password    string // env var VFS_FTP_PASSWORD
	Protocol    string
	DisableEPSV *bool // env var VFS_DISABLE_EPSV
	DebugWriter io.Writer
	TLSConfig   *tls.Config
	DialTimeout time.Duration
	Retry       vfs.Retry
	MaxRetries  int
}

const (
	protocolFTP   = "FTP"
	protocolFTPS  = "FTPS"
	protocolFTPES = "FTPES"

	defaultUsername        = "anonymous"
	defaultPassword        = "anonymous"
	defaultPort     uint16 = 21

	envDisableEPSV = "VFS_FTP_DISABLE_EPSV"
	envProtocol    = "VFS_FTP_PROTOCOL"
	envUsername    = "VFS_FTP_USERNAME"
	envPassword    = "VFS_FTP_PASSWORD" //nolint:gosec
)

func getClient(ctx context.Context, authority utils.Authority, opts Options) (types.Client, error) {
	// dial connection
	c, err := _ftp.Dial(fetchHostPortString(authority), fetchDialOptions(ctx, authority, opts)...)
	if err != nil {
		return nil, err
	}

	// login
	err = c.Login(fetchUsername(authority, opts), fetchPassword(opts))
	if err != nil {
		return nil, err
	}

	return c, nil
}

func fetchUsername(auth utils.Authority, opts Options) string {
	// set default username
	username := defaultUsername

	// override with env var, if any
	if _, ok := os.LookupEnv(envUsername); ok {
		username = os.Getenv(envUsername)
	}

	// override with authority, if any
	// TODO: should this be allowed since the URI will be "bob@acme.com" but an overridden value of "jim" would connect
	//   to a complete different account??
	if auth.UserInfo().Username() != "" {
		username = auth.UserInfo().Username()
	}

	// override with options, if any
	if opts.UserName != "" {
		username = opts.UserName
	}

	return username
}

// note: since the format "user:pass" in the authority userinfo field is deprecated (per https://tools.ietf.org/html/rfc3986#section-3.2.1)
// it is not used by fetchPassword and should never be included in a vfs URI
func fetchPassword(opts Options) string {
	// set default password
	password := defaultPassword

	// override with env var, if any
	if _, ok := os.LookupEnv(envPassword); ok {
		password = os.Getenv(envPassword)
	}

	// override with options, if any
	if opts.Password != "" {
		password = opts.Password
	}

	return password
}

func fetchHostPortString(auth utils.Authority) string {
	// get host
	host := auth.Host()

	// get port
	port := defaultPort
	if auth.Port() > 0 {
		port = auth.Port()
	}

	// return <host>:<port> string
	return fmt.Sprintf("%s:%d", host, port)
}

func fetchDialOptions(ctx context.Context, auth utils.Authority, opts Options) []_ftp.DialOption {
	// set context DialOption
	dialOptions := []_ftp.DialOption{
		_ftp.DialWithContext(ctx),
	}

	// determine DisableEPSV DialOption
	dialOptions = append(dialOptions, _ftp.DialWithDisabledEPSV(isDisableOption(opts)))

	// determine protocol-specific (FTPS/FTPeS) TLS DialOption, if any (defaults to plain FTP, no TLS)
	switch protocol := fetchProtocol(opts); {
	case strings.EqualFold(protocol, protocolFTPS):
		dialOptions = append(dialOptions, _ftp.DialWithTLS(fetchTLSConfig(auth, opts)))
	case strings.EqualFold(protocol, protocolFTPES):
		dialOptions = append(dialOptions, _ftp.DialWithExplicitTLS(fetchTLSConfig(auth, opts)))
	}

	// determine debug writer DialOption, if any
	if opts.DebugWriter != nil {
		dialOptions = append(dialOptions, _ftp.DialWithDebugOutput(opts.DebugWriter))
	}

	// determine dial timeout DialOption
	if opts.DialTimeout.Seconds() > 0 {
		dialOptions = append(dialOptions, _ftp.DialWithTimeout(opts.DialTimeout))
	}

	return dialOptions
}

func isDisableOption(opts Options) bool {
	// default to false, meaning EPSV stays enabled
	disableEpsv := false

	// override with env var, if any
	if _, ok := os.LookupEnv(envDisableEPSV); ok {
		setting := os.Getenv(envDisableEPSV)
		if strings.EqualFold(setting, "true") || setting == "1" {
			disableEpsv = true
		}
	}

	// override with Options, if any
	if opts.DisableEPSV != nil {
		disableEpsv = *opts.DisableEPSV
	}

	return disableEpsv
}

func fetchTLSConfig(auth utils.Authority, opts Options) *tls.Config {
	// setup basic TLS config for host
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec
		ClientSessionCache: tls.NewLRUClientSessionCache(0),
		ServerName:         auth.Host(),
	}

	// override with Options, if any
	if opts.TLSConfig != nil {
		tlsConfig = opts.TLSConfig
	}

	return tlsConfig
}

func fetchProtocol(opts Options) string {
	// set default protocol
	protocol := protocolFTP

	// override with env var
	if _, ok := os.LookupEnv(envProtocol); ok {
		protocol = os.Getenv(envProtocol)
	}

	// override with options value
	if opts.Protocol != "" {
		protocol = opts.Protocol
	}

	return protocol
}
