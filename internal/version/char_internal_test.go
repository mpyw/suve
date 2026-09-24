package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDigitChar(t *testing.T) {
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
		assert.Equal(t, tt.want, isDigitChar(tt.c), "isDigitChar(%q)", tt.c)
	}
}

func TestIsLetterChar(t *testing.T) {
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
		assert.Equal(t, tt.want, isLetterChar(tt.c), "isLetterChar(%q)", tt.c)
	}
}
