//nolint:testpackage // white-box: drives the process-wide App through RegisterTUIFlag/RegisterTUIDescription
package commands

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/provider"
)

// hasTUIFlag reports whether a command carries the --tui flag.
func hasTUIFlag(c *cli.Command) bool {
	for _, f := range c.Flags {
		if slices.Contains(f.Names(), tuiFlagName) {
			return true
		}
	}

	return false
}

// findGroup returns the top-level command with the given name, or nil.
func findGroup(name string) *cli.Command {
	for _, group := range App.Commands {
		if group.Name == name {
			return group
		}
	}

	return nil
}

// TestRegisterTUIFlag_RegistersEverywhere pins the load-bearing flag/usage
// registration that is otherwise only exercised by a manual build. After
// RegisterTUIFlag the --tui flag is present on the root, every provider group,
// and Azure's param/secret service subgroups; after RegisterTUIDescription the
// root usage advertises the TUI ("CLI/TUI" in the default build, before the GUI
// rewrite that would compose "CLI/GUI/TUI"). It asserts the OUTCOME rather than a
// call order, so it stays green if the wrapper chain is reordered.
//
// It mutates the process-wide App (the sole commands-package test to do so) and
// rebuilds a pristine App on cleanup; it is intentionally non-parallel.
//
//nolint:paralleltest // mutates the process-wide App; must not race other tests
func TestRegisterTUIFlag_RegistersEverywhere(t *testing.T) {
	t.Cleanup(func() { App = MakeApp() })

	require.False(t, hasTUIFlag(App), "precondition: --tui not yet registered on the root")

	RegisterTUIFlag()
	RegisterTUIDescription()

	assert.True(t, hasTUIFlag(App), "--tui is registered on the root")

	// Every provider group must carry --tui. Derive names from the provider set so
	// a renamed group is caught and no group name is hard-coded here.
	for _, p := range []provider.Provider{
		provider.ProviderAWS,
		provider.ProviderGoogleCloud,
		provider.ProviderAzure,
	} {
		name := groupName(p)

		group := findGroup(name)
		require.NotNilf(t, group, "the %s provider group must exist", name)
		assert.Truef(t, hasTUIFlag(group), "--tui is registered on the %s group", name)
	}

	assertAzureSubgroupsHaveTUI(t)

	assert.Contains(t, App.Usage, "CLI/TUI", "root usage advertises the TUI after RegisterTUIDescription")
}

// assertAzureSubgroupsHaveTUI checks that Azure's param and secret service
// subgroups (which carry the launched service) each expose --tui.
func assertAzureSubgroupsHaveTUI(t *testing.T) {
	t.Helper()

	azure := findGroup(groupName(provider.ProviderAzure))
	require.NotNil(t, azure, "the azure provider group must exist")

	var sawParam, sawSecret bool

	for _, sub := range azure.Commands {
		if svc := tuiService(sub.Name); svc != "" {
			assert.Truef(t, hasTUIFlag(sub), "--tui is registered on the azure %s subgroup", sub.Name)

			switch svc {
			case "param":
				sawParam = true
			case "secret":
				sawSecret = true
			}
		}
	}

	assert.True(t, sawParam, "the azure param subgroup must exist")
	assert.True(t, sawSecret, "the azure secret subgroup must exist")
}
