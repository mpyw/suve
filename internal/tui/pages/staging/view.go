package staging

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/hit"
)

// Operation keys (the neutral staging operation strings).
const (
	operationCreate = "create"
	operationUpdate = "update"
	operationDelete = "delete"
)

// lineDesc maps one rendered body line to what a mouse click on it acts on. It
// is intermediate layout state: View turns the visible descriptors into hit
// regions once per frame.
type lineDesc struct {
	// row is the selectable row index this line belongs to, or -1.
	row int
	// section is the section index when this line is a section header (whose
	// apply/reset buttons are hit-tested), or -1.
	section int
	// apply/reset are the [start,end) column ranges of the header's apply/reset
	// buttons (only meaningful when section >= 0).
	apply [2]int
	reset [2]int
}

// headerRanges holds the fixed-header affordances' [start,end) column ranges,
// computed from the plain layout so hit-testing is color-safe.
type headerRanges struct {
	view     [2]int
	applyAll [2]int
	resetAll [2]int
	refresh  [2]int
}

// View renders the staging page into the content area and rebuilds the hit map.
func (m *Model) View(width, height int) string {
	m.width, m.height = width, height
	if width <= 0 || height <= 0 {
		return ""
	}

	header, hRanges := m.headerLine(width)
	head := []string{header}

	noticeRow := -1

	if m.noticeVisible() {
		noticeRow = len(head)
		head = append(head, m.noticeLine(width))
	}

	// Reserve the last page row for the footer: normally the row-action hint so
	// the page-local bindings (e/u/t/x/enter/v, apply/reset) are discoverable —
	// they are not in the global keys.Map the help bar renders (#655) — but a
	// pending invalid-action status message takes that row while it is set (#684).
	footer := m.footerLine(width)

	bodyTop := len(head)
	bodyH := max(height-bodyTop-1, 0)

	lines, descs, rowFirst := m.bodyLines(width)
	m.clampScroll(len(lines), bodyH, rowFirst)

	visible, visDescs := window(lines, descs, m.scroll, bodyH)

	m.hits = m.buildHits(width, bodyTop, noticeRow, hRanges, visDescs)

	out := append(head, visible...) //nolint:gocritic // head is a fresh slice; appending body then footer is intentional
	out = append(out, footer)

	return strings.Join(out, "\n")
}

// buildHits assembles the frame's hit regions in page-local coordinates: the
// fixed-header affordances at row 0, the dismissible notice (when shown), and —
// from the VISIBLE body descriptors — each section's apply/reset buttons and each
// selectable row (spanning its consecutive lines). Rows fill the full width so a
// click anywhere on a row selects it; the section buttons sit above (higher Z) on
// their header line.
func (m *Model) buildHits(width, bodyTop, noticeRow int, h headerRanges, visDescs []lineDesc) *hit.Map {
	regions := []*lipgloss.Layer{
		rangeRegion(regionViewToggle, h.view, 0),
		rangeRegion(regionApplyAll, h.applyAll, 0),
		rangeRegion(regionResetAll, h.resetAll, 0),
		rangeRegion(regionRefresh, h.refresh, 0),
	}

	if noticeRow >= 0 {
		regions = append(regions, hit.Region(regionNotice, 0, noticeRow, width, 1))
	}

	for i := 0; i < len(visDescs); {
		d := visDescs[i]
		y := bodyTop + i

		switch {
		case d.section >= 0:
			regions = append(regions,
				rangeRegion(secApplyID(d.section), d.apply, y).Z(1),
				rangeRegion(secResetID(d.section), d.reset, y).Z(1),
			)
			i++
		case d.row >= 0:
			start := i
			for i < len(visDescs) && visDescs[i].row == d.row {
				i++
			}

			regions = append(regions, hit.Region(rowID(d.row), 0, bodyTop+start, width, i-start))
		default:
			i++
		}
	}

	return hit.New(regions...)
}

// rangeRegion builds a 1-row region for a [start,end) column range at row y.
func rangeRegion(id string, r [2]int, y int) *lipgloss.Layer {
	return hit.Region(id, r[0], y, r[1]-r[0], 1)
}

