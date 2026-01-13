package param

import "errors"

var (
	// ErrParameterNotFound is returned when a parameter is not found.
	ErrParameterNotFound = errors.New("parameter not found")
)
