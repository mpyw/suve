package version

import (
	"fmt"
	"strconv"

	"github.com/mpyw/suve/internal/version/internal"
)

// parseShift parses shift specifiers from the beginning of a string.
// Supports Git-like syntax: ~, ~N, ~~, ~1~2, etc.
// Returns total shift amount and error.
// Returns error if ~ is not followed by digit, ~, or end of string.
func parseShift(s string) (int, error) {
	total := 0
	i := 0

	for i < len(s) {
		if s[i] != '~' {
			// Unexpected character (not a shift start)
			return 0, fmt.Errorf("unexpected characters: %s", s[i:])
		}
		i++
		// After ~, expect: end of string, digit, or another ~
		if i < len(s) && !internal.IsDigit(s[i]) && s[i] != '~' {
			return 0, fmt.Errorf("invalid shift: ~ followed by %q", s[i:])
		}
		// Check for number after ~
		numStart := i
		for i < len(s) && internal.IsDigit(s[i]) {
			i++
		}
		if numStart == i {
			// Bare ~ means ~1
			total++
		} else {
			n, err := strconv.Atoi(s[numStart:i])
			if err != nil {
				return 0, fmt.Errorf("shift number too large: %s", s[numStart:i])
			}
			total += n
		}
	}

	return total, nil
}

// isShiftStart returns true if position i in string s looks like the start of a shift.
func isShiftStart(s string, i int) bool {
	if i >= len(s) || s[i] != '~' {
		return false
	}
	// ~ followed by digit, ~, or end = shift
	return i+1 >= len(s) || internal.IsDigit(s[i+1]) || s[i+1] == '~'
}
