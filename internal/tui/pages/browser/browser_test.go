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

// regionOrigin returns the drawn top-left of a hit region, so a mouse test
// derives its click coordinate from the rendered layout instead of hard-coding
// one. It fails when the region was not drawn.
func regionOrigin(t *testing.T, m *Model, id string) (int, int) {
	t.Helper()

	x, y, ok := m.hits.Origin(id)
	require.True(t, ok, "region %q was drawn", id)

	return x, y
}

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

// loadedHistoryModel builds a browser over a versioned source with a loaded
// selection and history, rendered once so the widgets carry the page's focus.
func loadedHistoryModel(t *testing.T) *Model {
	t.Helper()

	src := &stubSource{
		svcCap: awsParamCap(),
		history: []data.HistoryRow{
			{Version: "14", Label: "#14", IsCurrent: true},
			{Version: "13", Label: "#13"},
		},
	}
	m := newModel(t, src)
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/app/x"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/app/x"}})
	m, _ = update(t, m, historyLoadedMsg{seq: m.historySeq, rows: src.history})

	return m
}

// TestFocusHighlightDistinctBetweenPanes pins #685: the focused pane carries the
// active selection cursor (▸) and the unfocused pane a dimmed one (▹), and the
// two swap as focus moves list↔history — so the two panes never look equally
// selected at once. Rendering the page sets each widget's focus from the page's
// current focus, which the widget's own View then reflects.
func TestFocusHighlightDistinctBetweenPanes(t *testing.T) {
	t.Parallel()

	m := loadedHistoryModel(t)
	require.Equal(t, focusList, m.focus, "focus starts on the list")

	_ = m.View(m.width, m.height)

	assert.Contains(t, m.list.View(), "▸", "the focused list shows the active cursor")
	assert.NotContains(t, m.list.View(), "▹", "the focused list does not show the dimmed cursor")
	assert.Contains(t, m.history.View(), "▹", "the unfocused history shows the dimmed cursor")
	assert.NotContains(t, m.history.View(), "▸", "the unfocused history does not show the active cursor")

	// Enter moves focus into the history; the highlights swap.
	m, _ = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, focusHistory, m.focus, "enter moves focus into the history")

	_ = m.View(m.width, m.height)

	assert.Contains(t, m.history.View(), "▸", "the focused history shows the active cursor")
	assert.NotContains(t, m.history.View(), "▹", "the focused history does not show the dimmed cursor")
	assert.Contains(t, m.list.View(), "▹", "the unfocused list shows the dimmed cursor")
	assert.NotContains(t, m.list.View(), "▸", "the unfocused list does not show the active cursor")
}

// TestHistoryHeaderHintAdaptsToFocus pins #685's discoverability affordance: the
// history header advertises `enter: history` while the list is focused and
// `esc: list` once focus is in the history, so the enter→history / esc→list
// transitions are visible rather than trial-and-error.
func TestHistoryHeaderHintAdaptsToFocus(t *testing.T) {
	t.Parallel()

	m := loadedHistoryModel(t)

	const width = 80

	assert.Contains(t, m.historyHeaderLine(width), "enter: history", "the list advertises how to enter the history")
	assert.NotContains(t, m.historyHeaderLine(width), "esc: list")

	m, _ = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, focusHistory, m.focus)

	assert.Contains(t, m.historyHeaderLine(width), "esc: list", "the history advertises how to return to the list")
	assert.NotContains(t, m.historyHeaderLine(width), "enter: history")
}

// TestHistoryHeaderHintSuppressedWhenEmpty pins that the enter→history affordance
// is not advertised for an entry with no versions: onSelect no-ops there, so the
// header must not promise a transition that does nothing.
func TestHistoryHeaderHintSuppressedWhenEmpty(t *testing.T) {
	t.Parallel()

	src := &stubSource{svcCap: awsParamCap()} // no history rows
	m := newModel(t, src)
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/app/x"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/app/x"}})
	m, _ = update(t, m, historyLoadedMsg{seq: m.historySeq, rows: nil})
	require.Zero(t, m.history.Len(), "the entry has no versions")

	line := m.historyHeaderLine(80)
	assert.Contains(t, line, "History", "the header title still renders")
	assert.NotContains(t, line, "enter: history", "no false enter→history affordance for a version-less entry")
}

