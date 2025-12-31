// Package version provides shared version specification parsing logic.
package version

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mpyw/suve/internal/version/internal"
	"github.com/mpyw/suve/internal/version/shift"
)

// Common errors for version parsing.
var (
	ErrEmptySpec      = errors.New("empty specification")
	ErrEmptyName      = errors.New("empty name")
	ErrNoSpecifier    = errors.New("no specifier found")
	ErrAmbiguousTilde = errors.New("ambiguous tilde")
)

// Spec represents a parsed version specification.
type Spec[A any] struct {
	Name     string // Parameter/Secret name
	Absolute A      // Absolute version specifier (type-specific)
	Shift    int    // Relative shift (~N)
}

// HasShift returns true if a shift is specified.
func (s *Spec[A]) HasShift() bool {
	return s.Shift > 0
}

// AbsoluteParser handles type-specific parsing of absolute version specifiers.
type AbsoluteParser[A any] struct {
	// IsSpecifierStart checks if position i is the start of an absolute specifier.
	// Returns (true, nil) if it's a valid specifier start.
	// Returns (false, nil) if this character is not a specifier.
	// Returns (false, error) for invalid syntax (e.g., # at end of string).
	IsSpecifierStart func(s string, i int) (bool, error)

	// ParseAbsolute parses the absolute specifier from the remaining string.
	// remaining starts with the specifier character (# or :).
	// Returns the parsed value, the remaining string after parsing, and any error.
	ParseAbsolute func(remaining string) (A, string, error)

	// Zero returns the zero value of A (for when no absolute specifier is present).
	Zero func() A
}

// Parse parses a version specification string using the provided parser.
func Parse[A any](input string, parser AbsoluteParser[A]) (*Spec[A], error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, ErrEmptySpec
	}

	spec := &Spec[A]{
		Absolute: parser.Zero(),
	}

	// Find where specifiers start
	specStart, err := findSpecifierStart(input, parser.IsSpecifierStart)
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

	// Parse absolute specifier if present (not starting with ~)
	if remaining != "" && remaining[0] != '~' {
		var abs A
		abs, remaining, err = parser.ParseAbsolute(remaining)
		if err != nil {
			return nil, err
		}
		spec.Absolute = abs
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

// findSpecifierStart finds the start position of a specifier in the string.
func findSpecifierStart(s string, isAbsoluteStart func(s string, i int) (bool, error)) (int, error) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '~':
			// ~ is common to both SSM and SM
			if shift.IsShiftStart(s, i) {
				return i, nil
			}
			// Tilde followed by letter is ambiguous - error
			if i+1 < len(s) && internal.IsLetter(s[i+1]) {
				return 0, fmt.Errorf("%w: use ~N for version shift or avoid ~ followed by letters", ErrAmbiguousTilde)
			}
		default:
			// Check type-specific absolute specifiers
			isStart, err := isAbsoluteStart(s, i)
			if err != nil {
				return 0, err
			}
			if isStart {
				return i, nil
			}
		}
	}
	return 0, ErrNoSpecifier
}
