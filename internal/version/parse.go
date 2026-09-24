// parse.go is the grammar-neutral engine that every grammar file builds its
// Parse on: it splits a specification into the name, the absolute specifiers
// and the shift clause.

package version

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mpyw/suve/internal/version/internal"
)

// Errors for a specification the engine cannot split.
var (
	errParseEmptyName        = errors.New("empty name")
	errParseAmbiguousTilde   = errors.New("ambiguous tilde")
	errParseMultipleAbsolute = errors.New("multiple absolute version specifiers")
)

// specifierParser defines how to parse a single type of absolute specifier.
// Each grammar defines its own set of specifierParsers.
//
// For example, NumericGrammar has one parser for "#" (version number), while
// an OpaqueGrammar with labels has two parsers for "#" (version ID) and ":"
// (label).
//
//declscope:package // numeric.go and opaque.go build their parsers from it
type specifierParser[A any] struct {
	// PrefixChar is the character that starts this specifier (e.g., '#', ':').
	PrefixChar byte

	// IsChar returns true if the byte is valid within this specifier's value.
	// Used to determine where the specifier value ends.
	// Example: for "#123", IsChar would return true for '1', '2', '3'.
	IsChar func(byte) bool

	// Error is returned when PrefixChar is found but not followed by a valid char.
	// If nil, the PrefixChar is treated as part of the name instead of an error.
	// Example: "#" at end of input, or "#" followed by invalid char.
	Error error

	// Duplicated returns true if this specifier type is already set in abs.
	// Used to detect conflicting specifiers (e.g., both #id and :label in Secrets Manager).
	// Can be nil if duplicate checking is not needed.
	Duplicated func(abs A) bool

	// Apply assigns the parsed value to abs and returns the updated abs.
	// The value parameter is the string after PrefixChar (e.g., "123" for "#123").
	// Should only do assignment; validation errors (like overflow) are wrapped by caller.
	Apply func(value string, abs A) (A, error)
}

// absoluteParser holds the configuration for parsing absolute specifiers.
// Each grammar builds its own absoluteParser.
//
//declscope:package // numeric.go and opaque.go hand it to parseSpec
type absoluteParser[A any] struct {
	// Parsers is the list of specifier parsers to try, in order.
	Parsers []specifierParser[A]

	// Zero returns the zero/default value of A.
	// Called when no absolute specifier is present in the input.
	Zero func() A
}

// parseSpec parses a version specification string into a Spec.
//
// The parsing proceeds in three steps:
//  1. Find where the name ends (where specifiers begin)
//  2. Parse absolute specifier(s) if present
//  3. Parse shift specifier(s) if present
//
// Returns error if:
//   - Input is empty or whitespace only
//   - Name is empty (specifier at start)
//   - Invalid specifier syntax (e.g., "#" at end, ambiguous "~")
//   - Conflicting specifiers (e.g., both #id and :label)
//
//declscope:package // numeric.go and opaque.go run their Parse through it
func parseSpec[A any](input string, parser absoluteParser[A]) (*Spec[A], error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, ErrEmptySpec
	}

	// Step 1: Find where name ends and specifiers begin.
	// Example: "/my/param#3~1" -> nameEnd=9 (at '#')
	nameEnd, err := parseNameEnd(input, parser.Parsers)
	if err != nil {
		return nil, err
	}

	name := input[:nameEnd]

	// Specifier at start means empty name (e.g., "#3" or "~1")
	if name == "" && nameEnd < len(input) {
		return nil, errParseEmptyName
	}

	// No specifier found - entire input is the name
	if nameEnd == len(input) {
		return &Spec[A]{Name: name, Absolute: parser.Zero()}, nil
	}

	// Step 2: Parse absolute specifier(s).
	// Example: "#3" -> abs.Version=3, rest="~1"
	abs, rest, err := parseAbsolute(input[nameEnd:], parser.Parsers, parser.Zero())
	if err != nil {
		return nil, err
	}

	// Step 3: Parse shift specifier(s).
	// Example: "~1" -> s=1
	var s int
	if rest != "" {
		if s, err = parseShift(rest); err != nil {
			return nil, err
		}
	}

	return &Spec[A]{Name: name, Absolute: abs, Shift: s}, nil
}

