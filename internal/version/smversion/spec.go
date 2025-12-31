package smversion

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mpyw/suve/internal/version/internal"
	"github.com/mpyw/suve/internal/version/shift"
)

// Sentinel errors for version specification parsing.
var (
	ErrNoSpecifier    = errors.New("no specifier found")
	ErrEmptySpec      = errors.New("empty version specification")
	ErrEmptyName      = errors.New("empty name in version specification")
	ErrEmptyID        = errors.New("empty version ID after #")
	ErrEmptyLabel     = errors.New("empty label after colon")
	ErrAmbiguousTilde = errors.New("ambiguous tilde in name")
	ErrBothIDAndLabel = errors.New("cannot specify both #id and :label")
)

// Spec represents a parsed SM secret version specification.
//
// Grammar: <name>[#<id> | :<label>]<shift>*
//   - #<id>    optional version ID (0 or 1, mutually exclusive with :label)
//   - :<label> optional staging label (0 or 1, mutually exclusive with #id)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// Examples: my-secret, my-secret#abc123, my-secret:AWSCURRENT, my-secret~1
type Spec struct {
	Name  string  // Secret name
	ID    *string // Version ID (#uuid)
	Label *string // Staging label (:AWSCURRENT, :AWSPREVIOUS, etc.)
	Shift int     // Number of versions to go back (~N, ~~, etc.)
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

	// Parse #id if present
	if strings.HasPrefix(remaining, "#") {
		end := findNextSpecifier(remaining, 1)
		id := remaining[1:end]
		// id is guaranteed non-empty because findSpecifierStart only
		// returns a position for # when followed by isIDChar
		spec.ID = &id
		remaining = remaining[end:]
	}

	// Parse :label if present
	if strings.HasPrefix(remaining, ":") {
		end := findNextSpecifier(remaining, 1)
		label := remaining[1:end]
		// label is guaranteed non-empty because findSpecifierStart only
		// returns a position for : when followed by isLabelChar
		if !isValidLabel(label) {
			return nil, fmt.Errorf("invalid label: %s", label)
		}
		spec.Label = &label
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

	// Validate: #id and :label are mutually exclusive
	if spec.ID != nil && spec.Label != nil {
		return nil, ErrBothIDAndLabel
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
			// # followed by valid ID char = version ID specifier
			if i+1 < len(s) && isIDChar(s[i+1]) {
				return i, nil
			}
			// # at end of string is an error
			if i+1 >= len(s) {
				return 0, ErrEmptyID
			}
		case ':':
			// : followed by valid label start = label specifier
			if i+1 < len(s) && isLabelChar(s[i+1]) {
				return i, nil
			}
			// : at end of string is an error
			if i+1 >= len(s) {
				return 0, ErrEmptyLabel
			}
		case '~':
			if shift.IsShiftStart(s, i) {
				return i, nil
			}
			// Tilde followed by letter is ambiguous - error
			if i+1 < len(s) && internal.IsLetter(s[i+1]) {
				return 0, fmt.Errorf("%w: use ~N for version shift or avoid ~ followed by letters", ErrAmbiguousTilde)
			}
		}
	}
	return 0, ErrNoSpecifier
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
	// Empty check is not needed here because Parse guarantees
	// labels are non-empty before calling isValidLabel
	for i := 0; i < len(s); i++ {
		if !isLabelChar(s[i]) {
			return false
		}
	}
	return true
}

// HasShift returns true if a shift is specified.
func (s *Spec) HasShift() bool {
	return s.Shift > 0
}