// footerLine renders the reserved bottom row: a pending invalid-action status
// message when one is set (#684), otherwise the row-action hint.
func (m *Model) footerLine(width int) string {
	if m.status != "" {
		return m.styles.ErrorText.Render(clip(m.status, width))
	}

	return m.hintLine(width)
}

// hintLine renders the bottom row-action hint: the per-row bindings plus
// apply/reset. These are page-local (not in keys.Map), so the global help bar
// never lists them; this line makes them discoverable (#655).
func (m *Model) hintLine(width int) string {
	// `x` is view-aware: it hides the revealed diff comparison in diff view, and
	// reveals the selected masked value in value view (see onReveal).
	xHint := "x reveal"
	if m.diffView {
		xHint = "x hide"
	}

	hint := "e edit · u unstage · t tags · " + xHint + " · enter detail · v view · a apply · r reset"

	return m.styles.PageHint.Render(clip(hint, width))
}

// headerLine renders the fixed top line — the view toggle and the global
// apply-all / reset-all / refresh affordances — and returns each affordance's
// clickable column range, so a header click reduces to the same action its key
// equivalent performs. The ranges come from the plain layout (byte == column for
// this ASCII line), so styling never shifts a hit range (color-safe).
func (m *Model) headerLine(width int) (string, headerRanges) {
	const (
		applyText   = "A apply-all"
		resetText   = "R reset-all"
		refreshText = "ctrl+r refresh"
	)

	toggleText := "view: " + bracket("diff", m.diffView) + " " + bracket("value", !m.diffView)
	actionsText := applyText + " · " + resetText + " · " + refreshText
	plain := toggleText + "   " + actionsText

	ranges := headerRanges{
		view:     [2]int{0, lipgloss.Width(toggleText)},
		applyAll: indexSpan(plain, applyText),
		resetAll: indexSpan(plain, resetText),
		refresh:  indexSpan(plain, refreshText),
	}

	toggle := "view: " + m.styles.PaneTitle.Render(bracket("diff", m.diffView)) + " " + bracket("value", !m.diffView)
	actions := m.styles.PageHint.Render(actionsText)

	return clip(toggle+"   "+actions, width), ranges
}

// indexSpan finds sub in plain and returns its [start,end) column range.
func indexSpan(plain, sub string) [2]int {
	start := strings.Index(plain, sub)
	if start < 0 {
		return [2]int{0, 0}
	}

	return [2]int{start, start + len(sub)}
}

// noticeLine renders the dismissible auto-unstaged notice.
func (m *Model) noticeLine(width int) string {
	keys := m.autoUnstaged()
	names := make([]string, 0, len(keys))

	for _, k := range keys {
		names = append(names, keyLabel(k))
	}

	text := "⚠ auto-unstaged: " + strings.Join(names, ", ") + " (staged value now equals remote)  esc: dismiss"

	return m.styles.Banner.Render(clip(text, width))
}

// bodyLines builds every section's lines (unclipped by scroll) plus their
// descriptors and the body-line index each selectable row starts on. The running
// rowIdx advances one per selectable row (an entry, or a single tag change), so
// each line's descriptor maps to the exact row a click selects.
func (m *Model) bodyLines(width int) ([]string, []lineDesc, []int) {
	var (
		lines    []string
		descs    []lineDesc
		rowFirst = make([]int, len(m.rows))
		rowIdx   int
	)

	add := func(text string, d lineDesc) {
		lines = append(lines, clip(text, width))
		descs = append(descs, d)
	}
	lineCount := func() int { return len(lines) }

	for i, sec := range m.sections {
		if i > 0 {
			add("", lineDesc{row: -1, section: -1})
		}

		header, applyR, resetR := m.sectionHeader(sec, width)
		add(header, lineDesc{row: -1, section: i, apply: applyR, reset: resetR})

		if sec.err != "" {
			add("  "+m.styles.ErrorText.Render(sec.err), lineDesc{row: -1, section: -1})

			continue
		}

		if !sec.loaded {
			add("  "+m.styles.PageHint.Render("loading…"), lineDesc{row: -1, section: -1})

			continue
		}

		rowIdx = m.appendSectionRows(sec, rowIdx, rowFirst, add, lineCount)
	}

	return lines, descs, rowFirst
}

