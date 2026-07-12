//nolint:testpackage // white-box: exercises the unexported reducer, rows, and geometry
package staging

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

// canceledTag records a cancel-tag call for assertions.
type canceledTag struct {
	key    data.StagedKey
	tagKey string
}

// stubService is a controllable data.StagingService for Update-layer tests: it
// returns preset review/apply/reset results and records the mutating calls.
type stubService struct {
	service string
	label   string
	svcCap  capability.ServiceCapability

	review      data.StagingReview
	applyResult data.StagingApplyResult
	applyErr    error
	resetResult data.StagingResetResult

	unstaged      []data.StagedKey
	cancelAdds    []canceledTag
	cancelRemoves []canceledTag
	applied       []bool
	resetCalls    int
}

func (s *stubService) Service() string                          { return s.service }
func (s *stubService) Label() string                            { return s.label }
func (s *stubService) Capability() capability.ServiceCapability { return s.svcCap }

func (s *stubService) Review(context.Context) (data.StagingReview, error) {
	return s.review, nil
}

func (s *stubService) Apply(_ context.Context, ignoreConflicts bool) (data.StagingApplyResult, error) {
	s.applied = append(s.applied, ignoreConflicts)

	return s.applyResult, s.applyErr
}

func (s *stubService) Reset(context.Context) (data.StagingResetResult, error) {
	s.resetCalls++

	return s.resetResult, nil
}

func (s *stubService) Unstage(_ context.Context, key data.StagedKey) error {
	s.unstaged = append(s.unstaged, key)

	return nil
}

func (s *stubService) CancelAddTag(_ context.Context, key data.StagedKey, tagKey string) error {
	s.cancelAdds = append(s.cancelAdds, canceledTag{key: key, tagKey: tagKey})

	return nil
}

func (s *stubService) CancelRemoveTag(_ context.Context, key data.StagedKey, tagKey string) error {
	s.cancelRemoves = append(s.cancelRemoves, canceledTag{key: key, tagKey: tagKey})

	return nil
}

// capFor looks up a neutral capability for the fixtures.
//
//nolint:unparam // prov is "aws" for every current fixture but reads clearer explicit
func capFor(prov, service string) capability.ServiceCapability {
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

func keyPress(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// newModel builds a staging page over stub services, loads their reviews, and
// sizes it so View populates the geometry.
func newModel(t *testing.T, secs ...*stubService) *Model {
	t.Helper()

	services := make([]data.StagingService, len(secs))
	for i, s := range secs {
		services[i] = s
	}

	m := New(context.Background(), services, styles.New(), keys.Default())

	for i, s := range secs {
		m, _ = m.Update(reviewLoadedMsg{section: i, seq: m.sections[i].loadSeq, review: s.review})
	}

	m.width, m.height = 100, 30
	_ = m.View(m.width, m.height)

	return m
}

// updateEntryReview is a param section with an update entry.
func updateReview() data.StagingReview {
	return data.StagingReview{
		Entries: []data.StagedDiffRow{{
			Name: "/app/web/CDN_URL", Type: data.StagedDiffNormal, Operation: "update",
			RemoteValue: "https://old.example.com", StagedValue: "https://new.example.com",
		}},
	}
}

// TestUpdate_ViewToggle pins that `v` flips the diff/value view.
func TestUpdate_ViewToggle(t *testing.T) {
	t.Parallel()

	sec := &stubService{service: "param", label: "Param", svcCap: capFor("aws", "param"), review: updateReview()}
	m := newModel(t, sec)

	require.True(t, m.diffView, "diff is the default view")

	m, _ = m.Update(keyPress('v'))
	assert.False(t, m.diffView, "v switches to value view")

	m, _ = m.Update(keyPress('v'))
	assert.True(t, m.diffView, "v switches back to diff view")
}

// TestUpdate_ValueViewCollapsesMultiline pins that value view collapses a
// multi-line staged value to a single physical row (first line + " …"), so the
// body cannot overflow its box and desync the mouse hit-map. The full value
// stays reachable via the diff-detail page (enter).
func TestUpdate_ValueViewCollapsesMultiline(t *testing.T) {
	t.Parallel()

	sec := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: data.StagingReview{Entries: []data.StagedDiffRow{{
			Name: "/app/web/BLOB", Type: data.StagedDiffNormal, Operation: "create",
			StagedValue: "first-line\nSECOND_LINE_MARKER\nthird-line",
		}}},
	}
	m := newModel(t, sec)

	m, _ = m.Update(keyPress('v'))
	require.False(t, m.diffView, "switched to value view")

	screen := m.View(100, 30)
	assert.Contains(t, screen, "first-line …", "value collapses to its first line")
	assert.NotContains(t, screen, "SECOND_LINE_MARKER", "later lines never render as extra rows")
}

// tagOpsReview is a param section with one staged tag add and one staged tag
// removal (no entry rows), used to exercise the tag-row key handling.
func tagOpsReview() data.StagingReview {
	return data.StagingReview{
		Tags: []data.StagedTagRow{{
			Name:    "/app/api/DATABASE_URL",
			Adds:    []data.Tag{{Key: "owner", Value: "platform"}},
			Removes: []data.TagRemoval{{Key: "env", Value: "prod"}},
		}},
	}
}

// TestUpdate_UnstageTagRows pins #682: `u` is the single removal affordance for
// tag rows too — it cancels a staged tag add and a staged tag removal, each
// addressing the correct (item, tagKey).
func TestUpdate_UnstageTagRows(t *testing.T) {
	t.Parallel()

	sec := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: tagOpsReview(),
	}
	m := newModel(t, sec)

	require.Len(t, m.rows, 2, "one add row and one remove row")

	// Row 0 is the tag add; `u` cancels it.
	m.selected = 0
	m, cmd := m.Update(keyPress('u'))
	require.NotNil(t, cmd, "u dispatches a cancel on a tag-add row")

	m, _ = m.Update(cmd()) // feed the actionDoneMsg back, clearing the busy guard

	require.Len(t, sec.cancelAdds, 1)
	assert.Equal(t, "owner", sec.cancelAdds[0].tagKey)
	assert.Equal(t, "/app/api/DATABASE_URL", sec.cancelAdds[0].key.Name)

	// Row 1 is the tag removal; `u` cancels it too.
	m.selected = 1
	_, cmd = m.Update(keyPress('u'))
	require.NotNil(t, cmd, "u dispatches a cancel on a tag-remove row")
	_ = cmd()

	require.Len(t, sec.cancelRemoves, 1)
	assert.Equal(t, "env", sec.cancelRemoves[0].tagKey)
}

