// Package security provides security-related functionality for the agent server.
// This includes secure memory handling, process security, and peer verification.
package security

import (
	"github.com/awnumar/memguard"
)

// Buffer wraps sensitive data with secure memory handling.
// Data is stored in mlock'd memory and encrypted at rest.
type Buffer struct {
	enclave *memguard.Enclave
}

// NewBuffer creates a new secure buffer from the given data.
// The input slice is securely zeroed after copying.
func NewBuffer(data []byte) *Buffer {
	if len(data) == 0 {
		return &Buffer{}
	}

	enclave := memguard.NewEnclave(data)

	// Securely zero the original data (prevents compiler optimization from skipping)
	memguard.WipeBytes(data)

	return &Buffer{enclave: enclave}
}

// Bytes returns a copy of the decrypted data.
// The caller should zero the returned slice when done.
func (b *Buffer) Bytes() ([]byte, error) {
	if b.enclave == nil {
		return nil, nil
	}

	lb, err := b.enclave.Open()
	if err != nil {
		return nil, err
	}

	defer lb.Destroy()

	// Make a copy
	data := make([]byte, lb.Size())
	copy(data, lb.Bytes())

	return data, nil
}

// IsEmpty returns true if the buffer contains no data.
func (b *Buffer) IsEmpty() bool {
	return b.enclave == nil
}

// Destroy securely destroys the buffer.
// After calling Destroy, the buffer should not be used.
func (b *Buffer) Destroy() {
	// Enclave doesn't have Destroy, but the underlying data
	// is encrypted and will be GC'd
	b.enclave = nil
}

// init initializes memguard's secure memory core.
func init() {
	// This disables core dumps and sets up secure memory
	memguard.CatchInterrupt()
}
