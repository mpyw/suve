// Package internal provides shared utilities for version parsing.
package internal

// IsDigit returns true if c is an ASCII digit (0-9).
func IsDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// IsLetter returns true if c is an ASCII letter (a-z, A-Z).
func IsLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
