package browser

import (
	"errors"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
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
	case listLoadedMsg:
		return m, m.onListLoaded(msg)
	case detailLoadedMsg:
		m.onDetailLoaded(msg)

		return m, nil
	case historyLoadedMsg:
		m.onHistoryLoaded(msg)

		return m, nil
	case stagedLoadedMsg:
		return m, m.onStagedLoaded(msg)
	case nav.Reload:
		return m, m.reload()
	case namespacesLoadedMsg:
		m.onNamespacesLoaded(msg)

		return m, nil
	case debounceMsg:
		if msg.seq != m.debounceSeq {
			return m, nil // superseded by a later edit
		}

		return m, m.loadListCmd(false)
	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
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

// onListLoaded applies a list response when it is not stale, rebuilds the rows,
// and (re)loads the selection's detail/history.
func (m *Model) onListLoaded(msg listLoadedMsg) tea.Cmd {
	if msg.seq != m.listSeq {
		return nil // stale response — a newer list load supersedes it
	}

	m.loading = false

	if msg.err != nil {
		m.listErr = msg.err.Error()

		return nil
	}

	m.listErr = ""

	// Preserve the selection by IDENTITY, not index. A replace can insert or
	// remove rows above the selection, so capture the selected entry's key first
	// and re-resolve it to its new index after the rows rebuild — otherwise the
	// clamped index silently slides the detail onto a neighbor after a mutation
	// reload (the GUI tracks selection by name; #699). On append the existing
	// rows keep their indices, so the selection index is already correct.
	prevKey, hadSelection := m.selectedKey()

	if msg.append {
		m.items = append(m.items, msg.res.Items...)
	} else {
		m.items = msg.res.Items
	}

	m.nextToken = msg.res.NextToken
	m.rebuildRows()

	if !msg.append && hadSelection {
		m.reselect(prevKey)
	}

	return m.selectionCmd()
}

// onDetailLoaded applies a detail response and loads it into the value pane. A
// successful load clears the detail error so a prior transient failure never
// lingers over the freshly-loaded value.
func (m *Model) onDetailLoaded(msg detailLoadedMsg) {
	if msg.seq != m.detailSeq {
		return
	}

	if msg.err != nil {
		m.detailErr = msg.err.Error()
		m.detailOK = false

		return
	}

	m.detailErr = ""
	m.detail = msg.d
	m.detailOK = true
	m.valuePane.SetValue(msg.d.Value, msg.d.Secret)
}

// onHistoryLoaded applies a history response. A history error is surfaced on its
// own error line (never clearing the already-loaded value); a successful load
// clears it so a prior transient failure never lingers over good history.
func (m *Model) onHistoryLoaded(msg historyLoadedMsg) {
	if msg.seq != m.historySeq {
		return
	}

	if msg.err != nil {
		m.historyErr = msg.err.Error()

		return
	}

	m.historyErr = ""
	m.history.SetRows(historyEntries(m.styles, msg.rows, m.svcCap.TagsPerVersion))
	m.historyVersions = versionIDs(msg.rows)
}

// onStagedLoaded records the staged-key set, rebuilds the rows so badges appear,
// and reports the staged count to the app for the Staging tab badge. An ordinary
// probe read error is non-fatal (badges simply do not show), but a
// store-construction hard-fail (a staging key-loss while encrypted state exists)
// is surfaced on the error line so a launch-time key-loss is visible on the read
// path, not only when the user attempts a write.
func (m *Model) onStagedLoaded(msg stagedLoadedMsg) tea.Cmd {
	if msg.seq != m.stagedSeq {
		return nil
	}

	if msg.err != nil {
		var storeErr *data.StoreUnavailableError
		if errors.As(msg.err, &storeErr) {
			m.stagedErr = storeErr.Error()
		}

		return nil
	}

	m.stagedErr = ""
	m.stagedKeys = msg.keys
	m.rebuildRows()

	service := m.svcCap.Service
	count := len(msg.keys)

	return func() tea.Msg { return nav.StagedCount{Service: service, Count: count} }
}

// errLines returns the active error lines in a stable order — list, detail,
// history, then the staging-store hard-fail — each owned by its own source so a
// transient failure clears when that source next succeeds without masking or
// lingering over another source's state.
func (m *Model) errLines() []string {
	lines := make([]string, 0, 4) //nolint:mnd // the four error sources

	for _, e := range []string{m.listErr, m.detailErr, m.historyErr, m.stagedErr} {
		if e != "" {
			lines = append(lines, e)
		}
	}

	return lines
}

// reload re-fetches the list and staged flags after a mutation (the app forwards
// nav.Reload). The list load re-issues the selection's detail/history on landing,
// so the value and badges reflect the write.
func (m *Model) reload() tea.Cmd {
	cmds := []tea.Cmd{m.loadListCmd(false)}
	if m.staging != nil {
		cmds = append(cmds, m.loadStagedCmd())
	}

	return tea.Batch(cmds...)
}

// onNamespacesLoaded merges discovered namespaces into the header filter,
// preserving the null option first and the all-namespaces option last.
func (m *Model) onNamespacesLoaded(msg namespacesLoadedMsg) {
	if msg.seq != m.nsSeq || msg.err != nil {
		return
	}

	// Preserve the currently-selected namespace VALUE across the rebuild, so an
	// inserted discovered namespace never silently changes what the current index
	// points at.
	current := m.currentNamespace()
	m.namespaces = namespaceOptions(msg.names)

	m.nsIndex = 0
	for i, ns := range m.namespaces {
		if ns == current {
			m.nsIndex = i

			break
		}
	}
}

// CapturesInput reports whether a header text input (prefix or filter) is
// focused. While it is, the app forwards raw keystrokes here instead of applying
// its global key map, so a `q`/`1`/`y` typed into the filter is text, not a quit
// or tab jump.
func (m *Model) CapturesInput() bool {
	return m.focus == focusPrefix || m.focus == focusFilter
}

// handleKey routes a key: to a focused text input when editing, else to the
// page-local bindings, else to the focused list/history widget.
func (m *Model) handleKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	if m.focus == focusPrefix || m.focus == focusFilter {
		return m.handleInputKey(msg)
	}

	if handled, cmd := m.handleActionKey(msg); handled {
		return m, cmd
	}

	return m.handleNavKey(msg)
}

