// Package termquirk detects terminal environments that need rendering
// workarounds in the TUI, and provides the repaint helper those workarounds use.
package termquirk

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ScrollNeedsFullRepaint reports whether the current terminal mishandles Bubble
// Tea v2's scroll-region optimization and therefore needs scrolling to force a
// full repaint (tea.ClearScreen) instead.
//
// When a rendered block shifts vertically (any scroll — up/down keys, PgUp/PgDn,
// mouse wheel, a viewport moving), Bubble Tea's cell renderer emits scroll-region
// control sequences (ESC[…r + SU/SD/IL/DL/ReverseIndex) rather than rewriting
// cells. AWS CloudShell's browser terminal (xterm.js) negotiates synchronized
// output and scroll regions differently from a native terminal emulator, so
// those targeted writes land on the wrong rows and corrupt the display — while a
// resize (which forces a full repaint) clears it cleanly. Native terminals
// (Terminal.app, iTerm2, kitty, …) handle the optimization correctly, so they
// stay on the fast path untouched.
//
// True when running in a known browser-based cloud shell, or when
// SUVE_TUI_FULL_REPAINT is set to a non-empty value — an escape hatch for other
// affected browser terminals (Cloud9, Gitpod, Coder, …).
//
// Detection is by environment variable, never by TERM / terminal type: browser
// terminals (xterm.js, hterm, …) all report TERM=xterm-256color, identical to a
// native xterm/Terminal.app, so a TERM-based check would wrongly force the slow
// path on native terminals — the very case that must stay untouched. Each cloud
// shell instead exports a distinctive marker:
//   - AWS CloudShell        — AWS_EXECUTION_ENV=CloudShell
//   - Google Cloud Shell    — CLOUD_SHELL=true
//   - Azure Cloud Shell     — AZUREPS_HOST_ENVIRONMENT=cloud-shell/<ver>
func ScrollNeedsFullRepaint() bool {
	return os.Getenv("SUVE_TUI_FULL_REPAINT") != "" || inCloudShell()
}

// inCloudShell reports whether the process is running inside a known
// browser-based cloud shell, detected purely from its distinctive env markers.
func inCloudShell() bool {
	switch {
	case os.Getenv("AWS_EXECUTION_ENV") == "CloudShell": // AWS CloudShell
		return true
	case os.Getenv("CLOUD_SHELL") == "true": // Google Cloud Shell
		return true
	case strings.HasPrefix(os.Getenv("AZUREPS_HOST_ENVIRONMENT"), "cloud-shell"): // Azure Cloud Shell
		return true
	default:
		return false
	}
}

// RepaintOnScroll batches a tea.ClearScreen ahead of cmd when a scroll happened
// and the terminal needs a full repaint to avoid the scroll-region corruption;
// otherwise it returns cmd untouched (the fast path). cmd may be nil.
//
// Pass scrolled=false to skip the repaint when the viewport did not actually move
// (e.g. an in-window selection change, or a wheel clamped at an end), so a full
// repaint is paid only when a scroll-region optimization would otherwise fire.
// Callers that cannot cheaply tell whether the offset changed (a scroll offset
// computed later at view time) pass scrolled=true to repaint on every scroll
// input.
func RepaintOnScroll(scrolled bool, cmd tea.Cmd) tea.Cmd {
	if scrolled && ScrollNeedsFullRepaint() {
		return tea.Batch(tea.ClearScreen, cmd)
	}

	return cmd
}
