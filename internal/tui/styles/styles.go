// Package styles holds the single Styles struct that every TUI component draws
// with, built on lipgloss v2. Colors degrade automatically to the terminal's
// color profile at render time, and NO_COLOR strips them entirely so the TUI
// stays legible on monochrome terminals and in captured (golden) output.
package styles

import (
	"os"

	"charm.land/lipgloss/v2"
)

// Styles is the flat set of lipgloss styles shared across the TUI. It is passed
// by value into components so there is one source of truth for the palette.
type Styles struct {
	// StatusBar frames the top status line (provider + scope).
	StatusBar lipgloss.Style
	// StatusKey styles a scope key label (e.g. "profile:").
	StatusKey lipgloss.Style
	// StatusValue styles a scope value (e.g. the account id).
	StatusValue lipgloss.Style
	// TabActive styles the selected tab.
	TabActive lipgloss.Style
	// TabInactive styles an unselected tab.
	TabInactive lipgloss.Style
	// Separator styles the horizontal rule between the tab bar and the page.
	Separator lipgloss.Style
	// PageHint styles the muted placeholder text inside a page.
	PageHint lipgloss.Style
	// HelpBar styles the bottom help line.
	HelpBar lipgloss.Style
	// Dialog styles a modal dialog box drawn over the page.
	Dialog lipgloss.Style

	// Pane frames a titled content pane (the list and detail boxes).
	Pane lipgloss.Style
	// PaneFocused frames the pane that currently holds keyboard focus, with an
	// accent border so the active pane reads as distinct from the idle one.
	PaneFocused lipgloss.Style
	// PaneTitle styles a pane's title line.
	PaneTitle lipgloss.Style
	// Selection styles the selected row in the pane that currently holds focus
	// (the active cursor).
	Selection lipgloss.Style
	// SelectionInactive styles the selected row in a pane that does NOT hold focus,
	// dimmed so the two panes never look equally selected at once.
	SelectionInactive lipgloss.Style
	// FieldLabel styles a metadata row label (e.g. "Version").
	FieldLabel lipgloss.Style
	// Banner styles the detail pane's staged-changes warning banner.
	Banner lipgloss.Style
	// ErrorText styles the page's error line.
	ErrorText lipgloss.Style

	// DiffHeader / DiffHunk / DiffAdded / DiffRemoved style the four diff line
	// classes in the diff page's viewport.
	DiffHeader  lipgloss.Style
	DiffHunk    lipgloss.Style
	DiffAdded   lipgloss.Style
	DiffRemoved lipgloss.Style
}

// dialogPadX is the modal dialog's horizontal interior padding (each side), so
// content is not jammed against the box border (#698). Vertical padding is 0 to
// preserve the row budget: the tallest fixed dialog must still fit the 60×16
// minimum. It MUST stay in sync with dialogs.dialogChrome, which un-offsets a
// modal click by the same frame.
const dialogPadX = 2

// New builds the default Styles. When NO_COLOR is set the palette collapses to
// unstyled text (bold is kept for emphasis, which NO_COLOR does not govern),
// keeping the layout intact without any color escapes.
func New() Styles {
	if os.Getenv("NO_COLOR") != "" {
		return Styles{
			StatusBar:   lipgloss.NewStyle().Bold(true),
			StatusKey:   lipgloss.NewStyle(),
			StatusValue: lipgloss.NewStyle().Bold(true),
			TabActive:   lipgloss.NewStyle().Bold(true),
			TabInactive: lipgloss.NewStyle(),
			Separator:   lipgloss.NewStyle(),
			PageHint:    lipgloss.NewStyle(),
			HelpBar:     lipgloss.NewStyle(),
			Dialog:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, dialogPadX),
			Pane:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
			// No color to distinguish the focused pane border under NO_COLOR; the
			// selection marker and the adaptive hint carry the focus cue instead.
			PaneFocused:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
			PaneTitle:         lipgloss.NewStyle().Bold(true),
			Selection:         lipgloss.NewStyle().Bold(true),
			SelectionInactive: lipgloss.NewStyle(),
			FieldLabel:        lipgloss.NewStyle(),
			Banner:            lipgloss.NewStyle(),
			ErrorText:         lipgloss.NewStyle().Bold(true),
			DiffHeader:        lipgloss.NewStyle().Bold(true),
			DiffHunk:          lipgloss.NewStyle(),
			DiffAdded:         lipgloss.NewStyle(),
			DiffRemoved:       lipgloss.NewStyle(),
		}
	}

	var (
		accent   = lipgloss.Color("6")   // cyan
		muted    = lipgloss.Color("241") // gray
		subtle   = lipgloss.Color("244")
		fgBright = lipgloss.Color("15")
		yellow   = lipgloss.Color("3")
		green    = lipgloss.Color("2")
		red      = lipgloss.Color("1")
	)

	return Styles{
		StatusBar:         lipgloss.NewStyle().Bold(true).Foreground(fgBright),
		StatusKey:         lipgloss.NewStyle().Foreground(muted),
		StatusValue:       lipgloss.NewStyle().Foreground(accent).Bold(true),
		TabActive:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(accent).Padding(0, 1),
		TabInactive:       lipgloss.NewStyle().Foreground(subtle).Padding(0, 1),
		Separator:         lipgloss.NewStyle().Foreground(muted),
		PageHint:          lipgloss.NewStyle().Foreground(muted).Italic(true),
		HelpBar:           lipgloss.NewStyle().Foreground(muted),
		Dialog:            lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Foreground(fgBright).Padding(0, dialogPadX),
		Pane:              lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted),
		PaneFocused:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent),
		PaneTitle:         lipgloss.NewStyle().Bold(true).Foreground(accent),
		Selection:         lipgloss.NewStyle().Foreground(accent).Bold(true),
		SelectionInactive: lipgloss.NewStyle().Foreground(subtle),
		FieldLabel:        lipgloss.NewStyle().Foreground(muted),
		Banner:            lipgloss.NewStyle().Foreground(yellow),
		ErrorText:         lipgloss.NewStyle().Bold(true).Foreground(red),
		DiffHeader:        lipgloss.NewStyle().Bold(true).Foreground(accent),
		DiffHunk:          lipgloss.NewStyle().Foreground(muted),
		DiffAdded:         lipgloss.NewStyle().Foreground(green),
		DiffRemoved:       lipgloss.NewStyle().Foreground(red),
	}
}
