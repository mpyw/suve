package secretversion

import (
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/version"
	"github.com/mpyw/suve/internal/version/internal"
)

// SM-specific errors.
var (
	ErrInvalidID    = errors.New("# must be followed by a version ID")
	ErrInvalidLabel = errors.New(": must be followed by a label")
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

// hasAbsoluteSpec returns true if either ID or Label is already set.
func hasAbsoluteSpec(abs AbsoluteSpec) bool {
	return abs.ID != nil || abs.Label != nil
}

// parser defines the SM-specific parsing logic.
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

// ParseDiffArgs parses diff command arguments for Secrets Manager.
// This is a convenience wrapper around diff.ParseArgs with SM-specific settings.
func ParseDiffArgs(args []string) (*Spec, *Spec, error) {
	return diffargs.ParseArgs(
		args,
		Parse,
		hasAbsoluteSpec,
		"#:~",
		"usage: suve secret diff <spec1> [spec2] | <name> <version1> [version2]",
	)
}

func isIDChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-'
}

func isLabelChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-' || c == '_'
}
