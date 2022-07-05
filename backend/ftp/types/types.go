package types

type OpenType int

const (
	OpenRead OpenType = iota
	OpenWrite
)

type DataConn interface {
	Mode() OpenType
	Close() error
}
