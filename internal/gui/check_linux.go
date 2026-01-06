//go:build (production || dev) && linux

package gui

import (
	"errors"
	"os/exec"
	"strings"
)

// ErrMissingGUILibs is returned when required GUI libraries are not installed.
var ErrMissingGUILibs = errors.New(`GUI dependencies not found

The GUI requires GTK3 and WebKit2GTK to be installed.
See: https://wails.io/docs/guides/linux-distro-support/

After installing the dependencies, try running the GUI again.
Alternatively, use the CLI without the --gui flag`)

// checkGUIDependencies verifies that required GUI libraries are available.
// Uses ldconfig to query the dynamic linker cache for webkit2gtk.
// If ldconfig is unavailable, skips the check and lets the runtime handle it.
func checkGUIDependencies() error {
	if _, err := exec.LookPath("ldconfig"); err != nil {
		// ldconfig not available, skip check
		return nil
	}

	cmd := exec.Command("ldconfig", "-p")
	output, err := cmd.Output()
	if err != nil {
		// ldconfig failed, skip check
		return nil
	}

	if strings.Contains(string(output), "libwebkit2gtk") {
		return nil
	}

	return ErrMissingGUILibs
}
