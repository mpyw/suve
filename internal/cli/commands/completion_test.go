//nolint:testpackage // white-box: drives the process-wide App and its unexported --tui Before wrapper
package commands

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/detect"
)

func TestIsShellCompletion(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
		want bool
	}{
		"present": {
			args: []string{appName, nounParam, "show", "--generate-shell-completion"},
			want: true,
		},
		"present as only arg after program": {
			args: []string{appName, "--generate-shell-completion"},
			want: true,
		},
		"absent": {
			args: []string{appName, nounParam, "show", "/my/param"},
			want: false,
		},
		"empty": {
			args: nil,
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := IsShellCompletion(tt.args); got != tt.want {
				t.Errorf("IsShellCompletion(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// TestTUILaunch_ShellCompletion pins #749: the --tui Before wrapper must fall
// through to normal completion during a shell-completion run instead of entering
// the TUI. With no active provider, a real launch fails fast in launchTUIBare
// (uniqueTUIProvider → "no provider is active") BEFORE the TTY guard, so the
// return value cleanly distinguishes "attempted launch" (error) from "fell
// through to completion" (nil). urfave/cli detects completion from the LAST arg
// and from os.Args, so the test sets both to a `suve --tui <TAB>` invocation.
//
// It mutates the process-wide App and os.Args, so it rebuilds a pristine App on
// cleanup and is intentionally non-parallel.
//
//nolint:paralleltest // mutates the process-wide App and os.Args; must not race other tests
func TestTUILaunch_ShellCompletion(t *testing.T) {
	t.Cleanup(func() { App = MakeApp() })

	origArgs := os.Args

	t.Cleanup(func() { os.Args = origArgs })

	// A pristine App with no active provider, then wrap in the --tui Before hook.
	App = MakeAppWithDetect(detect.Result{})

	RegisterTUIFlag()

	t.Run("completion falls through and does not launch the TUI", func(t *testing.T) {
		args := []string{appName, "--tui", completionFlag}
		os.Args = args

		var out bytes.Buffer

		App.Writer = &out

		// If the guard were missing the wrapper would run launchTUIBare and return
		// the "no provider is active" error; instead completion runs and returns nil.
		err := App.Run(t.Context(), args)
		require.NoError(t, err, "shell completion must not trigger the --tui launch")
	})

	t.Run("without completion the wrapper still honors --tui", func(t *testing.T) {
		args := []string{appName, "--tui"}
		os.Args = args

		var out bytes.Buffer

		App.Writer = &out

		// The control: with no completion flag the wrapper enters launchTUIBare,
		// which errors on the (unconfigured) provider before any launch — proving
		// the completion guard, not a broken wrapper, is what suppresses the launch.
		err := App.Run(t.Context(), args)
		require.Error(t, err, "--tui without completion must attempt the launch")
		assert.Contains(t, err.Error(), "no provider is active")
	})
}
