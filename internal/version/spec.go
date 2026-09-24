// Package version parses the version specifiers that follow a name
// (#VERSION, :LABEL, ~SHIFT) for every cloud product.
//
// Three grammars cover the products: NumericGrammar (integer versions),
// OpaqueGrammar (opaque version ids, optionally with staging labels), and
// BareGrammar (unversioned; the whole argument is the name). products.go binds
// each product to one of them (AWSParameterStore, AWSSecretsManager,
// GoogleCloudSecretManager, AzureKeyVault, AzureAppConfiguration). Each versioned grammar's Suffix rebuilds the
// suffix that callers hand to the use cases.
//
// Version specification grammar:
//
//	<name><absolute>?<shift>*
//
// Where:
//   - <name>     is the parameter/secret name (required)
//   - <absolute> is a type-specific absolute version specifier (optional)
//   - <shift>    is ~ or ~N for relative version shift (optional, repeatable)
//
// Examples:
//   - Numeric: /my/param, /my/param#3, /my/param~1, /my/param#5~2
//   - Opaque: my-secret, my-secret#abc123, my-secret:AWSCURRENT, my-secret~1
package version

import (
	"errors"
	"strconv"
)

// ErrEmptySpec is returned for an empty or whitespace-only specification.
var ErrEmptySpec = errors.New("empty specification")

// Spec represents a parsed version specification.
// The type parameter A holds grammar-specific absolute version info
// (NumericAbsolute, OpaqueAbsolute, or BareAbsolute).
type Spec[A any] struct {
	Name     string // Parameter/secret name (e.g., "/my/param", "my-secret")
	Absolute A      // Absolute version specifier (type-specific, zero value if not specified)
	Shift    int    // Relative shift amount (0 means no shift, positive means go back N versions)
}

// HasShift returns true if a relative shift is specified (Shift > 0).
func (s *Spec[A]) HasShift() bool {
	return s.Shift > 0
}

// ShiftSuffix renders the shift clause ("~N", or "" without a shift), the tail
// of the suffix each service's Suffix rebuilds from a parsed spec.
func (s *Spec[A]) ShiftSuffix() string {
	if !s.HasShift() {
		return ""
	}

	return "~" + strconv.Itoa(s.Shift)
}