// TestStaleErrorClearedByLaterSuccessfulLoad pins #688: a transient history (or
// detail) error must not linger over a later successful load. The single
// selection funnel clears the per-source detail/history errors up front, and each
// source also clears its own error when it next succeeds.
func TestStaleErrorClearedByLaterSuccessfulLoad(t *testing.T) {
	t.Parallel()

	src := &stubSource{svcCap: awsParamCap()}
	m := newModel(t, src)

	items := []data.Item{{Name: "/a"}, {Name: "/b"}}
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: items}})

	// Entry A: the detail loads, but the history fetch fails transiently.
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/a"}})
	m, _ = update(t, m, historyLoadedMsg{seq: m.historySeq, err: errors.New("history fetch failed")})
	require.Contains(t, m.historyErr, "history fetch failed")
	require.NotEmpty(t, m.errLines(), "the transient history error is shown")

	// Select entry B: the selection funnel clears the stale per-source errors up
	// front, before B's responses even land.
	_ = m.move(1)
	assert.Empty(t, m.historyErr, "selecting a new entry clears the stale history error immediately")
	assert.Empty(t, m.detailErr)

	// B's detail and history both succeed → the error line stays clear.
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/b"}})
	m, _ = update(t, m, historyLoadedMsg{seq: m.historySeq, rows: nil})
	assert.Empty(t, m.errLines(), "a successful load after an error clears the error line")
}

// TestDetailErrorClearedOnItsOwnSuccessfulLoad pins that a detail error clears the
// moment a detail load succeeds, independent of the selection funnel — the
// per-source clearing path onDetailLoaded owns.
func TestDetailErrorClearedOnItsOwnSuccessfulLoad(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/a"}}}})

	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, err: errors.New("show failed")})
	require.Contains(t, m.detailErr, "show failed")

	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/a"}})
	assert.Empty(t, m.detailErr, "a successful detail load clears its own error")
	assert.True(t, m.detailOK)
}

// TestStagedErrorSurvivesSelectionChange pins the deliberate divergence: the
// staging-store hard-fail (a launch-time key-loss) is a persistent condition, so
// a selection change clears the transient detail/history errors but NOT the
// staged error.
func TestStagedErrorSurvivesSelectionChange(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/a"}, {Name: "/b"}}}})

	m.stagedErr = "cannot access the staging encryption key"
	m.detailErr = "stale detail error"

	_ = m.move(1)

	assert.Empty(t, m.detailErr, "the selection funnel clears the transient detail error")
	assert.Contains(t, m.stagedErr, "staging encryption key", "the persistent staged hard-fail survives a selection change")
}

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

// TestCopyDoesNotUnmask pins #689: copying a masked secret returns the real
// value for the clipboard WITHOUT unmasking the on-screen pane (the mask stays
// put, so a copy never becomes a standing disclosure), and it leaves a transient
// "value stays masked" status. An empty/absent value is not copyable (which
// would otherwise clear the clipboard).
func TestCopyDoesNotUnmask(t *testing.T) {
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
	assert.True(t, m.valuePane.Masked(), "copy must NOT unmask — the on-screen value stays masked (#689)")
	assert.Equal(t, "copied (value stays masked)", m.actionStatus, "a masked copy notes the value stays masked")

	// A revealed value copies too, and reports a plain "copied" note.
	m, _ = update(t, m, keyPress('x'))
	require.False(t, m.valuePane.Masked(), "x reveals")

	text, ok = m.CopyText()
	require.True(t, ok)
	assert.Equal(t, "s3cr3t", text)
	assert.False(t, m.valuePane.Masked(), "a copy never changes the mask state either way")
	assert.Equal(t, "copied", m.actionStatus)
}

