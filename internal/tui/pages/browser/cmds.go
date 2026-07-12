package browser

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
)

// Async result messages. Each carries the sequence its fetch was issued with, so
// the reducer can drop a stale response (a fetch that a newer one superseded).
type (
	listLoadedMsg struct {
		seq    int
		append bool
		res    data.ListResult
		err    error
	}
	detailLoadedMsg struct {
		seq int
		d   data.Detail
		err error
	}
	historyLoadedMsg struct {
		seq  int
		rows []data.HistoryRow
		err  error
	}
	stagedLoadedMsg struct {
		seq  int
		snap data.StagingSnapshot
		err  error
	}
	namespacesLoadedMsg struct {
		seq   int
		names []string
		err   error
	}
	// debounceMsg fires after a prefix/filter edit settles; a reload runs only
	// when its seq is still the latest edit.
	debounceMsg struct{ seq int }
)

// listParams snapshots the header state into a data.ListParams.
func (m *Model) listParams() data.ListParams {
	return data.ListParams{
		Prefix:    m.prefix.Value(),
		Filter:    m.filter.Value(),
		Recursive: m.recursive,
		WithValue: m.valuesOn,
		Namespace: m.currentNamespace(),
	}
}

// loadListCmd issues a list fetch guarded by a fresh listSeq. appendPage is
// reserved for secret NextToken paging (append rather than replace).
func (m *Model) loadListCmd(appendPage bool) tea.Cmd {
	m.listSeq++
	m.loading = true
	seq := m.listSeq
	ctx := m.ctx
	source := m.source
	params := m.listParams()

	return func() tea.Msg {
		res, err := source.List(ctx, params)

		return listLoadedMsg{seq: seq, append: appendPage, res: res, err: err}
	}
}

// loadDetailCmd issues a detail (Show) fetch for name/namespace.
func (m *Model) loadDetailCmd(name, namespace string) tea.Cmd {
	m.detailSeq++
	seq := m.detailSeq
	ctx := m.ctx
	source := m.source

	return func() tea.Msg {
		d, err := source.Show(ctx, name, namespace)

		return detailLoadedMsg{seq: seq, d: d, err: err}
	}
}

// loadHistoryCmd issues a history (Log) fetch, independent of the detail fetch
// so a history failure never blanks the value (GUI Promise.allSettled parity).
func (m *Model) loadHistoryCmd(name, namespace string) tea.Cmd {
	m.historySeq++
	seq := m.historySeq
	ctx := m.ctx
	source := m.source

	return func() tea.Msg {
		rows, err := source.History(ctx, name, namespace)

		return historyLoadedMsg{seq: seq, rows: rows, err: err}
	}
}

// loadStagedCmd probes which items have staged changes.
func (m *Model) loadStagedCmd() tea.Cmd {
	m.stagedSeq++
	seq := m.stagedSeq
	ctx := m.ctx
	probe := m.staging

	return func() tea.Msg {
		snap, err := probe.Staged(ctx)

		return stagedLoadedMsg{seq: seq, snap: snap, err: err}
	}
}

// loadNamespacesCmd discovers App Configuration namespaces for the header filter.
func (m *Model) loadNamespacesCmd() tea.Cmd {
	m.nsSeq++
	seq := m.nsSeq
	ctx := m.ctx
	source := m.source

	return func() tea.Msg {
		names, err := source.Namespaces(ctx)

		return namespacesLoadedMsg{seq: seq, names: names, err: err}
	}
}

// selectionCmd loads the detail and history for the currently-selected item, as
// two independent fetches. It clears the per-source detail/history errors up
// front (the single selection funnel, mirroring the GUI clearing its error at the
// start of selectParam) so a stale error from the previous entry never lingers
// over the new selection's loads.
func (m *Model) selectionCmd() tea.Cmd {
	m.detailErr = ""
	m.historyErr = ""

	item, ok := m.selectedItem()
	if !ok {
		m.detailOK = false

		return nil
	}

	return tea.Batch(
		m.loadDetailCmd(item.Name, item.Namespace),
		m.loadHistoryCmd(item.Name, item.Namespace),
	)
}

// selectedItem returns the item under the list's selection by INDEX, not name:
// rebuildRows builds the rows one-to-one from m.items in order, so the selected
// index maps back to its item even when App Configuration lists the same key
// under several namespaces (a name lookup would resolve every duplicate to the
// first, loading the wrong namespace).
func (m *Model) selectedItem() (data.Item, bool) {
	idx := m.list.Selected()
	if idx < 0 || idx >= len(m.items) {
		return data.Item{}, false
	}

	return m.items[idx], true
}

// selectedKey returns the identity (name+namespace) of the currently-selected
// item, so a reload can re-resolve the SAME entry to its new index rather than
// trusting the clamped index (#699).
func (m *Model) selectedKey() (data.StagedKey, bool) {
	item, ok := m.selectedItem()
	if !ok {
		return data.StagedKey{}, false
	}

	return dataStagedKey(item), true
}

// reselect moves the selection to the row whose identity matches key, called
// after a rebuild so an insert/remove above the previous selection keeps the
// detail on the same entry. When the entry is gone (e.g. it was the one just
// deleted), it leaves the clamped selection SetRows already produced —
// selectionCmd then loads whatever is now selected, or clears the detail when
// the list is empty (the graceful, GUI-parity fallback).
func (m *Model) reselect(key data.StagedKey) {
	for i, it := range m.items {
		if dataStagedKey(it) == key {
			m.list.SelectIndex(i)

			return
		}
	}
}
