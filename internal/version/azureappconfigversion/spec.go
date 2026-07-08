// Package azureappconfigversion provides version spec parsing for Azure App
// Configuration (bare key names only).
//
// Azure App Configuration has NO versioning: a key/label pair holds a single
// current value with no version history. It also imposes almost no restriction
// on key characters — ':' is the standard ASP.NET configuration-hierarchy
// separator (e.g. "Logging:LogLevel:Default"), and '#', '~' are legal too.
//
// Because there are no version specifiers to parse, this parser performs NO
// specifier splitting at all: the entire argument is taken verbatim as the key
// name. This is what makes keys containing ':', '#', or '~' addressable — the
// previous behavior mis-read those characters as (unsupported) version
// specifiers and rejected the key outright.
package azureappconfigversion

import (
	"errors"
	"strings"

	"github.com/mpyw/suve/internal/version"
)

// ErrVersioningUnsupported is returned by the provider's History (App
// Configuration keeps no version history). Version specifiers are no longer
// rejected at parse time — they are legal key characters — so this is not
// produced by Parse.
var ErrVersioningUnsupported = errors.New("the Azure App Configuration store does not support versions")

// AbsoluteSpec is empty: Azure App Configuration has no absolute version
// specifier. It exists only to satisfy the shared version.Spec type parameter.
type AbsoluteSpec struct{}

// Spec represents a parsed Azure App Configuration "version" specification.
//
// Grammar: <name>  (the whole argument is the key; nothing is split off)
//
// Examples: my-key, /app/config, Logging:LogLevel:Default, weird#key, a~b.
type Spec = version.Spec[AbsoluteSpec]

// Parse parses an Azure App Configuration key specification string. The entire
// (whitespace-trimmed) input is the key name; no version specifier is split
// off, so ':' / '#' / '~' are preserved verbatim. Empty input yields
// version.ErrEmptySpec.
func Parse(input string) (*Spec, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return nil, version.ErrEmptySpec
	}

	return &Spec{Name: name}, nil
}

// ParseDiffArgs parses diff command arguments for Azure App Configuration.
//
// Since keys are unversioned and carry no specifiers, only one or two bare keys
// are accepted (a single key compares against itself). The generic name+specifier
// concatenation used by versioned stores does not apply here.
func ParseDiffArgs(args []string) (*Spec, *Spec, error) {
	const usage = "usage: suve azure param diff <key1> [key2]"

	switch len(args) {
	case 1:
		spec, err := Parse(args[0])
		if err != nil {
			return nil, nil, err
		}

		return spec, &Spec{Name: spec.Name}, nil
	case 2: //nolint:mnd // two-key comparison
		spec1, err := Parse(args[0])
		if err != nil {
			return nil, nil, err
		}

		spec2, err := Parse(args[1])
		if err != nil {
			return nil, nil, err
		}

		return spec1, spec2, nil
	default:
		return nil, nil, errors.New(usage)
	}
}
