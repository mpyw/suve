// Package diffargs provides shared diff command argument parsing logic for SSM and SM.
//
// The diff command compares two versions of a parameter (SSM) or secret (SM).
// This package provides a generic ParseArgs function that handles the various
// argument patterns supported by both services.
//
// # Argument Patterns
//
// The diff command supports three formats for specifying versions to compare:
//
// ## Full Spec Format
//
// Each argument is a complete specification including name and version.
//
//   - 1 arg: Compare specified version against default (latest/AWSCURRENT)
//     suve param diff /app/config#3
//     suve secret diff my-secret:AWSPREVIOUS
//
//   - 2 args: Compare two fully-specified versions
//     suve param diff /app/config#1 /app/config#2
//     suve secret diff my-secret:AWSPREVIOUS my-secret:AWSCURRENT
//
// ## Partial Spec Format
//
// Name is specified separately from version specifiers.
//
//   - 2 args: Name + specifier → compare with default
//     suve param diff /app/config '#3'
//     suve secret diff my-secret ':AWSPREVIOUS'
//
//   - 3 args: Name + two specifiers
//     suve param diff /app/config '#1' '#2'
//     suve secret diff my-secret ':AWSPREVIOUS' ':AWSCURRENT'
//
// ## Mixed Format
//
// First argument is full spec, second is specifier-only (inherits name from first).
//
//   - 2 args: Full spec + specifier
//     suve param diff /app/config#1 '#2'
//     suve secret diff my-secret:AWSPREVIOUS ':AWSCURRENT'
//
// # Return Value Semantics
//
// ParseArgs always returns (spec1, spec2) where the comparison is performed as:
//
//	diff(spec1, spec2) = "what changed from spec1 to spec2"
//
// This means spec1 is the "old" version and spec2 is the "new" version.
// The diff output will show:
//   - Lines removed from spec1 as "-" (red)
//   - Lines added in spec2 as "+" (green)
package diffargs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mpyw/suve/internal/version"
)

// ParseArgs parses diff command arguments into two version specifications.
//
// This function is generic over the absolute specifier type A, which differs
// between SSM (AbsoluteSpec with Version *int64) and SM (AbsoluteSpec with
// ID *string and Label *string).
//
// # Parameters
//
//   - args: Command line arguments (1-3 arguments supported)
//   - parse: Service-specific parser function (e.g., paramversion.Parse, secretversion.Parse)
//   - hasAbsolute: Returns true if the absolute specifier is set (non-zero).
//     Used to distinguish "mixed" pattern from "partial spec" pattern in 2-arg case.
//     For SSM: func(abs) bool { return abs.Version != nil }
//     For SM: func(abs) bool { return abs.ID != nil || abs.Label != nil }
//   - prefixes: Characters that start a specifier (e.g., "#~" for SSM, "#:~" for SM).
//     Used to detect if second argument is specifier-only.
//   - usage: Error message to show when argument count is invalid.
//
// # Return Values
//
// Returns (spec1, spec2, nil) on success, where:
//   - spec1: The "from" version (shown with "-" in diff)
//   - spec2: The "to" version (shown with "+" in diff)
//
// Returns (nil, nil, error) on parse failure or invalid argument count.
//
// # Examples
//
// SSM usage:
//
//	spec1, spec2, err := ParseArgs(
//	    args,
//	    paramversion.Parse,
//	    func(abs paramversion.AbsoluteSpec) bool { return abs.Version != nil },
//	    "#~",
//	    "usage: suve param diff <spec1> [spec2] | <name> <version1> [version2]",
//	)
//
// SM usage:
//
//	spec1, spec2, err := ParseArgs(
//	    args,
//	    secretversion.Parse,
//	    func(abs secretversion.AbsoluteSpec) bool { return abs.ID != nil || abs.Label != nil },
//	    "#:~",
//	    "usage: suve secret diff <spec1> [spec2] | <name> <version1> [version2]",
//	)
func ParseArgs[A any](
	args []string,
	parse func(string) (*version.Spec[A], error),
	hasAbsolute func(A) bool,
	prefixes string,
	usage string,
) (*version.Spec[A], *version.Spec[A], error) {
	if len(args) == 0 || len(args) > 3 {
		return nil, nil, errors.New(usage)
	}

	switch len(args) {
	case 1:
		return parseOneArg(args[0], parse)
	case 2:
		return parseTwoArgs(args[0], args[1], parse, hasAbsolute, prefixes)
	default: // case 3
		return parseThreeArgs(args[0], args[1], args[2], parse)
	}
}

