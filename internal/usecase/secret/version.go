package secret

import (
	"strconv"
	"strings"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/version/secretversion"
)

// specSuffix reconstructs the version-spec suffix (the part after the name)
// from a parsed spec, so that name+suffix re-parses to an equivalent spec. It
// is handed to provider.Reader.Resolve, which re-parses name+suffix internally.
//
// Examples: {ID:"abc"}          -> "#abc"
//
//	{Label:"AWSCURRENT"}         -> ":AWSCURRENT"
//	{Shift:2}                    -> "~2"
//	{Label:"AWSCURRENT", Shift:1}-> ":AWSCURRENT~1"
//	{}                           -> ""  (current/latest)
func specSuffix(spec *secretversion.Spec) string {
	var b strings.Builder

	switch {
	case spec.Absolute.ID != nil:
		b.WriteString("#")
		b.WriteString(*spec.Absolute.ID)
	case spec.Absolute.Label != nil:
		b.WriteString(":")
		b.WriteString(*spec.Absolute.Label)
	}

	if spec.Shift > 0 {
		b.WriteString("~")
		b.WriteString(strconv.Itoa(spec.Shift))
	}

	return b.String()
}

// extraValue returns the value of the display-only Extra field with the given
// label (e.g. "ARN"), or "" when the entry has no such field. It is how the
// secret usecases surface provider metadata (like the Secrets Manager ARN) that
// the neutral domain.Entry keeps in its Extra bag.
func extraValue(entry *domain.Entry, label string) string {
	for _, f := range entry.Extra {
		if f.Label == label {
			return f.Value
		}
	}

	return ""
}

// stages converts a single provider-neutral version Label into the []string
// staging-label slice the secret outputs expose. An empty label yields nil so
// that callers omit the field entirely (matching the pre-migration behavior).
func stages(label string) []string {
	if label == "" {
		return nil
	}

	return []string{label}
}
