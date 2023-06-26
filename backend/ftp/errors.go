package ftp

type dataConnErr string

func (e dataConnErr) Error() string { return string(e) }

const singleOpInvalidDataconnType = dataConnErr("dataconn must be open for single op mode to conduct a single op action")
const readInvalidDataconnType = dataConnErr("dataconn must be open for read mode to conduct a read")
const writeInvalidDataconnType = dataConnErr("dataconn must be open for write mode to conduct a write")