// TestUpdate_TagRowRevealAndEnterNonDestructive pins #682: on a tag row `x` only
// toggles reveal (never cancels) and enter is a no-op (never cancels). Removal is
// `u`-only, so neither key touches the staged tag state.
func TestUpdate_TagRowRevealAndEnterNonDestructive(t *testing.T) {
	t.Parallel()

	sec := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: tagOpsReview(),
	}
	m := newModel(t, sec)
	require.Len(t, m.rows, 2, "one add row and one remove row")

	for _, row := range []int{0, 1} {
		m.selected = row

		// `x` reveals only: it flips reveal and dispatches nothing.
		before := m.reveal
		m, cmd := m.Update(keyPress('x'))
		assert.Nil(t, cmd, "x dispatches no command on a tag row")
		assert.Equal(t, !before, m.reveal, "x toggles reveal on a tag row")

		// enter is a no-op: no command, no cancel.
		_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		assert.Nil(t, cmd, "enter is a no-op on a tag row")
	}

	assert.Empty(t, sec.cancelAdds, "x/enter never cancel a staged tag add")
	assert.Empty(t, sec.cancelRemoves, "x/enter never cancel a staged tag removal")
}

// TestUpdate_EnterOnEntryOpensDetail pins that enter on an entry row opens the
// remote-vs-staged detail (the detail-only behavior of enter, #682).
func TestUpdate_EnterOnEntryOpensDetail(t *testing.T) {
	t.Parallel()

	sec := &stubService{service: "param", label: "Param", svcCap: capFor("aws", "param"), review: updateReview()}
	m := newModel(t, sec)

	m.selected = 0
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "enter on an entry row dispatches")

	msg, ok := cmd().(nav.OpenStagingDetail)
	require.True(t, ok, "enter on an entry row opens the diff detail")
	assert.Equal(t, "/app/web/CDN_URL", msg.Title)
}