// appendSectionRows appends a section's entry and tag rows, recording each
// selectable row's first body-line index (read from lineCount before its lines
// are added), and returns the running global row index.
func (m *Model) appendSectionRows(
	sec *section, rowIdx int, rowFirst []int, add func(string, lineDesc), lineCount func() int,
) int {
	entries := sec.entryRows()
	tags := sec.review.Tags

	if len(entries) == 0 && len(tags) == 0 {
		add("  "+m.styles.PageHint.Render("(nothing staged)"), lineDesc{row: -1, section: -1})

		return rowIdx
	}

	for _, e := range entries {
		rowFirst[rowIdx] = lineCount()

		for _, text := range m.entryLines(sec, e, rowIdx) {
			add(text, lineDesc{row: rowIdx, section: -1})
		}

		rowIdx++
	}

	for _, t := range tags {
		name := t.Name + nsBadge(sec, t.Namespace)

		for _, ad := range t.Adds {
			rowFirst[rowIdx] = lineCount()
			change := m.styles.DiffAdded.Render("+" + ad.Key + "=" + ad.Value)
			add(m.tagLine(rowIdx, name, change, "u unstage"), lineDesc{row: rowIdx, section: -1})
			rowIdx++
		}

		for _, rem := range t.Removes {
			rowFirst[rowIdx] = lineCount()

			change := m.styles.DiffRemoved.Render("−" + rem.Key)
			if rem.Value != "" {
				change += m.styles.PageHint.Render(" (was " + rem.Value + ")")
			}

			add(m.tagLine(rowIdx, name, change, "u unstage"), lineDesc{row: rowIdx, section: -1})
			rowIdx++
		}
	}

	return rowIdx
}

// entryLines renders one staged entry row (its header line plus, in diff view,
// the ± value lines). Every line belongs to the same selectable row.
func (m *Model) entryLines(sec *section, e data.StagedDiffRow, rowIdx int) []string {
	head := m.cursor(rowIdx) + m.opMarker(e.Operation) + "  " + e.Name + nsBadge(sec, e.Namespace)

	if e.Type == data.StagedDiffWarning {
		return []string{head, "    " + m.styles.ErrorText.Render("⚠ "+e.Warning)}
	}

	if !m.diffView {
		// Collapse to the first line (as diff view does): a multi-line staged
		// value must stay one physical row, else the body overflows its box and
		// the mouse hit-map (logical rows) desyncs from the screen rows. Full
		// values are viewable via enter → the diff detail page.
		value := firstLine(m.maskValue(e.StagedValue, sec.secret || e.Secret, rowIdx))
		if e.Operation == operationDelete {
			value = m.styles.PageHint.Render("(delete)")
		}

		return []string{head + "   " + value}
	}

	return append([]string{head}, m.diffLines(sec, e, rowIdx)...)
}

// diffLines renders an entry's Remote-vs-Staged ± lines. An update/delete is a
// real remote-vs-staged comparison, revealed by default so the change is
// meaningful (#735); `x` in diff view hides it (see maskDiffValue). A create has
// no remote to compare against — it is a lone new value, not a comparison — so
// it stays masked as a plain value (peekable per-row in value view), never
// revealed as part of the diff (#719).
func (m *Model) diffLines(sec *section, e data.StagedDiffRow, rowIdx int) []string {
	// A SecureString param row is secret even in a non-secret (param) section, so
	// OR the row's own flag into the section's (#677).
	secret := sec.secret || e.Secret

	if e.Operation == operationCreate {
		staged := firstLine(m.maskValue(e.StagedValue, secret, rowIdx))

		return []string{"    " + m.styles.DiffAdded.Render("+ "+staged)}
	}

	remote := firstLine(m.maskDiffValue(e.RemoteValue, secret))
	lines := []string{"    " + m.styles.DiffRemoved.Render("- "+remote)}

	if e.Operation != operationDelete {
		staged := firstLine(m.maskDiffValue(e.StagedValue, secret))
		lines = append(lines, "    "+m.styles.DiffAdded.Render("+ "+staged))
	}

	return lines
}

