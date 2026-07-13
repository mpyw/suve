//nolint:testpackage // white-box: drives the unexported loadedMsg/render and reads the rendered viewport
package diff

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/styles"
)

// newDiff builds a diff page sized for a render. It does not fetch (no source);
// tests feed a loadedMsg directly.
func newDiff(t *testing.T) *Model {
	t.Helper()

	m := New(context.Background(), nav.OpenDiff{Name: "api-key"}, styles.New(), keys.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	return m
}

// keyPress builds a printable key press.
func keyPress(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// TestSecretDiffRevealedByDefaultAndHideToggle pins the relaxed policy: the
// Compare/diff view is a surface the user explicitly opened to inspect the
// change, so a secret's values are SHOWN by default (#702/#735) — and `x` hides
// them again, masking both sides into differing bullet runs that still show a
// change without disclosing content.
func TestSecretDiffRevealedByDefaultAndHideToggle(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	const (
		oldSecret = "googlecloud-secret-2"           // 20 runes → 20 bullets
		newSecret = "googlecloud-secret-value-three" // 30 runes → capped 24 bullets
	)

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "api-key#2",
		NewLabel: "api-key#3",
		OldValue: oldSecret,
		NewValue: newSecret,
		Secret:   true,
	}})

	// Revealed by default: both cleartext values are shown so the diff is useful.
	shown := m.View(100, 40)
	assert.Contains(t, shown, oldSecret, "the old secret value is shown by default")
	assert.Contains(t, shown, newSecret, "the new secret value is shown by default")
	assert.NotContains(t, shown, "•", "a revealed secret diff has no mask bullets")

	// `x` hides it: neither cleartext value reaches the viewport, and a masked
	// difference still renders (differing bullet runs).
	m, _ = m.Update(keyPress('x'))
	require.True(t, m.masked, "x masks the secret diff")

	hidden := m.View(100, 40)
	assert.NotContains(t, hidden, oldSecret, "the old secret value must never be rendered while masked")
	assert.NotContains(t, hidden, newSecret, "the new secret value must never be rendered while masked")
	assert.NotContains(t, hidden, "secret", "no fragment of a secret value leaks while masked")
	assert.Contains(t, hidden, "•", "the masked diff renders bullet runs")
	assert.Contains(t, hidden, "-•", "a removed masked line is shown")
	assert.Contains(t, hidden, "+•", "an added masked line is shown")
	assert.NotContains(t, hidden, "(no differences)", "the differing versions produce a masked diff")

	// `x` again reveals: back to cleartext.
	m, _ = m.Update(keyPress('x'))
	require.False(t, m.masked, "x toggles back to revealed")
	assert.Contains(t, m.View(100, 40), newSecret, "toggling back shows the value again")
}

// TestSecretDiffHiddenEqualLengthsMaskToNoDifference pins that once hidden, two
// secret values that mask to the same run (equal length) collapse to
// "(no differences)" rather than leaking that they in fact differ — masking is
// length-only.
func TestSecretDiffHiddenEqualLengthsMaskToNoDifference(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "s#1",
		NewLabel: "s#2",
		OldValue: "aaaaaaaa", // 8 runes
		NewValue: "bbbbbbbb", // 8 runes → same 8 bullets when masked
		Secret:   true,
	}})

	// Hide first, then the equal-length masked runs are indistinguishable.
	m, _ = m.Update(keyPress('x'))

	out := m.View(100, 40)

	assert.NotContains(t, out, "aaaaaaaa")
	assert.NotContains(t, out, "bbbbbbbb")
	assert.Contains(t, out, "(no differences)", "equal-length masked values diff as identical")
}

// TestParamDiffAlwaysFormatsJSON pins the non-secret path: a param diff renders
// the real content (not masked), and a JSON value is ALWAYS pretty-printed
// before diffing (GUI parity, #732) — no manual toggle. The compact single-line
// objects never reach the screen; the expanded, indented JSON does.
func TestParamDiffAlwaysFormatsJSON(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "cfg#1",
		NewLabel: "cfg#2",
		OldValue: `{"a":1}`,
		NewValue: `{"a":2}`,
		Secret:   false,
	}})

	out := m.View(100, 40)
	assert.Contains(t, out, `"a": 1`, "the JSON old value is pretty-printed by default")
	assert.Contains(t, out, `"a": 2`, "the JSON new value is pretty-printed by default")
	assert.NotContains(t, out, `{"a":1}`, "the compact single-line form is never shown")
	assert.NotContains(t, out, "•", "a non-secret diff has no mask bullets")
}

