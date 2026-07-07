package infra

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/debug"
)

// setAWSTestEnv points the SDK at static test credentials so LoadConfig resolves
// offline (LoadDefaultConfig makes no network calls).
func setAWSTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
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

		ctx := debug.With(context.Background(), debug.Config{Enabled: true, Writer: &bytes.Buffer{}})

		cfg, err := LoadConfig(ctx)
		require.NoError(t, err)
		assert.NotNil(t, cfg.Logger)
		assert.NotZero(t, cfg.ClientLogMode)
	})
}
