//nolint:testpackage // white-box: exercises the unexported reducer, messages, and geometry
package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/provider/azure/appconfig/aznamespace"
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
func awsParamCap() capability.ServiceCapability { return lookup("aws", "param") }

// appConfigCap is the namespaced Azure App Configuration param capability.
func appConfigCap() capability.ServiceCapability { return lookup("azure", "param") }

// awsSecretCap is the AWS secret capability (has restore + tags).
func awsSecretCap() capability.ServiceCapability { return lookup("aws", "secret") }

// lookup returns the neutral capability for a provider/service, or the zero
// capability when the matrix has no such pair (a test typo surfaces as a clearly
// empty capability).
func lookup(prov, service string) capability.ServiceCapability {
	for _, pc := range capability.All() {
		if pc.Provider != prov {
			continue
		}

		for _, sc := range pc.Services {
			if sc.Service == service {
				return sc
			}
		}
	}

	return capability.ServiceCapability{}
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

// TestSelectionSurvivesReloadByIdentity pins #699: a mutation reload that inserts
// a row above the selection must keep the detail on the SAME entry. Selecting
// /zzz then reloading with /aaa inserted at the top (which shifts /zzz down one
// index) re-resolves the selection to /zzz's new index rather than leaving the
// clamped index pointing at the neighbor that inherited /zzz's old slot.
func TestSelectionSurvivesReloadByIdentity(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})

	// Initial list; select /zzz (index 1).
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "/bbb"}, {Name: "/zzz"},
	}}})
	m.list.SelectIndex(1)
	sel, ok := m.selectedItem()
	require.True(t, ok)
	require.Equal(t, "/zzz", sel.Name)

	// A mutation reload inserts /aaa at the top: /zzz moves from index 1 to 2.
	_ = m.loadListCmd(false) // advance listSeq as a reload would
	m, cmd := update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "/aaa"}, {Name: "/bbb"}, {Name: "/zzz"},
	}}})

	got, ok := m.selectedItem()
	require.True(t, ok)
	assert.Equal(t, "/zzz", got.Name, "selection re-resolves to the same entry after an insert above it")
	assert.Equal(t, 2, m.list.Selected(), "the selection index followed /zzz to its new position")
	assert.NotNil(t, cmd, "the reload (re)loads the resolved selection's detail")
}

// TestSelectionSurvivesReloadWithDuplicateNamespaces pins that the re-resolve is
// keyed on name+namespace, not name alone: App Configuration lists the same key
// under several namespaces, so an insert above the selection must keep it on the
// exact (name, namespace) pair, never collapsing onto the first same-name row.
func TestSelectionSurvivesReloadWithDuplicateNamespaces(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: appConfigCap()})

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "app/Feature", Namespace: ""},
		{Name: "app/Feature", Namespace: "prod"},
	}}})
	m.list.SelectIndex(1) // the prod duplicate
	sel, ok := m.selectedItem()
	require.True(t, ok)
	require.Equal(t, "prod", sel.Namespace)

	// Reload inserts a staging duplicate above prod: prod moves from 1 to 2.
	_ = m.loadListCmd(false)
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "app/Feature", Namespace: ""},
		{Name: "app/Feature", Namespace: "staging"},
		{Name: "app/Feature", Namespace: "prod"},
	}}})

	got, ok := m.selectedItem()
	require.True(t, ok)
	assert.Equal(t, "prod", got.Namespace, "selection stays on the exact (name, namespace), not the first same-name row")
	assert.Equal(t, 2, m.list.Selected())
}

