// Package paramtype maps between the provider-neutral domain.ValueType and the
// AWS SSM Parameter Store type names ("String", "SecureString", "StringList")
// used in CLI output and in the --type flag. It keeps the SSM-specific display
// and parsing concerns in the param CLI layer so the usecases carry no AWS type.
package paramtype

import "github.com/mpyw/suve/internal/domain"

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

// Parse maps an SSM --type flag value to a domain.ValueType. Unknown values map
// to domain.ValueTypePlaintext (matching the historical default of "String").
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
