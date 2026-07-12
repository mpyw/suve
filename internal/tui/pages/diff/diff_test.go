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

// TestSecretDiffMasksBothSides pins the leak guard: a SECRET diff whose two
// versions differ renders +/- lines, but every line is a run of mask bullets —
// no revealed secret value reaches the viewport. The two values differ in length
// so the masked runs differ, proving a change is still visible without content.
func TestSecretDiffMasksBothSides(t *testing.T) {
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

	out := m.View(100, 40)

	// No revealed secret anywhere in the output.
	assert.NotContains(t, out, oldSecret, "the old secret value must never be rendered")
	assert.NotContains(t, out, newSecret, "the new secret value must never be rendered")
	assert.NotContains(t, out, "secret", "no fragment of a secret value leaks")

	// A real, masked difference: both a removed and an added bullet line.
	assert.Contains(t, out, "•", "the masked diff renders bullet runs")
	assert.Contains(t, out, "-•", "a removed masked line is shown")
	assert.Contains(t, out, "+•", "an added masked line is shown")
	assert.NotContains(t, out, "(no differences)", "the differing versions produce a diff")
}

// TestSecretDiffEqualLengthsMaskToNoDifference pins that when two secret values
// mask to the same run (equal length), the diff collapses to "(no differences)"
// rather than leaking that they in fact differ — masking is length-only.
func TestSecretDiffEqualLengthsMaskToNoDifference(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "s#1",
		NewLabel: "s#2",
		OldValue: "aaaaaaaa", // 8 runes
		NewValue: "bbbbbbbb", // 8 runes → same 8 bullets
		Secret:   true,
	}})

	out := m.View(100, 40)

	assert.NotContains(t, out, "aaaaaaaa")
	assert.NotContains(t, out, "bbbbbbbb")
	assert.Contains(t, out, "(no differences)", "equal-length masked values diff as identical")
}

// TestParamDiffShownAndParseJSONToggle pins the non-secret path: a param diff
// renders the real content (not masked), and the J toggle re-diffs with
// jsonutil.TryFormat normalization (parity with the CLI's --parse-json).
func TestParamDiffShownAndParseJSONToggle(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "cfg#1",
		NewLabel: "cfg#2",
		OldValue: `{"a":1}`,
		NewValue: `{"a":2}`,
		Secret:   false,
	}})

	before := m.View(100, 40)
	assert.Contains(t, before, `{"a":1}`, "a non-secret param value is shown verbatim, never masked")
	assert.Contains(t, before, `{"a":2}`)
	assert.NotContains(t, before, "•", "a non-secret diff has no mask bullets")

	// Toggle parse-json: both sides reformat, so the diff now shows the expanded,
	// indented JSON (the "a": N lines) rather than the compact objects.
	m, _ = m.Update(keyPress('J'))
	require.True(t, m.parseJSON, "J enables parse-json")

	after := m.View(100, 40)
	assert.Contains(t, after, `"a": 1`, "parse-json expands the old value")
	assert.Contains(t, after, `"a": 2`, "parse-json expands the new value")
	assert.NotContains(t, after, `{"a":1}`, "the compact form is gone after formatting")
}
