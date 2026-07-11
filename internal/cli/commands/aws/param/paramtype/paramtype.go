// Package paramtype maps between the provider-neutral domain.ValueType and the
// AWS SSM Parameter Store type names ("String", "SecureString", "StringList")
// used in CLI output and in the --type flag. It keeps the SSM-specific display
// and parsing concerns in the param CLI layer so the usecases carry no AWS type.
package paramtype

import (
	"fmt"
	"strings"

	"github.com/mpyw/suve/internal/domain"
)

// SSM parameter type names as displayed by the CLI and accepted by --type.
const (
	String       = "String"
	SecureString = "SecureString"
	StringList   = "StringList"
)

// Options returns the SSM parameter type display names in their canonical
// order. It is the single source of truth for the set of selectable parameter
// types (e.g. the GUI type dropdown), so callers never hardcode the list.
func Options() []string {
	return []string{String, SecureString, StringList}
}

// Display maps a domain.ValueType to its SSM type name for output. It preserves
// the historical byte-for-byte rendering:
//   - domain.ValueTypePlaintext -> "String"
//   - domain.ValueTypeSecret    -> "SecureString"
//   - domain.ValueTypeList      -> "StringList"
func Display(t domain.ValueType) string {
	switch t {
	case domain.ValueTypeSecret:
		return SecureString
	case domain.ValueTypeList:
		return StringList
	case domain.ValueTypePlaintext:
		return String
	default:
		return String
	}
}

// Validate rejects a non-empty --type value that is not one of Options(). The
// empty string is allowed: it means the flag was not set, so the default
// ("String") applies. Callers MUST validate before Parse, because Parse maps any
// unknown value to plaintext — so a typo like "securestring" or "SecureSting"
// would otherwise silently store a value the user meant to encrypt as plaintext.
func Validate(s string) error {
	switch s {
	case "", String, SecureString, StringList:
		return nil
	default:
		return fmt.Errorf("invalid --type %q: must be one of %s", s, strings.Join(Options(), ", "))
	}
}

// Parse maps an SSM --type flag value to a domain.ValueType. Unknown values map
// to domain.ValueTypePlaintext (matching the historical default of "String");
// callers should Validate first to reject typos (see Validate).
func Parse(s string) domain.ValueType {
	switch s {
	case SecureString:
		return domain.ValueTypeSecret
	case StringList:
		return domain.ValueTypeList
	case String:
		return domain.ValueTypePlaintext
	default:
		return domain.ValueTypePlaintext
	}
}
