package commands

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/debug"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/detect"
)

// appName is the CLI binary name used to build probe apps in these tests. It is
// a constant so the literal is not repeated across each Run invocation.
const appName = "suve"

// runProbe assembles a minimal app wired exactly like the root (debugFlag +
// enableDebug Before) with a probe subcommand that records whether debug is
// enabled on the context its Action receives. It returns that value plus
// whatever the Before hook wrote to the app's ErrWriter.
func runProbe(t *testing.T, det detect.Result, args []string) (bool, string) {
	t.Helper()

	var (
		got    bool
		stderr bytes.Buffer
	)

	app := &cli.Command{
		Name:      appName,
		Version:   "test",
		ErrWriter: &stderr,
		Flags:     []cli.Flag{debugFlag()},
		Before:    enableDebug(det),
		Commands: []*cli.Command{
			{
				Name: "probe",
				Action: func(ctx context.Context, _ *cli.Command) error {
					got = debug.From(ctx).Enabled

					return nil
				},
			},
		},
	}

	require.NoError(t, app.Run(context.Background(), args))

	return got, stderr.String()
}

func TestEnableDebug_off(t *testing.T) {
	// Not parallel: neutralizes ambient SUVE_DEBUG from the developer's shell
	// via t.Setenv so the "off" assertion is hermetic.
	t.Setenv("SUVE_DEBUG", "")

	enabled, stderr := runProbe(t, detect.Result{}, []string{appName, "probe"})
	assert.False(t, enabled)
	assert.Empty(t, stderr)
}

func TestEnableDebug_flagBeforeSubcommand(t *testing.T) {
	t.Parallel()

	enabled, stderr := runProbe(t, detect.Result{}, []string{appName, "--debug", "probe"})
	assert.True(t, enabled)
	// The Before hook logs a one-shot summary of pre-API decisions.
	assert.Contains(t, stderr, "cli: suve version=test")
	assert.Contains(t, stderr, "flat aliases: param=(none) secret=(none) stage=(none)")
}

func TestEnableDebug_flagAfterSubcommand(t *testing.T) {
	t.Parallel()

	// The flag is persistent (not Local), so it is accepted in either position.
	enabled, _ := runProbe(t, detect.Result{}, []string{appName, "probe", "--debug"})
	assert.True(t, enabled)
}

func TestEnableDebug_env(t *testing.T) {
	// Not parallel: mutates process environment via t.Setenv.
	t.Setenv("SUVE_DEBUG", "1")

	enabled, _ := runProbe(t, detect.Result{}, []string{appName, "probe"})
	assert.True(t, enabled)
}

// TestEnableDebug_envLenient covers the lenient SUVE_DEBUG parsing: bool-ish
// values are honored, any other non-empty value enables debug, and no value
// ever makes the command fail (the strict flag-Source parsing it replaces
// hard-failed every invocation on SUVE_DEBUG=yes). Not parallel: subtests
// mutate the process environment via t.Setenv.
func TestEnableDebug_envLenient(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "yes", want: true},
		{value: "on", want: true},
		{value: "true", want: true},
		{value: "0", want: false},
		{value: "false", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("SUVE_DEBUG", tt.value)

			enabled, _ := runProbe(t, detect.Result{}, []string{appName, "probe"})
			assert.Equal(t, tt.want, enabled)
		})
	}
}

func TestEnableDebug_aliasSummary(t *testing.T) {
	t.Parallel()

	det := detect.Result{
		Param:          provider.ProviderAWS,
		Secret:         provider.ProviderGoogleCloud,
		AWSViaFallback: true,
	}

	_, stderr := runProbe(t, det, []string{appName, "--debug", "probe"})
	assert.Contains(t, stderr, "flat aliases: param=aws secret=gcloud stage=(none) (AWS via ~/.aws/credentials fallback)")
}
