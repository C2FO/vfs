/*
Package ftp - FTP VFS implementation.

TODO: this needs a compete rewrite

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

		  location, err := fs.NewLocation("myuser@server.com:22", "/some/path/")
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

		  // to pass specific client
		  sshClient, err := ssh.Dial("tcp", "myuser@server.com:21", &ssh.ClientConfig{
			  User:            "someuser",
			  Auth:            []ssh.AuthMethod{ssh.Password("mypassword")},
			  HostKeyCallback: ssh.InsecureIgnoreHostKey,
		  })
		  #handle error
		  client, err := _ftp.NewClient(sshClient)
		  #handle error

		  fs = fs.WithClient(client)

		  // to pass in client options. See Options for more info.  Note that changes to Options will make nil any client.
		  // This behavior ensures that changes to settings will get applied to a newly created client.
		  fs = fs.WithOptions(
			  ftp.Options{
				  KeyFilePath:   "/home/Bob/.ssh/id_rsa",
				  KeyPassphrase: "s3cr3t",
				  KnownHostsCallback: ssh.InsecureIgnoreHostKey,
			  },
		  )

		  location, err := fs.NewLocation("myuser@server.com:22", "/some/path/")
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
(Volume), auth is handled slightly differently than other vfs backends.

A client is initialized lazily, meaning we only make a connection to the server at the last moment so we are free to modify
options until then.  The authenticated session is closed any time WithOption(), WithClient(), or Close() occurs.  Currently,
that means that closing a file belonging to an fs will break the connection of any other open file on the same fs.

# USERNAME

User may only be set in the URI authority section (Volume in vfs parlance).

	 scheme             host
	 __/             ___/____  port
	/  \            /        \ /\
	ftp://someuser@server.com:22/path/to/file.txt
	       \____________________/ \______________/
	       \______/       \               \
	           /     authority section    path
	     username       (Volume)

ftp vfs backend accepts either a password or an ssh key, with or without a passphrase.

# PASSWORD

Passwords may be passed via Options.Password or via the environmental variable VFS_FTP_PASSWORD.
*/
package ftp
