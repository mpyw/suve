package version

import "strings"

// BareAbsolute is empty: a bare grammar has no absolute version specifier. It
// exists only to satisfy the shared Spec type parameter.
type BareAbsolute struct{}

// BareSpec is a spec parsed with a BareGrammar.
//
// Grammar: <name>  (the whole argument is the name; nothing is split off)
//
// Examples: my-key, /app/config, Logging:LogLevel:Default, weird#key, a~b.
type BareSpec = Spec[BareAbsolute]

// BareGrammar takes the whole argument as the name, for an unversioned service
// whose names may contain '#', ':' and '~'.
type BareGrammar struct{}

// Parse takes the entire (whitespace-trimmed) input as the name; no version
// specifier is split off, so ':' / '#' / '~' are preserved verbatim. Empty
// input yields ErrEmptySpec.
func (BareGrammar) Parse(input string) (*BareSpec, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return nil, ErrEmptySpec
	}

	return &BareSpec{Name: name}, nil
}

// Split parses input and returns the whole argument as the name. The suffix is
// always empty: an unversioned service has no version to select.
func (g BareGrammar) Split(input string) (name, suffix string, err error) {
	spec, err := g.Parse(input)
	if err != nil {
		return "", "", err
	}

	return spec.Name, "", nil
}
