package paramversion

import (
	"errors"
	"strconv"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/version"
	"github.com/mpyw/suve/internal/version/internal"
)

// ErrInvalidVersion is returned when # is not followed by a version number.
var ErrInvalidVersion = errors.New("# must be followed by a version number")

// AbsoluteSpec represents the absolute version specifier for SSM Parameter Store.
type AbsoluteSpec struct {
	Version *int64 // Explicit version number (#VERSION)
}

// Spec represents a parsed SSM Parameter Store parameter version specification.
//
// Grammar: <name>[#<N>]<shift>*
//   - #<N>     optional version number (0 or 1)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// Examples: /my/param, /my/param#3, /my/param~1, /my/param#5~2, /my/param~~.
type Spec = version.Spec[AbsoluteSpec]

// parser defines the SSM Parameter Store-specific parsing logic.
//
//nolint:gochecknoglobals // stateless parser configuration
var parser = version.AbsoluteParser[AbsoluteSpec]{
	Parsers: []version.SpecifierParser[AbsoluteSpec]{
		{
			PrefixChar: '#',
			IsChar:     internal.IsDigit,
			Error:      ErrInvalidVersion,
			Duplicated: func(abs AbsoluteSpec) bool {
				return abs.Version != nil
			},
			Apply: func(value string, abs AbsoluteSpec) (AbsoluteSpec, error) {
				v, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return abs, err
				}
				abs.Version = lo.ToPtr(v)

				return abs, nil
			},
		},
	},
	Zero: func() AbsoluteSpec {
		return AbsoluteSpec{}
	},
}

// Parse parses an SSM Parameter Store version specification string.
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

// ParseDiffArgs parses diff command arguments for SSM Parameter Store parameters.
// This is a convenience wrapper around diff.ParseArgs with SSM Parameter Store-specific settings.
func ParseDiffArgs(args []string) (*Spec, *Spec, error) {
	return diffargs.ParseArgs(
		args,
		Parse,
		func(abs AbsoluteSpec) bool { return abs.Version != nil },
		"#~",
		"usage: suve param diff <spec1> [spec2] | <name> <version1> [version2]",
	)
}
