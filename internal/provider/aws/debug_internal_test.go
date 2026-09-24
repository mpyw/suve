// In-package tests of debug.go's SDK debug logger.
//declscope:namespace debug

package aws

import (
	"bytes"
	"testing"

	"github.com/aws/smithy-go/logging"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/debug"
)

func TestDebugLogger_prefix(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := debugLogger{cfg: debug.Config{Enabled: true, Writer: &buf}}
	l.Logf(logging.Debug, "Request %s", "dump")

	// smithy output is routed through debug.Logf, so it carries the unified
	// prefix and the classification.
	assert.Regexp(t, `^\[suve debug \d{2}:\d{2}:\d{2}\.\d{3}\] aws sdk DEBUG: Request dump\n$`, buf.String())
}

func TestDebugLogger_redactsCredentials(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := debugLogger{cfg: debug.Config{Enabled: true, Writer: &buf}}
	// A realistic post-signing request dump: LogRequest runs after SigV4, so
	// these headers carry live, replayable credentials without redaction. The
	// dump also carries an invented header not on the allowlist, to prove the
	// allowlist fails closed (redacts what it does not recognize).
	l.Logf(logging.Debug, "Request\n"+
		"POST / HTTP/1.1\n"+
		"Host: ssm.us-east-1.amazonaws.com\n"+
		"Authorization: AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20260707/us-east-1/ssm/aws4_request, Signature=deadbeef\n"+
		"X-Amz-Security-Token: SESSIONTOKENVALUE123\n"+
		"X-Amz-Future-Auth: SUPERSECRETFUTURESCHEME\n"+
		"X-Amz-Target: AmazonSSM.DescribeParameters\n"+
		"X-Amz-Date: 20260707T000000Z\n")

	out := buf.String()
	// Known credential headers are redacted.
	assert.NotContains(t, out, "AKIDEXAMPLE")
	assert.NotContains(t, out, "deadbeef")
	assert.NotContains(t, out, "SESSIONTOKENVALUE123")
	assert.Contains(t, out, "Authorization: REDACTED")
	assert.Contains(t, out, "X-Amz-Security-Token: REDACTED")
	// Fail-closed: an unknown header is redacted too, not leaked.
	assert.NotContains(t, out, "SUPERSECRETFUTURESCHEME")
	assert.Contains(t, out, "X-Amz-Future-Auth: REDACTED")
	// Allowlisted diagnostic headers (and non-header lines) survive untouched.
	assert.Contains(t, out, "Host: ssm.us-east-1.amazonaws.com")
	assert.Contains(t, out, "X-Amz-Target: AmazonSSM.DescribeParameters")
	assert.Contains(t, out, "X-Amz-Date: 20260707T000000Z")
	assert.Contains(t, out, "POST / HTTP/1.1")
}

func TestDebugLogger_noRedactionKeepsCredentials(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := debugLogger{cfg: debug.Config{Enabled: true, Writer: &buf, NoRedaction: true}}
	l.Logf(logging.Debug, "Request\n"+
		"POST / HTTP/1.1\n"+
		"Authorization: AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE, Signature=deadbeef\n"+
		"X-Amz-Security-Token: SESSIONTOKENVALUE123\n")

	out := buf.String()
	// With --no-redaction the dump is passed through verbatim: nothing is masked.
	assert.NotContains(t, out, "REDACTED")
	assert.Contains(t, out, "AKIDEXAMPLE")
	assert.Contains(t, out, "deadbeef")
	assert.Contains(t, out, "SESSIONTOKENVALUE123")
}