// TestDiffReflowsOnResize pins #732's repaint fix: a WindowSizeMsg re-syncs the
// viewport content so formatted JSON reflows to the new width and never clips.
func TestDiffReflowsOnResize(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "cfg#1",
		NewLabel: "cfg#2",
		OldValue: `{"host":"a.internal","port":1}`,
		NewValue: `{"host":"b.internal","port":2}`,
		Secret:   false,
	}})

	wide := m.vp.View()

	// A height/width change must re-render the viewport content (not leave a stale,
	// clipped frame from the old size).
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	narrow := m.vp.View()

	require.NotEqual(t, wide, narrow, "a resize re-syncs the viewport content")
	assert.Contains(t, narrow, `"host": "a.internal"`, "the formatted JSON is still present after the resize")
}

// sbsLineWith reports whether any line of s contains every one of subs — used to
// assert the side-by-side layout colocates the old and new content on one row
// (the unified layout never does).
func sbsLineWith(s string, subs ...string) bool {
	for line := range strings.SplitSeq(s, "\n") {
		ok := true

		for _, sub := range subs {
			if !strings.Contains(line, sub) {
				ok = false

				break
			}
		}

		if ok {
			return true
		}
	}

	return false
}

// TestSideBySideToggle pins #674: `s` switches the diff page to the two-column
// (old | new) layout, colocating a changed line's old and new text on one row
// with the column gutter between them; `s` again restores the unified layout,
// where the two sides live on separate -/+ lines.
func TestSideBySideToggle(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "cfg#1",
		NewLabel: "cfg#2",
		OldValue: `{"host":"a.internal","port":1}`,
		NewValue: `{"host":"b.internal","port":2}`,
		Secret:   false,
	}})

	// Unified by default: the old and new values are on different lines, never one.
	unified := m.vp.View()
	assert.False(t, sbsLineWith(unified, `"host": "a.internal"`, `"host": "b.internal"`),
		"the unified layout keeps the two sides on separate lines")

	// Toggle to side-by-side: a changed line's old and new text share one row,
	// separated by the gutter.
	m, _ = m.Update(keyPress('s'))
	require.True(t, m.sideBySide, "s enables the side-by-side layout")

	sbs := m.vp.View()
	assert.True(t, sbsLineWith(sbs, `"host": "a.internal"`, "│", `"host": "b.internal"`),
		"the side-by-side layout colocates old and new across the gutter")
	assert.Contains(t, sbs, "cfg#1", "the old column is headed by its label")
	assert.Contains(t, sbs, "cfg#2", "the new column is headed by its label")

	// Toggle back: unified again.
	m, _ = m.Update(keyPress('s'))
	require.False(t, m.sideBySide, "s toggles back to unified")
	assert.False(t, sbsLineWith(m.vp.View(), `"host": "a.internal"`, `"host": "b.internal"`),
		"toggling back restores the unified layout")
}

// TestSideBySideReflowsOnResize pins that a WindowSizeMsg re-renders the
// side-by-side columns to the new width (post-#732 repaint discipline): the two
// column cells narrow with the terminal rather than leaving a stale wide frame.
func TestSideBySideReflowsOnResize(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "cfg#1",
		NewLabel: "cfg#2",
		OldValue: `{"host":"a.internal","port":1}`,
		NewValue: `{"host":"b.internal","port":2}`,
		Secret:   false,
	}})
	m, _ = m.Update(keyPress('s'))

	wide := m.vp.View()

	// A narrower (but still splittable) size must re-render the columns.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	narrow := m.vp.View()

	require.NotEqual(t, wide, narrow, "a resize re-syncs the side-by-side content")
	assert.True(t, m.sideBySide, "the layout stays side-by-side across a splittable resize")
	assert.True(t, sbsLineWith(narrow, `"host": "a.internal"`, "│", `"host": "b.internal"`),
		"the two columns still render after the resize")
}

