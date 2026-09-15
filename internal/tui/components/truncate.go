package components

import "charm.land/lipgloss/v2"

// truncate clamps a (possibly styled) line to width display columns.
//
//declscope:package // shared by the status bar, list, and history widgets
func truncate(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}
