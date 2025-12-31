package shift

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantShift int
		wantErr   bool
	}{
		// Basic cases
		{name: "empty string", input: "", wantShift: 0},
		{name: "bare tilde", input: "~", wantShift: 1},
		{name: "tilde with 1", input: "~1", wantShift: 1},
		{name: "tilde with 2", input: "~2", wantShift: 2},
		{name: "tilde with 0", input: "~0", wantShift: 0},
		{name: "tilde with 10", input: "~10", wantShift: 10},
		{name: "tilde with 100", input: "~100", wantShift: 100},

		// Multiple tildes
		{name: "double tilde", input: "~~", wantShift: 2},
		{name: "triple tilde", input: "~~~", wantShift: 3},
		{name: "quadruple tilde", input: "~~~~", wantShift: 4},

		// Cumulative
		{name: "cumulative ~1~2", input: "~1~2", wantShift: 3},
		{name: "cumulative ~2~3", input: "~2~3", wantShift: 5},
		{name: "cumulative ~~1", input: "~~1", wantShift: 2},
		{name: "cumulative ~1~~", input: "~1~~", wantShift: 3},
		{name: "cumulative ~0~0", input: "~0~0", wantShift: 0},

		// Error cases: not starting with ~
		{name: "no tilde at start", input: "abc", wantErr: true},
		{name: "number only", input: "123", wantErr: true},
		{name: "at sign", input: "@3", wantErr: true},
		{name: "colon", input: ":LABEL", wantErr: true},

		// Error cases: ~ followed by invalid char
		{name: "tilde followed by letter", input: "~a", wantErr: true},
		{name: "tilde followed by word", input: "~abc", wantErr: true},
		{name: "tilde with number then letter", input: "~2abc", wantErr: true},
		{name: "tilde then special char", input: "~!", wantErr: true},
		{name: "tilde then space", input: "~ ", wantErr: true},
		{name: "tilde then slash", input: "~/path", wantErr: true},

		// Error cases: overflow
		{name: "huge number overflow", input: "~99999999999999999999", wantErr: true},
		{name: "cumulative overflow", input: "~99999999999999999999~1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotShift, err := Parse(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantShift, gotShift)
		})
	}
}

func TestIsShiftStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		s    string
		i    int
		want bool
	}{
		// Out of bounds
		{name: "index equals length", s: "abc", i: 3, want: false},
		{name: "index exceeds length", s: "abc", i: 5, want: false},
		{name: "empty string index 0", s: "", i: 0, want: false},

		// Not a tilde
		{name: "not tilde - letter", s: "abc", i: 0, want: false},
		{name: "not tilde - at sign", s: "@3", i: 0, want: false},
		{name: "not tilde - colon", s: ":LABEL", i: 0, want: false},
		{name: "not tilde - digit", s: "123", i: 0, want: false},

		// Tilde at end of string
		{name: "tilde at end", s: "abc~", i: 3, want: true},
		{name: "tilde only", s: "~", i: 0, want: true},

		// Tilde followed by digit
		{name: "tilde followed by digit", s: "~1", i: 0, want: true},
		{name: "tilde followed by zero", s: "~0", i: 0, want: true},
		{name: "tilde followed by 9", s: "~9", i: 0, want: true},

		// Tilde followed by tilde
		{name: "tilde followed by tilde", s: "~~", i: 0, want: true},
		{name: "second tilde in ~~", s: "~~", i: 1, want: true},

		// Tilde followed by other characters (NOT a shift start)
		{name: "tilde followed by letter", s: "~a", i: 0, want: false},
		{name: "tilde followed by uppercase", s: "~A", i: 0, want: false},
		{name: "tilde followed by slash", s: "~/", i: 0, want: false},
		{name: "tilde followed by dash", s: "~-", i: 0, want: false},
		{name: "tilde followed by underscore", s: "~_", i: 0, want: false},

		// Middle of string
		{name: "tilde in middle followed by digit", s: "abc~1def", i: 3, want: true},
		{name: "tilde in middle followed by letter", s: "abc~def", i: 3, want: false},
		{name: "tilde in middle at end", s: "abc~", i: 3, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsShiftStart(tt.s, tt.i)
			assert.Equal(t, tt.want, got)
		})
	}
}
