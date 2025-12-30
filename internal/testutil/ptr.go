// Package testutil provides test helper functions.
package testutil

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T {
	return &v
}

// PtrEqual checks if two pointers point to equal values.
func PtrEqual[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
