//nolint:testpackage // white-box: exercises the unexported reducer, messages, and geometry
package browser

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/styles"
)

// stubSource is a controllable data.Source for Update-layer tests: it returns
// preset results and never touches a cloud. The golden tests exercise the real
// providermock-backed source (see the tui package).
type stubSource struct {
	svcCap  capability.ServiceCapability
	list    data.ListResult
	detail  data.Detail
	history []data.HistoryRow
	diff    data.DiffContent
	nsList  []string
}

func (s *stubSource) Capability() capability.ServiceCapability { return s.svcCap }
func (s *stubSource) List(context.Context, data.ListParams) (data.ListResult, error) {
	return s.list, nil
}
func (s *stubSource) Show(context.Context, string, string) (data.Detail, error) {
	return s.detail, nil
}
func (s *stubSource) History(context.Context, string, string) ([]data.HistoryRow, error) {
	return s.history, nil
}
func (s *stubSource) VersionContents(context.Context, string, string, string, string) (data.DiffContent, error) {
	return s.diff, nil
}
func (s *stubSource) Namespaces(context.Context) ([]string, error) { return s.nsList, nil }

// awsParamCap is a representative capability (versioned param).
func awsParamCap() capability.ServiceCapability {
	sc, _ := lookup("aws", "param")

	return sc
}

func lookup(prov, service string) (capability.ServiceCapability, bool) {
	for _, pc := range capability.All() {
		if pc.Provider != prov {
			continue
		}

		for _, sc := range pc.Services {
			if sc.Service == service {
				return sc, true
			}
		}
	}

	return capability.ServiceCapability{}, false
}

// newModel builds a browser model over a stub source.
func newModel(t *testing.T, src *stubSource) *Model {
	t.Helper()

	m := New(context.Background(), src, nil, styles.New(), keys.Default())
	m.width, m.height = 120, 30

	return m
}

func keyPress(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func update(t *testing.T, m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	t.Helper()

	return m.Update(msg)
}

// TestStaleListResponseDropped pins the sequence guard: a list response carrying
// an older sequence than the latest issued load is ignored, so a slow earlier
// fetch never overwrites the newer state.
func TestStaleListResponseDropped(t *testing.T) {
	t.Parallel()

	src := &stubSource{svcCap: awsParamCap()}
	m := newModel(t, src)

	// Two loads issued: listSeq advances to 2.
	_ = m.loadListCmd(false)
	_ = m.loadListCmd(false)
	require.Equal(t, 2, m.listSeq)

	// A response tagged with the stale seq 1 is dropped.
	m, _ = update(t, m, listLoadedMsg{seq: 1, res: data.ListResult{Items: []data.Item{{Name: "stale"}}}})
	assert.Empty(t, m.items, "a stale list response must not overwrite state")

	// The current seq 2 is applied.
	m, _ = update(t, m, listLoadedMsg{seq: 2, res: data.ListResult{Items: []data.Item{{Name: "fresh"}}}})
	require.Len(t, m.items, 1)
	assert.Equal(t, "fresh", m.items[0].Name)
}

// TestPrefixDebounceSequenceGuard pins the debounce: a burst of edits schedules
// several debounced reloads, but only the latest sequence actually reloads.
func TestPrefixDebounceSequenceGuard(t *testing.T) {
	t.Parallel()

	src := &stubSource{svcCap: awsParamCap()}
	m := newModel(t, src)

	// Focus the filter input and type two characters (two debounce ticks).
	m, _ = update(t, m, keyPress('/'))
	require.Equal(t, focusFilter, m.focus)
	m, _ = update(t, m, keyPress('a'))
	m, _ = update(t, m, keyPress('b'))
	require.Equal(t, 2, m.debounceSeq)

	// The stale tick is a no-op; the latest tick triggers a reload.
	_, stale := update(t, m, debounceMsg{seq: 1})
	assert.Nil(t, stale, "a superseded debounce tick must not reload")

	_, fresh := update(t, m, debounceMsg{seq: 2})
	assert.NotNil(t, fresh, "the latest debounce tick reloads")
}

// TestCompareSelectionOpensDiff pins compare mode: two picked history rows and
// enter emit an OpenDiff request carrying the two versions ordered
// CHRONOLOGICALLY (older → newer), regardless of the order they were picked in —
// so picking newest first still diffs #13 → #14, not the reverse.
func TestCompareSelectionOpensDiff(t *testing.T) {
	t.Parallel()

	src := &stubSource{
		svcCap: awsParamCap(),
		history: []data.HistoryRow{
			{Version: "14", Label: "#14", IsCurrent: true},
			{Version: "13", Label: "#13"},
			{Version: "12", Label: "#12"},
		},
	}
	m := newModel(t, src)

	// Load a selection so detail/history exist.
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/app/x"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/app/x"}})
	m, _ = update(t, m, historyLoadedMsg{seq: m.historySeq, rows: src.history})

	// Enter compare mode and pick the first two rows.
	m, _ = update(t, m, keyPress('c'))
	require.True(t, m.history.Compare())
	require.Equal(t, focusHistory, m.focus)

	m, _ = update(t, m, keyForSpace()) // pick #14 (index 0, newest) FIRST
	m, _ = update(t, m, keyPress('j')) // move to #13
	m, _ = update(t, m, keyForSpace()) // pick #13 (index 1, older) second

	_, _, ok := m.history.PickedVersions()
	require.True(t, ok, "two rows picked")

	_, cmd := update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter with two picks opens the diff")

	msg := cmd()
	open, ok := msg.(nav.OpenDiff)
	require.True(t, ok, "enter emits nav.OpenDiff")
	// Picked newest-first, but the diff must read old → new: the older #13 is
	// OldVersion and the newer #14 is NewVersion (history index order: higher
	// index = older).
	assert.Equal(t, "13", open.OldVersion, "older version is OldVersion regardless of pick order")
	assert.Equal(t, "14", open.NewVersion, "newer version is NewVersion")
}

