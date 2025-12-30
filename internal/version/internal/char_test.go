package internal

import "testing"

func TestIsDigit(t *testing.T) {
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
		if got := IsDigit(tt.c); got != tt.want {
			t.Errorf("IsDigit(%q) = %v, want %v", tt.c, got, tt.want)
		}
	}
}

func TestIsLetter(t *testing.T) {
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
		if got := IsLetter(tt.c); got != tt.want {
			t.Errorf("IsLetter(%q) = %v, want %v", tt.c, got, tt.want)
		}
	}
}