// TestSelectionSurvivesDeleteAboveSelection pins that a deletion which removes a
// row ABOVE the selection keeps the detail on the same entry. Selecting /bbb then
// deleting /aaa shifts /bbb from index 1 to 0; the OLD index-clamp would have held
// index 1 and landed on /zzz, so this fails against the pre-fix behavior (#699).
func TestSelectionSurvivesDeleteAboveSelection(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "/aaa"}, {Name: "/bbb"}, {Name: "/zzz"},
	}}})
	m.list.SelectIndex(1) // select /bbb
	sel, ok := m.selectedItem()
	require.True(t, ok)
	require.Equal(t, "/bbb", sel.Name)

	// Delete /aaa (above the selection): /bbb moves from index 1 to 0.
	_ = m.loadListCmd(false)
	m, cmd := update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "/bbb"}, {Name: "/zzz"},
	}}})

	got, ok := m.selectedItem()
	require.True(t, ok)
	assert.Equal(t, "/bbb", got.Name, "selection follows /bbb after the row above it is deleted")
	assert.Equal(t, 0, m.list.Selected())
	assert.NotNil(t, cmd, "the resolved selection's detail (re)loads")
}

// TestSelectionFallsBackWhenSelectedDeleted pins the graceful fallback half of
// #699: when the SELECTED entry itself is gone after a reload, the selection
// clamps into range and the fallback detail loads; when the list drains to empty
// the detail is cleared (no stale value, no phantom-entry data).
func TestSelectionFallsBackWhenSelectedDeleted(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "/aaa"}, {Name: "/bbb"}, {Name: "/zzz"},
	}}})
	m.list.SelectIndex(1) // select /bbb
	sel, ok := m.selectedItem()
	require.True(t, ok)
	require.Equal(t, "/bbb", sel.Name)

	// Delete /bbb (the selected entry): it is gone, so the selection clamps.
	_ = m.loadListCmd(false)
	m, cmd := update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "/aaa"}, {Name: "/zzz"},
	}}})

	got, ok := m.selectedItem()
	require.True(t, ok, "a non-empty list still has a selection")
	assert.Equal(t, "/zzz", got.Name, "selection clamps into range when the selected entry is gone")
	assert.NotNil(t, cmd, "the fallback selection loads its detail")

	// A further reload draining the list to empty clears the detail rather than
	// pointing it at a phantom entry.
	m.detailOK = true // pretend a detail is currently shown
	_ = m.loadListCmd(false)
	m, cmd = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{}}})
	_, ok = m.selectedItem()
	assert.False(t, ok, "an empty list has no selection")
	assert.False(t, m.detailOK, "the detail is cleared when nothing is selected")
	assert.Nil(t, cmd, "an empty list issues no detail load")
}

// TestLoadMoreInFlightGuard pins #700: loadMore issues an append when a next page
// is present, but a second loadMore fired while that append is still pending is a
// no-op — no new fetch (listSeq does not advance), so a hammered `L` can never
// splice a duplicate or stale page.
func TestLoadMoreInFlightGuard(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsSecretCap()})

	// A loaded page reports a real next page.
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{
		Items: []data.Item{{Name: "prod/a"}}, NextToken: "tok",
	}})
	require.Equal(t, "tok", m.nextToken)
	require.False(t, m.loading, "no fetch is in flight after the page loads")

	// First loadMore issues the append and marks the fetch in flight.
	seqBefore := m.listSeq
	cmd := m.loadMore()
	require.NotNil(t, cmd, "loadMore with a next page issues an append")
	require.True(t, m.loading, "the append is now in flight")
	require.Equal(t, seqBefore+1, m.listSeq, "the append advanced the list sequence")

	// A second loadMore while the first is still pending is a no-op.
	seqDuring := m.listSeq
	assert.Nil(t, m.loadMore(), "a second loadMore while one is in-flight is a no-op")
	assert.Equal(t, seqDuring, m.listSeq, "the blocked loadMore issues no fetch (no duplicate/stale splice)")

	// loadMore is likewise suppressed during an ordinary (non-append) reload.
	m2 := newModel(t, &stubSource{svcCap: awsSecretCap()})
	m2, _ = update(t, m2, listLoadedMsg{seq: m2.listSeq, res: data.ListResult{
		Items: []data.Item{{Name: "prod/a"}}, NextToken: "tok",
	}})
	_ = m2.loadListCmd(false) // a filter/mutation reload is now in flight
	require.True(t, m2.loading)
	assert.Nil(t, m2.loadMore(), "loadMore is a no-op while a full reload is pending")
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

