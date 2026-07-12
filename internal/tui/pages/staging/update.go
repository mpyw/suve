package staging

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/nav"
)

// Update handles forwarded messages. It returns itself as the page (the app
// stores it back on the stack).
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

		return m, nil
	case reviewLoadedMsg:
		return m, m.onReviewLoaded(msg)
	case actionDoneMsg:
		return m, m.onActionDone(msg)
	case nav.Reload:
		return m, m.reload()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	default:
		return m, nil
	}
}

// onReviewLoaded applies a staged-review response when fresh, rebuilds the rows,
// and reports the section's staged count as entries + tag changes (counted
// separately). The browser's staged probe reports the same entries+tags total,
// so the Staging tab badge reads one consistent count from either surface (#693).
func (m *Model) onReviewLoaded(msg reviewLoadedMsg) tea.Cmd {
	if msg.section >= len(m.sections) {
		return nil
	}

	sec := m.sections[msg.section]
	if msg.seq != sec.loadSeq {
		return nil // stale response superseded by a newer load
	}

	sec.loaded = true

	if msg.err != nil {
		sec.err = msg.err.Error()
		sec.review = data.StagingReview{}
	} else {
		sec.err = ""
		sec.review = msg.review
	}

	m.rebuildRows()

	service := sec.service
	count := sec.review.EntryCount() + sec.review.TagCount()

	return func() tea.Msg { return nav.StagedCount{Service: service, Count: count} }
}

// onActionDone clears the busy guard and, on success, reloads so the section and
// the badges reflect the change; an error is surfaced on the section line.
func (m *Model) onActionDone(msg actionDoneMsg) tea.Cmd {
	m.actionBusy = false

	if msg.err != nil {
		if msg.section < len(m.sections) {
			m.sections[msg.section].err = msg.err.Error()
		}

		return nil
	}

	return m.reload()
}

// handleKey routes a key press to a page action.
func (m *Model) handleKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	// Clear the transient invalid-action status on any key; the action below
	// re-sets it when the pressed key is itself invalid for the selected row.
	m.status = ""

	// The auto-unstaged notice dismisses on esc while it is showing.
	if key.Matches(msg, m.keys.Back) && m.noticeVisible() {
		m.noticeDismissed = true

		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Up):
		m.moveSelection(-1)

		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.moveSelection(1)

		return m, nil
	case key.Matches(msg, viewKey):
		return m, m.toggleView()
	case key.Matches(msg, m.keys.Select):
		return m, m.onEnter()
	case key.Matches(msg, revealKey):
		m.onReveal()

		return m, nil
	}

	return m.handleActionKey(msg)
}

// handleActionKey handles the row/section action keys.
func (m *Model) handleActionKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	switch {
	case key.Matches(msg, unstageKey):
		return m, m.unstageSelected()
	case key.Matches(msg, editKey):
		return m, m.editSelected()
	case key.Matches(msg, tagKey):
		return m, m.tagSelected()
	case key.Matches(msg, applyKey):
		return m, m.apply(false)
	case key.Matches(msg, applyAllKey):
		return m, m.apply(true)
	case key.Matches(msg, resetKey):
		return m, m.reset(false)
	case key.Matches(msg, resetAllKey):
		return m, m.reset(true)
	case key.Matches(msg, refreshKey):
		return m, m.reload()
	}

	return m, nil
}

// toggleView flips the diff/value view (the `v` key and a click on the header
// view toggle), resetting the reveal.
//
// Toggling diff↔value rewrites every row's layout (diff spreads a value over
// ±lines; value packs it onto the header line). Bubble Tea's cell renderer would
// service that with a scroll-region optimization (ESC[…r + ESC[nS) whose result
// depends on the exact prior frame, so under a terminal that negotiates
// synchronized output differently (CI) the targeted cell writes land wrong and
// the staged values render empty. Force a full repaint so the toggle frame is
// environment-independent, exactly like the initial full paint.
func (m *Model) toggleView() tea.Cmd {
	m.diffView = !m.diffView
	m.reveal = false
	m.diffHidden = false

	return tea.ClearScreen
}

