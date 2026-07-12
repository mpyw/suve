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
// discoverability surface here too. `x` is view-aware — it hides a revealed diff
// in diff view and reveals a masked value in value view (see onReveal) — so its
// label follows the current view. esc joins the map only while the auto-unstaged
// notice is showing, the sole thing esc dismisses here.
func (m *Model) HelpKeyMap() help.KeyMap {
	return keys.Bindings{Short: m.shortHelp(), Full: m.fullHelp()}
}

// xKey is the view-aware `x` binding: "hide" in diff view, "reveal" in value view.
func (m *Model) xKey() key.Binding {
	if m.diffView {
		return hideKey
	}

	return revealKey
}

// shortHelp is the one-line hint: move plus the most-used row actions.
func (m *Model) shortHelp() []key.Binding {
	return []key.Binding{moveKey, editKey, unstageKey, m.xKey(), detailKey, viewKey}
}

// fullHelp is the expanded reference: navigate / row actions / section+global.
func (m *Model) fullHelp() [][]key.Binding {
	navigate := []key.Binding{moveKey, detailKey}
	if m.noticeVisible() {
		navigate = append(navigate, m.keys.Back)
	}

	return [][]key.Binding{
		navigate,
		{editKey, unstageKey, tagKey, m.xKey()},
		{viewKey, applyKey, resetKey, applyAllKey, resetAllKey, refreshKey},
	}
}
