package browser

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/styles"
)

// View renders the browser page into the content area and records the geometry
// mouse handlers hit-test against.
func (m *Model) View(width, height int) string {
	m.width, m.height = width, height
	if width <= 0 || height <= 0 {
		return ""
	}

	header := m.renderHeader(width)
	headerH := lipgloss.Height(header)

	parts := []string{header}
	offset := headerH

	if m.err != "" {
		parts = append(parts, m.styles.ErrorText.Render(clip(m.err, width)))
		offset++
	}

	bodyH := max(height-offset, 0)
	parts = append(parts, m.renderBody(width, bodyH, offset))

	return strings.Join(parts, "\n")
}

// renderHeader renders the single prefix/filter/toggles line, plus the App
// Config namespace filter and a spinner while loading.
func (m *Model) renderHeader(width int) string {
	var b strings.Builder

	if m.svcCap.HasNamespaces {
		b.WriteString(m.styles.FieldLabel.Render("ns: "))
		b.WriteString(m.styles.StatusValue.Render(namespaceBadge(m.currentNamespace())))
		b.WriteString("   ")
	}

	b.WriteString(m.styles.FieldLabel.Render("prefix: "))
	b.WriteString(fieldValue(m.prefix.Value(), m.focus == focusPrefix))
	b.WriteString("   ")
	b.WriteString(m.styles.FieldLabel.Render("filter: "))
	b.WriteString(fieldValue(m.filter.Value(), m.focus == focusFilter))
	b.WriteString("   ")
	b.WriteString(toggle(m.styles, "values", m.valuesOn))

	if m.svcCap.Service == "param" && !m.svcCap.HasNamespaces {
		b.WriteString("  ")
		b.WriteString(toggle(m.styles, "recursive", m.recursive))
	}

	b.WriteString("  ")

	if m.loading {
		b.WriteString(m.spinner.View())
	} else {
		b.WriteString("⟳")
	}

	return clip(b.String(), width)
}

// renderBody renders the list and detail panes (side by side or stacked) and
// records their geometry at page-local yOffset.
func (m *Model) renderBody(width, height, yOffset int) string {
	if height <= 0 {
		return ""
	}

	if width >= twoPaneMinWidth {
		return m.renderTwoPane(width, height, yOffset)
	}

	return m.renderStacked(width, height, yOffset)
}

// renderTwoPane lays the list on the left and the detail on the right.
func (m *Model) renderTwoPane(width, height, yOffset int) string {
	listW := min(listPaneMaxWidth, width*listWidthNum/listWidthDen)
	detailW := width - listW

	listPane := m.renderListPane(listW, height, yOffset, 0)
	detailPane := m.renderDetailPane(detailW, height, yOffset, listW)

	return lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailPane)
}

// renderStacked lays the list above the detail.
func (m *Model) renderStacked(width, height, yOffset int) string {
	listH := max(height/2, stackedMinPaneHeight) //nolint:mnd // even split of the body height
	detailH := max(height-listH, 0)

	listPane := m.renderListPane(width, listH, yOffset, 0)
	detailPane := m.renderDetailPane(width, detailH, yOffset+listH, 0)

	return lipgloss.JoinVertical(lipgloss.Left, listPane, detailPane)
}

// renderListPane sizes the list widget, records its geometry, and frames it.
func (m *Model) renderListPane(width, height, paneTop, paneLeft int) string {
	innerW, innerH := components.PaneInner(width, height)
	m.list.SetSize(innerW, innerH)

	m.geom.listTop = paneTop + paneContentTop
	m.geom.listLeft = paneLeft + paneBorderLeft
	m.geom.listRight = m.geom.listLeft + innerW
	m.geom.listRows = innerH

	title := "entries (" + strconv.Itoa(m.list.Len()) + ")"

	return components.Pane(m.styles, title, m.list.View(), width, height)
}

// renderDetailPane sizes the detail widgets, records the history geometry, and
// frames the detail.
func (m *Model) renderDetailPane(width, height, paneTop, paneLeft int) string {
	innerW, innerH := components.PaneInner(width, height)

	title := "detail"
	if m.detailOK {
		title = m.detail.Name
	}

	body, historyLocalTop, historyRows := m.renderDetail(innerW, innerH)

	// History content sits at: pane top + border + title + lines before history.
	m.geom.historyTop = paneTop + paneContentTop + historyLocalTop
	m.geom.historyLeft = paneLeft + paneBorderLeft
	m.geom.historyRight = m.geom.historyLeft + innerW
	m.geom.historyRows = historyRows

	return components.Pane(m.styles, title, body, width, height)
}

