package internal_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/version/internal"
)

func TestIsDigit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		c    byte
		want bool
	}{
		{'0', true},
		{'5', true},
		{'9', true},
		{'a', false},
		{'Z', false},
		{'-', false},
		{'/', false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, internal.IsDigit(tt.c), "IsDigit(%q)", tt.c)
	}
}

func TestIsLetter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		c    byte
		want bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'m', true},
		{'M', true},
		{'0', false},
		{'9', false},
		{'-', false},
		{'_', false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, internal.IsLetter(tt.c), "IsLetter(%q)", tt.c)
	}
}
