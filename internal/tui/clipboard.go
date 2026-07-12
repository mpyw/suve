package tui

import (
	tea "charm.land/bubbletea/v2"
)

// setClipboard writes s to the system clipboard via Bubble Tea v2's built-in
// OSC52 command. It is a package variable so tests can stub it (the real OSC52
// byte sequence is terminal-dependent and must never be asserted directly); the
// `y` copy handler calls it through copyToClipboard.
//
//nolint:gochecknoglobals // swappable test seam for the OSC52 clipboard write
var setClipboard = tea.SetClipboard

// copyToClipboard returns a command that copies s to the system clipboard
// (OSC52, SSH-safe). It compensates for mouse capture intercepting the
// terminal's native drag-select copy.
func copyToClipboard(s string) tea.Cmd {
	return setClipboard(s)
}
