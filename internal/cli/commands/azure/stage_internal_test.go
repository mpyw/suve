// These are stage.go's tests: stageGlobalConfig is private to the stage
// namespace.
//declscope:namespace stage

package azure

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	azureinternal "github.com/mpyw/suve/internal/cli/commands/azure/internal"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

func TestStageGlobalConfig(t *testing.T) {
	t.Parallel()

	// App Configuration (param) and Key Vault (secret) are independent resources,
	// so each service carries its OWN scope resolver. Use distinguishable targets
	// to assert the resolvers are wired per-service rather than shared.
	paramResolver := func(_ context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Target: provider.AzureAppConfigScope("acme").Target()}, nil
	}
	secretResolver := func(_ context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Target: provider.AzureKeyVaultScope("acme").Target()}, nil
	}
	strategyForNamespace := func(_ context.Context, _ string) (staging.FullStrategy, error) {
		return nil, nil //nolint:nilnil // test stub
	}

	param := stgcli.CommandConfig{
		ParserFactory:        staging.AzureParamParserFactory,
		ScopeResolver:        paramResolver,
		StrategyForNamespace: strategyForNamespace,
	}
	secret := stgcli.CommandConfig{
		ParserFactory: staging.AzureSecretParserFactory,
		ScopeResolver: secretResolver,
	}

	cfg := stageGlobalConfig(param, secret)

	assert.Equal(t, "Azure", cfg.ProviderLabel)
	require.Len(t, cfg.Services, 2)

	// Param (App Configuration): keyed by store, has a namespace axis.
	assert.Equal(t, staging.ServiceParam, cfg.Services[0].Service)
	require.NotNil(t, cfg.Services[0].ScopeResolver)
	got, err := cfg.Services[0].ScopeResolver(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "store acme", got.Target.String())
	assert.NotNil(t, cfg.Services[0].StrategyForNamespace, "App Configuration must resolve per-namespace strategies")

	// Secret (Key Vault): keyed by vault, no namespace axis.
	assert.Equal(t, staging.ServiceSecret, cfg.Services[1].Service)
	require.NotNil(t, cfg.Services[1].ScopeResolver)
	got, err = cfg.Services[1].ScopeResolver(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "vault acme", got.Target.String())
	assert.Nil(t, cfg.Services[1].StrategyForNamespace, "Key Vault has no namespace axis")
}

// TestStageHelpWording verifies the stage help text names Azure and uses the explicit
// "suve azure stage" command paths, never another provider's wording or the flat
// alias paths.
func TestStageHelpWording(t *testing.T) {
	t.Parallel()

	for path, text := range stageHelpTexts(StageCommand(), "suve azure stage") {
		assert.NotContains(t, text, "AWS", path)
		for _, m := range regexp.MustCompile(`suve (?:aws|gcloud|azure|param|secret|stage|stg)\b`).FindAllString(text, -1) {
			assert.Equal(t, "suve azure", m, "%s: %q", path, text)
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

// stageScopeProbe runs `stage <args...>` with a probe leaf appended to both
// service subgroups and returns the staging target each resolver saw.
func stageScopeProbe(t *testing.T, args ...string) string {
	t.Helper()

	var got string

	stage := StageCommand()
	for _, sub := range stage.Commands {
		switch sub.Name {
		case stageNounSecret:
			sub.Commands = append(sub.Commands, &cli.Command{
				Name: "probe",
				Action: func(ctx context.Context, _ *cli.Command) error {
					got = stageResolvedTarget(azureinternal.KeyVaultStagingScopeResolver(ctx))

					return nil
				},
			})
		case "param":
			sub.Commands = append(sub.Commands, &cli.Command{
				Name: "probe",
				Action: func(ctx context.Context, _ *cli.Command) error {
					got = stageResolvedTarget(azureinternal.AppConfigStagingScopeResolver(ctx))

					return nil
				},
			})
		}
	}

	app := &cli.Command{Name: "suve", Commands: []*cli.Command{stage}}
	require.NoError(t, app.Run(t.Context(), append([]string{"suve", "stage"}, args...)))

	return got
}

func stageResolvedTarget(scope staging.ResolvedScope, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}

	return scope.Target.String()
}

// TestStageResourceFlagPosition verifies that --vault-name / --store-name name
// the staged resource wherever they appear (before the service subgroup, after
// it, or after the leaf), and that an explicit flag always beats the env var.
//
//nolint:paralleltest // uses t.Setenv (AZURE_KEYVAULT_NAME/AZURE_APPCONFIG_NAME); cannot run in parallel
func TestStageResourceFlagPosition(t *testing.T) {
	for _, env := range []string{"", "from-env"} {
		t.Run("env="+env, func(t *testing.T) {
			t.Setenv("AZURE_KEYVAULT_NAME", env)
			t.Setenv("AZURE_APPCONFIG_NAME", env)

			tests := []struct {
				name string
				args []string
				want string
			}{
				{"vault before subgroup", []string{"--vault-name", "vb", "secret", "probe"}, "vault vb"},
				{"vault after subgroup", []string{"secret", "--vault-name", "vb", "probe"}, "vault vb"},
				{"vault after leaf", []string{"secret", "probe", "--vault-name", "vb"}, "vault vb"},
				{"store before subgroup", []string{"--store-name", "sb", "param", "probe"}, "store sb"},
				{"store after subgroup", []string{"param", "--store-name", "sb", "probe"}, "store sb"},
				{"store after leaf", []string{"param", "probe", "--store-name", "sb"}, "store sb"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assert.Equal(t, tt.want, stageScopeProbe(t, tt.args...))
				})
			}
		})
	}

	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("AZURE_KEYVAULT_NAME", "va")
		t.Setenv("AZURE_APPCONFIG_NAME", "sa")

		assert.Equal(t, "vault va", stageScopeProbe(t, "secret", "probe"))
		assert.Equal(t, "store sa", stageScopeProbe(t, "param", "probe"))
	})
}
