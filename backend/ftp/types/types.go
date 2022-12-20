package types

import (
	"io"

	_ftp "github.com/jlaffaye/ftp"
)

type OpenType int

const (
	_ OpenType = iota
	OpenRead
	OpenWrite
)

type DataConn interface {
	Mode() OpenType
	io.ReadWriteCloser
}

// Client is an interface to make it easier to test
type Client interface {
	Delete(path string) error
	List(p string) ([]*_ftp.Entry, error) // NLST for just names
	Login(user string, password string) error
	MakeDir(path string) error
	Quit() error
	Rename(from, to string) error
	RetrFrom(path string, offset uint64) (*_ftp.Response, error)
	StorFrom(path string, r io.Reader, offset uint64) error
}
