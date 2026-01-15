package stage_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/stage"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := stage.Command()

	require.NotNil(t, cmd)
	assert.Equal(t, "stage", cmd.Name)
	assert.Contains(t, cmd.Aliases, "stg")
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.CommandNotFound)
}

func TestCommand_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := stage.Command()
	require.NotNil(t, cmd)

	// Extract subcommand names
	subcommandNames := lo.Map(cmd.Commands, func(c *cli.Command, _ int) string {
		return c.Name
	})

	// Verify expected subcommands
	expectedSubcommands := []string{
		"param",
		"secret",
		"status",
		"diff",
		"apply",
		"reset",
		"stash",
		"agent",
	}

	for _, expected := range expectedSubcommands {
		assert.Contains(t, subcommandNames, expected, "should have %s subcommand", expected)
	}
}

func TestCommand_ParamSubcommand(t *testing.T) {
	t.Parallel()

	cmd := stage.Command()
	require.NotNil(t, cmd)

	// Find param subcommand
	paramCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "param"
	})

	require.True(t, found, "should have param subcommand")
	require.NotNil(t, paramCmd)
	assert.Equal(t, "param", paramCmd.Name)
	assert.Contains(t, paramCmd.Aliases, "ssm")
	assert.Contains(t, paramCmd.Aliases, "ps")
}

func TestCommand_SecretSubcommand(t *testing.T) {
	t.Parallel()

	cmd := stage.Command()
	require.NotNil(t, cmd)

	// Find secret subcommand
	secretCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "secret"
	})

	require.True(t, found, "should have secret subcommand")
	require.NotNil(t, secretCmd)
	assert.Equal(t, "secret", secretCmd.Name)
	assert.Contains(t, secretCmd.Aliases, "sm")
}

func TestCommand_AgentSubcommand(t *testing.T) {
	t.Parallel()

	cmd := stage.Command()
	require.NotNil(t, cmd)

	// Find agent subcommand
	agentCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "agent"
	})

	require.True(t, found, "should have agent subcommand")
	require.NotNil(t, agentCmd)
	assert.Equal(t, "agent", agentCmd.Name)
	assert.NotEmpty(t, agentCmd.Commands) // Has start and stop
}
