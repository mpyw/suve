//go:build production || dev

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/cli/commands"
)

// TestComposedUsage_CLIGUITUI pins that the GUI and TUI usage rewrites compose to
// "CLI/GUI/TUI" in the GUI-embedded build, applied in main.go's order:
// RegisterTUIDescription turns "CLI" into "CLI/TUI", then registerGUIDescription
// turns the leading "CLI" into "CLI/GUI". Asserting the composed outcome guards
// the ordering without pinning the individual calls.
//
// It mutates the process-wide App and rebuilds a pristine App on cleanup;
// non-parallel so it does not race the other App-mutating test in this package.
//
//nolint:paralleltest // mutates the process-wide App; must not race other tests
func TestComposedUsage_CLIGUITUI(t *testing.T) {
	t.Cleanup(func() { commands.App = commands.MakeApp() })

	// Mirror main's registration order.
	registerGUIFlag()
	commands.RegisterTUIFlag()
	commands.RegisterTUIDescription()
	registerGUIDescription()

	assert.Contains(t, commands.App.Usage, "CLI/GUI/TUI",
		"the GUI and TUI usage rewrites compose to CLI/GUI/TUI in the GUI build")
}