// renderDetail builds the detail body and returns the line offset (within the
// body) at which the history content begins and how many history rows are shown,
// so the pane can record where clicks land.
func (m *Model) renderDetail(innerW, innerH int) (string, int, int) {
	if !m.detailOK {
		return m.styles.PageHint.Render("select an entry"), 0, 0
	}

	var lines []string

	if m.isSelectedStaged() {
		lines = append(lines, m.styles.Banner.Render(clip("⚠ staged changes — S: staging", innerW)))
		lines = append(lines, "")
	}

	lines = append(lines, m.valueLabelLine(innerW))
	m.valuePane.SetSize(innerW, valuePaneHeight)
	lines = append(lines, strings.Split(m.valuePane.View(), "\n")...)
	lines = append(lines, "")
	lines = append(lines, m.metaLines(innerW)...)

	if desc := m.detail.Description; desc != "" {
		lines = append(lines, fieldLine(m.styles, "Description", desc, innerW))
	}

	lines = append(lines, m.tagLine(innerW))

	if !m.svcCap.HasVersionHistory {
		return fitLines(lines, innerH), 0, 0
	}

	lines = append(lines, "")
	lines = append(lines, m.historyHeaderLine(innerW))

	historyLocalTop := len(lines)
	historyH := max(innerH-historyLocalTop, 0)
	m.history.SetSize(innerW, historyH)
	lines = append(lines, strings.Split(m.history.View(), "\n")...)

	return fitLines(lines, innerH), historyLocalTop, historyH
}

// valueLabelLine renders the "Value  (x to reveal)" label row.
func (m *Model) valueLabelLine(width int) string {
	label := m.styles.PaneTitle.Render("Value")
	if hint := m.valuePane.HintSuffix(); hint != "" {
		label += "   " + m.styles.PageHint.Render(hint)
	}

	return clip(label, width)
}

// metaLines renders the capability-gated metadata rows plus the state-or-labels
// badge row (whichever the version populates).
func (m *Model) metaLines(width int) []string {
	lines := make([]string, 0, len(m.detail.Meta)+1)

	for _, row := range m.detail.Meta {
		lines = append(lines, fieldLine(m.styles, row.Label, row.Value, width))
	}

	if badge := m.stateBadgeLine(width); badge != "" {
		lines = append(lines, badge)
	}

	return lines
}

// stateBadgeLine renders the version's State OR its staging labels — whichever is
// populated — as a "State"/"Labels" row, never inferring one from the other.
func (m *Model) stateBadgeLine(width int) string {
	switch {
	case len(m.detail.StagingLabels) > 0:
		return fieldLine(m.styles, "Labels", strings.Join(m.detail.StagingLabels, " "), width)
	case m.detail.State != "":
		return fieldLine(m.styles, "State", m.detail.State, width)
	default:
		return ""
	}
}

// tagLine renders the read-only tag bar.
func (m *Model) tagLine(width int) string {
	if len(m.detail.Tags) == 0 {
		return fieldLine(m.styles, "Tags", m.styles.PageHint.Render("(none)"), width)
	}

	return fieldLine(m.styles, "Tags", tagsInline(m.detail.Tags), width)
}

// historyHeaderLine renders the "History  c: compare mode" row.
func (m *Model) historyHeaderLine(width int) string {
	hint := "c: compare mode"
	if m.history.Compare() {
		hint = "space: pick · enter: diff · esc: exit"
	}

	return clip(m.styles.PaneTitle.Render("History")+"   "+m.styles.PageHint.Render(hint), width)
}

// isSelectedStaged reports whether the selected entry has staged changes. It
// resolves the item by index (see selectedItem) so a namespaced duplicate keys
// the banner off the correct (name, namespace) pair.
func (m *Model) isSelectedStaged() bool {
	item, ok := m.selectedItem()
	if !ok {
		return false
	}

	_, staged := m.stagedKeys[dataStagedKey(item)]

	return staged
}

// fieldLine renders a "Label  value" metadata row, clipped to width.
func fieldLine(st styles.Styles, label, value string, width int) string {
	const labelWidth = 11

	padded := label
	if lipgloss.Width(label) < labelWidth {
		padded = label + strings.Repeat(" ", labelWidth-lipgloss.Width(label))
	}

	return clip(st.FieldLabel.Render(padded)+" "+value, width)
}

// fieldValue renders a header input's value, showing a placeholder and a cursor
// when focused.
func fieldValue(value string, focused bool) string {
	if focused {
		return value + "▎"
	}

	if value == "" {
		return "—"
	}

	return value
}

// toggle renders an "name:on/off" toggle chip.
func toggle(st styles.Styles, name string, on bool) string {
	state := "off"
	if on {
		state = "on"
	}

	return st.FieldLabel.Render(name+":") + st.StatusValue.Render(state)
}

// fitLines pads/truncates lines to exactly height rows.
func fitLines(lines []string, height int) string {
	for len(lines) < height {
		lines = append(lines, "")
	}

	if height >= 0 && len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

// clip clamps a (possibly styled) line to width display columns.
func clip(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