// TestOnStagedLoadedSurfacesStoreHardFail pins the read-path key-loss surfacing:
// a staging store-construction hard-fail (a key-loss while encrypted state exists)
// is shown on the browser error line, while an ordinary transient probe read error
// stays quiet (badges just do not show — no error-line spam).
func TestOnStagedLoadedSurfacesStoreHardFail(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	_ = m.loadStagedCmd() // advance stagedSeq so the messages are current

	// A transient probe read error is swallowed.
	m, _ = update(t, m, stagedLoadedMsg{seq: m.stagedSeq, err: errors.New("probe timeout")})
	assert.Empty(t, m.err, "a transient probe error does not spam the error line")

	// A store-construction hard-fail (key-loss) is surfaced.
	hard := &data.StoreUnavailableError{Err: errors.New("cannot access the staging encryption key")}
	m, _ = update(t, m, stagedLoadedMsg{seq: m.stagedSeq, err: hard})
	assert.Contains(t, m.err, "staging encryption key", "a key-loss hard-fail is surfaced on the read path")
}

// TestOpenNewBlocksAllNamespaces pins the App Configuration create-block: a write
// targets one concrete namespace, so requesting the create dialog while the
// header filter is on `*` (all namespaces) emits an OpenError rather than the
// entry form; on a single concrete namespace it emits OpenEntryForm seeded with
// that namespace.
func TestOpenNewBlocksAllNamespaces(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: appConfigCap()})

	// New() seeds namespaces = ["", "*"]; select the all-namespaces filter.
	m.nsIndex = 1
	require.Equal(t, aznamespace.AllNamespacesFilter, m.currentNamespace())

	cmd := m.openNew()
	require.NotNil(t, cmd)
	_, blocked := cmd().(nav.OpenError)
	assert.True(t, blocked, "creating on * is blocked with an OpenError")

	// A single concrete namespace is allowed and seeds the form.
	m.nsIndex = 0
	require.Empty(t, m.currentNamespace(), "the null namespace is a concrete single namespace")

	cmd = m.openNew()
	require.NotNil(t, cmd)
	form, ok := cmd().(nav.OpenEntryForm)
	require.True(t, ok, "a concrete namespace opens the entry form")
	assert.False(t, form.Edit, "openNew requests a create, not an edit")
}

// TestOpenEditNoDetailGuard pins that Edit is a no-op until a detail has loaded
// (nothing to seed), and once loaded it emits an OpenEntryForm carrying the
// loaded name/namespace/value with Edit set.
func TestOpenEditNoDetailGuard(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})

	assert.Nil(t, m.openEdit(), "no detail loaded — edit is a no-op")

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/app/x"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/app/x", Value: "v1"}})

	cmd := m.openEdit()
	require.NotNil(t, cmd, "a loaded detail enables edit")
	form, ok := cmd().(nav.OpenEntryForm)
	require.True(t, ok, "edit emits nav.OpenEntryForm")
	assert.True(t, form.Edit, "the request is an edit")
	assert.Equal(t, "/app/x", form.Name)
	assert.Equal(t, "v1", form.Value, "the edit form is seeded from the loaded detail")
}

