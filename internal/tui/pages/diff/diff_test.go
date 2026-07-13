//nolint:testpackage // white-box: drives the unexported loadedMsg/render and reads the rendered viewport
package diff

import (
	"context"
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
	assert.Contains(t, diffShortDescs(nonSecret), "back", "esc back is always available")

	secret := newDiff(t)
	secret, _ = secret.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "s#1", NewLabel: "s#2", OldValue: "a", NewValue: "b", Secret: true,
	}})
	assert.Contains(t, diffShortDescs(secret), "hide/show", "a secret diff advertises the mask toggle")
}
