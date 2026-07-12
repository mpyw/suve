package browser

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/hit"
	"github.com/mpyw/suve/internal/tui/styles"
)

// headerSep is the three-space gap between header segments.
const headerSep = "   "

// headerSeg records a header segment's clickable column range in page-local
// coordinates: the region ID and its [x, x+w) span on the header row.
type headerSeg struct {
	id string
	x  int
	w  int
}

// View renders the browser page into the content area and rebuilds the hit map
// mouse handlers test against.
func (m *Model) View(width, height int) string {
	m.width, m.height = width, height
	if width <= 0 || height <= 0 {
		return ""
	}

	m.regions = m.regions[:0]

	header, segs := m.renderHeader(width)
	headerH := lipgloss.Height(header)

	for _, s := range segs {
		m.regions = append(m.regions, hit.Region(s.id, s.x, 0, s.w, 1))
	}

	parts := []string{header}
	offset := headerH

	for _, line := range m.errLines() {
		parts = append(parts, m.styles.ErrorText.Render(clip(line, width)))
		offset++
	}

	bodyH := max(height-offset, 0)
	parts = append(parts, m.renderBody(width, bodyH, offset))

	m.hits = hit.New(m.regions...)

	return strings.Join(parts, "\n")
}

// renderHeader renders the single prefix/filter/toggles line (plus the App Config
// namespace filter and a spinner while loading) and returns each clickable
// segment's column range, so a header click reduces to the same action its key
// equivalent performs. Segment widths come from the rendered display width, so
// styling never shifts a hit range (color-safe, like the section-button ranges).
func (m *Model) renderHeader(width int) (string, []headerSeg) {
	type piece struct {
		s  string
		id string // "" for an inert spacer
	}

	var pieces []piece

	if m.svcCap.HasNamespaces {
		pieces = append(pieces,
			piece{m.styles.FieldLabel.Render("ns: ") + m.styles.StatusValue.Render(namespaceBadge(m.currentNamespace())), regionNamespace},
			piece{headerSep, ""},
		)
	}

	pieces = append(pieces,
		piece{m.styles.FieldLabel.Render("prefix: ") + fieldValue(m.prefix.Value(), m.focus == focusPrefix), regionPrefix},
		piece{headerSep, ""},
		piece{m.styles.FieldLabel.Render("filter: ") + fieldValue(m.filter.Value(), m.focus == focusFilter), regionFilter},
		piece{headerSep, ""},
		piece{toggle(m.styles, "values", m.valuesOn), regionValues},
	)

	if m.svcCap.Service == "param" && !m.svcCap.HasNamespaces {
		pieces = append(pieces,
			piece{"  ", ""},
			piece{toggle(m.styles, "recursive", m.recursive), regionRecursive},
		)
	}

	refresh := "⟳"
	if m.loading {
		refresh = m.spinner.View()
	}

	pieces = append(pieces, piece{"  ", ""}, piece{refresh, regionRefresh})

	var (
		b    strings.Builder
		segs []headerSeg
		col  int
	)

	for _, p := range pieces {
		w := lipgloss.Width(p.s)
		if p.id != "" {
			segs = append(segs, headerSeg{id: p.id, x: col, w: w})
		}

		b.WriteString(p.s)

		col += w
	}

	return clip(b.String(), width), segs
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

// renderListPane sizes the list widget, records its hit region, and frames it. The
// pane is drawn focused (accent border, active selection cursor) when the list
// holds keyboard focus, so the user can see where the arrow keys will land.
func (m *Model) renderListPane(width, height, paneTop, paneLeft int) string {
	innerW, innerH := components.PaneInner(width, height)
	m.list.SetSize(innerW, innerH)

	focused := m.focus == focusList
	m.list.SetFocused(focused)

	listTop := paneTop + paneContentTop
	listLeft := paneLeft + paneBorderLeft
	// The list region stops at its content's right edge so it never swallows a
	// click/wheel aimed at the detail pane, which shares its vertical band in the
	// two-pane layout.
	m.regions = append(m.regions, hit.Region(regionList, listLeft, listTop, innerW, innerH))

	title := "entries (" + strconv.Itoa(m.list.Len()) + ")"

	return framePane(m.styles, focused, title, m.list.View(), width, height)
}

// renderDetailPane sizes the detail widgets, records the detail/value/history hit
// regions, and frames the detail. The pane is drawn focused when the history holds
// keyboard focus (the detail pane's only navigable widget), so the active pane is
// obvious.
func (m *Model) renderDetailPane(width, height, paneTop, paneLeft int) string {
	innerW, innerH := components.PaneInner(width, height)

	title := "detail"
	if m.detailOK {
		title = m.detail.Name
	}

	body, valueLabelLocalTop, historyLocalTop, historyRows := m.renderDetail(innerW, innerH)

	detailTop := paneTop + paneContentTop
	detailLeft := paneLeft + paneBorderLeft

	// The whole detail content is one region so a wheel anywhere in it scrolls the
	// value pane; the value-label row and the history band sit above it (higher Z)
	// so a click/wheel on them is resolved first.
	m.regions = append(m.regions, hit.Region(regionDetail, detailLeft, detailTop, innerW, innerH))

	if valueLabelLocalTop >= 0 {
		m.regions = append(m.regions,
			hit.Region(regionValueLabel, detailLeft, detailTop+valueLabelLocalTop, innerW, 1).Z(1))
	}

	if historyRows > 0 {
		m.regions = append(m.regions,
			hit.Region(regionHistory, detailLeft, detailTop+historyLocalTop, innerW, historyRows).Z(1))
	}

	return framePane(m.styles, m.focus == focusHistory, title, body, width, height)
}

// framePane frames a pane with the focused border when focused, else the idle one.
func framePane(st styles.Styles, focused bool, title, body string, width, height int) string {
	if focused {
		return components.PaneFocused(st, title, body, width, height)
	}

	return components.Pane(st, title, body, width, height)
}

// renderDetail builds the detail body and returns the body-local line the value
// label sits on (-1 when no detail is shown), the line the history content begins
// on, and how many history rows are shown, so the pane can place the value-label
// and history hit regions where they are drawn.
func (m *Model) renderDetail(innerW, innerH int) (body string, valueLabelTop, historyTop, historyRows int) {
	if !m.detailOK {
		return m.styles.PageHint.Render("select an entry"), -1, 0, 0
	}

	var lines []string

	if text, staged := m.selectedStagedBanner(); staged {
		lines = append(lines, m.styles.Banner.Render(clip(text, innerW)))
		lines = append(lines, "")
	}

	valueLabelTop = len(lines)
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
		return fitLines(lines, innerH), valueLabelTop, 0, 0
	}

	lines = append(lines, "")
	lines = append(lines, m.historyHeaderLine(innerW))

	historyTop = len(lines)
	historyH := max(innerH-historyTop, 0)
	m.history.SetSize(innerW, historyH)
	m.history.SetFocused(m.focus == focusHistory)
	lines = append(lines, strings.Split(m.history.View(), "\n")...)

	return fitLines(lines, innerH), valueLabelTop, historyTop, historyH
}