// parseOneArg handles full spec format with single argument.
//
// Pattern: "name#v" or "name~N" or "name:LABEL"
//
// The single argument specifies the "from" version, and it will be compared
// against the default version (latest for SSM, AWSCURRENT for SM).
//
// Examples:
//
//	"/app/config#3"         → spec1=#3, spec2=latest
//	"my-secret:AWSPREVIOUS" → spec1=AWSPREVIOUS, spec2=AWSCURRENT
//	"/app/config~1"         → spec1=~1 (previous), spec2=latest
//
// Note: If the argument has no specifier (e.g., just "/app/config"),
// both spec1 and spec2 will be default, resulting in "identical versions" warning.
func parseOneArg[A any](
	arg string,
	parse func(string) (*version.Spec[A], error),
) (*version.Spec[A], *version.Spec[A], error) {
	spec, err := parse(arg)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version specification: %w", err)
	}

	// spec2 is the default version (zero absolute specifier, no shift).
	// For SSM: latest version
	// For SM: AWSCURRENT label
	var zero A
	spec2 := &version.Spec[A]{Name: spec.Name, Absolute: zero, Shift: 0}

	return spec, spec2, nil
}

// parseTwoArgs handles two argument patterns with automatic format detection.
//
// This function detects three formats based on the second argument:
//
// # Format A: Full Spec x2 (both args are complete specifications)
//
// Detected when: second argument does NOT start with a prefix character.
//
//	"/app/config#1" "/app/config#2" → spec1=#1, spec2=#2
//	"secret-a:PREV" "secret-b:CURR" → spec1=secret-a:PREV, spec2=secret-b:CURR
//
// # Format B: Mixed (first has specifier, second is specifier-only)
//
// Detected when: second argument starts with prefix AND first argument has a specifier.
//
//	"/app/config#1" "#2"     → spec1=#1, spec2=#2
//	"my-secret:PREV" ":CURR" → spec1=PREV, spec2=CURR
//
// # Format C: Partial Spec (first is name-only, second is specifier-only)
//
// Detected when: second argument starts with prefix AND first argument has NO specifier.
// In this case, the order is swapped: spec1 gets the specifier, spec2 gets default.
//
//	"/app/config" "#3"  → spec1=#3, spec2=latest (NOT spec1=latest, spec2=#3)
//	"my-secret" ":PREV" → spec1=PREV, spec2=AWSCURRENT
//
// The partial spec swap ensures consistent semantics: you're always comparing
// "the specified version" against "the default version", regardless of argument order.
func parseTwoArgs[A any](
	arg1, arg2 string,
	parse func(string) (*version.Spec[A], error),
	hasAbsolute func(A) bool,
	prefixes string,
) (*version.Spec[A], *version.Spec[A], error) {
	// Parse the first argument (always a complete specification)
	spec1, err := parse(arg1)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid first argument: %w", err)
	}

	// Check if second argument is specifier-only (starts with #, :, or ~)
	// Examples: "#3", ":AWSPREVIOUS", "~1"
	if len(arg2) > 0 && strings.ContainsRune(prefixes, rune(arg2[0])) {
		// Specifier-only: prepend the name from first argument
		// "/app/config" + "#3" → "/app/config#3"
		spec2, err := parse(spec1.Name + arg2)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid second argument: %w", err)
		}

		// Detect format: mixed vs partial spec
		// Mixed: first arg has a specifier (version, label, or shift)
		// Partial Spec: first arg is name-only (no specifier)
		firstHasSpec := hasAbsolute(spec1.Absolute) || spec1.Shift > 0
		if firstHasSpec {
			// Mixed format: both have specifiers
			// "/app/config#1" "#2" → compare #1 with #2
			return spec1, spec2, nil
		}

		// Partial spec format: first arg has no specifier, swap the order
		// "/app/config" "#3" → compare #3 with latest
		// This makes the specified version the "from" and default the "to"
		var zero A
		return spec2, &version.Spec[A]{Name: spec1.Name, Absolute: zero, Shift: 0}, nil
	}

	// Full spec x2: second argument is a complete specification
	// "/app/config#1" "/app/config#2" → compare #1 with #2
	spec2, err := parse(arg2)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid second argument: %w", err)
	}

	return spec1, spec2, nil
}

// parseThreeArgs handles partial spec format with separate name and specifiers.
//
// Pattern: "name" "specifier1" "specifier2"
//
// The name is specified once, and both specifiers use that same name.
// This format is useful when you want to compare two versions of the same
// parameter/secret without repeating the name.
//
// Examples:
//
//	"/app/config" "#1" "#2"                      → compare /app/config#1 with /app/config#2
//	"my-secret" ":AWSPREVIOUS" ":AWSCURRENT"     → compare AWSPREVIOUS with AWSCURRENT
//	"/app/config" "~2" "~1"                      → compare 2-versions-ago with 1-version-ago
//
// Note: The specifiers don't need to start with a prefix character in this format,
// because we always concatenate name + specifier. However, in practice they usually
// do start with #, :, or ~ to avoid ambiguity.
func parseThreeArgs[A any](
	name, version1, version2 string,
	parse func(string) (*version.Spec[A], error),
) (*version.Spec[A], *version.Spec[A], error) {
	// Parse name + first specifier
	spec1, err := parse(name + version1)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version1: %w", err)
	}

	// Parse name + second specifier
	spec2, err := parse(name + version2)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version2: %w", err)
	}

	return spec1, spec2, nil
}
