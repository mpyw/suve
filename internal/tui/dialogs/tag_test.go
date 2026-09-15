//nolint:testpackage // white-box: drives the tag form's Update/submit and inspects its unexported state
package dialogs

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/styles"
)

// TestTagForm_StagedOnlySkipsConfirm pins the #679 fix for the tag dialog: a
// staged-only launch offers no Stage/Apply choice and forces a staged tag write; a
// browser launch opens the popup on submit.
func TestTagForm_StagedOnlySkipsConfirm(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}

	browser, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
	})
	b, ok := browser.(*tagForm)
	require.True(t, ok)
	assert.True(t, b.staged, "browser launch defaults to staged")

	b.tagKey = "owner"
	m, _ := b.beginSubmit()
	b, ok = m.(*tagForm)
	require.True(t, ok)
	assert.True(t, b.confirming, "a browser submit opens the Stage/Apply popup")
	assert.Contains(t, b.View(), "Apply immediately", "the popup offers the mode choice")

	staged, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(),
		Name: "/app/X", StagedOnly: true,
	})
	s, ok := staged.(*tagForm)
	require.True(t, ok)
	assert.True(t, s.staged, "a staged-only launch forces staged")

	s.tagKey = "owner"
	m, cmd := s.beginSubmit()
	s, ok = m.(*tagForm)
	require.True(t, ok)
	assert.False(t, s.confirming, "a staged-only submit shows no popup")
	require.NotNil(t, cmd, "a staged-only submit writes directly")

	execCmd(t, cmd)
	assert.True(t, mut.staged, "a staged-only submit writes staged, never immediate")
}

// TestTagForm_Routing pins that the tag form routes to AddTag/RemoveTag per the
// action select and carries the mode.
func TestTagForm_Routing(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
		Tags: []data.Tag{{Key: "owner", Value: "team"}},
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	d.tagKey, d.tagValue, d.staged = "env", "prod", true

	d.remove = false
	execCmd(t, d.submit())
	assert.True(t, mut.addTagCalled, "add action routes to AddTag with the free-text key")
	assert.Equal(t, "env", mut.tagKey)
	assert.True(t, mut.staged)

	// Remove routes the key chosen from the existing-tags select (removeKey), never
	// the free-text Add key.
	d.remove = true
	d.removeKey = "owner"
	execCmd(t, d.submit())
	assert.Equal(t, "owner", mut.tagKey, "remove action routes to RemoveTag with the selected key")
}

// TestTagForm_RemoveConstrainedToExistingTags pins the #705 fix: the Remove
// action is a select of the entry's CURRENT tags (labelled key=value, valued by
// key so it routes straight to RemoveTag), never a blind free-text key. The
// select seeds a present key by default, so a Remove always has a valid target,
// and choosing a present tag stages the untag with that key. Add stays a
// free-text key + value (adding a new tag is legitimately open-ended).
func TestTagForm_RemoveConstrainedToExistingTags(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
		Tags: []data.Tag{{Key: "env", Value: "prod"}, {Key: "team", Value: "api"}},
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	// Add (the default) is a free-text key input, unchanged.
	assert.Contains(t, stripANSI(d.View()), "Key", "Add offers a free-text key input")
	assert.Contains(t, stripANSI(d.View()), "(add only)", "Add offers the value input")

	// Remove offers a select of the existing tags, not a free-text key.
	_, isSelect := d.removeField().(*huh.Select[string])
	assert.True(t, isSelect, "Remove offers a select of existing tags, not a free-text key")

	d.remove = true
	require.NotNil(t, d.rebuildForm())
	assert.Equal(t, "env", d.removeKey, "the select seeds the first existing tag as the default target")

	view := stripANSI(d.View())
	assert.Contains(t, view, "env=prod", "the select lists the existing tags as key=value options")
	assert.Contains(t, view, "team=api")
	assert.NotContains(t, view, "(add only)", "the Add-only value input is gone in Remove mode")

	// Selecting a present tag stages the untag with that key.
	d.removeKey = "team"
	execCmd(t, d.submit())
	assert.Equal(t, "team", mut.tagKey, "the chosen present key is the untag target")
}

// TestTagForm_StagedOnlyIsAddOnly pins the staged-only surface (the staging
// review page) is Add-only: it never has the remote tag set to constrain a Remove,
// so with only one possible action the Action select is dropped entirely — the form
// opens straight on the Key field (no inert one-option select that still demands an
// Enter). Removing a remote tag is done from the browser.
func TestTagForm_StagedOnlyIsAddOnly(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(),
		Name: "/app/X", StagedOnly: true,
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	view := stripANSI(d.View())
	assert.NotContains(t, view, "Action", "a single-action tag form drops the Action select")
	assert.NotContains(t, view, "Remove tag", "the staged-only tag form offers no Remove action")
	assert.Contains(t, view, "Key", "the form opens straight on the Key field")
	assert.False(t, d.remove, "a staged-only tag write is always an Add")

	// A browser launch (not staged-only) with removable tags keeps the Action
	// select and can toggle to Remove.
	bm, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
		Tags: []data.Tag{{Key: "env", Value: "prod"}},
	})
	b, ok := bm.(*tagForm)
	require.True(t, ok)

	assert.Contains(t, stripANSI(b.View()), "Action", "a genuine Add/Remove choice keeps the Action select")

	_, _ = b.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	assert.True(t, b.remove, "a browser launch with tags can toggle to Remove")
}

