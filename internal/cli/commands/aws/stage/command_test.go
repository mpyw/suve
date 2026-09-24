package stage_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/aws/stage"
	"github.com/mpyw/suve/internal/staging"
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
		"export",
		"import",
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

func TestGlobalConfig(t *testing.T) {
	t.Parallel()

	cfg := stage.GlobalConfig()

	assert.Equal(t, "AWS", cfg.ProviderLabel)
	assert.NotNil(t, cfg.ScopeResolver)
	require.Len(t, cfg.Services, 2)
	assert.Equal(t, staging.ServiceParam, cfg.Services[0].Service)
	assert.Equal(t, staging.ServiceSecret, cfg.Services[1].Service)

	for _, svc := range cfg.Services {
		assert.NotNil(t, svc.ScopeResolver, svc.Service)
		assert.NotNil(t, svc.Factory, svc.Service)
	}

	// Parser factories are carried through and are network-free.
	assert.Equal(t, "SSM Parameter Store", cfg.Services[0].ParserFactory().ServiceName())
	assert.Equal(t, "Secrets Manager", cfg.Services[1].ParserFactory().ServiceName())
}