// TestParseJSONToggleFormatsValue pins #690: the detail value pane's `J` toggle
// pretty-prints a JSON value in the browser (parity with the diff page and the
// GUI), and toggles back to the raw compact form. A masked secret gates the
// toggle off (the pane requires reveal first).
func TestParseJSONToggleFormatsValue(t *testing.T) {
	t.Parallel()

	const compact = `{"host":"db.internal","port":5432}`

	// The pretty-printed form indents each member on its own line — the viewport
	// pads lines to width, so assert on a single indented member line (present only
	// when the value is formatted) rather than the whole multi-line block.
	const indentedMember = `  "host": "db.internal"`

	src := &stubSource{svcCap: awsParamCap()}
	m := newModel(t, src)

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/app/cfg"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/app/cfg", Value: compact}})

	m.valuePane.SetSize(80, 20)
	require.Contains(t, m.valuePane.View(), compact, "a non-secret JSON value renders raw by default")
	require.NotContains(t, m.valuePane.View(), indentedMember, "the raw form is not indented")

	// `J` pretty-prints it.
	m, _ = update(t, m, keyPress('J'))
	m.valuePane.SetSize(80, 20)
	assert.Contains(t, m.valuePane.View(), indentedMember, "J pretty-prints the JSON value onto indented lines")
	assert.NotContains(t, m.valuePane.View(), compact, "the compact single-line form is gone once formatted")

	// `J` again toggles back to the compact form.
	m, _ = update(t, m, keyPress('J'))
	m.valuePane.SetSize(80, 20)
	assert.Contains(t, m.valuePane.View(), compact, "J toggles back to the raw value")
	assert.NotContains(t, m.valuePane.View(), indentedMember, "the formatting is undone")
}

// TestParseJSONToggleGatedWhileMasked pins that `J` is a no-op while a secret is
// masked (the pane requires reveal first, so a masked secret is never
// normalized), and works once revealed.
func TestParseJSONToggleGatedWhileMasked(t *testing.T) {
	t.Parallel()

	const compact = `{"token":"abc"}`

	src := &stubSource{svcCap: awsParamCap()}
	m := newModel(t, src)

	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/s"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/s", Value: compact, Secret: true}})
	require.True(t, m.valuePane.Masked(), "secret starts masked")

	// J while masked does nothing — the pane stays masked (no format, no reveal).
	m, _ = update(t, m, keyPress('J'))
	m.valuePane.SetSize(80, 20)
	assert.True(t, m.valuePane.Masked(), "J must not reveal a masked secret")
	assert.NotContains(t, m.valuePane.View(), "token", "J must not format (or leak) a masked secret")

	// Reveal, then J formats.
	m, _ = update(t, m, keyPress('x'))
	m, _ = update(t, m, keyPress('J'))
	m.valuePane.SetSize(80, 20)
	assert.Contains(t, m.valuePane.View(), `  "token": "abc"`, "once revealed, J formats the JSON onto indented lines")
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
	_ = clicked.View(clicked.width, clicked.height) // rebuilds the hit map

	lx, ly := regionOrigin(t, clicked, regionList)
	x, y := lx, ly+2 // row index 2
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