// handleInputKey drives the focused text input; Enter/Esc commit it and reload.
func (m *Model) handleInputKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Select), key.Matches(msg, m.keys.Back):
		m.blurInputs()

		return m, m.loadListCmd(false)
	}

	var cmd tea.Cmd
	if m.focus == focusPrefix {
		m.prefix, cmd = m.prefix.Update(msg)
	} else {
		m.filter, cmd = m.filter.Update(msg)
	}

	// Debounce the reload so a burst of keystrokes issues one list load.
	return m, tea.Batch(cmd, m.debounce())
}

// handleActionKey handles the page-local action bindings. It returns handled=true
// when it claimed the key.
func (m *Model) handleActionKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch {
	case key.Matches(msg, prefixKey):
		m.focus = focusPrefix
		m.prefix.Focus()

		return true, nil
	case key.Matches(msg, filterKey):
		m.focus = focusFilter
		m.filter.Focus()

		return true, nil
	case key.Matches(msg, valuesKey):
		m.valuesOn = !m.valuesOn

		return true, m.loadListCmd(false)
	case key.Matches(msg, recursiveKey):
		// Param supports recursive listing; elsewhere `r` is a plain refresh.
		if m.svcCap.Service == "param" && !m.svcCap.HasNamespaces {
			m.recursive = !m.recursive
		}

		return true, m.loadListCmd(false)
	case key.Matches(msg, loadMoreKey):
		return true, m.loadMore()
	case key.Matches(msg, revealKey):
		m.valuePane.ToggleMask()

		return true, nil
	case key.Matches(msg, stagingKey):
		return true, func() tea.Msg { return nav.OpenStaging{} }
	case key.Matches(msg, compareKey):
		m.toggleCompare()

		return true, nil
	case key.Matches(msg, spaceKey):
		return true, m.handleSpace()
	case key.Matches(msg, newKey):
		return true, m.openNew()
	case key.Matches(msg, editKey):
		return true, m.openEdit()
	case key.Matches(msg, deleteKey):
		return true, m.openDelete()
	case key.Matches(msg, tagKey):
		return true, m.openTag()
	case key.Matches(msg, restoreKey):
		return true, m.openRestore()
	}

	return false, nil
}

