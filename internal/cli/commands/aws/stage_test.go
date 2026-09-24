package aws_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands/aws"
	"github.com/mpyw/suve/internal/staging"
)

func TestStageCommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "stage", cmd.Name)
	assert.Contains(t, cmd.Aliases, "stg")
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.CommandNotFound)
}

func TestStageCommand_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := aws.StageCommand()
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

func TestStageCommand_ParamSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageCommand()
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

func TestStageCommand_SecretSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageCommand()
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

func TestStageGlobalConfig(t *testing.T) {
	t.Parallel()

	cfg := aws.StageGlobalConfig()

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

// TestStageHelpWording verifies the stage help text names AWS and uses the explicit
// "suve aws stage" command paths, never another provider's wording or the flat
// alias paths.
func TestStageHelpWording(t *testing.T) {
	t.Parallel()

	for path, text := range stageHelpTexts(aws.StageCommand(), "suve aws stage") {
		for _, m := range regexp.MustCompile(`suve (?:aws|gcloud|azure|param|secret|stage|stg)\b`).FindAllString(text, -1) {
			assert.Equal(t, "suve aws", m, "%s: %q", path, text)
		}
	}
}

// stageHelpTexts returns every help string (usage, description, flag usages) in the
// command tree, keyed by the command path.
func stageHelpTexts(cmd *cli.Command, path string) map[string]string {
	texts := map[string]string{}

	var walk func(c *cli.Command, p string)
	walk = func(c *cli.Command, p string) {
		parts := []string{c.Usage, c.Description}
		for _, f := range c.Flags {
			if u, ok := f.(interface{ GetUsage() string }); ok {
				parts = append(parts, u.GetUsage())
			}
		}

		texts[p] = strings.Join(parts, "\n")

		for _, sub := range c.Commands {
			walk(sub, p+" "+sub.Name)
		}
	}
	walk(cmd, path)

	return texts
}

func TestStageParamCommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "param", cmd.Name)
	assert.Contains(t, cmd.Aliases, "ssm")
	assert.Contains(t, cmd.Aliases, "ps")
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.CommandNotFound)
}

func TestStageParamCommand_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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
		"export",
		"import",
	}

	for _, expected := range expectedSubcommands {
		assert.Contains(t, subcommandNames, expected, "should have %s subcommand", expected)
	}
}

func TestStageParamCommand_AddSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_EditSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_AddEditHaveTypeFlags(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
	require.NotNil(t, cmd)

	for _, name := range []string{"add", "edit"} {
		leaf, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
			return c.Name == name
		})
		require.True(t, found, "should have %s subcommand", name)

		flagNames := lo.FlatMap(leaf.Flags, func(f cli.Flag, _ int) []string {
			return f.Names()
		})
		assert.Contains(t, flagNames, "type", "%s should accept --type", name)
		assert.Contains(t, flagNames, "secure", "%s should accept --secure", name)
	}
}

func TestStageParamCommand_DeleteSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_StatusSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_DiffSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_ApplySubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_ResetSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_TagSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_UntagSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
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

func TestStageParamCommand_ExportImportSubcommands(t *testing.T) {
	t.Parallel()

	cmd := aws.StageParamCommand()
	require.NotNil(t, cmd)

	for _, name := range []string{"export", "import"} {
		leaf, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
			return c.Name == name
		})

		require.True(t, found, "should have %s subcommand", name)
		require.NotNil(t, leaf)
		assert.Equal(t, name, leaf.Name)
		// export/import are leaf commands taking a file path argument.
		assert.NotNil(t, leaf.Action)
		assert.Equal(t, "<file>", leaf.ArgsUsage)
	}
}

func TestStageSecretCommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "secret", cmd.Name)
	assert.Contains(t, cmd.Aliases, "sm")
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.CommandNotFound)
}

func TestStageSecretCommand_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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
		"export",
		"import",
	}

	for _, expected := range expectedSubcommands {
		assert.Contains(t, subcommandNames, expected, "should have %s subcommand", expected)
	}
}

func TestStageSecretCommand_AddSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_EditSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_DeleteSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_StatusSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_DiffSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_ApplySubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_ResetSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_TagSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_UntagSubcommand(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
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

func TestStageSecretCommand_ExportImportSubcommands(t *testing.T) {
	t.Parallel()

	cmd := aws.StageSecretCommand()
	require.NotNil(t, cmd)

	for _, name := range []string{"export", "import"} {
		leaf, found := lo.Find(cmd.Commands, func(c *cli.Command) bool {
			return c.Name == name
		})

		require.True(t, found, "should have %s subcommand", name)
		require.NotNil(t, leaf)
		assert.Equal(t, name, leaf.Name)
		// export/import are leaf commands taking a file path argument.
		assert.NotNil(t, leaf.Action)
		assert.Equal(t, "<file>", leaf.ArgsUsage)
	}
}
