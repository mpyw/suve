// Package keys defines the TUI's global key map on bubbles/v2/key, plus its
// bubbles/v2/help integration. Keeping the bindings data-driven (rather than
// string-matching in Update) lets the help bar render the same source of truth
// the reducer dispatches on.
package keys

import (
	"charm.land/bubbles/v2/key"
)

// Map is the global key map. Page-local maps are layered on top in later steps;
// this set is what the app shell dispatches and what the help bar renders.
type Map struct {
	// NextTab / PrevTab cycle tabs; Tab1..Tab3 jump directly.
	NextTab key.Binding
	PrevTab key.Binding
	Tab1    key.Binding
	Tab2    key.Binding
	Tab3    key.Binding

	// Up / Down / Select / Back drive the (placeholder) page focus and, from
	// Step 3 on, the master-detail browser.
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Back   key.Binding

	// Copy yanks the focused value to the system clipboard via OSC52.
	Copy key.Binding

	// Help toggles the short/full help bar; Quit exits the program.
	Help key.Binding
	Quit key.Binding
}

// Default returns the initial global key map. Bubble Tea v2 note: the space key
// is spelled "space" (not " "), so any future space binding must use that form.
func Default() Map {
	return Map{
		NextTab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev tab"),
		),
		Tab1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1/2/3", "jump to tab"),
		),
		Tab2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "tab 2"),
		),
		Tab3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "tab 3"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Copy: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp implements help.KeyMap: the one-line hint shown by default.
func (m Map) ShortHelp() []key.Binding {
	return []key.Binding{m.NextTab, m.Tab1, m.Help, m.Quit}
}

// FullHelp implements help.KeyMap: the expanded, column-grouped help shown when
// the help bar is toggled open.
func (m Map) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{m.NextTab, m.PrevTab, m.Tab1},
		{m.Up, m.Down, m.Select, m.Back},
		{m.Copy, m.Help, m.Quit},
	}
}
