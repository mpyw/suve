// An export shim: it opens delete.go's private functions to the black-box tests.
//declscope:namespace delete

package delete

// ValidateDeleteFlags exposes the unexported validateDeleteFlags for testing.
//
//nolint:gochecknoglobals // test-only export hook
var ValidateDeleteFlags = validateDeleteFlags
