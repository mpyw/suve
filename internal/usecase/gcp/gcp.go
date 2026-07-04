// Package gcp provides use cases for Google Cloud Secret Manager operations.
//
// The use cases are written against the provider-neutral Reader/Writer/Store
// interfaces, exactly like the AWS secret use cases, but expose Google
// Cloud-shaped outputs: integer versions, no ARN, no staging labels. The
// per-version state (enabled/disabled/destroyed) is surfaced where the provider
// supplies it via the neutral Version.Label.
package gcp

import (
	"errors"
	"strconv"
	"strings"

	"github.com/mpyw/suve/internal/version/gcpversion"
)

// ErrSecretNotFound is returned by the update use case when the target secret
// does not exist.
var ErrSecretNotFound = errors.New("secret not found")

// specSuffix reconstructs the version-spec suffix (the part after the name)
// from a parsed spec, so that name+suffix re-parses to an equivalent spec. It
// is handed to provider.Reader.Resolve, which re-parses name+suffix internally.
//
// Examples: {Version:3} -> "#3"; {Shift:2} -> "~2"; {} -> "" (latest).
func specSuffix(spec *gcpversion.Spec) string {
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