// TestOnStagedLoadedSurfacesProbeErrors pins the read-path error surfacing (#695):
// a transient probe read error shows a short "staged status unavailable" note
// rather than being swallowed (silent swallowing would hide every [staged] badge,
// making a staged entry look un-staged), and a store-construction hard-fail
// (a key-loss while encrypted state exists) shows its own actionable message.
func TestOnStagedLoadedSurfacesProbeErrors(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	_ = m.loadStagedCmd() // advance stagedSeq so the messages are current

	// A transient probe read error is surfaced as a non-fatal note, not swallowed.
	m, _ = update(t, m, stagedLoadedMsg{seq: m.stagedSeq, err: errors.New("probe timeout")})
	assert.Equal(t, "staged status unavailable", m.stagedErr, "a transient probe error is surfaced, not silently dropped")
	assert.Contains(t, m.errLines(), "staged status unavailable", "the note reaches the rendered error region")

	// A store-construction hard-fail (key-loss) is surfaced with its own message.
	hard := &data.StoreUnavailableError{Err: errors.New("cannot access the staging encryption key")}
	m, _ = update(t, m, stagedLoadedMsg{seq: m.stagedSeq, err: hard})
	assert.Contains(t, m.stagedErr, "staging encryption key", "a key-loss hard-fail is surfaced on the read path")
}

// TestStagedCountUsesEntriesPlusTags pins #693: the browser reports the Staging
// tab badge count as entries + tag changes (the staging page's definition), so an
// item with both an entry change and a tag change counts as 2 — never the
// deduplicated key count (1) the browser used before, which made the badge
// oscillate between the two surfaces.
func TestStagedCountUsesEntriesPlusTags(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsSecretCap()})
	_ = m.loadStagedCmd()

	key := data.StagedKey{Name: "prod/api/key"}
	// One item carries BOTH a staged entry change and a staged tag change: one
	// deduplicated key, but two changes.
	snap := data.StagingSnapshot{
		Keys:       map[data.StagedKey]struct{}{key: {}},
		DeleteKeys: map[data.StagedKey]struct{}{},
		EntryCount: 1,
		TagCount:   1,
	}

	_, cmd := update(t, m, stagedLoadedMsg{seq: m.stagedSeq, snap: snap})
	require.NotNil(t, cmd, "a fresh staged load reports the tab count")

	count, ok := cmd().(nav.StagedCount)
	require.True(t, ok, "onStagedLoaded emits nav.StagedCount")
	assert.Equal(t, 2, count.Count, "the badge counts entries + tags (2), not the deduplicated key (1)")
}

// TestEditDeleteTagGatedOnDeleteStaged pins #692: on an entry staged for deletion,
// e/d/t do not open their (dead-end) dialogs but set a one-line status message
// instead — matching the GUI, which hides those controls. A non-delete-staged
// entry keeps the affordances.
func TestEditDeleteTagGatedOnDeleteStaged(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsSecretCap()}) // has tags
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{
		{Name: "prod/live"}, {Name: "prod/doomed"},
	}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "prod/live", Value: "v"}})

	// prod/doomed (index 1) is staged for deletion.
	doomed := data.StagedKey{Name: "prod/doomed"}
	snap := data.StagingSnapshot{
		Keys:       map[data.StagedKey]struct{}{doomed: {}},
		DeleteKeys: map[data.StagedKey]struct{}{doomed: {}},
		EntryCount: 1,
	}
	m, _ = update(t, m, stagedLoadedMsg{seq: m.stagedSeq, snap: snap})

	m.list.SelectIndex(1) // select the delete-staged entry
	require.True(t, m.selectedIsDeleteStaged(), "prod/doomed is delete-staged")

	for _, tc := range []struct {
		key  rune
		want string
	}{
		{'e', "cannot edit: staged for deletion"},
		{'d', "already staged for deletion"},
		{'t', "cannot tag: staged for deletion"},
	} {
		m, cmd := update(t, m, keyPress(tc.key))
		assert.Nil(t, cmd, "%c on a delete-staged entry opens no dialog", tc.key)
		assert.Contains(t, m.actionStatus, tc.want, "%c surfaces the gate status", tc.key)
		assert.Contains(t, m.errLines(), m.actionStatus, "the gate status renders in the error region")
	}

	// A non-delete-staged entry keeps the affordances: e opens the edit form.
	m.list.SelectIndex(0)
	require.False(t, m.selectedIsDeleteStaged(), "prod/live is not delete-staged")
	m, cmd := update(t, m, keyPress('e'))
	require.NotNil(t, cmd, "edit is offered on a non-delete-staged entry")
	_, ok := cmd().(nav.OpenEntryForm)
	assert.True(t, ok, "e opens the entry form when the entry is not delete-staged")
	assert.Empty(t, m.actionStatus, "no gate status on a valid action")
}