// valueLabelLine renders the "Value  (x to reveal)" / "(J to format)" label row.
func (m *Model) valueLabelLine(width int) string {
	label := m.styles.PaneTitle.Render("Value")
	if hint := m.valuePane.HintSuffix(); hint != "" {
		label += "   " + m.styles.PageHint.Render(hint)
	}

	if hint := m.valuePane.ParseJSONHint(); hint != "" {
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

// historyHeaderLine renders the "History  <hint>" row. The hint adapts to focus so
// the enter→history / esc→list transitions are discoverable: from the list it
// advertises `enter: history`; in the history it advertises `esc: list`; in
// compare mode it advertises the pick/diff/exit keys.
func (m *Model) historyHeaderLine(width int) string {
	title := m.styles.PaneTitle.Render("History")

	var hint string

	switch {
	case m.history.Compare():
		hint = "space: pick · enter: diff · esc: exit"
	case m.history.Len() == 0:
		// No versions: neither entering the history nor compare mode does anything,
		// so advertise no (false) transition.
		return clip(title, width)
	case m.focus == focusHistory:
		hint = "esc: list · c: compare mode"
	default:
		hint = "enter: history · c: compare mode"
	}

	return clip(title+"   "+m.styles.PageHint.Render(hint), width)
}

// selectedStagedBanner returns the detail-pane banner text for the selected
// entry and whether it is staged at all. The text distinguishes a staged value
// change, a staged tag change, and both — mirroring the GUI's StagingBanner
// (internal/gui/frontend/src/lib/StagingBanner.svelte) so the affordance no
// longer collapses every staged kind into one message (#701). It resolves the
// item by index (see selectedItem) so a namespaced duplicate keys the banner off
// the correct (name, namespace) pair.
func (m *Model) selectedStagedBanner() (string, bool) {
	item, ok := m.selectedItem()
	if !ok {
		return "", false
	}

	key := dataStagedKey(item)
	if _, staged := m.stagedKeys[key]; !staged {
		return "", false
	}

	_, hasEntry := m.entryStagedKeys[key]
	_, hasTags := m.tagStagedKeys[key]

	switch {
	case hasEntry && hasTags:
		return "⚠ staged value and tag changes — S: staging", true
	case hasTags:
		return "⚠ staged tag changes — S: staging", true
	default:
		// A staged key with no tag change is a value/entry change (create, edit, or
		// delete); the value wording also covers the defensive case of a staged key
		// absent from both split sets.
		return "⚠ staged value changes — S: staging", true
	}
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
