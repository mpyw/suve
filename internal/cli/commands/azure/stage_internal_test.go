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
