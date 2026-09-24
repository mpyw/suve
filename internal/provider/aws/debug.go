package aws

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/smithy-go/logging"

	"github.com/mpyw/suve/internal/debug"
)

// debugSafeHeaders is the allowlist of HTTP header names whose values are safe to
// show in a debug dump. Any header NOT listed here has its value redacted, so
// the dump fails CLOSED: a new or unexpected credential-bearing header (a future
// auth scheme, a custom proxy header) is hidden by default rather than leaked.
// This mirrors azcore's allowlist model on the Azure side, and is the reason a
// denylist was rejected — the SDK's LogRequest dump runs AFTER SigV4 signing, so
// it carries the live Authorization header and, for temporary credentials, the
// session token. Names are lowercased for case-insensitive matching; the set is
// scoped to headers useful for diagnosing empty/unexpected output (#306): the
// target region (Host), the operation (X-Amz-Target), timing, and request IDs.
//
//nolint:gochecknoglobals // effectively const lookup table
var debugSafeHeaders = map[string]struct{}{
	"host":                  {},
	"user-agent":            {},
	"content-type":          {},
	"content-length":        {},
	"accept-encoding":       {},
	"date":                  {},
	"connection":            {},
	"server":                {},
	"x-amz-target":          {},
	"x-amz-date":            {},
	"amz-sdk-invocation-id": {},
	"amz-sdk-request":       {},
	"x-amzn-requestid":      {},
	"x-amz-request-id":      {},
	"x-amz-id-2":            {},
	"x-amzn-trace-id":       {},
	"x-amz-cf-id":           {},
}

// debugHeaderLineRegex splits one line of an HTTP request/response dump into header
// name and value. Non-header lines (the request/status line, the blank
// separator) do not match and pass through untouched.
var debugHeaderLineRegex = regexp.MustCompile(`^([A-Za-z0-9-]+):[ \t]*(.*)$`)

// redactDebugDump rewrites an HTTP dump so only allowlisted header values survive;
// every other header value becomes REDACTED (the name is kept, so the reader
// still sees the header exists). Fail-closed by design — see debugSafeHeaders.
func redactDebugDump(dump string) string {
	lines := strings.Split(dump, "\n")
	for i, line := range lines {
		m := debugHeaderLineRegex.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue // request/status line, blank separator, etc.
		}

		if _, ok := debugSafeHeaders[strings.ToLower(m[1])]; ok {
			continue
		}

		lines[i] = m[1] + ": REDACTED"
	}

	return strings.Join(lines, "\n")
}

// debugLogger adapts the debug writer to smithy's logging.Logger so SDK
// request/response dumps share the unified "[suve debug ...]" line prefix with
// every other provider (multi-line HTTP dumps are prefixed on their first line
// only). Header values are allowlisted before anything is written, mirroring
// azcore's log-policy behavior on the Azure side.
//
//declscope:package // config.go installs it as the SDK logger
type debugLogger struct {
	cfg debug.Config
}

// Logf implements smithy logging.Logger. Header/body redaction is applied
// unless --no-redaction is active, in which case the dump is passed through
// verbatim (secret values and credentials included).
func (l debugLogger) Logf(classification logging.Classification, format string, v ...any) {
	dump := fmt.Sprintf(format, v...)
	if !l.cfg.NoRedaction {
		dump = redactDebugDump(dump)
	}

	l.cfg.Logf("aws sdk %s: %s\n", classification, dump)
}
