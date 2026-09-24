// This package's subject is the Secrets Manager version spec itself, and
// spec.go holds its definition and the Parse API. Core, so that Parse does not
// have to become SpecParse. version.go stays a helper for display.
//declscope:core

package awssecretversion

import (
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/version"
	"github.com/mpyw/suve/internal/version/internal"
)

// Secrets Manager-specific errors.
var (
	ErrInvalidID    = errors.New("# must be followed by a version ID")
	ErrInvalidLabel = errors.New(": must be followed by a label")
)

// AbsoluteSpec represents the absolute version specifier for Secrets Manager.
type AbsoluteSpec struct {
	ID    *string // Version ID (#VERSION)
	Label *string // Staging label (:LABEL)
}

// Spec represents a parsed Secrets Manager secret version specification.
//
// Grammar: <name>[#<id> | :<label>]<shift>*
//   - #<id>    optional version ID (0 or 1, mutually exclusive with :LABEL)
//   - :<label> optional staging label (0 or 1, mutually exclusive with #VERSION)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// Examples: my-secret, my-secret#abc123, my-secret:AWSCURRENT, my-secret~1.
type Spec = version.Spec[AbsoluteSpec]

// hasAbsoluteSpec returns true if either ID or Label is already set.
func hasAbsoluteSpec(abs AbsoluteSpec) bool {
	return abs.ID != nil || abs.Label != nil
}

// parser defines the Secrets Manager-specific parsing logic.
//
//nolint:gochecknoglobals // stateless parser configuration
var parser = version.AbsoluteParser[AbsoluteSpec]{
	Parsers: []version.SpecifierParser[AbsoluteSpec]{
		{
			PrefixChar: '#',
			IsChar:     isIDChar,
			Error:      ErrInvalidID,
			Duplicated: hasAbsoluteSpec,
			Apply: func(value string, abs AbsoluteSpec) (AbsoluteSpec, error) {
				abs.ID = lo.ToPtr(value)

				return abs, nil
			},
		},
		{
			PrefixChar: ':',
			IsChar:     isLabelChar,
			Error:      ErrInvalidLabel,
			Duplicated: hasAbsoluteSpec,
			Apply: func(value string, abs AbsoluteSpec) (AbsoluteSpec, error) {
				abs.Label = lo.ToPtr(value)

				return abs, nil
			},
		},
	},
	Zero: func() AbsoluteSpec {
		return AbsoluteSpec{}
	},
}

// Parse parses a Secrets Manager version specification string.
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

// Suffix reconstructs the version-spec suffix (the part after the name) from a
// parsed spec, so that spec.Name+Suffix(spec) re-parses to an equivalent spec.
// It is what callers hand to provider.Reader.Resolve alongside the name.
//
// Examples: {ID:"abc"} -> "#abc"; {Label:"AWSCURRENT", Shift:1} ->
// ":AWSCURRENT~1"; {Shift:2} -> "~2"; {} -> "" (current).
func Suffix(spec *Spec) string {
	var abs string

	switch {
	case spec.Absolute.ID != nil:
		abs = "#" + *spec.Absolute.ID
	case spec.Absolute.Label != nil:
		abs = ":" + *spec.Absolute.Label
	}

	return abs + spec.ShiftSuffix()
}

// ParseDiffArgs parses diff command arguments for Secrets Manager.
// This is a convenience wrapper around diff.ParseArgs with Secrets Manager-specific settings.
func ParseDiffArgs(args []string) (*Spec, *Spec, error) {
	return diffargs.ParseArgs(
		args,
		Parse,
		hasAbsoluteSpec,
		"#:~",
		"usage: suve aws secret diff <spec1> [spec2] | <name> #<version1> [#<version2>]",
	)
}

// isIDChar reports whether c is valid within a Secrets Manager version id.
// Version ids are ClientRequestTokens (not just console UUIDs): a token created
// via the API may contain '_' and '.' as well, so accept them alongside
// letters, digits and '-'. Excludes the specifier characters '#', ':', '~'.
func isIDChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-' || c == '_' || c == '.'
}

func isLabelChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-' || c == '_'
}