// TestOpenNewCarriesDeleteStagedKeys pins that the create dialog request carries
// the delete-staged key set, so the entry form can reject a delete-staged name
// client-side (#692) instead of dead-ending on the reducer.
func TestOpenNewCarriesDeleteStagedKeys(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	doomed := data.StagedKey{Name: "/app/doomed"}
	m.deleteStagedKeys = map[data.StagedKey]struct{}{doomed: {}}

	cmd := m.openNew()
	require.NotNil(t, cmd)
	form, ok := cmd().(nav.OpenEntryForm)
	require.True(t, ok, "openNew emits nav.OpenEntryForm")
	assert.False(t, form.Edit)
	_, carried := form.DeleteStagedKeys[doomed]
	assert.True(t, carried, "the create request carries the delete-staged keys for client-side validation")
}

// TestStagedBannerDistinguishesKind pins #701: the detail-pane staged banner
// distinguishes a staged value change, a staged tag change, and both — matching
// the GUI's StagingBanner — rather than collapsing every staged kind into one
// message. Each case stages the default-selected entry with a different change
// kind and asserts the rendered banner wording.
func TestStagedBannerDistinguishesKind(t *testing.T) {
	t.Parallel()

	const name = "/app/x"

	key := data.StagedKey{Name: name}
	keySet := map[data.StagedKey]struct{}{key: {}}

	cases := []struct {
		label   string
		entry   bool
		tags    bool
		want    string
		notWant string
	}{
		{"value-only", true, false, "⚠ staged value changes — S: staging", "tag changes"},
		{"tag-only", false, true, "⚠ staged tag changes — S: staging", "value and tag"},
		{"both", true, true, "⚠ staged value and tag changes — S: staging", ""},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			m := newModel(t, &stubSource{svcCap: awsParamCap()})
			m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: name}}}})
			m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: name}})

			snap := data.StagingSnapshot{Keys: keySet}
			if tc.entry {
				snap.EntryKeys = keySet
			}

			if tc.tags {
				snap.TagKeys = keySet
			}

			m, _ = update(t, m, stagedLoadedMsg{seq: m.stagedSeq, snap: snap})

			out := m.View(m.width, m.height)
			assert.Contains(t, out, tc.want, "banner reflects the staged change kind")

			if tc.notWant != "" {
				assert.NotContains(t, out, tc.notWant, "banner must not use another kind's wording")
			}
		})
	}
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
	assert.False(t, form.StagedOnly, "a browser edit keeps the immediate-mode toggle (not a staged-only surface)")
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
	assert.False(t, open.StagedOnly, "a browser tag keeps the immediate-mode toggle (not a staged-only surface)")
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

	_ = m.View(m.width, m.height) // rebuilds the hit map, sizes the widgets and the value viewport

	_, _, listDrawn := m.hits.Origin(regionList)
	require.True(t, listDrawn, "the list region is drawn")

	_, _, historyDrawn := m.hits.Origin(regionHistory)
	require.True(t, historyDrawn, "the history region is drawn")

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

	x, y := regionOrigin(t, m, regionList)

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
	x, y := regionOrigin(t, m, regionList)

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

	x, y := regionOrigin(t, m, regionHistory)
	id, _, _, ok := m.hits.At(x, y)
	require.True(t, ok, "the point is inside a region")
	require.Equal(t, regionHistory, id, "the point resolves to the history region, not the list")

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

	// Detail pane, value/meta region: the value-label row, above the history band.
	x, y := regionOrigin(t, m, regionValueLabel)
	id, _, _, ok := m.hits.At(x, y)
	require.True(t, ok, "the point is inside a region")
	require.Contains(t, []string{regionValueLabel, regionDetail}, id,
		"the point is in the detail/value region, not the list or history")

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

	// Detail body region: a few rows below the value label (the value-pane area),
	// which shares the list's vertical band in the two-pane layout.
	dx, dy := regionOrigin(t, m, regionDetail)
	x, y := dx, dy+3
	id, _, _, ok := m.hits.At(x, y)
	require.True(t, ok, "the point is inside a region")
	require.Equal(t, regionDetail, id, "the point is in the detail body, not a list row")

	m, cmd := update(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	assert.Nil(t, cmd, "a detail-region click loads nothing (it is not a list row)")
	assert.Equal(t, 0, m.list.Selected(), "a detail-region click must not move the list selection")
}