// moveSelection shifts the selection by delta (clamped) and keeps it visible.
// It resets the reveal so a peek never persists across a selection move — the
// reveal is scoped to the row that was selected when `x` was pressed, mirroring
// the browser's per-entry reveal (#694).
func (m *Model) moveSelection(delta int) {
	if len(m.rows) == 0 {
		return
	}

	m.selected = max(0, min(m.selected+delta, len(m.rows)-1))
	m.scrollToSelection = true
	m.reveal = false
}

// onReveal toggles secret masking with `x`, view-aware:
//
//   - In DIFF view the remote-vs-staged comparison is revealed by default (the
//     surface the user explicitly opened to inspect the change, #735); `x`
//     toggles a page-level HIDE so the diff can still be masked.
//   - In VALUE view the raw staged value is masked by default; `x` unmasks only
//     the SELECTED row's value (never page-global), and moving the selection
//     resets it (#694).
//
// It never removes a staged change — that is `u`'s job on every row kind
// (including tag rows) — so a user who learned `x` never destroys a staged
// change by peeking (#682).
func (m *Model) onReveal() {
	if m.diffView {
		m.diffHidden = !m.diffHidden

		return
	}

	m.reveal = !m.reveal
}

// onEnter opens the full-diff detail for an entry row. On a tag row it is a
// no-op: enter is detail-only, never a destructive cancel (#682). Removing a
// staged tag change is `u`'s job (see unstageSelected).
func (m *Model) onEnter() tea.Cmd {
	row, ok := m.selectedRow()
	if !ok || row.kind != rowEntry {
		return nil
	}

	return m.openDetail(row)
}

// unstageSelected removes the selected row's staged change: for an entry row the
// whole entry (and its tags); for a tag row that single tag add/removal. `u` is
// the one removal affordance across all row kinds — `x` reveals and enter shows
// detail, neither ever removes (#682).
func (m *Model) unstageSelected() tea.Cmd {
	row, ok := m.selectedRow()
	if !ok || m.actionBusy {
		return nil
	}

	m.actionBusy = true

	switch row.kind {
	case rowTagAdd:
		return m.cancelAddTagCmd(row.section, row.key, row.tagKey)
	case rowTagRemove:
		return m.cancelRemoveTagCmd(row.section, row.key, row.tagKey)
	default: // rowEntry
		return m.unstageCmd(row.section, row.key)
	}
}

// editSelected reuses the mutation entry form to edit a staged create/update's
// value; it is a no-op on a tag row or a staged delete (nothing to edit). The
// form is opened staged-only (no immediate-mode escape hatch): this is a staged
// surface, so an immediate write would bypass the staging store and orphan the
// staged draft.
func (m *Model) editSelected() tea.Cmd {
	row, ok := m.selectedRow()
	if !ok || row.kind != rowEntry || row.entry.Operation == operationDelete {
		return nil
	}

	sec := m.sections[row.section]

	return func() tea.Msg {
		return nav.OpenEntryForm{
			Service:    sec.service,
			Edit:       true,
			Name:       row.entry.Name,
			Namespace:  row.entry.Namespace,
			Value:      row.entry.StagedValue,
			StagedOnly: true,
		}
	}
}

// tagSelected reuses the tag form to stage a tag add on the selected row's item,
// opened staged-only (no immediate-mode escape hatch) for the same reason as
// editSelected: this is a staged surface. Tagging a delete-staged entry is a
// statically impossible transition (the reducer returns ErrCannotTagDelete), so
// it is gated off at the affordance — mirroring editSelected and the GUI's
// hidden "+ Add Tag" — with a one-line status message instead of a guaranteed
// dead-end form (#684).
func (m *Model) tagSelected() tea.Cmd {
	row, ok := m.selectedRow()
	if !ok {
		return nil
	}

	if row.kind == rowEntry && row.entry.Operation == operationDelete {
		m.status = "cannot tag: staged for deletion — reset first"

		return nil
	}

	sec := m.sections[row.section]

	return func() tea.Msg {
		return nav.OpenTag{
			Service: sec.service, Name: row.key.Name, Namespace: row.key.Namespace, StagedOnly: true,
		}
	}
}

