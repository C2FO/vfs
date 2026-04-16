package ftp

type dataConnError string

func (e dataConnError) Error() string { return string(e) }

const errDataconnSingleOpInvalid = dataConnError("dataconn must be open for single op mode to conduct a single op action")
const errDataconnReadInvalid = dataConnError("dataconn must be open for read mode to conduct a read")
const errDataconnWriteInvalid = dataConnError("dataconn must be open for write mode to conduct a write")