// TestMouseClickHistoryRowInCompareSelectsRow pins that, in compare mode, a click
// on a history row selects and picks that row — the click counterpart of the
// keyboard compare flow — and that the click completing the second pick OPENS the
// diff, reducing to the same nav.OpenDiff enter produces (#663 closes the "compare
// can pick but not open via mouse" gap). Coordinates are derived from the history
// region, never hard-coded.
func TestMouseClickHistoryRowInCompareSelectsRow(t *testing.T) {
	t.Parallel()

	m := loadedWheelModel(t)

	// Enter compare mode (focuses the history).
	m, _ = update(t, m, keyPress('c'))
	require.True(t, m.history.Compare())
	require.Equal(t, focusHistory, m.focus)

	// Click the history row the region maps to line 1 (index 1).
	hx, hy := regionOrigin(t, m, regionHistory)
	x, y := hx, hy+1
	id, _, dy, ok := m.hits.At(x, y)
	require.True(t, ok, "the point is inside the history region")
	require.Equal(t, regionHistory, id)
	require.Equal(t, 1, dy, "the in-region offset is the clicked line")

	m, cmd := update(t, m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	assert.Equal(t, 1, m.history.Selected(), "a history-row click selects that row")
	assert.Nil(t, cmd, "the first pick click does not open the diff yet")

	// Click a second row so exactly two picks are marked: the completing click opens
	// the diff, exactly as enter would after two keyboard picks.
	x2, y2 := hx, hy
	m, cmd = update(t, m, tea.MouseClickMsg{X: x2, Y: y2, Button: tea.MouseLeft})
	i, j, picked := m.history.PickedVersions()
	require.True(t, picked, "two history-row clicks mark two compare picks")
	assert.ElementsMatch(t, []int{0, 1}, []int{i, j}, "the two clicked rows are the picks")

	require.NotNil(t, cmd, "the click completing the second pick opens the diff")
	_, isDiff := cmd().(nav.OpenDiff)
	assert.True(t, isDiff, "a compare click that completes two picks opens the diff (like enter)")
}

// TestMouseClickHeaderRegionsMatchKeys pins #663's browser-header coverage: a
// click on each header affordance reduces to the SAME action its key equivalent
// performs — focusing the prefix/filter inputs (p, /), toggling values/recursive
// (v, r), and refreshing (⟳). Coordinates come from the drawn header regions.
func TestMouseClickHeaderRegionsMatchKeys(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/a"}}}})
	_ = m.View(m.width, m.height)

	// prefix field click focuses the prefix input (like `p`).
	px, py := regionOrigin(t, m, regionPrefix)
	m, _ = update(t, m, tea.MouseClickMsg{X: px, Y: py, Button: tea.MouseLeft})
	assert.Equal(t, focusPrefix, m.focus, "clicking the prefix field focuses it (like p)")

	// Commit (esc), then the filter field click focuses the filter (like `/`).
	m, _ = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	_ = m.View(m.width, m.height)
	fx, fy := regionOrigin(t, m, regionFilter)
	m, _ = update(t, m, tea.MouseClickMsg{X: fx, Y: fy, Button: tea.MouseLeft})
	assert.Equal(t, focusFilter, m.focus, "clicking the filter field focuses it (like /)")

	m, _ = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	_ = m.View(m.width, m.height)

	// values chip click toggles values-mode and reloads (like `v`).
	vx, vy := regionOrigin(t, m, regionValues)
	valSeq := m.listSeq
	m, cmd := update(t, m, tea.MouseClickMsg{X: vx, Y: vy, Button: tea.MouseLeft})
	assert.True(t, m.valuesOn, "clicking values toggles it on (like v)")
	assert.NotNil(t, cmd, "the values toggle reloads")
	assert.Greater(t, m.listSeq, valSeq)

	_ = m.View(m.width, m.height)

	// recursive chip click toggles recursive and reloads (like `r`).
	rx, ry := regionOrigin(t, m, regionRecursive)
	recBefore := m.recursive
	m, cmd = update(t, m, tea.MouseClickMsg{X: rx, Y: ry, Button: tea.MouseLeft})
	assert.Equal(t, !recBefore, m.recursive, "clicking recursive toggles it (like r)")
	assert.NotNil(t, cmd, "the recursive toggle reloads")

	_ = m.View(m.width, m.height)

	// refresh (⟳) click reloads the list.
	gx, gy := regionOrigin(t, m, regionRefresh)
	refSeq := m.listSeq
	_, cmd = update(t, m, tea.MouseClickMsg{X: gx, Y: gy, Button: tea.MouseLeft})
	assert.NotNil(t, cmd, "clicking the refresh affordance reloads the list")
	assert.Greater(t, m.listSeq, refSeq)
}

// TestMouseClickNamespaceBadgeCyclesFilter pins that clicking the App Config
// namespace badge cycles the namespace filter, reducing to the same action space
// performs there.
func TestMouseClickNamespaceBadgeCyclesFilter(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: appConfigCap()})
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "app/x"}}}})
	_ = m.View(m.width, m.height)
	require.Equal(t, 0, m.nsIndex, "the namespace filter starts at the first option")

	nx, ny := regionOrigin(t, m, regionNamespace)
	m, cmd := update(t, m, tea.MouseClickMsg{X: nx, Y: ny, Button: tea.MouseLeft})
	assert.Equal(t, 1, m.nsIndex, "clicking the ns badge cycles the namespace (like space)")
	assert.NotNil(t, cmd, "cycling the namespace reloads the list")
}