// TestOpenTagHasTagsGate pins the tag dialog is offered only for a service with
// tags: a no-tags capability makes Tag a no-op, while a tagging service with a
// selected entry emits OpenTag for it.
func TestOpenTagHasTagsGate(t *testing.T) {
	t.Parallel()

	noTagsCap := awsParamCap()
	noTagsCap.HasTags = false
	noTags := newModel(t, &stubSource{svcCap: noTagsCap})
	noTags, _ = update(t, noTags, listLoadedMsg{seq: noTags.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/x"}}}})
	assert.Nil(t, noTags.openTag(), "a no-tags service does not open the tag dialog")

	tagging := newModel(t, &stubSource{svcCap: awsParamCap()})
	tagging, _ = update(t, tagging, listLoadedMsg{seq: tagging.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/x"}}}})
	cmd := tagging.openTag()
	require.NotNil(t, cmd, "a tagging service with a selection opens the tag dialog")
	open, ok := cmd().(nav.OpenTag)
	require.True(t, ok, "tag emits nav.OpenTag")
	assert.Equal(t, "/x", open.Name)
}

// TestOpenRestoreHasRestoreGate pins the restore dialog is offered only for a
// service that supports restoring soft-deleted entries: a param service (no
// restore) makes Restore a no-op, while AWS secret emits OpenRestore seeded with
// the selection.
func TestOpenRestoreHasRestoreGate(t *testing.T) {
	t.Parallel()

	noRestore := newModel(t, &stubSource{svcCap: awsParamCap()})
	require.False(t, noRestore.svcCap.HasRestore)
	assert.Nil(t, noRestore.openRestore(), "a service without restore does not open the restore dialog")

	restorable := newModel(t, &stubSource{svcCap: awsSecretCap()})
	require.True(t, restorable.svcCap.HasRestore)
	restorable, _ = update(t, restorable, listLoadedMsg{seq: restorable.listSeq, res: data.ListResult{Items: []data.Item{{Name: "prod/x"}}}})
	cmd := restorable.openRestore()
	require.NotNil(t, cmd, "a restorable service opens the restore dialog")
	open, ok := cmd().(nav.OpenRestore)
	require.True(t, ok, "restore emits nav.OpenRestore")
	assert.Equal(t, "prod/x", open.Name, "the restore form is seeded with the selection")
}

// wheel builds a mouse-wheel event at a page-local point.
func wheel(button tea.MouseButton, x, y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{Button: button, X: x, Y: y}
}

// listTopRow returns the row index currently at the top of the list viewport.
// The list widget exposes its scroll offset only indirectly, through the same
// RowAtLine hit test clicks use, so line 0 maps to the top visible row (== the
// scroll offset). Reading it this way asserts real scroll state without reaching
// into the widget's private offset.
func listTopRow(t *testing.T, m *Model) int {
	t.Helper()

	idx, ok := m.list.RowAtLine(0)
	require.True(t, ok, "list has a visible top row")

	return idx
}

// historyTopRow returns the row index at the top of the history viewport (its
// scroll offset), read through the widget's own RowAtLine hit test.
func historyTopRow(t *testing.T, m *Model) int {
	t.Helper()

	idx, ok := m.history.RowAtLine(0)
	require.True(t, ok, "history has a visible top row")

	return idx
}

// loadedWheelModel builds a browser over a versioned (aws param) source with
// enough list items, history rows, and value lines that every pane can actually
// scroll, renders it once (recording geometry and sizing the widgets/value
// viewport), and returns the ready model.
func loadedWheelModel(t *testing.T) *Model {
	t.Helper()

	const (
		nItems   = 60
		nHistory = 40
		nValue   = 20
	)

	items := make([]data.Item, nItems)
	for i := range items {
		items[i] = data.Item{Name: fmt.Sprintf("/app/k%02d", i)}
	}

	history := make([]data.HistoryRow, nHistory)
	for i := range history {
		v := nHistory - i
		history[i] = data.HistoryRow{Version: fmt.Sprintf("%d", v), Label: fmt.Sprintf("#%d", v), IsCurrent: i == 0}
	}

	valueLines := make([]string, nValue)
	for i := range valueLines {
		valueLines[i] = fmt.Sprintf("value-line-%02d", i)
	}

	src := &stubSource{svcCap: awsParamCap(), history: history}
	m := newModel(t, src)
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: items}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: items[0].Name, Value: strings.Join(valueLines, "\n")}})
	m, _ = update(t, m, historyLoadedMsg{seq: m.historySeq, rows: history})

	_ = m.View(m.width, m.height) // records geometry, sizes the widgets and the value viewport

	require.Positive(t, m.geom.listRows, "the list region is drawn")
	require.Positive(t, m.geom.historyRows, "the history region is drawn")

	return m
}

