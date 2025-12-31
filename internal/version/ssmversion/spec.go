package ssmversion

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mpyw/suve/internal/version/internal"
	"github.com/mpyw/suve/internal/version/shift"
)

// Sentinel errors for version specification parsing.
var (
	ErrNoSpecifier    = errors.New("no specifier found")
	ErrEmptySpec      = errors.New("empty version specification")
	ErrEmptyName      = errors.New("empty name in version specification")
	ErrAmbiguousTilde = errors.New("ambiguous tilde in name")
)

// Spec represents a parsed SSM parameter version specification.
//
// Grammar: <name>[#<N>]<shift>*
//   - #<N>     optional version number (0 or 1)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// Examples: /my/param, /my/param#3, /my/param~1, /my/param#5~2, /my/param~~
type Spec struct {
	Name    string // Parameter name
	Version *int64 // Explicit version number (#VERSION)
	Shift   int    // Number of versions to go back (~SHIFT)
}

// Parse parses an SSM version specification string.
//
// Grammar: <name>[#<N>]<shift>*
//
// Shift syntax (Git-like, repeatable):
//   - ~      go back 1 version
//   - ~N     go back N versions (e.g., ~2)
//   - ~~     go back 2 versions (same as ~1~1)
//   - ~1~2   cumulative: go back 3 versions
func Parse(input string) (*Spec, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, ErrEmptySpec
	}

	spec := &Spec{}

	// Find where specifiers start
	specStart, err := findSpecifierStart(input)
	if err != nil {
		if errors.Is(err, ErrNoSpecifier) {
			spec.Name = input
			return spec, nil
		}
		return nil, err
	}

	spec.Name = input[:specStart]
	if spec.Name == "" {
		return nil, ErrEmptyName
	}

	remaining := input[specStart:]

	// Parse #version if present
	if strings.HasPrefix(remaining, "#") {
		end := findShiftStart(remaining, 1)
		versionStr := remaining[1:end]
		// versionStr is guaranteed non-empty because findSpecifierStart only
		// returns a position for # when followed by a digit
		v, err := strconv.ParseInt(versionStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid version number: %s", versionStr)
		}
		spec.Version = &v
		remaining = remaining[end:]
	}

	// Parse shifts
	if remaining != "" {
		s, err := shift.Parse(remaining)
		if err != nil {
			return nil, err
		}
		spec.Shift = s
	}

	return spec, nil
}

// findSpecifierStart finds the index where specifiers begin.
// Returns the position of the first specifier.
// Returns ErrNoSpecifier if no specifier found.
// Returns other errors for invalid patterns.
func findSpecifierStart(s string) (int, error) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '#':
			if i+1 < len(s) && internal.IsDigit(s[i+1]) {
				return i, nil
			}
		case '~':
			if shift.IsShiftStart(s, i) {
				return i, nil
			}
			// Tilde followed by letter is ambiguous - error
			if i+1 < len(s) && internal.IsLetter(s[i+1]) {
				return 0, fmt.Errorf("%w: use ~SHIFT (e.g., ~1) for version shift or avoid ~ followed by letters", ErrAmbiguousTilde)
			}
		}
	}
	return 0, ErrNoSpecifier
}

// findShiftStart finds the next shift start after position start.
func findShiftStart(s string, start int) int {
	for i := start; i < len(s); i++ {
		if shift.IsShiftStart(s, i) {
			return i
		}
	}
	return len(s)
}

// HasShift returns true if a shift is specified.
func (s *Spec) HasShift() bool {
	return s.Shift > 0
}
