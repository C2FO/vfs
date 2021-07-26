package vfs

// Error is a type that allows for error constants below
type Error string

func (e Error) Error() string { return string(e) }

// CopyToNotPossible - CopyTo/MoveTo operations are only possible when seek position is 0,0
const CopyToNotPossible = Error("current cursor offset is not 0 as required for this operation")
