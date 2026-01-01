package smutil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/smutil"
)

func TestTruncateVersionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "long UUID gets truncated",
			input:    "12345678-1234-1234-1234-123456789012",
			expected: "12345678",
		},
		{
			name:     "exactly 8 chars unchanged",
			input:    "12345678",
			expected: "12345678",
		},
		{
			name:     "short ID unchanged",
			input:    "v1",
			expected: "v1",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
		{
			name:     "9 chars truncated to 8",
			input:    "123456789",
			expected: "12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := smutil.TruncateVersionID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
