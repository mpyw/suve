package param_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/stage/param"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()

	require.NotNil(t, cmd)
	assert.Equal(t, "param", cmd.Name)
	assert.Contains(t, cmd.Aliases, "ssm")
	assert.Contains(t, cmd.Aliases, "ps")
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.CommandNotFound)
}

func TestCommand_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Extract subcommand names
	subcommandNames := lo.Map(cmd.Commands, func(c *cli.Command, _ int) string {
		return c.Name
	})

	// Verify expected subcommands
	expectedSubcommands := []string{
		"add",
		"edit",
		"delete",
		"status",
		"diff",
		"apply",
		"reset",
		"tag",
		"untag",
		"stash",
	}

	for _, expected := range expectedSubcommands {
		assert.Contains(t, subcommandNames, expected, "should have %s subcommand", expected)
	}
}

func TestCommand_AddSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find add subcommand
	addCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "add"
	})

	require.True(t, found, "should have add subcommand")
	require.NotNil(t, addCmd)
	assert.Equal(t, "add", addCmd.Name)
	assert.NotEmpty(t, addCmd.Usage)
	assert.NotNil(t, addCmd.Action)
}

func TestCommand_EditSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find edit subcommand
	editCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "edit"
	})

	require.True(t, found, "should have edit subcommand")
	require.NotNil(t, editCmd)
	assert.Equal(t, "edit", editCmd.Name)
	assert.NotEmpty(t, editCmd.Usage)
	assert.NotNil(t, editCmd.Action)
}

func TestCommand_DeleteSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find delete subcommand
	deleteCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "delete"
	})

	require.True(t, found, "should have delete subcommand")
	require.NotNil(t, deleteCmd)
	assert.Equal(t, "delete", deleteCmd.Name)
	assert.NotEmpty(t, deleteCmd.Usage)
	assert.NotNil(t, deleteCmd.Action)
}

func TestCommand_StatusSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find status subcommand
	statusCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "status"
	})

	require.True(t, found, "should have status subcommand")
	require.NotNil(t, statusCmd)
	assert.Equal(t, "status", statusCmd.Name)
	assert.NotNil(t, statusCmd.Action)
}

func TestCommand_DiffSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find diff subcommand
	diffCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "diff"
	})

	require.True(t, found, "should have diff subcommand")
	require.NotNil(t, diffCmd)
	assert.Equal(t, "diff", diffCmd.Name)
	assert.NotNil(t, diffCmd.Action)
}

func TestCommand_ApplySubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find apply subcommand
	applyCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "apply"
	})

	require.True(t, found, "should have apply subcommand")
	require.NotNil(t, applyCmd)
	assert.Equal(t, "apply", applyCmd.Name)
	assert.NotNil(t, applyCmd.Action)
}

func TestCommand_ResetSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find reset subcommand
	resetCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "reset"
	})

	require.True(t, found, "should have reset subcommand")
	require.NotNil(t, resetCmd)
	assert.Equal(t, "reset", resetCmd.Name)
	assert.NotNil(t, resetCmd.Action)
}

func TestCommand_TagSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find tag subcommand
	tagCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "tag"
	})

	require.True(t, found, "should have tag subcommand")
	require.NotNil(t, tagCmd)
	assert.Equal(t, "tag", tagCmd.Name)
	assert.NotNil(t, tagCmd.Action)
}

func TestCommand_UntagSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find untag subcommand
	untagCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "untag"
	})

	require.True(t, found, "should have untag subcommand")
	require.NotNil(t, untagCmd)
	assert.Equal(t, "untag", untagCmd.Name)
	assert.NotNil(t, untagCmd.Action)
}

func TestCommand_StashSubcommand(t *testing.T) {
	t.Parallel()

	cmd := param.Command()
	require.NotNil(t, cmd)

	// Find stash subcommand
	stashCmd, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
		return c.Name == "stash"
	})

	require.True(t, found, "should have stash subcommand")
	require.NotNil(t, stashCmd)
	assert.Equal(t, "stash", stashCmd.Name)
	// Stash has nested subcommands (push, pop, apply, show, drop)
	assert.NotEmpty(t, stashCmd.Commands)
}