// openNew requests the create dialog. Creating while viewing all/multiple App
// Configuration namespaces is blocked (a write targets one concrete namespace —
// GUI parity); the browser surfaces the block as an error dialog.
func (m *Model) openNew() tea.Cmd {
	if m.svcCap.HasNamespaces && m.currentNamespace() == aznamespace.AllNamespacesFilter {
		return func() tea.Msg {
			return nav.OpenError{
				Title:   "Cannot create here",
				Message: "Select a single namespace (not *) before creating a setting.",
			}
		}
	}

	return func() tea.Msg {
		return nav.OpenEntryForm{Service: m.svcCap.Service, Namespace: m.currentNamespace()}
	}
}

// openEdit requests the edit dialog, seeded from the loaded detail (value/type/
// description). It is a no-op until a detail is loaded.
func (m *Model) openEdit() tea.Cmd {
	if !m.detailOK {
		return nil
	}

	req := nav.OpenEntryForm{
		Service:     m.svcCap.Service,
		Edit:        true,
		Name:        m.detail.Name,
		Namespace:   m.detail.Namespace,
		Value:       m.detail.Value,
		TypeLabel:   m.detail.TypeLabel,
		Description: m.detail.Description,
	}

	return func() tea.Msg { return req }
}

// openDelete requests the delete-confirm dialog for the selected entry.
func (m *Model) openDelete() tea.Cmd {
	item, ok := m.selectedItem()
	if !ok {
		return nil
	}

	return func() tea.Msg {
		return nav.OpenDelete{Service: m.svcCap.Service, Name: item.Name, Namespace: item.Namespace}
	}
}

// openTag requests the tag add/remove dialog for the selected entry (only when
// the service supports tags).
func (m *Model) openTag() tea.Cmd {
	if !m.svcCap.HasTags {
		return nil
	}

	item, ok := m.selectedItem()
	if !ok {
		return nil
	}

	return func() tea.Msg {
		return nav.OpenTag{Service: m.svcCap.Service, Name: item.Name, Namespace: item.Namespace}
	}
}

// openRestore requests the restore dialog (name input), only when the service
// supports restoring soft-deleted entries. The selected entry seeds the name.
func (m *Model) openRestore() tea.Cmd {
	if !m.svcCap.HasRestore {
		return nil
	}

	var name string
	if item, ok := m.selectedItem(); ok {
		name = item.Name
	}

	return func() tea.Msg {
		return nav.OpenRestore{Service: m.svcCap.Service, Name: name}
	}
}

// handleNavKey drives the focused list/history widget and the enter/esc
// focus transitions.
func (m *Model) handleNavKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		return m, m.move(-1)
	case key.Matches(msg, m.keys.Down):
		return m, m.move(1)
	case key.Matches(msg, m.keys.Select):
		return m, m.onSelect()
	case key.Matches(msg, m.keys.Back):
		m.onBack()

		return m, nil
	}

	return m, nil
}

// move shifts the focused widget's selection; moving the list reloads the
// selection's detail/history.
func (m *Model) move(delta int) tea.Cmd {
	if m.focus == focusHistory {
		m.history.Move(delta)

		return nil
	}

	m.list.Move(delta)

	return m.selectionCmd()
}

// onSelect: from the list it moves focus into the history; from the history in
// compare mode with two picks it opens the diff page.
func (m *Model) onSelect() tea.Cmd {
	if m.focus == focusList {
		if m.svcCap.HasVersionHistory && m.history.Len() > 0 {
			m.focus = focusHistory
		}

		return nil
	}

	return m.openDiff()
}

// onBack: from the history it returns focus to the list (clearing compare);
// from the list it is a no-op (the app owns page-stack pops).
func (m *Model) onBack() {
	if m.focus == focusHistory {
		m.history.SetCompare(false)
		m.focus = focusList
	}
}

