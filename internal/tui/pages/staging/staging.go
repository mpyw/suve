// Package staging implements the TUI's staging review page: per-service sections
// listing staged entries (as Remote-vs-Staged diffs or raw staged values) and
// independent staged tag changes, with unstage (`u`, the single removal
// affordance for both entries and tag changes) / edit-staged row actions and the
// apply and reset flows. It completes the TUI's stage → review →
// apply loop. Only the services the launched scope offers get a section; every
// staged read is a tea.Cmd guarded by a monotonic sequence (the browser's
// loadSeq pattern), and a staging read failure degrades to a per-section error
// line rather than blocking the rest of the app.
package staging

import (
	"context"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/hit"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/styles"
)

// serviceSecret is the secret service key (the sections' masking axis).
const serviceSecret = "secret"

// Clickable region IDs for the staging hit map. The fixed-header affordances have
// stable IDs; a section's apply/reset buttons and a selectable row carry their
// index as a suffix (secApplyID / secResetID / rowID) so a click resolves to the
// exact section or row it lands on.
const (
	regionNotice     = "notice"
	regionViewToggle = "view-toggle"
	regionApplyAll   = "apply-all"
	regionResetAll   = "reset-all"
	regionRefresh    = "refresh"
	prefixSecApply   = "sec-apply-"
	prefixSecReset   = "sec-reset-"
	prefixRow        = "row-"
)

// Page-local key bindings not present in the global map.
//
//nolint:gochecknoglobals // immutable page-local bindings
var (
	viewKey     = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view"))
	revealKey   = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "reveal"))
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
	// reveal unmasks the SELECTED row's secret staged value (never page-global);
	// reset on a selection move, a view toggle, and reload (#694).
	reveal bool
	// noticeDismissed hides the auto-unstaged notice until the next load.
	noticeDismissed bool
	// status is a transient one-line message shown in the footer for an invalid
	// action on the selected row (e.g. tagging a delete-staged entry, #684). It is
	// cleared on the next interaction (key or mouse) and on every reload, so it can
	// never outlive the row it referred to.
	status string

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

	// hits is the last-rendered hit map: one compositor region per clickable area
	// (the header toggle/apply-all/reset-all/refresh, the dismissible notice, each
	// section's apply/reset buttons, and each selectable row), rebuilt every View
	// so a mouse coordinate is hit-tested against the layers rather than a
	// hand-maintained geometry.
	hits *hit.Map
}

// secApplyID / secResetID / rowID build the suffixed region IDs for a section's
// apply/reset buttons and a selectable row.
func secApplyID(section int) string { return prefixSecApply + strconv.Itoa(section) }
func secResetID(section int) string { return prefixSecReset + strconv.Itoa(section) }
func rowID(row int) string          { return prefixRow + strconv.Itoa(row) }

// idIndex parses the integer suffix an ID carries after prefix (e.g. "row-3"→3),
// reporting ok=false when id does not carry that prefix + integer.
func idIndex(id, prefix string) (int, bool) {
	rest, found := strings.CutPrefix(id, prefix)
	if !found {
		return 0, false
	}

	n, err := strconv.Atoi(rest)

	return n, err == nil
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
