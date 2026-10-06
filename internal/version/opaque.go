package version

// OpaqueAbsolute is the absolute specifier of an opaque grammar: an optional
// version id (#VERSION) or, where the grammar allows labels, a staging label
// (:LABEL). At most one of them is set.
type OpaqueAbsolute struct {
	ID    *string // Version ID (#VERSION)
	Label *string // Staging label (:LABEL)
}

// IsSet reports whether an explicit #VERSION or :LABEL was given.
func (a OpaqueAbsolute) IsSet() bool {
	return a.ID != nil || a.Label != nil
}

// OpaqueSpec is a spec parsed with an OpaqueGrammar.
//
// Grammar: <name>[#<id> | :<label>]<shift>*
//   - #<id>    optional version ID (0 or 1, mutually exclusive with :LABEL)
//   - :<label> optional staging label, only where the grammar has labels
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// Examples: my-secret, my-secret#abc123, my-secret:AWSCURRENT, my-secret~1.
type OpaqueSpec = Spec[OpaqueAbsolute]

// OpaqueGrammar parses opaque version ids (#VERSION), optional staging labels
// (:LABEL), and ~SHIFT, the grammar of services whose version ids carry no
// order.
type OpaqueGrammar struct {
	// isIDChar reports whether c is valid within a version id.
	//
	//declscope:shared // products.go defines each product's grammar with it
	isIDChar func(c byte) bool
	// invalidIDError is returned when # is not followed by a version id.
	//
	//declscope:shared // products.go defines each product's grammar with it
	invalidIDError error
	// labels accepts a :LABEL specifier.
	//
	//declscope:shared // products.go defines each product's grammar with it
	labels bool
	// labelError is returned when ':' does not start a valid label. Without
	// labels, every ':' is rejected with it.
	//
	//declscope:shared // products.go defines each product's grammar with it
	labelError error
}

// Parse parses a version specification string.
//
// Shift syntax (Git-like, repeatable):
//   - ~     go back 1 version
//   - ~N    go back N versions
//   - ~~    go back 2 versions (same as ~1~1)
//   - ~1~2  cumulative: go back 3 versions
func (g OpaqueGrammar) Parse(input string) (*OpaqueSpec, error) {
	parsers := []specifierParser[OpaqueAbsolute]{
		{
			prefixChar:        '#',
			isChar:            g.isIDChar,
			invalidValueError: g.invalidIDError,
			duplicated:        OpaqueAbsolute.IsSet,
			apply: func(value string, abs OpaqueAbsolute) (OpaqueAbsolute, error) {
				abs.ID = new(value)

				return abs, nil
			},
		},
	}

	if g.labels {
		parsers = append(parsers, specifierParser[OpaqueAbsolute]{
			prefixChar:        ':',
			isChar:            isOpaqueLabelChar,
			invalidValueError: g.labelError,
			duplicated:        OpaqueAbsolute.IsSet,
			apply: func(value string, abs OpaqueAbsolute) (OpaqueAbsolute, error) {
				abs.Label = new(value)

				return abs, nil
			},
		})
	} else if g.labelError != nil {
		parsers = append(parsers, rejectingLabelParser[OpaqueAbsolute](g.labelError))
	}

	return parseSpec(input, absoluteParser[OpaqueAbsolute]{
		parsers: parsers,
		zero:    func() OpaqueAbsolute { return OpaqueAbsolute{} },
	})
}

// Suffix reconstructs the version-spec suffix (the part after the name) from a
// parsed spec, so that spec.Name+Suffix(spec) re-parses to an equivalent spec.
// It is what callers hand to provider.Reader.Resolve alongside the name.
//
// Examples: {ID:"abc"} -> "#abc"; {Label:"AWSCURRENT", Shift:1} ->
// ":AWSCURRENT~1"; {Shift:2} -> "~2"; {} -> "" (current).
func (OpaqueGrammar) Suffix(spec *OpaqueSpec) string {
	var abs string

	switch {
	case spec.Absolute.ID != nil:
		abs = "#" + *spec.Absolute.ID
	case spec.Absolute.Label != nil:
		abs = ":" + *spec.Absolute.Label
	}

	return abs + spec.ShiftSuffix()
}

// isOpaqueLabelChar reports whether c is valid within a staging label.
func isOpaqueLabelChar(c byte) bool {
	return isLetterChar(c) || isDigitChar(c) || c == '-' || c == '_'
}

// Split parses input and returns its name plus the rebuilt suffix (Suffix), the
// pair the use cases take.
func (g OpaqueGrammar) Split(input string) (name, suffix string, err error) {
	spec, err := g.Parse(input)
	if err != nil {
		return "", "", err
	}

	return spec.Name, g.Suffix(spec), nil
}
