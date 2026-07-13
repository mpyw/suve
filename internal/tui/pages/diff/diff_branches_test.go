//nolint:testpackage // white-box: drives NewStatic and the unexported handleKey/error-render paths
package diff

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/styles"
)

// TestNewStatic_Golden pins the static-content constructor behind the staging
// page's remote-vs-staged detail: NewStatic renders an already-known two-sided
// diff (no fetch) whose title is the new label and whose body is the colorized
// unified diff.
func TestNewStatic_Golden(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := NewStatic(data.DiffContent{
		OldLabel: "remote",
		NewLabel: "staged",
		OldValue: "host: a.internal\nport: 1\n",
		NewValue: "host: b.internal\nport: 2\n",
	}, styles.New(), keys.Default())

	assert.Nil(t, m.Init(), "a static page has no fetch to dispatch")
	assert.True(t, m.loaded, "a static page opens already loaded")

	golden.RequireEqual(t, []byte(m.View(80, 24)))
}

// TestDiff_BackKeyPops pins the diff page's Back key: it emits nav.PopPage so the
// shell pops the pushed diff and returns to the base tab.
func TestDiff_BackKeyPops(t *testing.T) {
	t.Parallel()

	m := newDiff(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd, "Back dispatches")
	_, ok := cmd().(nav.PopPage)
	assert.True(t, ok, "Back emits nav.PopPage")
}

// TestDiff_ScrollKeyForwardedToViewport pins that an unclaimed movement key falls
// through handleKey to the viewport (page scrolling), rather than being dropped.
func TestDiff_ScrollKeyForwardedToViewport(t *testing.T) {
	t.Parallel()

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "a#1", NewLabel: "a#2",
		OldValue: distinctLines("old"), NewValue: distinctLines("new"),
	}})
	require.True(t, m.vp.AtTop(), "the diff opens at the top")

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.False(t, m.vp.AtTop(), "a movement key scrolls the viewport")
}

// TestDiff_MouseWheelScrolls pins the diff page's wheel handling: a wheel event is
// forwarded into the viewport so the diff body scrolls.
func TestDiff_MouseWheelScrolls(t *testing.T) {
	t.Parallel()

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "a#1", NewLabel: "a#2",
		OldValue: distinctLines("old"), NewValue: distinctLines("new"),
	}})
	require.True(t, m.vp.AtTop())

	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.False(t, m.vp.AtTop(), "a wheel-down scrolls the diff body")
}

// TestDiff_ErrorTruncatedAtNarrowWidth pins the load-error render path: a failed
// load records the error, and View clamps a long error line to the pane's inner
// width (vpWidth) via truncateLine so it never overflows the box.
func TestDiff_ErrorTruncatedAtNarrowWidth(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	const longErr = "AccessDeniedException: the caller is not authorized to read this parameter version in this account and region"

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{err: assertError(longErr)})
	require.Equal(t, longErr, m.err, "the load error is recorded")

	out := m.View(40, 20)
	require.NotEmpty(t, out)

	// The pane's inner width is far below the error length, so the rendered error
	// line is clamped rather than spilling past the box edge.
	for line := range strings.SplitSeq(out, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 40, "no rendered line exceeds the terminal width")
	}

	assert.Positive(t, m.vpWidth(), "vpWidth reports the pane inner width")
}

// TestSideBySideTruncatesLongLines pins the side-by-side cell truncation
// (sideCell's over-width branch): at a narrow-but-splittable width, a line longer
// than the per-column budget is clamped so the gutter and pane border stay
// aligned, while the layout stays side-by-side.
func TestSideBySideTruncatesLongLines(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	long := strings.Repeat("x", 120)

	m := newDiff(t)
	m, _ = m.Update(loadedMsg{content: data.DiffContent{
		OldLabel: "cfg#1", NewLabel: "cfg#2",
		OldValue: "prefix-" + long + "-old",
		NewValue: "prefix-" + long + "-new",
	}})
	m, _ = m.Update(keyPress('s'))
	require.True(t, m.sideBySide, "s enables side-by-side")

	// A narrow but still-splittable width forces each column below the line length,
	// exercising sideCell's truncation branch.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 70, Height: 40})
	colW, ok := m.splitColumnWidth()
	require.True(t, ok, "the width is still splittable")
	require.Less(t, colW, len(long), "the column is narrower than the long line")

	out := m.vp.View()
	require.NotEmpty(t, out)

	for line := range strings.SplitSeq(out, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 70, "each side-by-side row is clamped to the terminal width")
	}
}

// assertError is a tiny error wrapper so the load-error test can inject a fixed
// message without importing errors for a one-liner.
type assertError string

func (e assertError) Error() string { return string(e) }

// distinctLines builds 200 distinct "prefix-NNN" lines so a diff of two such
// blocks yields a body long enough to overflow the viewport and scroll.
func distinctLines(prefix string) string {
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("%s-%03d", prefix, i)
	}

	return strings.Join(lines, "\n") + "\n"
}
