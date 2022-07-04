package ftp

import (
	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v6"

	"github.com/c2fo/vfs/v6/utils"
)

const systemWideKnownHosts = "/etc/ssh/ssh_known_hosts"

type Options struct {
	UserName   string `json:"username,omitempty"` // env var VFS_FTP_USERNAME
	Password   string `json:"password,omitempty"` // env var VFS_FTP_PASSWORD
	Retry      vfs.Retry
	MaxRetries int
	protocol   string `json:"username,omitempty"` // env var VFS_FTP_PROTOCOL
}

func getClient(authority utils.Authority, opts Options) (*_ftp.ServerConn, error) {

	return nil, nil
}

// ---part of uri
// host
// ports

// --- could be part of uri or not
// username

// password (env var, explicit)
// protocol (ftp, ftpes(port 21, can encrypt only auth, or commands, or data, or all), ftps (990, encryption from start, consumes more resources))
// debugging (io.writer)
// mode (passive, extended passive)
// anonymous?
// client Certificates (or just let tls.Config handle that)

// dataconn is automatically opened and closed when needed
// command conn is used when Dial is used
// Might open on each action... COULD cache conn with timeout. Enable timer when only when no file is open.  Reset timer
//     after every command.  Close command conn when timer ends.  Timer on a channel on fs.  Starts after Login()

/*
	go func() {
		count := 1
		for {
			count += 1
			fmt.Printf("   secondCount: %d\n", count)
			time.Sleep(time.Second)
		}
	}()

	time.Sleep(time.Second)

	timer := time.NewTimer(5 * time.Second)
	go func() {
		fmt.Println("conn timer started")
		<-timer.C
		fmt.Println("closing conn")
	}()

	time.Sleep(time.Second)

	if !timer.Stop() {
		fmt.Println("conn timer s stopped")
	}
	timer.Reset(5 * time.Second)
	fmt.Println("conn timer reset to 5 sec")

	time.Sleep(7 * time.Second)


*/
