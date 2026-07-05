// Package gcloudversion provides version spec parsing for Google Cloud Secret
// Manager (name#VERSION~SHIFT).
//
// Google Cloud secret versions are positive integers (1, 2, 3, ...) or the
// "latest" alias; there are NO staging labels. The grammar therefore mirrors
// the SSM Parameter Store grammar (an integer #VERSION plus ~SHIFT) rather than
// the Secrets Manager one, and a ":LABEL" specifier is rejected at parse time
// with a clear error so the mistake never reaches the provider.
package gcloudversion

import (
	"errors"
	"strconv"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/version"
	"github.com/mpyw/suve/internal/version/internal"
)

// Google Cloud Secret Manager-specific errors.
var (
	// ErrInvalidVersion is returned when # is not followed by a version number.
	ErrInvalidVersion = errors.New("# must be followed by a version number")
	// ErrLabelUnsupported is returned when a :LABEL specifier is used. Google
	// Cloud Secret Manager has no staging labels (versions are integers or
	// "latest"), so a colon specifier is always invalid.
	ErrLabelUnsupported = errors.New(
		": staging labels are not supported for Google Cloud Secret Manager " +
			"(versions are integers or \"latest\")",
	)
)

// AbsoluteSpec represents the absolute version specifier for Google Cloud
// Secret Manager.
type AbsoluteSpec struct {
	Version *int64 // Explicit version number (#VERSION)
}

// Spec represents a parsed Google Cloud Secret Manager version specification.
//
// Grammar: <name>[#<N>]<shift>*
//   - #<N>     optional version number (0 or 1)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// A ":LABEL" specifier is rejected: Google Cloud has no staging labels.
//
// Examples: my-secret, my-secret#3, my-secret~1, my-secret#5~2, my-secret~~.
type Spec = version.Spec[AbsoluteSpec]

// parser defines the Google Cloud Secret Manager-specific parsing logic. The
// '#' parser accepts an integer version; the ':' parser exists ONLY to reject
// staging-label syntax cleanly (its IsChar never matches, so any ':' triggers
// ErrLabelUnsupported rather than being silently folded into the name).
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
		{
			PrefixChar: ':',
			// IsChar never matches: a ':' can never begin a valid specifier, so
			// findNameEnd immediately returns ErrLabelUnsupported instead of
			// treating the ':' as part of the name.
			IsChar: func(byte) bool { return false },
			Error:  ErrLabelUnsupported,
			Apply: func(_ string, abs AbsoluteSpec) (AbsoluteSpec, error) {
				return abs, ErrLabelUnsupported
			},
		},
	},
	Zero: func() AbsoluteSpec {
		return AbsoluteSpec{}
	},
}

// Parse parses a Google Cloud Secret Manager version specification string.
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

// ParseDiffArgs parses diff command arguments for Google Cloud Secret Manager.
// This is a convenience wrapper around diffargs.ParseArgs with Google Cloud
// Secret Manager-specific settings.
func ParseDiffArgs(args []string) (*Spec, *Spec, error) {
	return diffargs.ParseArgs(
		args,
		Parse,
		func(abs AbsoluteSpec) bool { return abs.Version != nil },
		"#~",
		"usage: suve gcloud secret diff <spec1> [spec2] | <name> <version1> [version2]",
	)
}
