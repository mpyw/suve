package browser

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

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
		m.onStagedLoaded(msg)

		return m, nil
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
		m.err = msg.err.Error()

		return nil
	}

	m.err = ""

	if msg.append {
		m.items = append(m.items, msg.res.Items...)
	} else {
		m.items = msg.res.Items
	}

	m.nextToken = msg.res.NextToken
	m.rebuildRows()

	return m.selectionCmd()
}

// onDetailLoaded applies a detail response and loads it into the value pane.
func (m *Model) onDetailLoaded(msg detailLoadedMsg) {
	if msg.seq != m.detailSeq {
		return
	}

	if msg.err != nil {
		m.err = msg.err.Error()
		m.detailOK = false

		return
	}

	m.detail = msg.d
	m.detailOK = true
	m.valuePane.SetValue(msg.d.Value, msg.d.Secret)
}

// onHistoryLoaded applies a history response. A history error is surfaced on the
// error line but never clears the already-loaded value.
func (m *Model) onHistoryLoaded(msg historyLoadedMsg) {
	if msg.seq != m.historySeq {
		return
	}

	if msg.err != nil {
		m.err = msg.err.Error()

		return
	}

	m.history.SetRows(historyEntries(m.styles, msg.rows, m.svcCap.TagsPerVersion))
	m.historyVersions = versionIDs(msg.rows)
}

// onStagedLoaded records the staged-key set and rebuilds the rows so badges
// appear. A probe error is non-fatal (badges simply do not show).
func (m *Model) onStagedLoaded(msg stagedLoadedMsg) {
	if msg.seq != m.stagedSeq || msg.err != nil {
		return
	}

	m.stagedKeys = msg.keys
	m.rebuildRows()
}

// onNamespacesLoaded merges discovered namespaces into the header filter,
// preserving the null option first and the all-namespaces option last.
func (m *Model) onNamespacesLoaded(msg namespacesLoadedMsg) {
	if msg.seq != m.nsSeq || msg.err != nil {
		return
	}

	// Preserve the currently-selected namespace VALUE across the rebuild, so an
	// inserted discovered namespace never silently changes what the current index
	// points at (#Step-3 review).
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
	}

	return false, nil
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

// openDiff opens the diff page for the two picked history versions.
func (m *Model) openDiff() tea.Cmd {
	i, j, ok := m.history.PickedVersions()
	if !ok {
		return nil
	}

	rows := m.currentHistoryVersions()
	if i >= len(rows) || j >= len(rows) {
		return nil
	}

	req := nav.OpenDiff{
		Source:     m.source,
		Name:       m.detail.Name,
		Namespace:  m.currentNamespace(),
		OldVersion: rows[i],
		NewVersion: rows[j],
		Secret:     m.detail.Secret,
	}

	return func() tea.Msg { return req }
}

// loadMore appends the next secret page when a NextToken is present.
func (m *Model) loadMore() tea.Cmd {
	if m.nextToken == "" {
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