// parseNameEnd scans input to find where the name ends (specifier starts).
//
// Returns the index of the first specifier character, or len(input) if none found.
// A specifier starts when we find:
//   - '~' followed by end-of-string, digit, or another '~' (shift specifier)
//   - PrefixChar followed by a valid char for that specifier (absolute specifier)
//
// Returns error for:
//   - '~' followed by a letter (ambiguous: could be part of the name or a shift typo)
//   - PrefixChar at end or followed by invalid char, when Error is set
func parseNameEnd[A any](input string, parsers []specifierParser[A]) (int, error) {
	for i := range len(input) {
		// Check for shift specifier (~)
		if input[i] == '~' {
			if parsesAsShift(input, i) {
				return i, nil // Found shift start
			}
			// "~" followed by letter is ambiguous (e.g., "param~backup")
			if i+1 < len(input) && internal.IsLetter(input[i+1]) {
				return 0, fmt.Errorf("%w: use ~N for version shift", errParseAmbiguousTilde)
			}
			// "~" followed by other char - treat as part of name, keep scanning
			continue
		}

		// Check for absolute specifiers (e.g., '#', ':')
		for _, p := range parsers {
			if input[i] != p.PrefixChar {
				continue
			}
			// PrefixChar followed by valid char = specifier start
			if i+1 < len(input) && p.IsChar(input[i+1]) {
				return i, nil
			}
			// PrefixChar at end or followed by invalid char
			if p.Error != nil {
				return 0, p.Error
			}
			// No error set - treat as part of name
		}
	}

	return len(input), nil // No specifier found
}

// parseAbsolute parses absolute specifier(s) from the start of s.
//
// Repeatedly matches PrefixChar + value until no more matches.
// Stops when encountering '~' (shift) or unrecognized character.
//
// Example: "#3:LABEL" with Secrets Manager parser -> parses "#3", then ":LABEL"
// Example: "#3~1" -> parses "#3", returns "~1" as remaining
//
// Returns error for duplicate/conflicting specifiers or Apply failures.
func parseAbsolute[A any](s string, parsers []specifierParser[A], abs A) (A, string, error) {
	for len(s) > 0 && s[0] != '~' {
		// Find parser for this prefix character
		p, ok := matchParser(s[0], parsers)
		if !ok {
			break // Unknown char - stop parsing absolute specifiers
		}

		// Find end of specifier value (scan while IsChar returns true)
		end := 1
		for end < len(s) && p.IsChar(s[end]) {
			end++
		}

		// Check for duplicate/conflicting specifiers
		if p.Duplicated != nil && p.Duplicated(abs) {
			return abs, "", errParseMultipleAbsolute
		}

		// Apply the parsed value
		var err error
		if abs, err = p.Apply(s[1:end], abs); err != nil {
			return abs, "", fmt.Errorf("invalid specifier value %q: %w", s[1:end], err)
		}

		s = s[end:] // Advance past this specifier
	}

	return abs, s, nil
}

// matchParser finds the parser whose PrefixChar matches ch.
// Returns (parser, true) if found, (zero, false) if not.
func matchParser[A any](ch byte, parsers []specifierParser[A]) (specifierParser[A], bool) {
	for _, p := range parsers {
		if ch == p.PrefixChar {
			return p, true
		}
	}

	return specifierParser[A]{}, false
}

// rejectingLabelParser is a ':' parser that exists only to reject staging-label
// syntax, for a grammar without labels. Its IsChar never matches, so any ':'
// in the name triggers err rather than being folded into the name, and a ':'
// after an absolute specifier fails in Apply.
//
//declscope:package // numeric.go and opaque.go reject ':' with it
func rejectingLabelParser[A any](err error) specifierParser[A] {
	return specifierParser[A]{
		PrefixChar: ':',
		IsChar:     func(byte) bool { return false },
		Error:      err,
		Apply: func(_ string, abs A) (A, error) {
			return abs, err
		},
	}
}