// TestSideBySideFallsBackBelowMinWidth pins the min-width fallback (#674): when
// the terminal is too narrow to split two readable columns, side-by-side degrades
// to the unified layout even though the toggle stays on (so widening restores it).
func TestSideBySideFallsBackBelowMinWidth(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "cfg#1",
		NewLabel: "cfg#2",
		OldValue: `{"host":"a.internal","port":1}`,
		NewValue: `{"host":"b.internal","port":2}`,
		Secret:   false,
	}})
	m, _ = m.Update(keyPress('s'))

	// Too narrow to split: falls back to unified (the two sides no longer colocate).
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	require.True(t, m.sideBySide, "the toggle stays on while narrow")

	narrow := m.vp.View()
	assert.False(t, sbsLineWith(narrow, `"host": "a.internal"`, `"host": "b.internal"`),
		"below the min split width the layout degrades to unified")

	// Widening restores the two-column layout without a second keypress.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	assert.True(t, sbsLineWith(m.vp.View(), `"host": "a.internal"`, "│", `"host": "b.internal"`),
		"widening past the min width restores side-by-side")
}

// TestSideBySideHonorsMask pins that the side-by-side layout obeys the same
// reveal/hide policy as unified: a revealed secret shows both values across the
// gutter, and `x` masks BOTH columns into bullet runs — no cleartext leaks in
// either column (#674, #702/#735).
func TestSideBySideHonorsMask(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	const (
		oldSecret = "googlecloud-secret-2"
		newSecret = "googlecloud-secret-value-three"
	)

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "api-key#2",
		NewLabel: "api-key#3",
		OldValue: oldSecret,
		NewValue: newSecret,
		Secret:   true,
	}})
	m, _ = m.Update(keyPress('s'))

	revealed := m.vp.View()
	assert.Contains(t, revealed, oldSecret, "side-by-side reveals the old secret by default")
	assert.Contains(t, revealed, newSecret, "side-by-side reveals the new secret by default")

	// `x` masks both columns: no cleartext, bullet runs on both sides.
	m, _ = m.Update(keyPress('x'))
	masked := m.vp.View()
	assert.NotContains(t, masked, oldSecret, "the old secret must not leak in side-by-side while masked")
	assert.NotContains(t, masked, newSecret, "the new secret must not leak in side-by-side while masked")
	assert.NotContains(t, masked, "secret", "no fragment of a secret leaks in side-by-side while masked")
	assert.Contains(t, masked, "•", "the masked side-by-side diff renders bullet runs")
}

// diffShortDescs flattens the diff page's short-help descriptions.
func diffShortDescs(m *Model) []string {
	km := m.HelpKeyMap()
	descs := make([]string, 0, len(km.ShortHelp()))

	for _, b := range km.ShortHelp() {
		descs = append(descs, b.Help().Desc)
	}

	return descs
}

// TestHelpKeyMap_MaskGatedOnSecret pins that the diff page's help advertises the
// `x` hide/show toggle only on a loaded secret diff — it is a no-op on a
// non-secret diff, so it must not appear there.
func TestHelpKeyMap_MaskGatedOnSecret(t *testing.T) {
	t.Parallel()

	nonSecret := newDiff(t)
	nonSecret, _ = nonSecret.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "a#1", NewLabel: "a#2", OldValue: "1", NewValue: "2", Secret: false,
	}})
	assert.NotContains(t, diffShortDescs(nonSecret), "hide/show", "a non-secret diff has no mask toggle")
	assert.NotContains(t, diffShortDescs(nonSecret), "parse-json", "the parse-json toggle is gone (JSON is always formatted, #732)")
	assert.Contains(t, diffShortDescs(nonSecret), "side-by-side", "a loaded diff advertises the layout toggle (#674)")
	assert.Contains(t, diffShortDescs(nonSecret), "back", "esc back is always available")

	secret := newDiff(t)
	secret, _ = secret.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "s#1", NewLabel: "s#2", OldValue: "a", NewValue: "b", Secret: true,
	}})
	assert.Contains(t, diffShortDescs(secret), "hide/show", "a secret diff advertises the mask toggle")
}