// keyForSpace builds the space key press (Bubble Tea v2 spells it "space").
func keyForSpace() tea.KeyPressMsg { return tea.KeyPressMsg{Code: ' '} }

// TestMaskToggle pins that x flips the detail value pane's mask for a secret and
// that the golden default is masked.
func TestMaskToggle(t *testing.T) {
	t.Parallel()

	src := &stubSource{svcCap: awsParamCap()}
	m := newModel(t, src)

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/s"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/s", Value: "hunter2", Secret: true}})
	require.True(t, m.valuePane.Masked(), "secret value starts masked")

	m, _ = update(t, m, keyPress('x'))
	assert.False(t, m.valuePane.Masked(), "x reveals")

	m, _ = update(t, m, keyPress('x'))
	assert.True(t, m.valuePane.Masked(), "x re-masks")
}

// TestCopyRevealsThenCopies pins that a copy reveals a masked secret first (so it
// is never copied while masked) and returns the value, and that an empty/absent
// value is not copyable (which would otherwise clear the clipboard).
func TestCopyRevealsThenCopies(t *testing.T) {
	t.Parallel()

	src := &stubSource{svcCap: awsParamCap()}
	m := newModel(t, src)

	// No detail yet: nothing to copy.
	_, ok := m.CopyText()
	assert.False(t, ok, "nothing to copy before a detail loads")

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/s"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/s", Value: "s3cr3t", Secret: true}})
	require.True(t, m.valuePane.Masked())

	text, ok := m.CopyText()
	require.True(t, ok)
	assert.Equal(t, "s3cr3t", text, "copy returns the real value")
	assert.False(t, m.valuePane.Masked(), "copy reveals first — never copies while masked")
}

// TestMouseClickSelectEqualsKeySelect pins the epic's mouse rule: a click on a
// list row reduces to the same selection (and detail load) a key move produces,
// with the coordinate derived from the drawn geometry — never hard-coded.
func TestMouseClickSelectEqualsKeySelect(t *testing.T) {
	t.Parallel()

	items := []data.Item{{Name: "/a"}, {Name: "/b"}, {Name: "/c"}}

	// Keyboard: move down twice selects index 2.
	keyed := newModel(t, &stubSource{svcCap: awsParamCap()})
	keyed, _ = update(t, keyed, listLoadedMsg{seq: keyed.listSeq, res: data.ListResult{Items: items}})
	keyed, _ = update(t, keyed, keyPress('j'))
	keyed, _ = update(t, keyed, keyPress('j'))
	require.Equal(t, 2, keyed.list.Selected())

	// Mouse: click the row the geometry maps to index 2.
	clicked := newModel(t, &stubSource{svcCap: awsParamCap()})
	clicked, _ = update(t, clicked, listLoadedMsg{seq: clicked.listSeq, res: data.ListResult{Items: items}})
	_ = clicked.View(clicked.width, clicked.height) // records geometry

	x, y := clicked.geom.listLeft, clicked.geom.listTop+2 // row index 2
	c, cmd := update(t, clicked, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})

	assert.Equal(t, keyed.list.Selected(), c.list.Selected(), "click selects the same row as the key move")
	assert.Equal(t, 2, c.list.Selected())
	assert.NotNil(t, cmd, "a row click loads the selection's detail")
}

// TestValuesToggleReloads pins that toggling values-mode reloads the list.
func TestValuesToggleReloads(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	before := m.listSeq

	_, cmd := update(t, m, keyPress('v'))
	assert.True(t, m.valuesOn)
	assert.NotNil(t, cmd, "values toggle reloads")
	assert.Greater(t, m.listSeq, before)
}

// TestSelectedItemByIndexWithDuplicateNames pins that the selection resolves an
// item by INDEX, not name, so App Configuration's same-key-across-namespaces
// case loads the correct (name, namespace) pair rather than the first duplicate.
func TestSelectedItemByIndexWithDuplicateNames(t *testing.T) {
	t.Parallel()

	items := []data.Item{
		{Name: "app/Feature", Namespace: ""},
		{Name: "app/Feature", Namespace: "staging"},
		{Name: "app/Feature", Namespace: "prod"},
	}

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: items}})

	m.list.SelectIndex(1)
	got, ok := m.selectedItem()
	require.True(t, ok)
	assert.Equal(t, "staging", got.Namespace, "index 1 resolves to the staging duplicate, not the first")

	m.list.SelectIndex(2)
	got, ok = m.selectedItem()
	require.True(t, ok)
	assert.Equal(t, "prod", got.Namespace, "index 2 resolves to the prod duplicate")
}

// TestStagingJump pins that S emits the OpenStaging navigation request.
func TestStagingJump(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})

	_, cmd := update(t, m, keyPress('S'))
	require.NotNil(t, cmd)
	_, ok := cmd().(nav.OpenStaging)
	assert.True(t, ok, "S emits nav.OpenStaging")
}