// TestUpdate_TagGatedOnDeleteStaged pins #684: `t` is not dispatched on a
// delete-staged entry (a statically impossible transition) and instead sets a
// one-line status message; it is still offered on a normal entry row.
func TestUpdate_TagGatedOnDeleteStaged(t *testing.T) {
	t.Parallel()

	sec := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: data.StagingReview{Entries: []data.StagedDiffRow{
			{Name: "/app/web/GONE", Type: data.StagedDiffNormal, Operation: "delete", RemoteValue: "v"},
			{Name: "/app/web/CDN_URL", Type: data.StagedDiffNormal, Operation: "update", StagedValue: "v"},
		}},
	}
	m := newModel(t, sec)

	// Row 0 is the delete: `t` is gated off with a status message, no OpenTag.
	m.selected = 0
	m, cmd := m.Update(keyPress('t'))
	assert.Nil(t, cmd, "t is not dispatched on a delete-staged row")
	assert.Equal(t, "cannot tag: staged for deletion — reset first", m.status, "the gate surfaces a status message")
	assert.Contains(t, m.View(100, 30), "cannot tag: staged for deletion", "the status message renders in the footer")

	// Row 1 is a normal update: `t` opens the tag form (gate is narrow).
	m.selected = 1
	_, cmd = m.Update(keyPress('t'))
	require.NotNil(t, cmd, "t is still offered on a non-delete row")
	msg, ok := cmd().(nav.OpenTag)
	require.True(t, ok, "t opens the tag form on a non-delete row")
	assert.Equal(t, "/app/web/CDN_URL", msg.Name)

	// The status message is transient: the next key press clears it.
	m.selected = 0
	m, _ = m.Update(keyPress('t'))
	require.NotEmpty(t, m.status, "the gate re-sets the status")
	m, _ = m.Update(keyPress('j'))
	assert.Empty(t, m.status, "any subsequent key clears the transient status")

	// A reload also clears it, so the message can never outlive its row.
	m.selected = 0
	m, _ = m.Update(keyPress('t'))
	require.NotEmpty(t, m.status, "the gate re-sets the status")
	m, _ = m.Update(reviewLoadedMsg{section: 0, seq: m.sections[0].loadSeq, review: sec.review})
	assert.Empty(t, m.status, "a reload clears the transient status")
}

// TestUpdate_UnstageAndApplyKeys pins the `u` unstage action (entry + its tags)
// and the `A` apply-all fan-out request across every section.
func TestUpdate_UnstageAndApplyKeys(t *testing.T) {
	t.Parallel()

	param := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: data.StagingReview{Entries: []data.StagedDiffRow{{Name: "/p", Operation: "update", StagedValue: "v"}}},
	}
	secret := &stubService{
		service: "secret", label: "Secret", svcCap: capFor("aws", "secret"),
		review: data.StagingReview{Entries: []data.StagedDiffRow{{Name: "s", Operation: "create", StagedValue: "v"}}},
	}
	m := newModel(t, param, secret)

	// `u` unstages the selected row's item.
	m.selected = 0
	_, cmd := m.Update(keyPress('u'))
	require.NotNil(t, cmd)
	_ = cmd()

	require.Len(t, param.unstaged, 1)
	assert.Equal(t, "/p", param.unstaged[0].Name)

	// `A` requests apply-all across every service.
	_, cmd = m.Update(keyPress('A'))
	require.NotNil(t, cmd)
	msg, ok := cmd().(nav.OpenApply)
	require.True(t, ok, "A opens the apply-all confirmation")
	assert.True(t, msg.Global)
	assert.ElementsMatch(t, []string{"param", "secret"}, msg.Services)
}

// TestUpdate_EditAndTagAreStagedOnly pins that the staging review page's `e`
// (edit) and `t` (tag) launch their dialogs staged-only: the emitted
// OpenEntryForm/OpenTag carry StagedOnly=true, so the shared mutation dialogs
// hide the Stage/Apply-immediately mode toggle. An immediate write from this
// staged surface would bypass the staging store and orphan the staged draft the
// dialog was launched from (issue #679).
func TestUpdate_EditAndTagAreStagedOnly(t *testing.T) {
	t.Parallel()

	sec := &stubService{service: "param", label: "Param", svcCap: capFor("aws", "param"), review: updateReview()}
	m := newModel(t, sec)

	m.selected = 0

	// `e` edits the selected staged entry, staged-only.
	_, cmd := m.Update(keyPress('e'))
	require.NotNil(t, cmd, "e emits an open-form command")
	form, ok := cmd().(nav.OpenEntryForm)
	require.True(t, ok, "e emits nav.OpenEntryForm")
	assert.True(t, form.Edit, "the request is an edit")
	assert.Equal(t, "/app/web/CDN_URL", form.Name)
	assert.True(t, form.StagedOnly, "a staging-review edit is staged-only (no immediate-mode escape hatch)")

	// `t` opens the tag dialog for the selected item, staged-only.
	_, cmd = m.Update(keyPress('t'))
	require.NotNil(t, cmd, "t emits an open-tag command")
	tag, ok := cmd().(nav.OpenTag)
	require.True(t, ok, "t emits nav.OpenTag")
	assert.Equal(t, "/app/web/CDN_URL", tag.Name)
	assert.True(t, tag.StagedOnly, "a staging-review tag add is staged-only (no immediate-mode escape hatch)")
}

