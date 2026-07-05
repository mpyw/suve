// Package azureappconfigversion provides version spec parsing for Azure App
// Configuration (bare key names only).
//
// Azure App Configuration has NO versioning: a key/label pair holds a single
// current value with no version history. This parser therefore accepts a bare
// name only; ANY version specifier (#VERSION, ~SHIFT, or :LABEL) is rejected at
// parse time with ErrVersioningUnsupported, so the mistake produces a clean
// error before any API call rather than reaching the provider.
package azureappconfigversion

import (
	"errors"

	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/version"
)

// ErrVersioningUnsupported is returned when any version specifier (#VERSION,
// ~SHIFT, or :LABEL) is used. Azure App Configuration does not support versions.
var ErrVersioningUnsupported = errors.New("the Azure App Configuration store does not support versions")

// AbsoluteSpec is empty: Azure App Configuration has no absolute version
// specifier. It exists only to satisfy the shared version.Spec type parameter.
type AbsoluteSpec struct{}

// Spec represents a parsed Azure App Configuration "version" specification.
//
// Grammar: <name>  (bare name only; no specifiers permitted)
//
// Examples: my-key, /app/config. Any #VERSION, ~SHIFT, or :LABEL is rejected.
type Spec = version.Spec[AbsoluteSpec]

// parser rejects the two absolute specifier prefixes ('#', ':') cleanly: their
// IsChar never matches, so findNameEnd returns ErrVersioningUnsupported instead
// of folding the prefix into the name. The '~' shift is handled by the shared
// framework (it is not a configurable prefix) and is rejected post-parse in
// Parse via HasShift.
//
//nolint:gochecknoglobals // stateless parser configuration
var parser = version.AbsoluteParser[AbsoluteSpec]{
	Parsers: []version.SpecifierParser[AbsoluteSpec]{
		{
			PrefixChar: '#',
			IsChar:     func(byte) bool { return false },
			Error:      ErrVersioningUnsupported,
			Apply: func(_ string, abs AbsoluteSpec) (AbsoluteSpec, error) {
				return abs, ErrVersioningUnsupported
			},
		},
		{
			PrefixChar: ':',
			IsChar:     func(byte) bool { return false },
			Error:      ErrVersioningUnsupported,
			Apply: func(_ string, abs AbsoluteSpec) (AbsoluteSpec, error) {
				return abs, ErrVersioningUnsupported
			},
		},
	},
	Zero: func() AbsoluteSpec {
		return AbsoluteSpec{}
	},
}

// Parse parses an Azure App Configuration key specification string. It accepts a
// bare name only; any version specifier yields ErrVersioningUnsupported.
//
// Empty-input and empty-name errors from the shared grammar are surfaced
// unchanged; every other parse failure (a '#'/':' specifier, or a '~' shift) is
// normalized to ErrVersioningUnsupported so the caller gets one clear message.
func Parse(input string) (*Spec, error) {
	spec, err := version.Parse(input, parser)

	switch {
	case errors.Is(err, version.ErrEmptySpec), errors.Is(err, version.ErrEmptyName):
		return nil, err
	case err != nil:
		return nil, ErrVersioningUnsupported
	case spec.HasShift():
		return nil, ErrVersioningUnsupported
	}

	return spec, nil
}

// ParseDiffArgs parses diff command arguments for Azure App Configuration. Two
// bare keys may be compared; any version specifier yields
// ErrVersioningUnsupported.
func ParseDiffArgs(args []string) (*Spec, *Spec, error) {
	return diffargs.ParseArgs(
		args,
		Parse,
		func(AbsoluteSpec) bool { return false },
		"",
		"usage: suve azure param diff <key1> [key2]",
	)
}
