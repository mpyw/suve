package agent

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := Command()

	require.NotNil(t, cmd)
	assert.Equal(t, "agent", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.Len(t, cmd.Commands, 2)
}

func TestCommand_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := Command()
	require.NotNil(t, cmd)

	// Extract subcommand names
	subcommandNames := lo.Map(cmd.Commands, func(c *cli.Command, _ int) string {
		return c.Name
	})

	assert.Contains(t, subcommandNames, "start")
	assert.Contains(t, subcommandNames, "stop")
}

func TestStartCommand(t *testing.T) {
	t.Parallel()

	cmd := startCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "start", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.Action)
}

func TestStartCommand_HasExpectedFlags(t *testing.T) {
	t.Parallel()

	cmd := startCommand()
	require.NotNil(t, cmd)

	// Extract flag names
	flagNames := lo.Map(cmd.Flags, func(f cli.Flag, _ int) string {
		names := f.Names()
		if len(names) > 0 {
			return names[0]
		}
		return ""
	})

	assert.Contains(t, flagNames, "account")
	assert.Contains(t, flagNames, "region")
}

func TestStopCommand(t *testing.T) {
	t.Parallel()

	cmd := stopCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "stop", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.Action)
}

func TestStopCommand_NoFlags(t *testing.T) {
	t.Parallel()

	cmd := stopCommand()
	require.NotNil(t, cmd)

	// Stop command should have no flags
	assert.Empty(t, cmd.Flags)
}
