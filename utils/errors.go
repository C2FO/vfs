package utils

import "fmt"

// WrapReadError returns a wrapped read error
func WrapReadError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("read error: %w", err)
}

// WrapSeekError returns a wrapped seek error
func WrapSeekError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("seek error: %w", err)
}

// WrapWriteError returns a wrapped write error
func WrapWriteError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("write error: %w", err)
}

// WrapCloseError returns a wrapped close error
func WrapCloseError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("close error: %w", err)
}

// WrapTouchError returns a wrapped touch error
func WrapTouchError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("touch error: %w", err)
}

// WrapExistsError returns a wrapped exists error
func WrapExistsError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("exists error: %w", err)
}

// WrapSizeError returns a wrapped size error
func WrapSizeError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("size error: %w", err)
}

// WrapLastModifiedError returns a wrapped lastModified error
func WrapLastModifiedError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("lastModified error: %w", err)
}

// WrapDeleteError returns a wrapped delete error
func WrapDeleteError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("delete error: %w", err)
}

// WrapCopyToLocationError returns a wrapped copyToLocation error
func WrapCopyToLocationError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("copyToLocation error: %w", err)
}

// WrapCopyToFileError returns a wrapped copyToFile error
func WrapCopyToFileError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("copyToFile error: %w", err)
}

// WrapMoveToLocationError returns a wrapped moveToLocation error
func WrapMoveToLocationError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("moveToLocation error: %w", err)
}

// WrapMoveToFileError returns a wrapped moveToFile error
func WrapMoveToFileError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("moveToFile error: %w", err)
}

// WrapStatError returns a wrapped Stat error
func WrapStatError(err error) error {
	return fmt.Errorf("stat error: %w", err)
}
