// Package diff renders the TUI's diff page: two versions of one entry compared
// with github.com/aymanbagabas/go-udiff (the same engine the CLI uses), colorized
// per line in a scrollable viewport. The default layout is a unified diff; `s`
// toggles a SIDE-BY-SIDE (two-column, old | new) layout that degrades back to
// unified on a terminal too narrow to split (#674). Both layouts share the same
// udiff engine — side-by-side is a rendering option, not a second diff. A value
// that parses as JSON is ALWAYS pretty-printed before diffing (parity with the
// GUI, which formats every JSON value; #732) — no manual toggle — in both
// columns. When pretty-printing collapses the two sides to identical text even
// though the raw values differ (a whitespace-only difference), the page says so
// rather than showing a bare "(no differences)".
package diff

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	udiff "github.com/aymanbagabas/go-udiff"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/styles"
	"github.com/mpyw/suve/internal/tui/termquirk"
)

// scrollKey is a help-only binding advertising viewport scrolling (the diff
// page forwards unclaimed movement keys straight to the viewport).
//
//nolint:gochecknoglobals // immutable page-local binding
var scrollKey = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll"))

// maskKey toggles masking of a secret diff. The Compare/diff view is a surface
// the user explicitly opened to inspect the change, so its values are REVEALED
// by default (#702/#735); `x` hides them again. It is a no-op on a non-secret
// diff (whose values are never masked).
//
//nolint:gochecknoglobals // immutable page-local binding
var maskKey = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "hide/show"))

// layoutKey toggles between the unified and side-by-side diff layouts (#674).
// The help text is rebuilt per frame (see layoutBinding) so it names the layout
// the press switches TO, not a fixed label.
//
//nolint:gochecknoglobals // immutable page-local binding
var layoutKey = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "side-by-side"))

// Side-by-side layout geometry. sideGutter separates the old (left) and new
// (right) columns; minSideColumn is the narrowest a single column may be before
// the page degrades to unified rather than split the width too far (#674).
const (
	sideGutter    = " │ "
	minSideColumn = 24
	sideColumns   = 2
)

// loadedMsg carries the fetched two-version contents back to the model.
type loadedMsg struct {
	content data.DiffContent
	err     error
}

// Model is the diff page: it fetches two versions' values once, then renders (and
// re-renders, for parse-json) their unified diff.
type Model struct {
	// ctx is the Run context threaded through the fetch command.
	ctx        context.Context //nolint:containedctx // the fetch command needs the Run context; mirrors the GUI
	source     data.Source
	name       string
	namespace  string
	oldVersion string
	newVersion string

	styles styles.Styles
	keys   keys.Map
	vp     viewport.Model

	width  int
	height int

	content data.DiffContent
	loaded  bool
	// masked hides a secret diff's values. It defaults to false: an explicitly
	// opened Compare/diff view reveals values so the diff is meaningful
	// (#702/#735); `x` toggles it to mask both sides.
	masked bool
	// sideBySide selects the two-column (old | new) layout. It defaults to false
	// (unified); `s` toggles it (#674). When the terminal is too narrow to split
	// (see splitColumnWidth) the render falls back to unified regardless.
	sideBySide bool
	err        string
}

// New builds a diff page from a navigation request. ctx is the Run context
// threaded through the fetch. It does not fetch yet; Init dispatches the load.
func New(ctx context.Context, req nav.OpenDiff, st styles.Styles, km keys.Map) *Model {
	return &Model{
		ctx:        ctx,
		source:     req.Source,
		name:       req.Name,
		namespace:  req.Namespace,
		oldVersion: req.OldVersion,
		newVersion: req.NewVersion,
		styles:     st,
		keys:       km,
		vp:         viewport.New(),
	}
}

// NewStatic builds a diff page over already-known content (no fetch). It backs
// the staging page's remote-vs-staged detail, where the two sides are the staged
// review's values rather than two provider versions. Secret masking is honored
// exactly as in the fetched path.
func NewStatic(content data.DiffContent, st styles.Styles, km keys.Map) *Model {
	return &Model{
		name:    content.NewLabel,
		content: content,
		loaded:  true,
		styles:  st,
		keys:    km,
		vp:      viewport.New(),
	}
}

