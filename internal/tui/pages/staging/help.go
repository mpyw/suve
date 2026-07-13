package staging

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/mpyw/suve/internal/tui/keys"
)

// HelpKeyMap reports the staging page's bindings for the adaptive help bar. The
// page's row/section actions (`e`/`u`/`t`/`x`/enter/`v`/`a`/`r`…) used to live
// only in an in-body hint line because the global-only help bar could not show
// them (#655); now the bar renders them per page (#681), so they are the single
// discoverability surface here too.
//
// The bar adapts to BOTH the current view AND the selected row's kind so it lists
// exactly what the selected row supports and never advertises a no-op (#683):
//
//   - `x` is view-aware — it hides a revealed diff in diff view and reveals a
//     masked value in value view (see onReveal) — so its label follows the view.
//   - `e` edit, `enter` detail, `t` tags and `x` reveal/hide gate on the selected
//     row's kind, cross-checked against the update.go handlers, so a tag row or a
//     delete-staged entry never lists an action that would silently do nothing
//     there (editSelected/onEnter/tagSelected all return early on those rows).
//
// esc joins the map only while the auto-unstaged notice is showing, the sole
// thing esc dismisses here.
func (m *Model) HelpKeyMap() help.KeyMap {
	return keys.Bindings{Short: m.shortHelp(), Full: m.fullHelp()}
}

// xKey is the view-aware `x` binding: "hide" in diff view, "reveal" in value
// view. A create row is the exception in diff view — it renders as a lone
// masked value that `x` reveals per-row (#760), so its label reads "reveal".
func (m *Model) xKey() key.Binding {
	if m.diffView && !m.selectedIsCreate() {
		return hideKey
	}

	return revealKey
}

// Row-scoped action applicability, each mirroring the guard in the matching
// update.go handler so the help lists exactly what the selected row supports.
// When nothing is selected (an empty or not-yet-loaded page) they default to the
// entry-row set, leaving that pre-load state's help unchanged — the row-kind
// gating is scoped to a loaded, selected row (#683). `u` unstage is always
// listed: it is the page's signature action and applies to every row kind.

// canEditRow reports whether `e` edit acts on the selected row: only a staged
// create/update entry (editSelected no-ops on a tag row and a staged delete).
func (m *Model) canEditRow() bool {
	row, ok := m.selectedRow()

	return !ok || (row.kind == rowEntry && row.entry.Operation != operationDelete)
}

// canDetailRow reports whether `enter` opens detail for the selected row: only an
// entry row, any operation (onEnter no-ops on a tag row).
func (m *Model) canDetailRow() bool {
	row, ok := m.selectedRow()

	return !ok || row.kind == rowEntry
}

// canTagRow reports whether `t` tags the selected row's item: every row except a
// delete-staged entry, which tagSelected gates off (ErrCannotTagDelete, #684).
func (m *Model) canTagRow() bool {
	row, ok := m.selectedRow()

	return !ok || row.kind != rowEntry || row.entry.Operation != operationDelete
}

// canRevealRow reports whether `x` reveals/hides a value for the selected row.
// Only entry rows carry a value; on a tag row `x` has no row value to act on, so
// advertising its "reveal"/"hide" label there would be misleading.
func (m *Model) canRevealRow() bool {
	row, ok := m.selectedRow()

	return !ok || row.kind == rowEntry
}

// shortHelp is the one-line hint: move plus the most-used actions applicable to
// the selected row (unstage works on every row kind; edit / x / detail gate on
// the row).
func (m *Model) shortHelp() []key.Binding {
	short := []key.Binding{moveKey}

	if m.canEditRow() {
		short = append(short, editKey)
	}

	short = append(short, unstageKey)

	if m.canRevealRow() {
		short = append(short, m.xKey())
	}

	if m.canDetailRow() {
		short = append(short, detailKey)
	}

	return append(short, viewKey)
}

// fullHelp is the expanded reference: navigate / row actions / section+global,
// each column gated on the selected row's kind so it lists only keys that act
// here.
func (m *Model) fullHelp() [][]key.Binding {
	navigate := []key.Binding{moveKey}
	if m.canDetailRow() {
		navigate = append(navigate, detailKey)
	}

	if m.noticeVisible() {
		navigate = append(navigate, m.keys.Back)
	}

	return [][]key.Binding{
		navigate,
		m.rowActionsColumn(),
		{viewKey, applyKey, resetKey, applyAllKey, resetAllKey, refreshKey},
	}
}

// rowActionsColumn is the row-action group for the selected row: unstage on every
// row; edit / tag / x gated to the rows their handlers actually act on.
func (m *Model) rowActionsColumn() []key.Binding {
	var col []key.Binding

	if m.canEditRow() {
		col = append(col, editKey)
	}

	col = append(col, unstageKey)

	if m.canTagRow() {
		col = append(col, tagKey)
	}

	if m.canRevealRow() {
		col = append(col, m.xKey())
	}

	return col
}
