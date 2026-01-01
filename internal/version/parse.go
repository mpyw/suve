// Package version provides shared version specification parsing logic
// for SSM Parameter Store and Secrets Manager version specifiers.
//
// Version specification grammar:
//
//	<name><absolute>?<shift>*
//
// Where:
//   - <name>     is the parameter/secret name (required)
//   - <absolute> is a type-specific absolute version specifier (optional)
//   - <shift>    is ~ or ~N for relative version shift (optional, repeatable)
//
// Examples:
//   - SSM: /my/param, /my/param#3, /my/param~1, /my/param#5~2
//   - SM:  my-secret, my-secret#abc123, my-secret:AWSCURRENT, my-secret~1
package version

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mpyw/suve/internal/version/internal"
)

// Common errors for version parsing.
var (
	ErrEmptySpec            = errors.New("empty specification")
	ErrEmptyName            = errors.New("empty name")
	ErrAmbiguousTilde       = errors.New("ambiguous tilde")
	ErrMultipleAbsoluteSpec = errors.New("multiple absolute version specifiers")
)

// Spec represents a parsed version specification.
// The type parameter A holds service-specific absolute version info
// (e.g., version number for SSM, version ID or label for SM).
type Spec[A any] struct {
	Name     string // Parameter/secret name (e.g., "/my/param", "my-secret")
	Absolute A      // Absolute version specifier (type-specific, zero value if not specified)
	Shift    int    // Relative shift amount (0 means no shift, positive means go back N versions)
}

// HasShift returns true if a relative shift is specified (Shift > 0).
func (s *Spec[A]) HasShift() bool {
	return s.Shift > 0
}

// SpecifierParser defines how to parse a single type of absolute specifier.
// Each service (SSM/SM) defines its own set of SpecifierParsers.
//
// For example, SSM has one parser for "#" (version number),
// while SM has two parsers for "#" (version ID) and ":" (label).
type SpecifierParser[A any] struct {
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
	// Used to detect conflicting specifiers (e.g., both #id and :label in SM).
	// Can be nil if duplicate checking is not needed.
	Duplicated func(abs A) bool

	// Apply assigns the parsed value to abs and returns the updated abs.
	// The value parameter is the string after PrefixChar (e.g., "123" for "#123").
	// Should only do assignment; validation errors (like overflow) are wrapped by caller.
	Apply func(value string, abs A) (A, error)
}

// AbsoluteParser holds the configuration for parsing absolute specifiers.
// Each service (SSM/SM) creates its own AbsoluteParser instance.
type AbsoluteParser[A any] struct {
	// Parsers is the list of specifier parsers to try, in order.
	Parsers []SpecifierParser[A]

	// Zero returns the zero/default value of A.
	// Called when no absolute specifier is present in the input.
	Zero func() A
}

// Parse parses a version specification string into a Spec.
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
func Parse[A any](input string, parser AbsoluteParser[A]) (*Spec[A], error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, ErrEmptySpec
	}

	// Step 1: Find where name ends and specifiers begin.
	// Example: "/my/param#3~1" -> nameEnd=9 (at '#')
	nameEnd, err := findNameEnd(input, parser.Parsers)
	if err != nil {
		return nil, err
	}

	name := input[:nameEnd]

	// Specifier at start means empty name (e.g., "#3" or "~1")
	if name == "" && nameEnd < len(input) {
		return nil, ErrEmptyName
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

// findNameEnd scans input to find where the name ends (specifier starts).
//
// Returns the index of the first specifier character, or len(input) if none found.
// A specifier starts when we find:
//   - '~' followed by end-of-string, digit, or another '~' (shift specifier)
//   - PrefixChar followed by a valid char for that specifier (absolute specifier)
//
// Returns error for:
//   - '~' followed by a letter (ambiguous: could be part of the name or a shift typo)
//   - PrefixChar at end or followed by invalid char, when Error is set
func findNameEnd[A any](input string, parsers []SpecifierParser[A]) (int, error) {
	for i := 0; i < len(input); i++ {
		// Check for shift specifier (~)
		if input[i] == '~' {
			if isShiftStart(input, i) {
				return i, nil // Found shift start
			}
			// "~" followed by letter is ambiguous (e.g., "param~backup")
			if i+1 < len(input) && internal.IsLetter(input[i+1]) {
				return 0, fmt.Errorf("%w: use ~N for version shift", ErrAmbiguousTilde)
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
// Example: "#3:LABEL" with SM parser -> parses "#3", then ":LABEL"
// Example: "#3~1" -> parses "#3", returns "~1" as remaining
//
// Returns error for duplicate/conflicting specifiers or Apply failures.
func parseAbsolute[A any](s string, parsers []SpecifierParser[A], abs A) (A, string, error) {
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
			return abs, "", ErrMultipleAbsoluteSpec
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
func matchParser[A any](ch byte, parsers []SpecifierParser[A]) (SpecifierParser[A], bool) {
	for _, p := range parsers {
		if ch == p.PrefixChar {
			return p, true
		}
	}
	return SpecifierParser[A]{}, false
}
