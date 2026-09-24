package version

import (
	"errors"
	"strconv"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/version/internal"
)

// ErrInvalidNumericVersion is returned when # is not followed by a version number.
var ErrInvalidNumericVersion = errors.New("# must be followed by a version number")

// NumericAbsolute is the absolute specifier of a numeric grammar: an optional
// integer version (#VERSION).
type NumericAbsolute struct {
	Version *int64 // Explicit version number (#VERSION)
}

// IsSet reports whether an explicit #VERSION was given.
func (a NumericAbsolute) IsSet() bool {
	return a.Version != nil
}

// NumericSpec is a spec parsed with a NumericGrammar.
//
// Grammar: <name>[#<N>]<shift>*
//   - #<N>     optional version number (0 or 1)
//   - <shift>  ~ or ~<N>, repeatable (0 or more, cumulative)
//
// Examples: /my/param, /my/param#3, /my/param~1, /my/param#5~2, /my/param~~.
type NumericSpec = Spec[NumericAbsolute]

// NumericGrammar parses integer versions (#VERSION) plus ~SHIFT, the grammar
// of services whose versions count up from 1.
type NumericGrammar struct {
	// LabelError, when set, rejects every ':' with this error, for a service
	// whose names never contain ':' and that has no staging labels. When nil,
	// ':' is part of the name.
	LabelError error
}

// Parse parses a version specification string.
//
// Shift syntax (Git-like, repeatable):
//   - ~      go back 1 version
//   - ~N     go back N versions (e.g., ~2)
//   - ~~     go back 2 versions (same as ~1~1)
//   - ~1~2   cumulative: go back 3 versions
func (g NumericGrammar) Parse(input string) (*NumericSpec, error) {
	parsers := []SpecifierParser[NumericAbsolute]{
		{
			PrefixChar: '#',
			IsChar:     internal.IsDigit,
			Error:      ErrInvalidNumericVersion,
			Duplicated: NumericAbsolute.IsSet,
			Apply: func(value string, abs NumericAbsolute) (NumericAbsolute, error) {
				v, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return abs, err
				}

				abs.Version = lo.ToPtr(v)

				return abs, nil
			},
		},
	}
	if g.LabelError != nil {
		parsers = append(parsers, rejectingLabelParser[NumericAbsolute](g.LabelError))
	}

	return Parse(input, AbsoluteParser[NumericAbsolute]{
		Parsers: parsers,
		Zero:    func() NumericAbsolute { return NumericAbsolute{} },
	})
}

// Suffix reconstructs the version-spec suffix (the part after the name) from a
// parsed spec, so that spec.Name+Suffix(spec) re-parses to an equivalent spec.
// It is what callers hand to provider.Reader.Resolve alongside the name.
//
// Examples: {Version:3} -> "#3"; {Shift:2} -> "~2"; {Version:5, Shift:2} ->
// "#5~2"; {} -> "" (latest).
func (NumericGrammar) Suffix(spec *NumericSpec) string {
	var abs string
	if spec.Absolute.Version != nil {
		abs = "#" + strconv.FormatInt(*spec.Absolute.Version, 10)
	}

	return abs + spec.ShiftSuffix()
}

// Split parses input and returns its name plus the rebuilt suffix (Suffix), the
// pair the use cases take.
func (g NumericGrammar) Split(input string) (name, suffix string, err error) {
	spec, err := g.Parse(input)
	if err != nil {
		return "", "", err
	}

	return spec.Name, g.Suffix(spec), nil
}
