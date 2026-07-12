package browser

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/mpyw/suve/internal/tui/keys"
)

// HelpKeyMap reports the browser page's context-aware bindings for the help bar.
// Before this the bar showed only the global tab/help/quit, so the browser's
// page-local mutation keys — `e` edit above all — were invisible on screen
// (#681). The map adapts to focus (a focused filter shows only commit/cancel;
// the list and history panes advertise their own transitions) and gates each
// binding on capability (tags only with HasTags, restore only with HasRestore,
// load-more only with a next page, copy/reveal/parse-json only with a loaded
// detail), so the bar never lists a key that would do nothing here.
func (m *Model) HelpKeyMap() help.KeyMap {
	// A focused header field owns every keystroke (CapturesInput), so only the
	// commit transition is meaningful while editing.
	if m.focus == focusPrefix || m.focus == focusFilter {
		editing := []key.Binding{applyInputKey}

		return keys.Bindings{Short: editing, Full: [][]key.Binding{editing}}
	}

	return keys.Bindings{Short: m.shortHelp(), Full: m.fullHelp()}
}

// shortHelp is the one-line hint: focus-aware so the pane the arrow keys drive
// advertises its own transitions (the history pane's compare/diff/list, the
// list's edit/new/filter).
func (m *Model) shortHelp() []key.Binding {
	if m.focus == focusHistory {
		if m.history.Compare() {
			return []key.Binding{moveKey, spaceKey, diffPickKey, backListKey}
		}

		return []key.Binding{moveKey, backListKey, compareKey}
	}

	short := []key.Binding{moveKey, filterKey, editKey, newKey}
	if m.svcCap.HasVersionHistory {
		short = append(short, historyKey)
	}

	return short
}

// fullHelp is the expanded, column-grouped reference: navigate / filter+view /
// mutate / compare+staging, each column gated on the service's capability and
// the current load state so it lists only keys that act here.
func (m *Model) fullHelp() [][]key.Binding {
	return [][]key.Binding{
		m.navigateColumn(),
		m.viewColumn(),
		m.mutateColumn(),
		m.compareColumn(),
	}
}

// navigateColumn is the movement/selection group.
func (m *Model) navigateColumn() []key.Binding {
	col := []key.Binding{moveKey}

	if m.svcCap.HasVersionHistory {
		col = append(col, historyKey)
	}

	col = append(col, m.keys.Back)

	// Space picks a compare row (with history) or cycles the namespace filter
	// (App Config); it does nothing on a service with neither.
	if m.svcCap.HasNamespaces || m.svcCap.HasVersionHistory {
		col = append(col, spaceKey)
	}

	return col
}

// viewColumn is the list-shaping and value-display group.
func (m *Model) viewColumn() []key.Binding {
	col := []key.Binding{prefixKey, filterKey, valuesKey, recursiveKey}

	if m.detailOK {
		col = append(col, revealKey, parseJSONKey)
	}

	// Load-more only applies while a next page is pending (secret pagination).
	if m.nextToken != "" {
		col = append(col, loadMoreKey)
	}

	return col
}

// mutateColumn is the create/edit/delete/tag/restore/copy group.
func (m *Model) mutateColumn() []key.Binding {
	col := []key.Binding{newKey, editKey, deleteKey}

	if m.svcCap.HasTags {
		col = append(col, tagKey)
	}

	if m.svcCap.HasRestore {
		col = append(col, restoreKey)
	}

	// Copy only does something when a value is loaded to copy.
	if m.detailOK {
		col = append(col, m.keys.Copy)
	}

	return col
}

// compareColumn is the compare/staging-jump group.
func (m *Model) compareColumn() []key.Binding {
	var col []key.Binding

	if m.svcCap.HasVersionHistory {
		col = append(col, compareKey)
	}

	// The `S` jump only reaches a Staging tab when a staging seam is wired.
	if m.staging != nil {
		col = append(col, stagingKey)
	}

	return col
}
