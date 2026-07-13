//nolint:testpackage // white-box: drives the real App/dialog wiring and the unexported cursor scanner
package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/nav"
)

// --- unit tests for the reverse-cursor-cell scanner ---

func TestSGRHasReverse(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		params string
		want   bool
	}{
		"bare reverse":            {"7", true},
		"reverse then fg color":   {"7;38;2;2;186;132", true},
		"fg color then reverse":   {"38;2;2;186;132;7", true},
		"reset":                   {"", false},
		"reverse off":             {"27", false},
		"256 color index 7 only":  {"38;5;7", false}, // must NOT be read as reverse
		"256 color 7 bg only":     {"48;5;7", false},
		"256 color 7 then revers": {"38;5;7;7", true},
		"truecolor with 7 in rgb": {"38;2;7;7;7", false}, // 7s are r/g/b, not reverse
		"truecolor rgb then rev":  {"38;2;7;7;7;7", true},
		"plain fg":                {"38;5;238", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, sgrHasReverse(tc.params))
		})
	}
}

func TestCursorCellPos(t *testing.T) {
	t.Parallel()

	// Row 0: prompt "> " then "ab" then a reverse-video cursor cell.
	line := "\x1b[38;2;247;128;226m> \x1b[mab" + lipgloss.NewStyle().Reverse(true).Render(" ") + "   "
	content := "title\n" + line

	col, row, ok := cursorCellPos(content)
	require.True(t, ok)
	assert.Equal(t, 1, row, "cursor is on the second line")
	assert.Equal(t, 4, col, `caret sits after "> ab" (4 display cells)`)
}

func TestCursorCellPos_NoReverse(t *testing.T) {
	t.Parallel()

	content := "just\n\x1b[38;5;7mcolored\x1b[m text with a 7 in the color"
	_, _, ok := cursorCellPos(content)
	assert.False(t, ok, "a color parameter containing 7 must not be read as a caret")
}

// --- end-to-end tests: the App draws a real caret at the focused text field ---

// drainCmd runs a command tree eagerly, returning every produced message so a
// test can feed the initialization messages back into the model synchronously.
func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if msg == nil {
		return nil
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drainCmd(c)...)
		}

		return out
	}

	return []tea.Msg{msg}
}

// applyMsg feeds a message to the App and eagerly settles every command it
// produces (draining batches), returning the updated App. It uses a checked type
// assertion so the root model contract is verified as it drives the shell.
func applyMsg(t *testing.T, m *App, msg tea.Msg) *App {
	t.Helper()

	next, cmd := m.Update(msg)

	app, ok := next.(*App)
	require.True(t, ok, "App.Update must return *App")

	for _, sub := range drainCmd(cmd) {
		app = applyMsg(t, app, sub)
	}

	return app
}

// openTextDialog builds a real App, sizes it, dispatches the given open-dialog
// message, settles the resulting dialog's init commands, and replays any extra
// keys (navigation, typing). It returns the App with the dialog open.
func openTextDialog(t *testing.T, service string, open tea.Msg, keys ...tea.Msg) *App {
	t.Helper()

	m := newApp(config{
		scope:   provider.Scope{Provider: provider.ProviderAWS},
		service: service,
		mutatorFor: func(string) data.Mutator {
			return capMutator{cap: goldenCap("aws", service)}
		},
	})

	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = applyMsg(t, m, open)

	for _, k := range keys {
		m = applyMsg(t, m, k)
	}

	return m
}

func typeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// requireCaretOnScreen asserts the App draws a real cursor and that it lands on
// the reverse-video caret cell actually drawn in the composited screen — proving
// the visible caret and the terminal cursor coincide at the insertion point.
func requireCaretOnScreen(t *testing.T, m *App) tea.Position {
	t.Helper()

	view := m.View()
	require.NotNil(t, view.Cursor, "a focused text field must have a real terminal cursor")

	lines := strings.Split(view.Content, "\n")
	require.Less(t, view.Cursor.Y, len(lines), "cursor row is within the screen")

	idx := indexReverseSGR(lines[view.Cursor.Y])
	require.GreaterOrEqual(t, idx, 0, "the cursor row must carry the drawn caret cell")
	assert.Equal(t, view.Cursor.X, lipgloss.Width(lines[view.Cursor.Y][:idx]),
		"the real cursor must sit on the drawn caret cell")

	return view.Cursor.Position
}

func TestAppCursor_EntryFormName(t *testing.T) {
	t.Parallel()

	m := openTextDialog(t, "param", nav.OpenEntryForm{Service: "param"}, typeKey('a'), typeKey('b'))
	pos := requireCaretOnScreen(t, m)

	// Typing one more rune advances the caret by exactly one cell.
	m = applyMsg(t, m, typeKey('c'))
	next := requireCaretOnScreen(t, m)

	assert.Equal(t, pos.Y, next.Y, "still on the name row")
	assert.Equal(t, pos.X+1, next.X, "the caret tracks the inserted rune")
}

func TestAppCursor_EntryFormValue(t *testing.T) {
	t.Parallel()

	// Tab off the name field, past the Type select, onto the multi-line Value field.
	m := openTextDialog(t, "param", nav.OpenEntryForm{Service: "param"},
		tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyTab},
		typeKey('x'))
	requireCaretOnScreen(t, m)
}

func TestAppCursor_RestoreName(t *testing.T) {
	t.Parallel()

	m := openTextDialog(t, "secret", nav.OpenRestore{Service: "secret", Name: "prod/api/deleted"},
		typeKey('z'))
	requireCaretOnScreen(t, m)
}

func TestAppCursor_TagKey(t *testing.T) {
	t.Parallel()

	// The tag form opens on the Action select (no caret); tab to the Key input.
	m := openTextDialog(t, "param", nav.OpenTag{Service: "param", Name: "/app/api/DB"},
		tea.KeyPressMsg{Code: tea.KeyTab}, typeKey('k'))
	requireCaretOnScreen(t, m)
}

func TestAppCursor_NoneWhenNoTextField(t *testing.T) {
	t.Parallel()

	// The delete-confirm dialog has only selects/confirm rows — no text caret.
	m := openTextDialog(t, "secret", nav.OpenDelete{Service: "secret", Name: "prod/api/old"})
	assert.Nil(t, m.View().Cursor, "a dialog with no focused text field draws no cursor")

	// The tag form opens focused on the Action select — also no caret yet.
	m2 := openTextDialog(t, "param", nav.OpenTag{Service: "param", Name: "/app/api/DB"})
	assert.Nil(t, m2.View().Cursor, "a select-focused dialog draws no cursor")
}

func TestAppCursor_NoneWhenNoDialog(t *testing.T) {
	t.Parallel()

	m := newApp(config{
		scope:   provider.Scope{Provider: provider.ProviderAWS},
		service: "param",
		mutatorFor: func(string) data.Mutator {
			return capMutator{cap: goldenCap("aws", "param")}
		},
	})
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	assert.Nil(t, m.View().Cursor, "no cursor is drawn when no dialog is open")
}
