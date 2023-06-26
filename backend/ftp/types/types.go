package types

import (
	"io"
	"time"

	_ftp "github.com/jlaffaye/ftp"
)

// OpenType represents the mode(read or write) that we open a file for.
type OpenType int

const (
	_ OpenType = iota
	// OpenRead denotes Read mode
	OpenRead
	// OpenWrite denotes Write mode
	OpenWrite
	SingleOp
)

// DataConn represents a data connection
type DataConn interface {
	Mode() OpenType
	Delete(path string) error
	GetEntry(p string) (*_ftp.Entry, error)
	List(p string) ([]*_ftp.Entry, error) // NLST for just names
	MakeDir(path string) error
	Rename(from, to string) error
	IsSetTimeSupported() bool
	SetTime(path string, t time.Time) error
	IsTimePreciseInList() bool
	io.ReadWriteCloser
}

// Client is an interface to make it easier to test
type Client interface {
	Delete(path string) error
	GetEntry(p string) (*_ftp.Entry, error)
	List(p string) ([]*_ftp.Entry, error) // NLST for just names
	Login(user string, password string) error
	MakeDir(path string) error
	Quit() error
	Rename(from, to string) error
	RetrFrom(path string, offset uint64) (*_ftp.Response, error)
	StorFrom(path string, r io.Reader, offset uint64) error
	IsSetTimeSupported() bool
	SetTime(path string, t time.Time) error
	IsTimePreciseInList() bool
}
