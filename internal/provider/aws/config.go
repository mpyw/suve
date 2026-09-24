//declscope:namespace aws

package aws

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/logging"
	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/debug"
)

// safeHeaders is the allowlist of HTTP header names whose values are safe to
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
var safeHeaders = map[string]struct{}{
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

// headerLineRegex splits one line of an HTTP request/response dump into header
// name and value. Non-header lines (the request/status line, the blank
// separator) do not match and pass through untouched.
var headerLineRegex = regexp.MustCompile(`^([A-Za-z0-9-]+):[ \t]*(.*)$`)

// redactDump rewrites an HTTP dump so only allowlisted header values survive;
// every other header value becomes REDACTED (the name is kept, so the reader
// still sees the header exists). Fail-closed by design — see safeHeaders.
func redactDump(dump string) string {
	lines := strings.Split(dump, "\n")
	for i, line := range lines {
		m := headerLineRegex.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue // request/status line, blank separator, etc.
		}

		if _, ok := safeHeaders[strings.ToLower(m[1])]; ok {
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
type debugLogger struct {
	cfg debug.Config
}

// Logf implements smithy logging.Logger. Header/body redaction is applied
// unless --no-redaction is active, in which case the dump is passed through
// verbatim (secret values and credentials included).
func (l debugLogger) Logf(classification logging.Classification, format string, v ...any) {
	dump := fmt.Sprintf(format, v...)
	if !l.cfg.NoRedaction {
		dump = redactDump(dump)
	}

	l.cfg.Logf("aws sdk %s: %s\n", classification, dump)
}

// LoadConfig loads the default AWS configuration. When debug is enabled on the
// context it turns on SDK request/response/retry logging plus config resolution
// warnings, and logs a one-line summary of the effective region, profile, and
// credentials source — the facts a user needs first when a command unexpectedly
// returns nothing (see #306). By default the bodyless LogRequest/LogResponse
// modes are used (metadata only, no secret values); --no-redaction switches to
// the WithBody modes so full request/response payloads are logged too.
func LoadConfig(ctx context.Context) (aws.Config, error) {
	d := debug.From(ctx)
	if !d.Enabled {
		return config.LoadDefaultConfig(ctx)
	}

	logMode := aws.LogRequest | aws.LogResponse | aws.LogRetries
	if d.NoRedaction {
		logMode = aws.LogRequestWithBody | aws.LogResponseWithBody | aws.LogRetries
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithClientLogMode(logMode),
		config.WithLogger(debugLogger{cfg: d}),
		config.WithLogConfigurationWarnings(true),
	)
	if err != nil {
		return cfg, err
	}

	logEffectiveConfig(ctx, d, cfg)

	return cfg, nil
}

// logEffectiveConfig emits the one-line effective-configuration summary under
// debug. Resolving the credentials source calls Retrieve, which is cached by
// the SDK's CredentialsCache, so the first API call would perform the same work
// anyway; a resolution failure is logged (with the reason) instead of being
// returned, so the command still fails at the API call exactly as it would
// without --debug.
func logEffectiveConfig(ctx context.Context, d debug.Config, cfg aws.Config) {
	profile := lo.CoalesceOrEmpty(os.Getenv("AWS_PROFILE"), os.Getenv("AWS_DEFAULT_PROFILE"), "default")

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		d.Logf("aws: region=%q profile=%q credentials resolution failed: %v\n", cfg.Region, profile, err)

		return
	}

	d.Logf("aws: region=%q profile=%q credentials-source=%s\n", cfg.Region, profile, creds.Source)
}
