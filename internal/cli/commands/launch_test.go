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
	"github.com/mpyw/suve/internal/provider/detect"
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

	// Every provider's service subgroup preselects its service (#995), not only
	// Azure's.
	for _, tt := range []struct {
		args    []string
		scope   provider.Scope
		service string
	}{
		{[]string{"suve", "aws", "secret", "--fake-ui"}, provider.Scope{Provider: provider.ProviderAWS}, "secret"},
		{[]string{"suve", "aws", "param", "--fake-ui"}, provider.Scope{Provider: provider.ProviderAWS}, "param"},
		{[]string{"suve", "gcloud", "--project", "p", "secret", "--fake-ui"}, provider.GoogleCloudScope("p"), "secret"},
		// Under stage, the scope flags given on the stage command or its service
		// subgroup reach the launch scope (#1001).
		{
			[]string{"suve", "azure", "stage", "param", "--store-name", "s", "--fake-ui"},
			provider.AzureAppConfigScope("s"), "param",
		},
		{
			[]string{"suve", "azure", "stage", "--vault-name", "v", "secret", "--fake-ui"},
			provider.AzureKeyVaultScope("v"), "secret",
		},
		{
			[]string{"suve", "azure", "stage", "--vault-name", "v", "--store-name", "s", "--fake-ui"},
			provider.Scope{Provider: provider.ProviderAzure, VaultName: "v", StoreName: "s"}, "",
		},
		{
			[]string{"suve", "azure", "stage", "param", "--store-name", "s", "status", "--fake-ui"},
			provider.AzureAppConfigScope("s"), "param",
		},
		{[]string{"suve", "aws", "stage", "secret", "--fake-ui"}, provider.Scope{Provider: provider.ProviderAWS}, "secret"},
		{[]string{"suve", "gcloud", "--project", "p", "stage", "--fake-ui"}, provider.GoogleCloudScope("p"), ""},
	} {
		gotScope, gotService = provider.Scope{}, "unset"
		require.ErrorIs(t, App.Run(t.Context(), tt.args), errLaunched, "%v", tt.args)
		assert.Equal(t, tt.scope, gotScope, "%v", tt.args)
		assert.Equal(t, tt.service, gotService, "%v", tt.args)
	}
}

// TestRegisterLaunchMode_ExplicitEmptyNamespace verifies an explicit
// --namespace "" (the null namespace) survives the UI's env hydration instead
// of being refilled from AZURE_APPCONFIG_NAMESPACE, as the CLI already honors
// it (#1002). It mutates the process-wide App and env, so it is not parallel.
func TestRegisterLaunchMode_ExplicitEmptyNamespace(t *testing.T) {
	t.Cleanup(func() { App = MakeApp() })

	t.Setenv("AZURE_APPCONFIG_NAME", "env-store")
	t.Setenv("AZURE_APPCONFIG_NAMESPACE", "dev")
	t.Setenv("AZURE_KEYVAULT_NAME", "")

	errLaunched := errors.New("launched")

	var hydrated provider.Scope

	// A fresh App per run: urfave/cli keeps flag state between runs of one
	// command tree, and the real CLI runs its tree once.
	run := func(args ...string) provider.Scope {
		t.Helper()

		App = MakeApp()
		RegisterLaunchMode(LaunchMode{
			Flag: "fake-ui",
			Bare: func(ctx context.Context) (context.Context, error) { return ctx, errLaunched },
			Launch: func(ctx context.Context, scope provider.Scope, _ string) (context.Context, error) {
				var err error

				hydrated, err = detect.HydrateScope(detect.OSEnvironment(), scope)
				require.NoError(t, err)

				return ctx, errLaunched
			},
		})

		hydrated = provider.Scope{}
		require.ErrorIs(t, App.Run(t.Context(), append([]string{"suve"}, args...)), errLaunched)

		return hydrated
	}

	got := run("azure", "param", "--fake-ui")
	assert.Equal(t, "env-store", got.StoreName)
	assert.Equal(t, "dev", got.AppConfigNamespace, "without the flag the env namespace applies")

	got = run("azure", "param", "--namespace", "", "--fake-ui")
	assert.Equal(t, "env-store", got.StoreName)
	assert.Empty(t, got.AppConfigNamespace, `--namespace "" must select the null namespace`)

	got = run("azure", "param", "--store-name", "flag-store", "--namespace", "prod", "--fake-ui")
	assert.Equal(t, "flag-store", got.StoreName)
	assert.Equal(t, "prod", got.AppConfigNamespace)
}
