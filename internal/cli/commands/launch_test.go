//nolint:testpackage // white-box: exercises the unexported launch-scope helpers
package commands

import (
	"context"
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