// TestMouseWheelOverListScrollsList pins issue #653's missing wheel coverage: a
// wheel over the list region scrolls the LIST (and only the list), with the
// coordinate derived from the recorded geometry — never hard-coded.
func TestMouseWheelOverListScrollsList(t *testing.T) {
	t.Parallel()

	m := loadedWheelModel(t)

	require.Zero(t, listTopRow(t, m), "the list starts at the top")
	require.Zero(t, historyTopRow(t, m), "the history starts at the top")
	valueBefore := m.valuePane.View()

	x, y := m.geom.listLeft, m.geom.listTop

	m, cmd := update(t, m, wheel(tea.MouseWheelDown, x, y))
	assert.Nil(t, cmd, "a list wheel emits no command")
	assert.Equal(t, 1, listTopRow(t, m), "wheel-down over the list scrolls the list one row down")
	assert.Zero(t, historyTopRow(t, m), "the list wheel must not scroll history")
	assert.Equal(t, valueBefore, m.valuePane.View(), "the list wheel must not scroll the value pane")

	m, _ = update(t, m, wheel(tea.MouseWheelUp, x, y))
	assert.Zero(t, listTopRow(t, m), "wheel-up over the list scrolls back to the top")
}

// TestMouseWheelDirectionOverList pins wheelDelta's sign at the Update layer:
// wheel-down and wheel-up move the viewport in opposite directions. Two downs
// then one up net to one row below the start.
func TestMouseWheelDirectionOverList(t *testing.T) {
	t.Parallel()

	m := loadedWheelModel(t)
	x, y := m.geom.listLeft, m.geom.listTop

	m, _ = update(t, m, wheel(tea.MouseWheelDown, x, y))
	m, _ = update(t, m, wheel(tea.MouseWheelDown, x, y))
	require.Equal(t, 2, listTopRow(t, m), "two wheel-downs scroll two rows down")

	m, _ = update(t, m, wheel(tea.MouseWheelUp, x, y))
	assert.Equal(t, 1, listTopRow(t, m), "wheel-up moves opposite to wheel-down")
}

// TestWheelDeltaDirection pins the pure delta mapping: down is +1, up is -1, and
// a non-wheel button yields no scroll.
func TestWheelDeltaDirection(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1, wheelDelta(tea.MouseWheelDown), "wheel-down scrolls toward later rows")
	assert.Equal(t, -1, wheelDelta(tea.MouseWheelUp), "wheel-up scrolls toward earlier rows")
	assert.Equal(t, 0, wheelDelta(tea.MouseLeft), "a non-wheel button does not scroll")
}

// TestMouseWheelOverHistoryScrollsHistory pins that a wheel over the history
// region scrolls the HISTORY (and only history): the list and value pane stay
// put. Before the detail-pane hit-region fix, the unbounded list band shadowed
// this point and the list scrolled instead.
func TestMouseWheelOverHistoryScrollsHistory(t *testing.T) {
	t.Parallel()

	m := loadedWheelModel(t)

	require.Zero(t, listTopRow(t, m), "the list starts at the top")
	require.Zero(t, historyTopRow(t, m), "the history starts at the top")
	valueBefore := m.valuePane.View()

	x, y := m.geom.historyLeft, m.geom.historyTop
	require.True(t, m.geom.inHistory(x, y), "the point is inside the history region")
	require.False(t, m.geom.inList(x, y), "the point is NOT inside the list region (bounded on the right)")

	m, cmd := update(t, m, wheel(tea.MouseWheelDown, x, y))
	assert.Nil(t, cmd, "a history wheel emits no command")
	assert.Equal(t, 1, historyTopRow(t, m), "wheel-down over the history scrolls the history one row down")
	assert.Zero(t, listTopRow(t, m), "the history wheel must not scroll the list")
	assert.Equal(t, valueBefore, m.valuePane.View(), "the history wheel must not scroll the value pane")

	m, _ = update(t, m, wheel(tea.MouseWheelUp, x, y))
	assert.Zero(t, historyTopRow(t, m), "wheel-up over the history scrolls back to the top")
}

