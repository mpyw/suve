package ssmversion

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/mpyw/suve/internal/version"
	"github.com/mpyw/suve/internal/version/internal"
	"github.com/mpyw/suve/internal/version/shift"
)

// Re-export common errors for backward compatibility.
var (
	ErrNoSpecifier    = version.ErrNoSpecifier
	ErrEmptySpec      = version.ErrEmptySpec
	ErrEmptyName      = version.ErrEmptyName
	ErrAmbiguousTilde = version.ErrAmbiguousTilde
)

// SSM-specific errors.
var (
	ErrEmptyVersion = errors.New("empty version number after #")
)

// AbsoluteSpec represents the absolute version specifier for SSM.
type AbsoluteSpec struct {
	Version *int64 // Explicit version number (#VERSION)
}

// Spec represents a parsed SSM parameter version specification.
//
// Grammar: <name>[#<N>]<shift>*
//   - #<N>     optional version number (0 or 1)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// Examples: /my/param, /my/param#3, /my/param~1, /my/param#5~2, /my/param~~
type Spec = version.Spec[AbsoluteSpec]

// parser defines the SSM-specific parsing logic.
var parser = version.AbsoluteParser[AbsoluteSpec]{
	IsSpecifierStart: func(s string, i int) (bool, error) {
		switch s[i] {
		case '#':
			// # followed by digit = version specifier
			if i+1 < len(s) && internal.IsDigit(s[i+1]) {
				return true, nil
			}
			// # at end of string is an error
			if i+1 >= len(s) {
				return false, ErrEmptyVersion
			}
		}
		return false, nil
	},
	ParseAbsolute: func(remaining string) (AbsoluteSpec, string, error) {
		var abs AbsoluteSpec

		// Parse #version if present
		if len(remaining) > 0 && remaining[0] == '#' {
			end := findNextSpecifier(remaining, 1)
			versionStr := remaining[1:end]
			v, err := strconv.ParseInt(versionStr, 10, 64)
			if err != nil {
				return AbsoluteSpec{}, "", fmt.Errorf("invalid version number: %s", versionStr)
			}
			abs.Version = &v
			remaining = remaining[end:]
		}

		return abs, remaining, nil
	},
	Zero: func() AbsoluteSpec {
		return AbsoluteSpec{}
	},
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
	return version.Parse(input, parser)
}

// findNextSpecifier finds the next specifier start after position start.
func findNextSpecifier(s string, start int) int {
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '#':
			if i+1 < len(s) && internal.IsDigit(s[i+1]) {
				return i
			}
		case '~':
			if shift.IsShiftStart(s, i) {
				return i
			}
		}
	}
	return len(s)
}
