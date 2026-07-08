package version

import (
	"fmt"
	"math"
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
			// Bare ~ means ~1.
			if total == math.MaxInt {
				return 0, errShiftOutOfRange
			}

			total++
		} else {
			n, err := strconv.Atoi(s[numStart:i])
			if err != nil {
				return 0, fmt.Errorf("shift number too large: %s", s[numStart:i])
			}

			// Overflow-checked cumulative sum: n is non-negative (digits only),
			// so an unchecked `total += n` could wrap to a negative total, which
			// HasShift() then reads as "no shift" and silently resolves to latest
			// (e.g. `~MAX~MAX`). Reject instead.
			if n > math.MaxInt-total {
				return 0, errShiftOutOfRange
			}

			total += n
		}
	}

	return total, nil
}

// errShiftOutOfRange is returned when a ~N shift (or a cumulative ~N~M sum)
// exceeds what an int can hold.
var errShiftOutOfRange = fmt.Errorf("shift out of range")

// isShiftStart returns true if position i in string s looks like the start of a shift.
func isShiftStart(s string, i int) bool {
	if i >= len(s) || s[i] != '~' {
		return false
	}
	// ~ followed by digit, ~, or end = shift
	return i+1 >= len(s) || internal.IsDigit(s[i+1]) || s[i+1] == '~'
}
