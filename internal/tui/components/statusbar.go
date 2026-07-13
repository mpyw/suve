// Package components holds the leaf render widgets of the TUI app shell: the
// status bar and the tab bar. They are pure value types — given a width they
// return a styled string — so they are trivially unit-testable and hold no
// Bubble Tea state of their own.
package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/styles"
)

// AWSIdentity carries the async-resolved AWS caller identity shown in the
// status bar. It is a plain data struct (no SDK types) so the status bar never
// depends on the AWS provider package; the launch layer fills it via an
// injected fetcher.
type AWSIdentity struct {
	Account string
	Region  string
	Profile string
}

// StatusBar renders the fixed top line: the provider and its scope. Provider
// and scope never change for the process lifetime (they are fixed at launch),
// so the only mutable input is the AWS identity, which loads asynchronously.
type StatusBar struct {
	Scope  provider.Scope
	Styles styles.Styles

	// Identity is the resolved AWS caller identity (AWS scope only); nil until
	// the async STS lookup returns.
	Identity *AWSIdentity
	// Loading is true while the AWS identity lookup is in flight, so the bar can
	// show a placeholder instead of an empty account.
	Loading bool
}

// View renders the status bar to a single line, truncated to width.
func (s StatusBar) View(width int) string {
	segs := s.scopeSegments()

	parts := make([]string, 0, 1+len(segs))
	parts = append(parts, s.Styles.StatusValue.Render(providerLabel(s.Scope.Provider)))
	parts = append(parts, segs...)

	line := s.Styles.StatusBar.Render("suve") + s.Styles.StatusKey.Render("  ") +
		strings.Join(parts, s.Styles.StatusKey.Render(" · "))

	return truncate(line, width)
}

// scopeSegments renders the provider-specific scope fields as "key:value"
// styled segments, omitting empty ones.
func (s StatusBar) scopeSegments() []string {
	switch s.Scope.Provider {
	case provider.ProviderAWS:
		return s.awsSegments()
	case provider.ProviderGoogleCloud:
		return s.kvSegments("project", s.Scope.ProjectID)
	case provider.ProviderAzure:
		var out []string //nolint:prealloc // 0–3 optional segments; capacity would be a magic number

		out = append(out, s.kvSegments("vault", s.Scope.VaultName)...)
		out = append(out, s.kvSegments("store", s.Scope.StoreName)...)
		out = append(out, s.kvSegments("ns", s.Scope.AppConfigNamespace)...)

		return out
	default:
		return nil
	}
}

// awsSegments renders the AWS profile/account/region trio, showing a loading
// placeholder for the async-resolved account/region until STS returns.
func (s StatusBar) awsSegments() []string {
	if s.Identity == nil {
		if s.Loading {
			return []string{s.Styles.StatusKey.Render("account: loading…")}
		}

		return nil
	}

	var out []string //nolint:prealloc // 0–3 optional segments; capacity would be a magic number

	out = append(out, s.kvSegments("profile", s.Identity.Profile)...)
	out = append(out, s.kvSegments("account", s.Identity.Account)...)
	out = append(out, s.kvSegments("region", s.Identity.Region)...)

	return out
}

// kvSegments renders a single "key:value" segment, or nothing when value is
// empty.
func (s StatusBar) kvSegments(key, value string) []string {
	if value == "" {
		return nil
	}

	return []string{s.Styles.StatusKey.Render(key+":") + s.Styles.StatusValue.Render(value)}
}

// providerLabel maps a provider to its status-bar label.
func providerLabel(p provider.Provider) string {
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

// truncate clamps a (possibly styled) line to width display columns.
func truncate(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}
