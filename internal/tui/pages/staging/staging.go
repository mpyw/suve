// Package staging implements the TUI's staging review page: per-service sections
// listing staged entries (as Remote-vs-Staged diffs or raw staged values) and
// independent staged tag changes, with unstage / edit-staged / cancel-tag row
// actions and the apply and reset flows. It completes the TUI's stage → review →
// apply loop. Only the services the launched scope offers get a section; every
// staged read is a tea.Cmd guarded by a monotonic sequence (the browser's
// loadSeq pattern), and a staging read failure degrades to a per-section error
// line rather than blocking the rest of the app.
package staging

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/styles"
)

// serviceSecret is the secret service key (the sections' masking axis).
const serviceSecret = "secret"

// Page-local key bindings not present in the global map.
//
//nolint:gochecknoglobals // immutable page-local bindings
var (
	viewKey     = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view"))
	revealKey   = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "reveal/cancel-tag"))
	resetKey    = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset"))
	unstageKey  = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "unstage"))
	editKey     = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	tagKey      = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tags"))
	applyKey    = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "apply"))
	applyAllKey = key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "apply-all"))
	resetAllKey = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reset-all"))
	refreshKey  = key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "refresh"))
)

// section is one service's staged review plus its load/error state.
type section struct {
	svc     data.StagingService
	service string // "param" / "secret"
	label   string
	secret  bool

	review  data.StagingReview
	loaded  bool
	err     string
	loadSeq int
}

// entryRows returns the section's still-staged entry rows (auto-unstaged rows are
// excluded — they moved to the dismissible notice).
func (s *section) entryRows() []data.StagedDiffRow {
	rows := make([]data.StagedDiffRow, 0, len(s.review.Entries))

	for _, e := range s.review.Entries {
		if e.Type != data.StagedDiffAutoUnstaged {
			rows = append(rows, e)
		}
	}

	return rows
}

// Model is the staging page.
type Model struct {
	// ctx is the Run context threaded through every staged read/write command, so
	// a fetch is cancelled when the program exits.
	ctx context.Context //nolint:containedctx // fetch commands need the Run context; mirrors the browser

	sections []*section

	styles styles.Styles
	keys   keys.Map

	width  int
	height int

	// diffView selects diff (Remote vs Staged) vs value (raw staged) rendering.
	diffView bool
	// reveal unmasks secret staged values in value view (never persisted, reset
	// on load).
	reveal bool
	// noticeDismissed hides the auto-unstaged notice until the next load.
	noticeDismissed bool

	// rows is the flattened list of selectable rows (entries + tag changes across
	// all sections), rebuilt on every load; selected indexes into it.
	rows     []rowRef
	selected int

	// actionBusy guards the inline row actions (unstage / cancel-tag) so a
	// double-press never fires two writes at once (#568).
	actionBusy bool

	// scroll is the section-body scroll offset (wheel/overflow).
	scroll int
	// scrollToSelection, set when the selection moves, tells the next View to
	// scroll the body so the selected row is visible.
	scrollToSelection bool

	// geom records the last-rendered line hit-map so mouse handlers hit-test what
	// is actually on screen (never a hard-coded coordinate) — the browser geom
	// pattern, kept consistent for the #663 compositor migration.
	geom stagingGeom
}

// New builds the staging page over the offered services' staging seams (param
// and/or secret, in tab order). ctx is the Run context threaded through reads.
func New(ctx context.Context, services []data.StagingService, st styles.Styles, km keys.Map) *Model {
	sections := make([]*section, 0, len(services))
	for _, svc := range services {
		sections = append(sections, &section{
			svc:     svc,
			service: svc.Service(),
			label:   svc.Label(),
			secret:  svc.Service() == serviceSecret,
		})
	}

	return &Model{
		ctx:      ctx,
		sections: sections,
		styles:   st,
		keys:     km,
		diffView: true, // diff is the default view (the mock's [diff] value)
	}
}

// Init dispatches the initial staged reads for every section.
func (m *Model) Init() tea.Cmd {
	return m.reload()
}

// CapturesInput is always false: the staging page has no focused text input, so
// the app's global key map stays active.
func (m *Model) CapturesInput() bool { return false }

// reload re-reads every section's staged review, each guarded by a fresh
// sequence, and clears the dismissible notice so a fresh auto-unstage shows.
func (m *Model) reload() tea.Cmd {
	m.noticeDismissed = false
	m.reveal = false

	cmds := make([]tea.Cmd, 0, len(m.sections))
	for i := range m.sections {
		cmds = append(cmds, m.reviewCmd(i))
	}

	return tea.Batch(cmds...)
}

// reviewCmd issues a staged-review read for section i, tagged with a fresh
// sequence so a stale response is dropped.
func (m *Model) reviewCmd(i int) tea.Cmd {
	sec := m.sections[i]
	sec.loadSeq++
	seq := sec.loadSeq
	ctx := m.ctx
	svc := sec.svc

	return func() tea.Msg {
		review, err := svc.Review(ctx)

		return reviewLoadedMsg{section: i, seq: seq, review: review, err: err}
	}
}
