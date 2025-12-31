package smversion

import (
	"errors"
	"fmt"

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

// SM-specific errors.
var (
	ErrEmptyID        = errors.New("empty version ID after #")
	ErrEmptyLabel     = errors.New("empty label after colon")
	ErrBothIDAndLabel = errors.New("cannot specify both #VERSION and :LABEL")
)

// AbsoluteSpec represents the absolute version specifier for SM.
type AbsoluteSpec struct {
	ID    *string // Version ID (#VERSION)
	Label *string // Staging label (:LABEL)
}

// Spec represents a parsed SM secret version specification.
//
// Grammar: <name>[#<id> | :<label>]<shift>*
//   - #<id>    optional version ID (0 or 1, mutually exclusive with :LABEL)
//   - :<label> optional staging label (0 or 1, mutually exclusive with #VERSION)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// Examples: my-secret, my-secret#abc123, my-secret:AWSCURRENT, my-secret~1
type Spec = version.Spec[AbsoluteSpec]

// parser defines the SM-specific parsing logic.
var parser = version.AbsoluteParser[AbsoluteSpec]{
	IsSpecifierStart: func(s string, i int) (bool, error) {
		switch s[i] {
		case '#':
			// # followed by valid ID char = version ID specifier
			if i+1 < len(s) && isIDChar(s[i+1]) {
				return true, nil
			}
			// # at end of string is an error
			if i+1 >= len(s) {
				return false, ErrEmptyID
			}
		case ':':
			// : followed by valid label start = label specifier
			if i+1 < len(s) && isLabelChar(s[i+1]) {
				return true, nil
			}
			// : at end of string is an error
			if i+1 >= len(s) {
				return false, ErrEmptyLabel
			}
		}
		return false, nil
	},
	ParseAbsolute: func(remaining string) (AbsoluteSpec, string, error) {
		var abs AbsoluteSpec

		// Parse #id if present
		if len(remaining) > 0 && remaining[0] == '#' {
			end := findNextSpecifier(remaining, 1)
			id := remaining[1:end]
			abs.ID = &id
			remaining = remaining[end:]
		}

		// Parse :label if present
		if len(remaining) > 0 && remaining[0] == ':' {
			end := findNextSpecifier(remaining, 1)
			label := remaining[1:end]
			if !isValidLabel(label) {
				return AbsoluteSpec{}, "", fmt.Errorf("invalid label: %s", label)
			}
			abs.Label = &label
			remaining = remaining[end:]
		}

		// Validate: #id and :label are mutually exclusive
		if abs.ID != nil && abs.Label != nil {
			return AbsoluteSpec{}, "", ErrBothIDAndLabel
		}

		return abs, remaining, nil
	},
	Zero: func() AbsoluteSpec {
		return AbsoluteSpec{}
	},
}

// Parse parses an SM version specification string.
//
// Grammar: <name>[#<id> | :<label>]<shift>*
//
// Shift syntax (Git-like, repeatable):
//   - ~     go back 1 version
//   - ~N    go back N versions
//   - ~~    go back 2 versions (same as ~1~1)
//   - ~1~2  cumulative: go back 3 versions
func Parse(input string) (*Spec, error) {
	return version.Parse(input, parser)
}

// findNextSpecifier finds the next specifier start after position start.
func findNextSpecifier(s string, start int) int {
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '#':
			if i+1 < len(s) && isIDChar(s[i+1]) {
				return i
			}
		case ':':
			if i+1 < len(s) && isLabelChar(s[i+1]) {
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

func isIDChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-'
}

func isLabelChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-' || c == '_'
}

func isValidLabel(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isLabelChar(s[i]) {
			return false
		}
	}
	return true
}