// Init dispatches the one-shot version-contents fetch, or nothing for a static
// page whose content is already known.
func (m *Model) Init() tea.Cmd {
	if m.source == nil {
		return nil
	}

	return m.loadCmd()
}

// loadCmd fetches both versions' raw values off the update loop.
func (m *Model) loadCmd() tea.Cmd {
	ctx := m.ctx
	source := m.source
	name, ns, oldV, newV := m.name, m.namespace, m.oldVersion, m.newVersion

	return func() tea.Msg {
		content, err := source.VersionContents(ctx, name, oldV, newV, ns)

		return loadedMsg{content: content, err: err}
	}
}

// Update handles fetch results, keys, and mouse wheel scrolling. It returns
// itself as a page (the app stores it back).
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()

		return m, nil
	case loadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()

			return m, nil
		}

		m.content = msg.content
		m.loaded = true
		m.err = ""
		m.render()

		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		return m, m.scrollViewport(msg)
	default:
		return m, nil
	}
}

// scrollViewport forwards msg to the diff viewport and, when the viewport's
// scroll offset actually changes on a terminal that mishandles Bubble Tea's
// scroll-region optimization (CloudShell), forces a full repaint so the scroll
// renders cleanly (see internal/tui/termquirk).
func (m *Model) scrollViewport(msg tea.Msg) tea.Cmd {
	before := m.vp.YOffset()

	var cmd tea.Cmd

	m.vp, cmd = m.vp.Update(msg)

	return termquirk.RepaintOnScroll(m.vp.YOffset() != before, cmd)
}

// handleKey handles the page-local keys (mask toggle, back) and forwards the
// rest to the viewport for scrolling.
func (m *Model) handleKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		return m, func() tea.Msg { return nav.PopPage{} }
	case key.Matches(msg, maskKey):
		m.masked = !m.masked
		m.render()

		return m, nil
	case key.Matches(msg, layoutKey):
		m.sideBySide = !m.sideBySide
		m.render()

		return m, nil
	}

	return m, m.scrollViewport(msg)
}

// HelpKeyMap reports the diff page's context-aware bindings for the help bar:
// scroll always, the layout toggle (`s`, side-by-side ⇄ unified) on a loaded
// diff, the mask toggle only on a loaded secret diff (`x` is a no-op on a
// non-secret diff, so it is hidden there), then back. The help bar makes
// `s`/`x`/esc discoverable — previously undocumented (#681, #674).
func (m *Model) HelpKeyMap() help.KeyMap {
	bindings := []key.Binding{scrollKey}

	if m.loaded {
		bindings = append(bindings, m.layoutBinding())
	}

	if m.loaded && m.content.Secret {
		bindings = append(bindings, maskKey)
	}

	bindings = append(bindings, m.keys.Back)

	return keys.Bindings{Short: bindings, Full: [][]key.Binding{bindings}}
}

// layoutBinding names the layout the `s` press switches TO, so the help bar reads
// "s side-by-side" while unified and "s unified" while side-by-side.
func (m *Model) layoutBinding() key.Binding {
	next := "side-by-side"
	if m.sideBySide {
		next = "unified"
	}

	return key.NewBinding(key.WithKeys("s"), key.WithHelp("s", next))
}

// resizeViewport sizes the inner viewport to the page area minus the pane border
// and title, then re-renders.
func (m *Model) resizeViewport() {
	innerW, innerH := components.PaneInner(m.width, m.height)
	m.vp.SetWidth(innerW)
	m.vp.SetHeight(innerH)
	m.render()
}

