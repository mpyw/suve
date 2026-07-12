// Package diff renders the TUI's unified-diff page: two versions of one entry
// compared with github.com/aymanbagabas/go-udiff (the same engine the CLI uses),
// colorized per line in a scrollable viewport. `J` re-diffs with parse-json
// normalization (parity with the CLI's --parse-json), recomputing the diff from
// the already-fetched values without another round trip.
package diff

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mpyw/suve/internal/cli/output"
	"github.com/mpyw/suve/internal/jsonutil"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/styles"
)

// parseJSONKey toggles JSON normalization of both values before diffing.
//
//nolint:gochecknoglobals // immutable page-local binding
var parseJSONKey = key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "parse-json"))

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

	content   data.DiffContent
	loaded    bool
	parseJSON bool
	err       string
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
		var cmd tea.Cmd

		m.vp, cmd = m.vp.Update(msg)

		return m, cmd
	default:
		return m, nil
	}
}

// handleKey handles the page-local keys (parse-json toggle, back) and forwards
// the rest to the viewport for scrolling.
func (m *Model) handleKey(msg tea.KeyPressMsg) (*Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		return m, func() tea.Msg { return nav.PopPage{} }
	case key.Matches(msg, parseJSONKey):
		m.parseJSON = !m.parseJSON
		m.render()

		return m, nil
	}

	var cmd tea.Cmd

	m.vp, cmd = m.vp.Update(msg)

	return m, cmd
}

// resizeViewport sizes the inner viewport to the page area minus the pane border
// and title, then re-renders.
func (m *Model) resizeViewport() {
	innerW, innerH := components.PaneInner(m.width, m.height)
	m.vp.SetWidth(innerW)
	m.vp.SetHeight(innerH)
	m.render()
}

// render recomputes the colorized diff into the viewport for the current
// parse-json state.
func (m *Model) render() {
	if !m.loaded {
		return
	}

	oldVal, newVal := m.content.OldValue, m.content.NewValue

	// A secret diff is masked on BOTH sides before diffing, so a revealed value
	// never reaches the viewport (or a golden). Masking is per-line and length-
	// capped, so a change still shows as differing bullet runs without disclosing
	// content. Parse-json is skipped for a secret: the masked bullets are not JSON,
	// and normalizing the raw secret first could leak its structure.
	switch {
	case m.content.Secret:
		oldVal = components.MaskValue(oldVal)
		newVal = components.MaskValue(newVal)
	case m.parseJSON:
		if f, ok := jsonutil.TryFormat(oldVal); ok {
			oldVal = f
		}

		if f, ok := jsonutil.TryFormat(newVal); ok {
			newVal = f
		}
	}

	raw := output.DiffRaw(m.content.OldLabel, m.content.NewLabel, oldVal, newVal)
	m.vp.SetContent(m.colorize(raw))
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