// TestMouseClickValueLabelTogglesMask pins that clicking the "Value (x to reveal)"
// label toggles the mask, reducing to the same action `x` performs.
func TestMouseClickValueLabelTogglesMask(t *testing.T) {
	t.Parallel()

	m := newModel(t, &stubSource{svcCap: awsParamCap()})
	m, _ = update(t, m, listLoadedMsg{seq: m.listSeq, res: data.ListResult{Items: []data.Item{{Name: "/s"}}}})
	m, _ = update(t, m, detailLoadedMsg{seq: m.detailSeq, d: data.Detail{Name: "/s", Value: "hunter2", Secret: true}})
	_ = m.View(m.width, m.height)
	require.True(t, m.valuePane.Masked(), "the secret value starts masked")

	vx, vy := regionOrigin(t, m, regionValueLabel)
	m, _ = update(t, m, tea.MouseClickMsg{X: vx, Y: vy, Button: tea.MouseLeft})
	assert.False(t, m.valuePane.Masked(), "clicking the value label reveals (like x)")

	_ = m.View(m.width, m.height)
	m, _ = update(t, m, tea.MouseClickMsg{X: vx, Y: vy, Button: tea.MouseLeft})
	assert.True(t, m.valuePane.Masked(), "clicking the value label again re-masks")
}
