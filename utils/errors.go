package utils

import "fmt"

// WrapReadError returns a wrapped read error
func WrapReadError(err error) error {
	return fmt.Errorf("read error: %s", err.Error())
}

// WrapSeekError returns a wrapped seek error
func WrapSeekError(err error) error {
	return fmt.Errorf("seek error: %s", err.Error())
}

// WrapWriteError returns a wrapped write error
func WrapWriteError(err error) error {
	return fmt.Errorf("write error: %s", err.Error())
}

// WrapCloseError returns a wrapped close error
func WrapCloseError(err error) error {
	return fmt.Errorf("close error: %s", err.Error())
}