// render recomputes the colorized diff into the viewport. A value that parses as
// JSON is always pretty-printed before diffing (GUI parity, #732).
func (m *Model) render() {
	if !m.loaded {
		return
	}

	oldVal, newVal := m.content.OldValue, m.content.NewValue

	// A Compare/diff view is a surface the user explicitly opened to inspect the
	// change, so a secret's values are shown by default (#702/#735). Pressing `x`
	// masks BOTH sides before diffing (per-line, length-capped bullets), so a
	// change still shows as differing runs without disclosing content — and while
	// masked, formatting is skipped: the bullets are not JSON, and normalizing the
	// raw secret first could leak its structure.
	if m.content.Secret && m.masked {
		oldVal = components.MaskValue(oldVal)
		newVal = components.MaskValue(newVal)
	} else {
		if f, ok := jsonutil.TryFormat(oldVal); ok {
			oldVal = f
		}

		if f, ok := jsonutil.TryFormat(newVal); ok {
			newVal = f
		}
	}

	raw := output.DiffRaw(m.content.OldLabel, m.content.NewLabel, oldVal, newVal)

	// A whitespace-only difference collapses under pretty-printing: the formatted
	// sides are identical even though the raw values differ. Say so, rather than a
	// bare "(no differences)", so a real (formatting-hidden) change is not read as
	// no change at all — mirroring the GUI's always-formatted comparison (#732).
	if raw == "" && (!m.content.Secret || !m.masked) && m.content.OldValue != m.content.NewValue {
		m.vp.SetContent(m.styles.PageHint.Render(
			"(no differences after JSON formatting — the values differ only in whitespace/formatting)"))

		return
	}

	// Side-by-side layout, when requested AND the terminal is wide enough to split
	// two readable columns; otherwise fall through to unified (#674). raw != ""
	// means there is a real change to lay out; an empty diff keeps the unified
	// "(no differences)" path below.
	if raw != "" && m.sideBySide {
		if colW, ok := m.splitColumnWidth(); ok {
			m.vp.SetContent(m.renderSideBySide(oldVal, newVal, colW))

			return
		}
	}

	m.vp.SetContent(m.colorize(raw))
}

// splitColumnWidth reports the per-column width for the side-by-side layout and
// whether the terminal is wide enough to split. Below minSideColumn per column
// the page stays unified rather than crush both sides (#674).
func (m *Model) splitColumnWidth() (int, bool) {
	innerW, _ := components.PaneInner(m.width, m.height)
	colW := (innerW - lipgloss.Width(sideGutter)) / sideColumns

	return colW, colW >= minSideColumn
}

// renderSideBySide lays the same udiff hunks out as two aligned columns — old on
// the left, new on the right — reusing the already-masked-or-formatted values so
// the reveal/hide and always-format-JSON policies hold identically to the unified
// path (#674, #702/#732/#735). Removed/added lines carry a -/+ marker so the
// change is legible even under NO_COLOR (where the diff styles are bare); each
// cell is truncated and padded to colW so the gutter and pane border stay aligned.
func (m *Model) renderSideBySide(oldVal, newVal string, colW int) string {
	edits := udiff.Strings(oldVal, newVal)

	u, err := udiff.ToUnifiedDiff(m.content.OldLabel, m.content.NewLabel, oldVal, edits, udiff.DefaultContextLines)
	if err != nil {
		// A malformed diff should never happen for consistent edits; fall back to
		// the unified colorized form rather than render a broken grid.
		return m.colorize(output.DiffRaw(m.content.OldLabel, m.content.NewLabel, oldVal, newVal))
	}

	rows := []string{m.sideRow(
		m.styles.DiffHeader.Render(sideCell(m.content.OldLabel, colW)),
		m.styles.DiffHeader.Render(sideCell(m.content.NewLabel, colW)),
	)}

	for hi, h := range u.Hunks {
		if hi > 0 {
			// A gap between hunks: a dim rule on both sides so non-contiguous regions
			// are not read as adjacent.
			rule := m.styles.DiffHunk.Render(sideCell(strings.Repeat("┈", colW), colW))
			rows = append(rows, m.sideRow(rule, rule))
		}

		rows = append(rows, m.hunkRows(h, colW)...)
	}

	return strings.Join(rows, "\n")
}

