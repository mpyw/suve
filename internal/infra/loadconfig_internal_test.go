package infra

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/aws/smithy-go/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/debug"
)

// setAWSTestEnv points the SDK at static test credentials so LoadConfig resolves
// offline (LoadDefaultConfig makes no network calls, and Retrieve resolves from
// the environment).
func setAWSTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	// Neutralize any profile or shared-config override leaking in from the
	// developer's shell so the effective-config summary is deterministic.
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", os.DevNull)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", os.DevNull)
}

// TestLoadConfig_debug exercises both branches of LoadConfig.
//
//nolint:paralleltest // subtests use t.Setenv (via setAWSTestEnv), so they cannot run in parallel
func TestLoadConfig_debug(t *testing.T) {
	t.Run("without debug", func(t *testing.T) {
		setAWSTestEnv(t)

		cfg, err := LoadConfig(context.Background())
		require.NoError(t, err)
		// No client log mode is enabled unless debug is requested.
		assert.Zero(t, cfg.ClientLogMode)
	})

	t.Run("with debug", func(t *testing.T) {
		setAWSTestEnv(t)

		var buf bytes.Buffer

		ctx := debug.With(context.Background(), debug.Config{Enabled: true, Writer: &buf})

		cfg, err := LoadConfig(ctx)
		require.NoError(t, err)
		assert.NotNil(t, cfg.Logger)
		assert.NotZero(t, cfg.ClientLogMode)
		// The default (redacted) mode logs metadata only — bodies are not dumped.
		assert.False(t, cfg.ClientLogMode.IsRequestWithBody())
		assert.False(t, cfg.ClientLogMode.IsResponseWithBody())

		// The effective-configuration summary is logged immediately, before any
		// service call, so the user sees region/profile/credentials up front.
		assert.Contains(t, buf.String(), `aws: region="us-east-1" profile="default" credentials-source=EnvConfigCredentials`)
	})

	t.Run("with debug and no redaction", func(t *testing.T) {
		setAWSTestEnv(t)

		var buf bytes.Buffer

		ctx := debug.With(context.Background(), debug.Config{Enabled: true, Writer: &buf, NoRedaction: true})

		cfg, err := LoadConfig(ctx)
		require.NoError(t, err)
		// --no-redaction switches to the WithBody modes so payloads are logged.
		assert.True(t, cfg.ClientLogMode.IsRequestWithBody())
		assert.True(t, cfg.ClientLogMode.IsResponseWithBody())
	})
}

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
