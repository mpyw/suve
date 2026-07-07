package infra

import (
	"bytes"
	"context"
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
	// Neutralize any profile leaking in from the developer's shell so the
	// effective-config summary is deterministic.
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_DEFAULT_PROFILE", "")
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

		// The effective-configuration summary is logged immediately, before any
		// service call, so the user sees region/profile/credentials up front.
		assert.Contains(t, buf.String(), `aws: region="us-east-1" profile="default" credentials-source=EnvConfigCredentials`)
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