// openDetail pushes the full remote-vs-staged diff for an entry row.
func (m *Model) openDetail(row rowRef) tea.Cmd {
	sec := m.sections[row.section]

	req := nav.OpenStagingDetail{
		Title:    row.entry.Name,
		OldLabel: "remote",
		NewLabel: "staged",
		OldValue: row.entry.RemoteValue,
		NewValue: row.entry.StagedValue,
		// A SecureString param row is secret even in a non-secret section (#677).
		Secret: sec.secret || row.entry.Secret,
	}

	return func() tea.Msg { return req }
}

// apply opens the apply confirmation for the selected section (global=false) or
// every section (global=true).
func (m *Model) apply(global bool) tea.Cmd {
	return m.applyServices(m.targetServices(global), global)
}

// applyServices opens the apply confirmation for a set of services, carrying the
// staged counts for the confirmation. A key press and a click on a section's
// apply button both reduce to this, so mouse and keyboard stay in lockstep.
func (m *Model) applyServices(services []string, global bool) tea.Cmd {
	if len(services) == 0 {
		return nil
	}

	entries, tags := m.totalCounts(serviceSet(services))
	if entries == 0 && tags == 0 {
		return nil
	}

	req := nav.OpenApply{Services: services, Global: global, EntryCount: entries, TagCount: tags}

	return func() tea.Msg { return req }
}

// reset opens the reset confirmation for the selected section or every section.
func (m *Model) reset(global bool) tea.Cmd {
	return m.resetServices(m.targetServices(global), global)
}

// resetServices opens the reset confirmation for a set of services (the reduction
// target for both the reset keys and a click on a section's reset button).
func (m *Model) resetServices(services []string, global bool) tea.Cmd {
	if len(services) == 0 {
		return nil
	}

	req := nav.OpenReset{Services: services, Global: global}

	return func() tea.Msg { return req }
}

// targetServices resolves the service keys an apply/reset targets: every section
// for a global action, else just the selected row's section.
func (m *Model) targetServices(global bool) []string {
	if global {
		services := make([]string, 0, len(m.sections))
		for _, sec := range m.sections {
			services = append(services, sec.service)
		}

		return services
	}

	if len(m.sections) == 0 {
		return nil
	}

	return []string{m.sections[m.selectedSection()].service}
}

// serviceSet builds a set of the given service keys.
func serviceSet(services []string) map[string]bool {
	set := make(map[string]bool, len(services))
	for _, s := range services {
		set[s] = true
	}

	return set
}

// noticeVisible reports whether the auto-unstaged notice is currently shown.
func (m *Model) noticeVisible() bool {
	return !m.noticeDismissed && len(m.autoUnstaged()) > 0
}

// maskValue masks a secret value for rendering unless the reveal is on AND the
// row is the selected one. The reveal is scoped to the selected row (and reset
// on a selection move), so pressing `x` unmasks only that row's value rather
// than every staged secret across all sections (#694).
func (m *Model) maskValue(value string, secret bool, rowIdx int) string {
	revealed := m.reveal && rowIdx == m.selected
	if secret && !revealed {
		return components.MaskValue(value)
	}

	return value
}

// maskDiffValue masks a secret value for the DIFF view. The diff view is shown
// revealed by default so the remote-vs-staged change is meaningful (#735); it is
// masked only while the page-level hide is on (`x` in diff view). Unlike
// maskValue this is page-level, not per-selected-row: the whole diff view masks
// or reveals together.
func (m *Model) maskDiffValue(value string, secret bool) string {
	if secret && m.diffHidden {
		return components.MaskValue(value)
	}

	return value
}
