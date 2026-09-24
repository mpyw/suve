// These are stage.go's tests: stageGlobalConfig is private to the stage
// namespace.
//declscope:namespace stage

package azure

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
)

func TestStageGlobalConfig(t *testing.T) {
	t.Parallel()

	// App Configuration (param) and Key Vault (secret) are independent resources,
	// so each service carries its OWN scope resolver. Use distinguishable targets
	// to assert the resolvers are wired per-service rather than shared.
	paramResolver := func(_ context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Target: "store acme"}, nil
	}
	secretResolver := func(_ context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Target: "vault acme"}, nil
	}
	strategyForNamespace := func(_ context.Context, _ string) (staging.FullStrategy, error) {
		return nil, nil //nolint:nilnil // test stub
	}

	param := stgcli.CommandConfig{
		ParserFactory:        staging.AzureAppConfigParamParserFactory,
		ScopeResolver:        paramResolver,
		StrategyForNamespace: strategyForNamespace,
	}
	secret := stgcli.CommandConfig{
		ParserFactory: staging.AzureKeyVaultSecretParserFactory,
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
	assert.Equal(t, "store acme", got.Target)
	assert.NotNil(t, cfg.Services[0].StrategyForNamespace, "App Configuration must resolve per-namespace strategies")

	// Secret (Key Vault): keyed by vault, no namespace axis.
	assert.Equal(t, staging.ServiceSecret, cfg.Services[1].Service)
	require.NotNil(t, cfg.Services[1].ScopeResolver)
	got, err = cfg.Services[1].ScopeResolver(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "vault acme", got.Target)
	assert.Nil(t, cfg.Services[1].StrategyForNamespace, "Key Vault has no namespace axis")
}
