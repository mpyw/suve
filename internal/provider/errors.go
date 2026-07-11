package provider

import "errors"

// Sentinel errors returned by provider implementations so that callers can
// classify failures without importing any cloud SDK. Adapters wrap the
// underlying provider error with these via fmt.Errorf("%w", ...).
var (
	// ErrNotFound indicates the requested entry does not exist.
	ErrNotFound = errors.New("provider: entry not found")
	// ErrAlreadyExists indicates a create was attempted on an entry that
	// already exists.
	ErrAlreadyExists = errors.New("provider: entry already exists")
	// ErrBinaryValue indicates the entry holds a binary value that suve cannot
	// represent as text (e.g. an AWS Secrets Manager SecretBinary secret).
	// Callers must not treat such an entry as an empty-string value.
	ErrBinaryValue = errors.New("binary value is not supported")
)
