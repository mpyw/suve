//declscope:core
//declscope:package
//
// This file is, by design, the package's shared render vocabulary: every
// declaration is a small helper consumed by several widgets. It joins the core
// namespace (a "widgets" prefix on each name would add noise, not information)
// and everything it declares is package-wide.

package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/styles"
)

// clamp constrains v to [0, hi]. Every index the list and history widgets
// clamp is an offset into a slice, so the lower bound is always zero; hi may
// be negative when the slice is empty, and the result is then zero too.
func clamp(v, hi int) int {
	if hi < 0 {
		return 0
	}

	return max(0, min(v, hi))
}

// truncate clamps a (possibly styled) line to width display columns.
func truncate(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// renderBadges renders trailing chips as "[a] [b]".
func renderBadges(st styles.Styles, badges []string) string {
	if len(badges) == 0 {
		return ""
	}

	parts := make([]string, len(badges))
	for i, b := range badges {
		parts[i] = st.PageHint.Render("[" + b + "]")
	}

	return strings.Join(parts, " ")
}
