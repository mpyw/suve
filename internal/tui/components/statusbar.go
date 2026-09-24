// Package components holds the leaf render widgets of the TUI app shell: the
// status bar and the tab bar. They are pure value types — given a width they
// return a styled string — so they are trivially unit-testable and hold no
// Bubble Tea state of their own.
package components

import (
	"strings"

	"github.com/samber/lo"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/styles"
)

// StatusBar renders the fixed top line: the provider and its target (see
// provider.Target). Provider and scope never change for the process lifetime
// (they are fixed at launch), so the only mutable input is the target, which a
// provider may resolve asynchronously (AWS: the STS caller identity).
type StatusBar struct {
	Scope  provider.Scope
	Styles styles.Styles

	// Target is what the scope points at. While Target.Pending is set, the bar
	// shows a loading placeholder after the known parts.
	Target provider.Target
}

// View renders the status bar to a single line, truncated to width.
func (s StatusBar) View(width int) string {
	segs := s.targetSegments()

	parts := make([]string, 0, 1+len(segs))
	parts = append(parts, s.Styles.StatusValue.Render(statusBarProviderLabel(s.Scope.Provider)))
	parts = append(parts, segs...)

	line := s.Styles.StatusBar.Render("suve") + s.Styles.StatusKey.Render("  ") +
		strings.Join(parts, s.Styles.StatusKey.Render(" · "))

	return truncate(line, width)
}

// targetSegments renders the target as styled "label:value" segments, skipping
// the parts without a value. A pending target (a network lookup still in
// flight, e.g. the AWS caller identity) ends with a loading placeholder.
func (s StatusBar) targetSegments() []string {
	out := lo.FilterMap(s.Target.Segments, func(seg provider.TargetSegment, _ int) (string, bool) {
		return s.Styles.StatusKey.Render(seg.Label+":") + s.Styles.StatusValue.Render(seg.Value), seg.Value != ""
	})

	if s.Target.Pending {
		out = append(out, s.Styles.StatusKey.Render("loading…"))
	}

	return out
}

// statusBarProviderLabel maps a provider to its status-bar label.
func statusBarProviderLabel(p provider.Provider) string {
	switch p {
	case provider.ProviderAWS:
		return "aws"
	case provider.ProviderGoogleCloud:
		return "googlecloud"
	case provider.ProviderAzure:
		return "azure"
	default:
		return string(p)
	}
}
