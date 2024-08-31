package utils

import "fmt"

// WrapReadError returns a wrapped read error
func WrapReadError(err error) error {
	return fmt.Errorf("read error: %w", err)
}

// WrapSeekError returns a wrapped seek error
func WrapSeekError(err error) error {
	return fmt.Errorf("seek error: %w", err)
}

// WrapWriteError returns a wrapped write error
func WrapWriteError(err error) error {
	return fmt.Errorf("write error: %w", err)
}

// WrapCloseError returns a wrapped close error
func WrapCloseError(err error) error {
	return fmt.Errorf("close error: %w", err)
}

// WrapExistsError returns a wrapped touch error
func WrapExistsError(err error) error {
	return fmt.Errorf("exists error: %w", err)
}

// WrapListError returns a wrapped list error
func WrapListError(err error) error {
	return fmt.Errorf("list error: %w", err)
}

// WrapListByPrefixError returns a wrapped list error
func WrapListByPrefixError(err error) error {
	return fmt.Errorf("list by prefix error: %w", err)
}

// WrapListError returns a wrapped list error
func WrapListByRegexError(err error) error {
	return fmt.Errorf("list by regex error: %w", err)
}
