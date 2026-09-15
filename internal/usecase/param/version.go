package param

import (
	"strconv"
	"strings"

	"github.com/mpyw/suve/internal/version/awsparamversion"
)

// versionSpecSuffix reconstructs the version-spec suffix (the part after the name)
// from a parsed spec, so that name+suffix re-parses to an equivalent spec. It
// is handed to provider.Reader.Resolve, which re-parses name+suffix internally.
//
// Examples: {Version:3}          -> "#3"
//
//	{Shift:2}            -> "~2"
//	{Version:5, Shift:2} -> "#5~2"
//	{}                   -> ""  (latest)
//
//declscope:package // diff and show rebuild the suffix they pass to Resolve
func versionSpecSuffix(spec *awsparamversion.Spec) string {
	var b strings.Builder

	if spec.Absolute.Version != nil {
		b.WriteString("#")
		b.WriteString(strconv.FormatInt(*spec.Absolute.Version, 10))
	}

	if spec.Shift > 0 {
		b.WriteString("~")
		b.WriteString(strconv.Itoa(spec.Shift))
	}

	return b.String()
}

// parseVersion converts a provider version id ("3") to the int64 version number
// used by the SSM-facing usecase outputs. A non-numeric or empty id yields 0.
//
//declscope:package // create, diff, log, show and update convert a version id with it
func parseVersion(id string) int64 {
	v, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0
	}

	return v
}