// TestTagForm_EmptyTagsFallbackToAdd pins the #761 defensive fallback: if the
// Remove action is somehow active with an empty tag set (it should never be
// offered), rebuilding the form falls back to Add rather than building a select
// with nothing to pick.
func TestTagForm_EmptyTagsFallbackToAdd(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	d.remove = true
	require.NotNil(t, d.rebuildForm())
	assert.False(t, d.remove, "an empty tag set falls back to Add rather than a dead-end Remove")
	assert.NotContains(t, stripANSI(d.View()), "(no tags to remove)", "no dead-end note is rendered")
}

// TestTagForm_ActionToggleMorphsKeyField pins that toggling the action select
// rebuilds the form so the key field morphs between the free-text Add input and
// the Remove select — the mechanism behind the #705 constraint.
func TestTagForm_ActionToggleMorphsKeyField(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
		Tags: []data.Tag{{Key: "env", Value: "prod"}},
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	_, _ = d.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	require.False(t, d.builtRemove, "the form starts on the Add branch")

	// Right arrow toggles the inline action select to Remove; the next Update pass
	// rebuilds the form onto the Remove branch.
	_, _ = d.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	assert.True(t, d.remove, "right toggles the action to Remove")
	assert.True(t, d.builtRemove, "the form rebuilt onto the Remove branch")
	assert.Contains(t, stripANSI(d.View()), "env=prod", "the Remove select is now shown")
}

// TestTagForm_EmptyTagsHidesRemove pins the #761 fix: an entry with no loaded
// tags has nothing to untag, so the Action toggle offers Add only — the user is
// never lured into a Remove that cannot select anything. Toggling the action can
// never reach Remove, and the "(no tags to remove)" dead-end is never rendered.
func TestTagForm_EmptyTagsHidesRemove(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
		// No Tags: an empty tag set must not offer Remove.
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	require.False(t, d.remove, "the form starts on Add")

	// Toggling the inline action select right cannot reach Remove: it is not
	// offered when there is nothing to remove.
	_, _ = d.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	assert.False(t, d.remove, "an entry with no tags never offers Remove, so the action stays on Add")

	// The empty-state dead-end note is never rendered.
	assert.NotContains(t, stripANSI(d.View()), "(no tags to remove)", "no dead-end note is ever shown")
	assert.Empty(t, mut.tagKey, "no tag write is routed")
}

// TestTagForm_EscDiscardGuard pins the #790 guard on the tag form: a clean form
// closes on the first Esc; a form with a typed Add key arms on the first Esc and
// discards only on the second.
func TestTagForm_EscDiscardGuard(t *testing.T) {
	t.Parallel()

	newTag := func() *tagForm {
		m, _ := NewTagForm(TagInput{
			Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsParamCap()},
			Service: "param", Styles: styles.New(), Name: "/app/X",
		})
		d, ok := m.(*tagForm)
		require.True(t, ok)

		return d
	}

	// Clean form: the first Esc cancels immediately.
	clean := newTag()
	_, cmd := clean.Update(keyEsc())
	assert.True(t, isCanceled(cmd), "esc on a clean tag form cancels immediately")

	// Dirty form (a typed Add key): first Esc arms, second discards.
	dirty := newTag()
	dirty.tagKey = "env"

	_, cmd = dirty.Update(keyEsc())
	assert.True(t, dirty.armed, "first esc on a dirty tag form arms the discard")
	assert.Equal(t, discardNotice, dirty.notice)
	assert.False(t, isCanceled(cmd), "first esc on a dirty tag form does not cancel")

	_, cmd = dirty.Update(keyEsc())
	assert.True(t, isCanceled(cmd), "a second consecutive esc discards")
}

// TestTagForm_ConfirmCommit pins that completing the tag form opens the Stage/Apply
// popup and Enter there commits through AddTag. The required Add-key validation is
// huh's (run inline as the form advances); the empty-key case is pinned separately
// on the validator.
func TestTagForm_ConfirmCommit(t *testing.T) {
	t.Parallel()

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	d.tagKey, d.tagValue = "env", "prod"

	m, _ = d.beginSubmit()
	d, ok = m.(*tagForm)
	require.True(t, ok)
	require.True(t, d.confirming, "completing the tag form opens the Stage/Apply popup")
	assert.False(t, d.busy, "opening the popup does not yet write")

	m, cmd := d.Update(keyEnter())
	d, ok = m.(*tagForm)
	require.True(t, ok)
	require.NotNil(t, cmd, "enter in the popup emits the mutation command")
	assert.True(t, d.busy)

	_ = cmd()

	assert.True(t, mut.addTagCalled, "committing routes through AddTag")
	assert.Equal(t, "env", mut.tagKey)
}

// TestTagForm_ResultVoicing pins the tag status voicing (staged/applied,
// add/removal) and that it reaches the MutationDoneMsg status line. Tags carry no
// auto-unstage/skip outcome today (TagOutput/UntagOutput hold only the name), so
// the tag equivalent that surfaces is the staged/applied voicing.
func TestTagForm_ResultVoicing(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Staged tag add.", tagStatus(false, true))
	assert.Equal(t, "Staged tag removal.", tagStatus(true, true))
	assert.Equal(t, "Applied tag add.", tagStatus(false, false))
	assert.Equal(t, "Applied tag removal.", tagStatus(true, false))

	mut := &fakeMutator{svcCap: awsParamCap()}
	m, _ := NewTagForm(TagInput{
		Ctx: context.Background(), Mutator: mut, Service: "param", Styles: styles.New(), Name: "/app/X",
	})
	d, ok := m.(*tagForm)
	require.True(t, ok)

	d.remove, d.staged = true, true

	_, cmd := d.onResult(mutationResultMsg{outcome: data.WriteOutcome{}})
	done, ok := cmd().(MutationDoneMsg)
	require.True(t, ok, "a successful tag write emits MutationDoneMsg")
	assert.Equal(t, "Staged tag removal.", done.Status, "tag voicing surfaces in the status line")
}
