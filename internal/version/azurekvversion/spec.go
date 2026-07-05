// Package azurekvversion provides version spec parsing for Azure Key Vault
// secrets (name#VERSION~SHIFT).
//
// Azure Key Vault secret versions are opaque strings (32-character hex ids) or
// the empty/current alias; there are NO staging labels. The grammar mirrors the
// AWS Secrets Manager one for the "#" specifier (an opaque version id plus
// ~SHIFT), but a ":LABEL" specifier is rejected at parse time with a clear error
// so the mistake never reaches the provider.
package azurekvversion

import (
	"errors"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/cli/diffargs"
	"github.com/mpyw/suve/internal/version"
	"github.com/mpyw/suve/internal/version/internal"
)

// Azure Key Vault-specific errors.
var (
	// ErrInvalidID is returned when # is not followed by a version id.
	ErrInvalidID = errors.New("# must be followed by a version id")
	// ErrLabelUnsupported is returned when a :LABEL specifier is used. Azure Key
	// Vault has no staging labels (versions are opaque ids or the current
	// version), so a colon specifier is always invalid.
	ErrLabelUnsupported = errors.New(
		": staging labels are not supported for Azure Key Vault " +
			"(versions are opaque ids or the current version)",
	)
)

// AbsoluteSpec represents the absolute version specifier for Azure Key Vault.
type AbsoluteSpec struct {
	ID *string // Explicit version id (#VERSION)
}

// Spec represents a parsed Azure Key Vault version specification.
//
// Grammar: <name>[#<id>]<shift>*
//   - #<id>    optional version id (0 or 1)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// A ":LABEL" specifier is rejected: Azure Key Vault has no staging labels.
//
// Examples: my-secret, my-secret#abc123, my-secret~1, my-secret#abc123~2, my-secret~~.
type Spec = version.Spec[AbsoluteSpec]

// hasAbsoluteSpec returns true if an ID is already set.
func hasAbsoluteSpec(abs AbsoluteSpec) bool {
	return abs.ID != nil
}

// parser defines the Azure Key Vault-specific parsing logic. The '#' parser
// accepts an opaque version id; the ':' parser exists ONLY to reject
// staging-label syntax cleanly (its IsChar never matches, so any ':' triggers
// ErrLabelUnsupported rather than being silently folded into the name).
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

// Parse parses an Azure Key Vault version specification string.
//
// Grammar: <name>[#<id>]<shift>*
//
// Shift syntax (Git-like, repeatable):
//   - ~      go back 1 version
//   - ~N     go back N versions (e.g., ~2)
//   - ~~     go back 2 versions (same as ~1~1)
//   - ~1~2   cumulative: go back 3 versions
func Parse(input string) (*Spec, error) {
	return version.Parse(input, parser)
}

// ParseDiffArgs parses diff command arguments for Azure Key Vault. This is a
// convenience wrapper around diffargs.ParseArgs with Azure Key Vault-specific
// settings.
func ParseDiffArgs(args []string) (*Spec, *Spec, error) {
	return diffargs.ParseArgs(
		args,
		Parse,
		hasAbsoluteSpec,
		"#~",
		"usage: suve azure secret diff <spec1> [spec2] | <name> <version1> [version2]",
	)
}

// isIDChar reports whether c is valid within a Key Vault version id (hex-like:
// letters, digits, and dashes).
func isIDChar(c byte) bool {
	return internal.IsLetter(c) || internal.IsDigit(c) || c == '-'
}
