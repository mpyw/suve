//nolint:testpackage // white-box: exercises the unexported launch-scope helpers
package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/provider"
)

// runLaunchScope drives a throwaway cli.Command carrying the given flags/args
// and returns the launchScope it derives for provider p. Running the command is
// the reliable way to populate urfave/cli flag values.
func runLaunchScope(t *testing.T, p provider.Provider, flags []cli.Flag, args []string) provider.Scope {
	t.Helper()

	var got provider.Scope

	cmd := &cli.Command{
		Name:  args[0],
		Flags: flags,
		Action: func(_ context.Context, c *cli.Command) error {
			got = launchScope(c, p)

			return nil
		},
	}
	require.NoError(t, cmd.Run(t.Context(), args))

	return got
}

func TestLaunchScope_CarriesAzureFields(t *testing.T) {
	t.Parallel()

	flags := []cli.Flag{
		&cli.StringFlag{Name: "vault-name"},
		&cli.StringFlag{Name: "store-name"},
		&cli.StringFlag{Name: "namespace"},
	}
	got := runLaunchScope(t, provider.ProviderAzure, flags,
		[]string{"param", "--store-name", "my-store", "--namespace", "dev"})

	assert.Equal(t, provider.Scope{
		Provider: provider.ProviderAzure, StoreName: "my-store", AppConfigNamespace: "dev",
	}, got)
}

func TestLaunchScope_CarriesGoogleCloudProject(t *testing.T) {
	t.Parallel()

	flags := []cli.Flag{&cli.StringFlag{Name: "project"}}
	got := runLaunchScope(t, provider.ProviderGoogleCloud, flags,
		[]string{"secret", "--project", "my-project"})

	assert.Equal(t, provider.GoogleCloudScope("my-project"), got)
}

func TestLaunchScope_AWSHasNoScopeFlag(t *testing.T) {
	t.Parallel()

	got := runLaunchScope(t, provider.ProviderAWS, nil, []string{"aws"})

	assert.Equal(t, provider.Scope{Provider: provider.ProviderAWS}, got)
}

func TestLaunchGroupProvider(t *testing.T) {
	t.Parallel()

	for _, p := range []provider.Provider{provider.ProviderAWS, provider.ProviderGoogleCloud, provider.ProviderAzure} {
		assert.Equal(t, p, launchGroupProvider(groupName(p)))
	}

	for _, name := range []string{"param", "secret", "stage", "googlecloud", ""} {
		assert.Empty(t, launchGroupProvider(name), "name %q", name)
	}
}

func TestLaunchService(t *testing.T) {
	t.Parallel()

	// The canonical subgroup names map to their service identifier; anything else
	// (group level, unknown) carries no specific service.
	assert.Equal(t, "param", launchService("param"))
	assert.Equal(t, "secret", launchService("secret"))
	assert.Empty(t, launchService("azure"))
	assert.Empty(t, launchService(""))
	assert.Empty(t, launchService("stage"))
}

// TestRegisterLaunchMode_Launches drives the registered flag end to end: the
// root flag runs Bare, a group flag runs Launch with the group's provider and
// scope flags, and an Azure subgroup flag also carries its service.
//
//nolint:paralleltest // mutates the process-wide App; must not race other tests
func TestRegisterLaunchMode_Launches(t *testing.T) {
	t.Cleanup(func() { App = MakeApp() })

	// Flags may fall back to env; keep the expected scopes independent of it.
	for _, k := range []string{"GOOGLE_CLOUD_PROJECT", "AZURE_KEYVAULT_NAME", "AZURE_APPCONFIG_NAME", "AZURE_APPCONFIG_NAMESPACE"} {
		t.Setenv(k, "")
	}

	errLaunched := errors.New("launched")

	var (
		bare       bool
		gotScope   provider.Scope
		gotService string
	)

	RegisterLaunchMode(LaunchMode{
		Flag:       "fake-ui",
		RootUsage:  "root",
		GroupUsage: "group",
		Bare: func(ctx context.Context) (context.Context, error) {
			bare = true

			return ctx, errLaunched
		},
		Launch: func(ctx context.Context, scope provider.Scope, service string) (context.Context, error) {
			gotScope, gotService = scope, service

			return ctx, errLaunched
		},
	})

	require.ErrorIs(t, App.Run(t.Context(), []string{"suve", "--fake-ui"}), errLaunched)
	assert.True(t, bare)

	require.ErrorIs(t, App.Run(t.Context(), []string{"suve", "gcloud", "--project", "p", "--fake-ui"}), errLaunched)
	assert.Equal(t, provider.GoogleCloudScope("p"), gotScope)
	assert.Empty(t, gotService)

	require.ErrorIs(t, App.Run(t.Context(), []string{"suve", "azure", "param", "--store-name", "s", "--fake-ui"}), errLaunched)
	assert.Equal(t, provider.AzureAppConfigScope("s"), gotScope)
	assert.Equal(t, "param", gotService)
}