// tagLine renders one tag-change row: cursor, "tags", the item name, the change,
// and the removal affordance (`u`).
func (m *Model) tagLine(rowIdx int, name, change, unstage string) string {
	return m.cursor(rowIdx) + m.styles.FieldLabel.Render("tags") + "  " + name + "  " + change +
		"   " + m.styles.PageHint.Render(unstage)
}

// sectionHeader renders a section header and returns the apply/reset button
// column ranges (computed from the plain layout so hit-testing is color-safe).
func (m *Model) sectionHeader(sec *section, width int) (string, [2]int, [2]int) {
	title := sec.label + " (" + strconv.Itoa(sec.review.EntryCount()+sec.review.TagCount()) + ")"

	const applyText = "a apply"

	const resetText = "r reset"

	buttons := applyText + " · " + resetText
	gap := max(width-lipgloss.Width(title)-lipgloss.Width(buttons)-1, 1)
	plain := title + strings.Repeat(" ", gap) + buttons

	applyStart := strings.Index(plain, applyText)
	resetStart := strings.LastIndex(plain, resetText)

	styled := m.styles.PaneTitle.Render(title) + strings.Repeat(" ", gap) +
		m.styles.PageHint.Render(applyText+" · "+resetText)

	return clip(styled, width),
		[2]int{applyStart, applyStart + len(applyText)},
		[2]int{resetStart, resetStart + len(resetText)}
}

// opMarker renders a color-coded operation marker with a plain-text fallback
// under NO_COLOR.
func (m *Model) opMarker(op string) string {
	switch op {
	case operationCreate:
		return m.styles.DiffAdded.Render("create")
	case operationDelete:
		return m.styles.DiffRemoved.Render("delete")
	default:
		return m.styles.Banner.Render("update")
	}
}

// cursor renders the selection cursor for a row.
func (m *Model) cursor(rowIdx int) string {
	if rowIdx == m.selected {
		return m.styles.StatusValue.Render("▸ ")
	}

	return "  "
}

// clampScroll keeps the scroll offset in range and, when the selection just
// moved, scrolls so the selected row's first line is visible.
func (m *Model) clampScroll(total, bodyH int, rowFirst []int) {
	maxScroll := max(total-bodyH, 0)

	if m.scrollToSelection && m.selected < len(rowFirst) {
		first := rowFirst[m.selected]
		if first < m.scroll {
			m.scroll = first
		}

		if first >= m.scroll+bodyH {
			m.scroll = first - bodyH + 1
		}

		m.scrollToSelection = false
	}

	m.scroll = max(0, min(m.scroll, maxScroll))
}

// window returns the visible slice of body lines/descs for the scroll offset,
// padding with blanks so the body fills its height (keeping the layout stable).
func window(lines []string, descs []lineDesc, offset, height int) ([]string, []lineDesc) {
	if height <= 0 {
		return nil, nil
	}

	end := min(offset+height, len(lines))

	var (
		outLines []string
		outDescs []lineDesc
	)

	if offset < len(lines) {
		outLines = append(outLines, lines[offset:end]...)
		outDescs = append(outDescs, descs[offset:end]...)
	}

	for len(outLines) < height {
		outLines = append(outLines, "")
		outDescs = append(outDescs, lineDesc{row: -1, section: -1})
	}

	return outLines, outDescs
}

// bracket renders a "[on]"/"off" toggle option.
func bracket(label string, on bool) string {
	if on {
		return "[" + label + "]"
	}

	return label
}

// nsBadge renders an App Configuration namespace badge (empty for other
// providers/services).
func nsBadge(sec *section, namespace string) string {
	if !sec.svc.Capability().HasNamespaces {
		return ""
	}

	if namespace == "" {
		return " " + aznamespace.NullDisplay
	}

	return " [" + namespace + "]"
}

// keyLabel renders a staged key with its namespace badge for the notice.
func keyLabel(k data.StagedKey) string {
	if k.Namespace == "" {
		return k.Name
	}

	return k.Name + " [" + k.Namespace + "]"
}

// firstLine collapses a multi-line value to its first line with an ellipsis, so
// a diff row stays one line.
func firstLine(s string) string {
	if head, _, found := strings.Cut(s, "\n"); found {
		return head + " …"
	}

	return s
}

// clip clamps a (possibly styled) line to width display columns.
func clip(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
