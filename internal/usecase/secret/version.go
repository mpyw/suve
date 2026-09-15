package secret

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/version/awssecretversion"
)

// versionSpecSuffix reconstructs the version-spec suffix (the part after the name)
// from a parsed spec, so that name+suffix re-parses to an equivalent spec. It
// is handed to provider.Reader.Resolve, which re-parses name+suffix internally.
//
// Examples: {ID:"abc"}          -> "#abc"
//
//	{Label:"AWSCURRENT"}         -> ":AWSCURRENT"
//	{Shift:2}                    -> "~2"
//	{Label:"AWSCURRENT", Shift:1}-> ":AWSCURRENT~1"
//	{}                           -> ""  (current/latest)
//
//declscope:package // diff and show rebuild the suffix they pass to Resolve
func versionSpecSuffix(spec *awssecretversion.Spec) string {
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

// versionExtraValue returns the value of the display-only Extra field with the given
// label (e.g. "ARN"), or "" when the entry has no such field. It is how the
// secret usecases surface provider metadata (like the Secrets Manager ARN) that
// the neutral domain.Entry keeps in its Extra bag.
//
//declscope:package // show and log read the Extra fields with it
func versionExtraValue(entry *domain.Entry, label string) string {
	for _, f := range entry.Extra {
		if f.Label == label {
			return f.Value
		}
	}

	return ""
}

// versionStages returns a deterministically sorted copy of a version's AWS Secrets
// Manager staging labels for stable output. An empty slice yields nil so that
// callers omit the field entirely (matching the pre-migration behavior).
//
//declscope:package // show and log format the staging label with it
func versionStages(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}

	out := slices.Clone(labels)
	slices.Sort(out)

	return out
}