// hunkRows aligns one hunk's lines into side-by-side rows. Within a hunk, udiff
// emits a change as a run of deletions followed by a run of insertions; those
// runs are zipped row-by-row (old | new), padding the shorter run with blanks so
// a pure add/remove leaves the opposite column empty. Equal (context) lines
// appear unchanged in both columns.
func (m *Model) hunkRows(h *udiff.Hunk, colW int) []string {
	var rows []string

	lines := h.Lines
	for i := 0; i < len(lines); {
		if lines[i].Kind == udiff.Equal {
			text := "  " + lineContent(lines[i].Content)
			cell := sideCell(text, colW)
			rows = append(rows, m.sideRow(cell, cell))
			i++

			continue
		}

		var dels, inss []string

		for i < len(lines) && lines[i].Kind == udiff.Delete {
			dels = append(dels, lineContent(lines[i].Content))
			i++
		}

		for i < len(lines) && lines[i].Kind == udiff.Insert {
			inss = append(inss, lineContent(lines[i].Content))
			i++
		}

		rows = append(rows, m.changeRows(dels, inss, colW)...)
	}

	return rows
}

// changeRows zips a run of deletions and a run of insertions into aligned rows.
func (m *Model) changeRows(dels, inss []string, colW int) []string {
	rows := make([]string, 0, max(len(dels), len(inss)))

	for j := 0; j < len(dels) || j < len(inss); j++ {
		left := sideCell("", colW)
		if j < len(dels) {
			left = m.styles.DiffRemoved.Render(sideCell("- "+dels[j], colW))
		}

		right := sideCell("", colW)
		if j < len(inss) {
			right = m.styles.DiffAdded.Render(sideCell("+ "+inss[j], colW))
		}

		rows = append(rows, m.sideRow(left, right))
	}

	return rows
}

// sideRow joins a left and right column cell with the gutter.
func (m *Model) sideRow(left, right string) string {
	return left + sideGutter + right
}

// lineContent strips a udiff line's trailing newline for cell rendering.
func lineContent(s string) string { return strings.TrimSuffix(s, "\n") }

// sideCell truncates s to width columns, then space-pads it to exactly width so
// the columns stay aligned. It operates on unstyled text; callers apply a diff
// style to the returned, already-sized cell.
func sideCell(s string, width int) string {
	w := lipgloss.Width(s)
	switch {
	case w > width:
		return lipgloss.NewStyle().MaxWidth(width).Render(s)
	case w < width:
		return s + strings.Repeat(" ", width-w)
	default:
		return s
	}
}

// colorize styles each diff line by class (header / hunk / added / removed),
// mirroring the CLI's colorDiff so a `+`/`-` inside a hunk is not misread as a
// file header.
func (m *Model) colorize(diff string) string {
	if diff == "" {
		return m.styles.PageHint.Render("(no differences)")
	}

	lines := strings.Split(diff, "\n")
	inHunk := false

	for i, line := range lines {
		switch {
		case !inHunk && (strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++")):
			lines[i] = m.styles.DiffHeader.Render(line)
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			lines[i] = m.styles.DiffHunk.Render(line)
		case strings.HasPrefix(line, "-"):
			lines[i] = m.styles.DiffRemoved.Render(line)
		case strings.HasPrefix(line, "+"):
			lines[i] = m.styles.DiffAdded.Render(line)
		}
	}

	return strings.Join(lines, "\n")
}

// View renders the diff page into the given content area.
func (m *Model) View(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	if m.width != width || m.height != height {
		m.width, m.height = width, height
		m.resizeViewport()
	}

	title := "diff: " + m.name
	if m.loaded {
		title = "diff: " + m.content.OldLabel + " → " + m.content.NewLabel
	}

	// Surface the layout toggle so switching to/from side-by-side is discoverable.
	if m.loaded {
		if m.sideBySide {
			title += "  ·  s: unified"
		} else {
			title += "  ·  s: side-by-side"
		}
	}

	// Surface the mask toggle so hiding a revealed secret diff is discoverable.
	if m.loaded && m.content.Secret {
		if m.masked {
			title += "  ·  x: show"
		} else {
			title += "  ·  x: hide"
		}
	}

	body := m.vp.View()
	if m.err != "" {
		body = m.styles.ErrorText.Render(truncateLine(m.err, m.vpWidth()))
	} else if !m.loaded {
		body = m.styles.PageHint.Render("loading…")
	}

	return components.Pane(m.styles, title, body, width, height)
}

// vpWidth is the inner content width for error rendering.
func (m *Model) vpWidth() int {
	w, _ := components.PaneInner(m.width, m.height)

	return w
}

// truncateLine clamps s to width columns.
func truncateLine(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