// toggleCompare flips compare mode and moves focus into the history.
func (m *Model) toggleCompare() {
	if !m.svcCap.HasVersionHistory || m.history.Len() == 0 {
		return
	}

	m.focus = focusHistory
	m.history.SetCompare(!m.history.Compare())
}

// handleSpace picks a compare row in the history, or cycles the App Config
// namespace filter (the two are mutually exclusive: App Config has no history).
func (m *Model) handleSpace() tea.Cmd {
	if m.svcCap.HasNamespaces {
		return m.cycleNamespace()
	}

	if m.focus == focusHistory && m.history.Compare() {
		m.history.TogglePick()
	}

	return nil
}

// openDiff opens the diff page for the two picked history versions, ordered
// chronologically (older → newer) regardless of pick order. History rows are in
// display order (newest first, index 0), so the HIGHER index is the older
// version and becomes OldVersion; the lower index is the newer NewVersion. This
// keeps the diff reading old → new even when the user picks newest first.
func (m *Model) openDiff() tea.Cmd {
	i, j, ok := m.history.PickedVersions()
	if !ok {
		return nil
	}

	rows := m.currentHistoryVersions()
	if i >= len(rows) || j >= len(rows) {
		return nil
	}

	oldIdx, newIdx := max(i, j), min(i, j)

	req := nav.OpenDiff{
		Source:     m.source,
		Name:       m.detail.Name,
		Namespace:  m.currentNamespace(),
		OldVersion: rows[oldIdx],
		NewVersion: rows[newIdx],
	}

	return func() tea.Msg { return req }
}

// loadMore appends the next secret page when a NextToken is present. It is a
// no-op while a list fetch is already in flight: m.loading is set by loadListCmd
// for BOTH a full reload and a previous append, so a single guard mirrors the
// GUI's `loading || loadingMore` check and stops a hammered `L` from splicing a
// duplicate or stale page (#700). The stale-seq guard in onListLoaded is the
// backstop; this keeps a superseded append from ever being issued.
func (m *Model) loadMore() tea.Cmd {
	if m.nextToken == "" || m.loading {
		return nil
	}

	return m.loadListCmd(true)
}

// debounce schedules a settled reload for the current edit sequence.
func (m *Model) debounce() tea.Cmd {
	m.debounceSeq++
	seq := m.debounceSeq

	return tea.Tick(debounceDelay, func(time.Time) tea.Msg {
		return debounceMsg{seq: seq}
	})
}

// blurInputs leaves any focused text input and returns focus to the list. It
// advances debounceSeq so a still-pending debounce tick from the last keystroke
// is invalidated — the immediate commit reload supersedes it (no duplicate load).
func (m *Model) blurInputs() {
	m.prefix.Blur()
	m.filter.Blur()
	m.focus = focusList
	m.debounceSeq++
}

// cycleNamespace advances the App Config namespace filter and reloads the list.
func (m *Model) cycleNamespace() tea.Cmd {
	if len(m.namespaces) == 0 {
		return nil
	}

	m.nsIndex = (m.nsIndex + 1) % len(m.namespaces)

	return m.loadListCmd(false)
}

// currentNamespace returns the active App Config namespace filter value ("" for
// the null namespace or non-App-Config providers).
func (m *Model) currentNamespace() string {
	if !m.svcCap.HasNamespaces || m.nsIndex >= len(m.namespaces) {
		return ""
	}

	return m.namespaces[m.nsIndex]
}

// currentHistoryVersions returns the raw version identifiers in current display
// order, so a picked row index maps to its version.
func (m *Model) currentHistoryVersions() []string { return m.historyVersions }

// CopyText reveals the detail value pane (so a masked secret is not copied
// silently) and returns the revealed value for the clipboard. The app calls this
// for the global `y` copy; false means there is nothing to copy.
func (m *Model) CopyText() (string, bool) {
	if !m.detailOK {
		return "", false
	}

	m.valuePane.Reveal()

	v := m.valuePane.RevealedValue()
	if v == "" {
		return "", false
	}

	return v, true
}

// dataStagedKey builds the staged-key lookup for an item.
func dataStagedKey(it data.Item) data.StagedKey {
	return data.StagedKey{Name: it.Name, Namespace: it.Namespace}
}
