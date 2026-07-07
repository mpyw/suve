package commands

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/debug"
)

// appName is the CLI binary name used to build probe apps in these tests. It is
// a constant so the literal is not repeated across each Run invocation.
const appName = "suve"

// runProbe assembles a minimal app wired exactly like the root (debugFlag +
// enableDebug Before) with a probe subcommand that records whether debug is
// enabled on the context its Action receives. It returns that value.
func runProbe(t *testing.T, args []string) bool {
	t.Helper()

	var got bool

	app := &cli.Command{
		Name:   appName,
		Flags:  []cli.Flag{debugFlag()},
		Before: enableDebug,
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

	return got
}

func TestEnableDebug_off(t *testing.T) {
	t.Parallel()

	assert.False(t, runProbe(t, []string{appName, "probe"}))
}

func TestEnableDebug_flagBeforeSubcommand(t *testing.T) {
	t.Parallel()

	assert.True(t, runProbe(t, []string{appName, "--debug", "probe"}))
}

func TestEnableDebug_flagAfterSubcommand(t *testing.T) {
	t.Parallel()

	// The flag is persistent (not Local), so it is accepted in either position.
	assert.True(t, runProbe(t, []string{appName, "probe", "--debug"}))
}

func TestEnableDebug_env(t *testing.T) {
	// Not parallel: mutates process environment via t.Setenv.
	t.Setenv("SUVE_DEBUG", "1")

	assert.True(t, runProbe(t, []string{appName, "probe"}))
}
