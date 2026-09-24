//go:build production || dev

package main

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/mpyw/suve/internal/cli/commands"
)

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
