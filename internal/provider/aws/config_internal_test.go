// In-package tests of config.go's config loading.
//declscope:namespace config

package aws

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/debug"
)

// setConfigTestEnv points the SDK at static test credentials so LoadConfig resolves
// offline (LoadDefaultConfig makes no network calls, and Retrieve resolves from
// the environment).
func setConfigTestEnv(t *testing.T) {
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
//nolint:paralleltest // subtests use t.Setenv (via setConfigTestEnv), so they cannot run in parallel
func TestLoadConfig_debug(t *testing.T) {
	t.Run("without debug", func(t *testing.T) {
		setConfigTestEnv(t)

		cfg, err := LoadConfig(context.Background())
		require.NoError(t, err)
		// No client log mode is enabled unless debug is requested.
		assert.Zero(t, cfg.ClientLogMode)
	})

	t.Run("with debug", func(t *testing.T) {
		setConfigTestEnv(t)

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
		setConfigTestEnv(t)

		var buf bytes.Buffer

		ctx := debug.With(context.Background(), debug.Config{Enabled: true, Writer: &buf, NoRedaction: true})

		cfg, err := LoadConfig(ctx)
		require.NoError(t, err)
		// --no-redaction switches to the WithBody modes so payloads are logged.
		assert.True(t, cfg.ClientLogMode.IsRequestWithBody())
		assert.True(t, cfg.ClientLogMode.IsResponseWithBody())
	})
}
