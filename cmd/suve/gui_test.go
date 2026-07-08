//go:build production || dev

package main

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/provider"
)

// runScope drives a throwaway cli.Command carrying the given flags/args and
// returns the guiScope it derives for provider p. Running the command is the
// reliable way to populate urfave/cli flag values.
func runScope(t *testing.T, p provider.Provider, flags []cli.Flag, args []string) provider.Scope {
	t.Helper()

	var got provider.Scope

	cmd := &cli.Command{
		Name:  args[0],
		Flags: flags,
		Action: func(_ context.Context, c *cli.Command) error {
			got = guiScope(c, p)

			return nil
		},
	}
	require.NoError(t, cmd.Run(t.Context(), args))

	return got
}

func TestGuiScope_CarriesAzureFields(t *testing.T) {
	t.Parallel()

	flags := []cli.Flag{
		&cli.StringFlag{Name: "vault-name"},
		&cli.StringFlag{Name: "store-name"},
		&cli.StringFlag{Name: "namespace"},
	}
	got := runScope(t, provider.ProviderAzure, flags,
		[]string{"param", "--store-name", "my-store", "--namespace", "dev"})

	assert.Equal(t, provider.ProviderAzure, got.Provider)
	assert.Equal(t, "my-store", got.StoreName)
	// The launch scope carries --namespace so the GUI opens on it.
	assert.Equal(t, "dev", got.AppConfigNamespace)
}

func TestGuiScope_CarriesGoogleCloudProject(t *testing.T) {
	t.Parallel()

	flags := []cli.Flag{&cli.StringFlag{Name: "project"}}
	got := runScope(t, provider.ProviderGoogleCloud, flags,
		[]string{"secret", "--project", "my-project"})

	assert.Equal(t, provider.ProviderGoogleCloud, got.Provider)
	assert.Equal(t, "my-project", got.ProjectID)
}

func TestGuiService(t *testing.T) {
	t.Parallel()

	// The canonical subgroup names map to their service identifier; anything else
	// (group level, unknown) carries no specific service.
	assert.Equal(t, "param", guiService("param"))
	assert.Equal(t, "secret", guiService("secret"))
	assert.Empty(t, guiService("azure"))
	assert.Empty(t, guiService(""))
	assert.Empty(t, guiService("stage"))
}

// TestRegisterGUIFlag_AttachesServiceSubgroups verifies --gui is attached to
// Azure's param/secret subgroups (which carry the launched service), not only
// to the provider group. It is the sole test touching the process-wide
// commands.App (no other test reads or mutates it), so parallel execution is
// safe.
func TestRegisterGUIFlag_AttachesServiceSubgroups(t *testing.T) {
	t.Parallel()

	registerGUIFlag()

	hasGUIFlag := func(c *cli.Command) bool {
		for _, f := range c.Flags {
			if slices.Contains(f.Names(), guiFlagName) {
				return true
			}
		}

		return false
	}

	var azure *cli.Command

	for _, group := range commands.App.Commands {
		if group.Name == "azure" {
			azure = group

			break
		}
	}

	require.NotNil(t, azure, "azure group must exist")
	assert.True(t, hasGUIFlag(azure), "azure group carries --gui")

	var sawParam, sawSecret bool

	for _, sub := range azure.Commands {
		switch sub.Name {
		case "param":
			sawParam = true

			assert.True(t, hasGUIFlag(sub), "azure param subgroup carries --gui")
		case "secret":
			sawSecret = true

			assert.True(t, hasGUIFlag(sub), "azure secret subgroup carries --gui")
		}
	}

	assert.True(t, sawParam, "azure param subgroup must exist")
	assert.True(t, sawSecret, "azure secret subgroup must exist")
}