// TestUpdate_AutoUnstagedNotice pins the dismissible auto-unstaged notice: it
// shows after a review that auto-unstaged an entry, and esc dismisses it.
func TestUpdate_AutoUnstagedNotice(t *testing.T) {
	t.Parallel()

	sec := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: data.StagingReview{
			Entries: []data.StagedDiffRow{{Name: "/app/web/OLD_FLAG", Type: data.StagedDiffAutoUnstaged}},
		},
	}
	m := newModel(t, sec)

	require.True(t, m.noticeVisible(), "an auto-unstaged entry shows the notice")
	assert.Contains(t, m.View(100, 30), "/app/web/OLD_FLAG", "the notice names the auto-unstaged entry")

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, m.noticeVisible(), "esc dismisses the notice")
}

// TestUpdate_BadgeCountAuthoritative pins that a section reports the exact staged
// count — entries plus tag changes counted separately (an item with both is two,
// not one) — fixing the browser probe's dedup undercount.
func TestUpdate_BadgeCountAuthoritative(t *testing.T) {
	t.Parallel()

	sec := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: data.StagingReview{
			Entries: []data.StagedDiffRow{{Name: "/x", Type: data.StagedDiffNormal, Operation: "update"}},
			Tags:    []data.StagedTagRow{{Name: "/x", Adds: []data.Tag{{Key: "k", Value: "v"}}}},
		},
	}

	m := New(context.Background(), []data.StagingService{sec}, styles.New(), keys.Default())
	_, cmd := m.Update(reviewLoadedMsg{section: 0, seq: m.sections[0].loadSeq, review: sec.review})

	require.NotNil(t, cmd)
	msg, ok := cmd().(nav.StagedCount)
	require.True(t, ok, "the section reports a staged count")
	assert.Equal(t, "param", msg.Service)
	assert.Equal(t, 2, msg.Count, "one entry + one tag change counts as two")
}

// TestUpdate_MouseClickApplyResetSelectRow pins the mouse rule: a click on a
// section's apply/reset button reduces to the SAME internal action its key
// equivalent performs, and a click on a tag row only selects it (never the
// destructive cancel — removal is `u`-only, #682), with coordinates read from
// the rendered geometry (never hard-coded).
func TestUpdate_MouseClickApplyResetSelectRow(t *testing.T) {
	t.Parallel()

	sec := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: data.StagingReview{
			Entries: []data.StagedDiffRow{{
				Name: "/app/web/CDN_URL", Type: data.StagedDiffNormal, Operation: "update",
				RemoteValue: "old", StagedValue: "new",
			}},
			Tags: []data.StagedTagRow{{Name: "/app/api/DB", Adds: []data.Tag{{Key: "owner", Value: "platform"}}}},
		},
	}
	m := newModel(t, sec)

	// Click the section header's apply button → apply confirmation for the section.
	headerLine, header := findHeaderLine(m)
	require.GreaterOrEqual(t, headerLine, 0, "a section header was rendered")

	x := (header.apply[0] + header.apply[1]) / 2
	_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: m.geom.bodyTop + headerLine, Button: tea.MouseLeft})
	require.NotNil(t, cmd)
	applyMsg, ok := cmd().(nav.OpenApply)
	require.True(t, ok, "clicking apply opens the apply confirmation")
	assert.Equal(t, []string{"param"}, applyMsg.Services)
	assert.False(t, applyMsg.Global)

	// Click the reset button → reset confirmation.
	x = (header.reset[0] + header.reset[1]) / 2
	_, cmd = m.Update(tea.MouseClickMsg{X: x, Y: m.geom.bodyTop + headerLine, Button: tea.MouseLeft})
	require.NotNil(t, cmd)
	_, ok = cmd().(nav.OpenReset)
	assert.True(t, ok, "clicking reset opens the reset confirmation")

	// Click the tag-add row → it only selects the row (no destructive cancel).
	tagLine := findRowLine(m, 1) // row 1 is the tag add (row 0 is the entry)
	require.GreaterOrEqual(t, tagLine, 0, "the tag row was rendered")

	m.selected = 0
	_, cmd = m.Update(tea.MouseClickMsg{X: 4, Y: m.geom.bodyTop + tagLine, Button: tea.MouseLeft})
	assert.Nil(t, cmd, "clicking a tag row dispatches nothing (enter is a no-op there)")
	assert.Equal(t, 1, m.selected, "clicking the tag row selects it")
	assert.Empty(t, sec.cancelAdds, "clicking a tag row never cancels its staged add")
}

// findHeaderLine returns the body-relative line index of the first section header
// and its descriptor.
func findHeaderLine(m *Model) (int, lineDesc) {
	for i, d := range m.geom.lines {
		if d.section >= 0 {
			return i, d
		}
	}

	return -1, lineDesc{}
}

// findRowLine returns the body-relative line index of the given selectable row.
func findRowLine(m *Model, row int) int {
	for i, d := range m.geom.lines {
		if d.row == row {
			return i
		}
	}

	return -1
}
