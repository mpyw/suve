// Package hit provides compositor-based mouse hit-testing for the TUI's pages
// and dialogs. Each clickable region is a lipgloss v2 Layer with an ID placed at
// its drawn location; lipgloss.Compositor.Hit resolves a point to the top-most
// region and its bounds, from which a handler derives the in-region offset. This
// replaces the hand-rolled point-in-rectangle geometry the pages used to keep in
// sync with the renderer, so a mouse coordinate is hit-tested against the layers
// once rather than double-managed.
package hit

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Map wraps a lipgloss.Compositor of ID'd region layers. A nil Map (or one built
// with no regions) simply never hits, so callers need not guard for a page that
// has not rendered yet.
type Map struct {
	comp *lipgloss.Compositor
}

// New builds a Map from region layers (typically produced by Region). Regions
// may overlap: a higher Z region wins the hit, so a sub-region (e.g. a history
// band inside a detail pane) is placed above the pane it sits in.
func New(regions ...*lipgloss.Layer) *Map {
	return &Map{comp: lipgloss.NewCompositor(regions...)}
}

// Region builds an ID'd layer sized w×h at (x, y). The content is a blank block
// of exactly that size; only its bounds matter to Compositor.Hit. Width/height
// below one are clamped to one so a degenerate region still has a hit cell.
func Region(id string, x, y, w, h int) *lipgloss.Layer {
	return lipgloss.NewLayer(block(w, h)).X(x).Y(y).ID(id)
}

// At hit-tests (x, y) against the map. It returns the ID of the top-most region
// containing the point and the point's offset within that region (dx, dy from the
// region's top-left), or ok=false when no region is hit.
func (m *Map) At(x, y int) (id string, dx, dy int, ok bool) {
	if m == nil || m.comp == nil {
		return "", 0, 0, false
	}

	h := m.comp.Hit(x, y)
	if h.Empty() {
		return "", 0, 0, false
	}

	origin := h.Bounds().Min

	return h.ID(), x - origin.X, y - origin.Y, true
}

// Origin returns the top-left (x, y) of the region with the given ID, or
// ok=false when there is no such region. It lets a test derive a click
// coordinate from the drawn layout instead of hard-coding one.
func (m *Map) Origin(id string) (x, y int, ok bool) {
	if m == nil || m.comp == nil {
		return 0, 0, false
	}

	l := m.comp.GetLayer(id)
	if l == nil {
		return 0, 0, false
	}

	// Every region layer is a direct child of the compositor's root at (0,0), so
	// its own X/Y are already absolute.
	return l.GetX(), l.GetY(), true
}

// block builds an h-row, w-column blank string whose lipgloss width/height are
// exactly w×h, so a layer built from it has those bounds.
func block(w, h int) string {
	if w < 1 {
		w = 1
	}

	if h < 1 {
		h = 1
	}

	row := strings.Repeat(" ", w)
	if h == 1 {
		return row
	}

	rows := make([]string, h)
	for i := range rows {
		rows[i] = row
	}

	return strings.Join(rows, "\n")
}
