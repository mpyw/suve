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

// TestUpdate_CancelTagOps pins that `x` cancels a staged tag add and enter
// cancels a staged tag removal, each addressing the correct (item, tagKey).
func TestUpdate_CancelTagOps(t *testing.T) {
	t.Parallel()

	sec := &stubService{
		service: "param", label: "Param", svcCap: capFor("aws", "param"),
		review: data.StagingReview{
			Tags: []data.StagedTagRow{{
				Name:    "/app/api/DATABASE_URL",
				Adds:    []data.Tag{{Key: "owner", Value: "platform"}},
				Removes: []data.TagRemoval{{Key: "env", Value: "prod"}},
			}},
		},
	}
	m := newModel(t, sec)

	require.Len(t, m.rows, 2, "one add row and one remove row")

	// Row 0 is the add; `x` cancels it.
	m.selected = 0
	m, cmd := m.Update(keyPress('x'))
	require.NotNil(t, cmd)

	m, _ = m.Update(cmd()) // feed the actionDoneMsg back, clearing the busy guard

	require.Len(t, sec.cancelAdds, 1)
	assert.Equal(t, "owner", sec.cancelAdds[0].tagKey)
	assert.Equal(t, "/app/api/DATABASE_URL", sec.cancelAdds[0].key.Name)

	// Row 1 is the removal; enter (↩) cancels it.
	m.selected = 1
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	_ = cmd()

	require.Len(t, sec.cancelRemoves, 1)
	assert.Equal(t, "env", sec.cancelRemoves[0].tagKey)
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

// TestUpdate_MouseClickApplyResetCancelTag pins the mouse rule: a click on a
// section's apply/reset button and on a tag-cancel row reduces to the SAME
// internal action its key equivalent performs, with coordinates read from the
// rendered geometry (never hard-coded).
func TestUpdate_MouseClickApplyResetCancelTag(t *testing.T) {
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

	// Click the tag-add row → cancel that add (reduces to the `x` key path).
	tagLine := findRowLine(m, 1) // row 1 is the tag add (row 0 is the entry)
	require.GreaterOrEqual(t, tagLine, 0, "the tag row was rendered")

	_, cmd = m.Update(tea.MouseClickMsg{X: 4, Y: m.geom.bodyTop + tagLine, Button: tea.MouseLeft})
	require.NotNil(t, cmd)
	_ = cmd()

	require.Len(t, sec.cancelAdds, 1, "clicking the tag row cancels its staged add")
	assert.Equal(t, "owner", sec.cancelAdds[0].tagKey)
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
