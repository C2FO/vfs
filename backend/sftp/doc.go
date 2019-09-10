/*
Package sftp SFTP VFS implementation.

Usage

Rely on github.com/c2fo/vfs/v5/backend

  import(
	  "github.com/c2fo/vfs/v5/backend"
	  "github.com/c2fo/vfs/v5/backend/sftp"
  )

  func UseFs() error {
	  fs, err := backend.Backend(sftp.Scheme)
	  ...
  }

Or call directly:

  import "github.com/c2fo/vfs/v5/backend/sftp"

  func DoSomething() {
	  fs := sftp.NewFilesystem()

	  location, err := fs.NewLocation("myuser@server.com:22", "/some/path/")
	  if err != nil {
		 #handle error
	  }
	  ...
  }

sftp can be augmented with some implementation-specific methods.  Backend returns vfs.Filesystem interface so it
would have to be cast as sftp.Filesystem to use them.

These methods are chainable:
(*FileSystem) WithClient(client interface{}) *FileSystem
(*FileSystem) WithOptions(opts vfs.Options) *FileSystem


  func DoSomething() {

	  // cast if fs was created using backend.Backend().  Not necessary if created directly from sftp.NewFilesystem().
	  fs, err := backend.Backend(sftp.Scheme)
	  fs = fs.(*sftp.Filesystem)

	  // to pass specific client
	  sshClient, err := ssh.Dial("tcp", "myuser@server.com:22", &ssh.ClientConfig{
		  User:            "someuser",
		  Auth:            []ssh.AuthMethod{ssh.Password("mypassword")},
		  HostKeyCallback: ssh.InsecureIgnoreHostKey,
	  })
	  #handle error
	  client, err := _sftp.NewClient(sshClient)
	  #handle error

	  fs = fs.WithClient(client)

	  // to pass in client options. See Options for more info.  Note that changes to Options will make nil any client.
	  // This behavior ensures that changes to settings will get applied to a newly created client.
	  fs = fs.WithOptions(
		  sftp.Options{
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

Authentication

Authentication, by default, occurs automatically when Client() is called. Since user is part of the URI authority section
(Volume), auth is handled slightly differently than other vfs backends.

A client is initialized lazily, meaning we only make a connection to the server at the last moment so we are free to modify
options until then.  The authenticated session is closed any time WithOption(), WithClient(), or Close() occurs.  Currently,
that means that closing a file belonging to an fs will break the connection of any other open file on the same fs.

USERNAME

User may only be set in the URI authority section (Volume in vfs parlance).

	 scheme             host
	 __/             ___/____  port
	/  \            /        \ /\
	sftp://someuser@server.com:22/path/to/file.txt
		   \____________________/ \______________/
		   \______/       \               \
			  /     authority section    path
		 username       (Volume)

sftp vfs backend accepts either a password or an ssh key, with or without a passphrase.

PASSWORD/PASSPHRASE

Passwords may be passed via Options.Password or via the environmental variable VFS_SFTP_PASSWORD.

SSH keys may be passed via Options.KeyFilePath and (optionally) Options.KeyPassphrase.  They can also be passed via
environmental variables VFS_SFTP_KEYFILE and VFS_SFTP_KEYFILE_PASSPHRASE, respectively.

Note that as of Go 1.12, OPENSSH private key format is not supported when encrypted (with passphrase).
See https://github.com/golang/go/issues/18692
To force creation of PEM format(instead of OPENSSH format), use `ssh-keygen -m PEM`

KNOWN HOSTS

Known hosts ensures that the server you're connecting to hasn't been somehow redirected to another server, collecting
your info (man-in-the-middle attack).  Handling for this can be accomplished via:
1. Options.KnownHostsString which accepts a string.
2. Options.KnownHostsFile or environmental variable VFS_SFTP_KNOWN_HOSTS_FILE which accepts a path to a known_hosts file.
3. Options.KnownHostsCallback which allows you to specify any of the ssh.AuthMethod functions.  Environmental variable
   VFS_SFTP_INSECURE_KNOWN_HOSTS will set this callback function to ssh.InsecureIgnoreHostKey which may be helpful
   for testing but should not be used in production.

*/
package sftp