// TestMouseWheelOverValueRegionScrollsValuePane pins the documented default: a
// wheel over the value/meta region (the detail pane, above the history band)
// scrolls the value pane — not the list, not the history. The point is derived
// from the recorded geometry: the detail pane's left content column at the list's
// top content row, which sits above the history band.
func TestMouseWheelOverValueRegionScrollsValuePane(t *testing.T) {
	t.Parallel()

	m := loadedWheelModel(t)

	require.Zero(t, listTopRow(t, m))
	require.Zero(t, historyTopRow(t, m))
	valueBefore := m.valuePane.View()

	// Detail pane, value/meta region: right pane's left column, above the history.
	x, y := m.geom.historyLeft, m.geom.listTop
	require.Less(t, y, m.geom.historyTop, "the point is above the history band")
	require.False(t, m.geom.inList(x, y), "the point is NOT in the list region")
	require.False(t, m.geom.inHistory(x, y), "the point is NOT in the history region")

	m, _ = update(t, m, wheel(tea.MouseWheelDown, x, y))
	assert.NotEqual(t, valueBefore, m.valuePane.View(), "a wheel over the value region scrolls the value pane")
	assert.Zero(t, listTopRow(t, m), "the value wheel must not scroll the list")
	assert.Zero(t, historyTopRow(t, m), "the value wheel must not scroll history")
}

// TestMouseClickInDetailRegionDoesNotSelectListRow pins the click side of the
// same hit-region fix: a left click in the detail pane (the value/meta region,
// which shares the list's vertical band in the two-pane layout) must NOT
// re-select a list row. Before the fix, the unbounded list band mapped this
// click onto a list row and reloaded its detail.
func TestMouseClickInDetailRegionDoesNotSelectListRow(t *testing.T) {
	t.Parallel()

	m := loadedWheelModel(t)

	m.list.SelectIndex(0)
	require.Equal(t, 0, m.list.Selected())

	// Value/meta region: detail pane, above the history band.
	x, y := m.geom.historyLeft, m.geom.listTop
	require.False(t, m.geom.inList(x, y), "the point is not in the list region")
	require.False(t, m.geom.inHistory(x, y), "the point is not in the history region")

	m, cmd := update(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	assert.Nil(t, cmd, "a detail-region click loads nothing (it is not a list row)")
	assert.Equal(t, 0, m.list.Selected(), "a detail-region click must not move the list selection")
}

// TestMouseClickHistoryRowInCompareSelectsRow pins that, in compare mode, a click
// on a history row selects that row and marks it as a compare pick — the click
// counterpart of the keyboard compare flow. The row coordinate is derived from
// the history geometry, never hard-coded.
func TestMouseClickHistoryRowInCompareSelectsRow(t *testing.T) {
	t.Parallel()

	m := loadedWheelModel(t)

	// Enter compare mode (focuses the history).
	m, _ = update(t, m, keyPress('c'))
	require.True(t, m.history.Compare())
	require.Equal(t, focusHistory, m.focus)

	// Click the history row the geometry maps to line 1 (index 1).
	x, y := m.geom.historyLeft, m.geom.historyTop+1
	line, ok := m.geom.historyLine(x, y)
	require.True(t, ok, "the point is inside the history region")
	require.Equal(t, 1, line)

	m, _ = update(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	assert.Equal(t, 1, m.history.Selected(), "a history-row click selects that row")

	// Click a second row so exactly two picks are marked, then confirm both.
	x2, y2 := m.geom.historyLeft, m.geom.historyTop
	m, _ = update(t, m, tea.MouseClickMsg{X: x2, Y: y2, Button: tea.MouseLeft})
	i, j, picked := m.history.PickedVersions()
	require.True(t, picked, "two history-row clicks mark two compare picks")
	assert.ElementsMatch(t, []int{0, 1}, []int{i, j}, "the two clicked rows are the picks")
}
