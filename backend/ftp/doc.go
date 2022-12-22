/*
Package ftp - FTP VFS implementation.

# Usage

Rely on github.com/c2fo/vfs/v6/backend

	  import(
		  "github.com/c2fo/vfs/v6/backend"
		  "github.com/c2fo/vfs/v6/backend/ftp"
	  )

	  func UseFs() error {
		  fs := backend.Backend(ftp.Scheme)
		  ...
	  }

Or call directly:

	  import "github.com/c2fo/vfs/v6/backend/ftp"

	  func DoSomething() {
		  fs := ftp.NewFilesystem()

		  location, err := fs.NewLocation("myuser@server.com:21", "/some/path/")
		  if err != nil {
			 #handle error
		  }
		  ...
	  }

ftp can be augmented with some implementation-specific methods.  Backend returns vfs.Filesystem interface so it
would have to be cast as ftp.Filesystem to use them.

These methods are chainable:
(*FileSystem) WithClient(client interface{}) *FileSystem
(*FileSystem) WithOptions(opts vfs.Options) *FileSystem

	  func DoSomething() {

		  // cast if fs was created using backend.Backend().  Not necessary if created directly from ftp.NewFilesystem().
		  fs := backend.Backend(ftp.Scheme)
		  fs = fs.(*ftp.Filesystem)

		  // to pass specific client implementing types.Client interface (in this case, _ftp github.com/jlaffaye/ftp)
		  client, _ := _ftp.Dial("server.com:21")
		  fs = fs.WithClient(client)

		  // to pass in client options. See Options for more info.  Note that changes to Options will make nil any client.
		  // This behavior ensures that changes to settings will get applied to a newly created client.
		  fs = fs.WithOptions(
			  ftp.Options{
				  Password: "s3cr3t",
				  DisableEPSV: true,
				  Protocol: ftp.ProtocolFTPES,
				  DialTimeout: 15 * time.Second
				  DebugWriter: os.Stdout
			  },
		  )

		  location, err := fs.NewLocation("myuser@server.com:21", "/some/path/")
		  #handle error

		  file := location.NewFile("myfile.txt")
		  #handle error

		  _, err := file.Write([]bytes("some text")
		  #handle error

		  err := file.Close()
		  #handle error

	  }

# Authentication

Authentication, by default, occurs automatically when Client() is called. Since user is part of the URI authority section
(Volume), auth is handled slightly differently than other vfs backends (except SFTP).

A client is initialized lazily, meaning we only make a connection to the server at the last moment, so we are free to modify
options until then.  The authenticated session is closed any time WithOption() or WithClient() occurs.

## USERNAME

User may only be set in the URI authority section (Volume in vfs parlance).

	 scheme             host
	 __/             ___/____  port
	/  \            /        \ /\
	ftp://someuser@server.com:22/path/to/file.txt
	       \____________________/ \______________/
	       \______/       \               \
	           /     authority section    path
	     username       (Volume)

ftp vfs backend defaults to "anonymous" if no username is provided in the authority, ie "ftp://service.com/".

## PASSWORD

Passwords may be passed via Options.Password or via the environmental variable *VFS_FTP_PASSWORD*.  If not password is provided,
default is "anonymous".  Password precedence is default, env var, Options.Password, such that env var, if set, overrides default
and Options.Password, if set, overrides env var.

# Protocol

The ftp backend supports the following FTP protocols: FTP (unencrypted), FTPS (implicit TLS), and FTPES (explicit TLS).  Protocol can be set
by env var *VFS_FTP_PROTOCOL* or in Options.Protocol.  Options values take precedence over env vars.

By default, FTPS and FTPS will use the following TLS configuration but can be overridden(recommended) with Options.TLSConfig:

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		ClientSessionCache: tls.NewLRUClientSessionCache(0),
		ServerName:         hostname,
	}

See https://pkg.go.dev/crypto/tls#Config for all TLS configuration options.

# Other Options

DebugWriter *io.Writer* - captures FTP command details to any writer.

DialTimeout *time.Duration - sets timeout for connecting only.

DisableEPSV bool - Extended Passive mode (EPSV) is attempted by default. Set to true to use regular Passive mode (PASV).
*/
package ftp
