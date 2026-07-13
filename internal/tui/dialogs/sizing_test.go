//nolint:testpackage // white-box: drives the dialogs' Update/View and inspects their size-aware layout
package dialogs

import (
	"context"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/tui/styles"
)

// The minimum supported terminal size (#686): every dialog must keep its
// controls and close hint reachable, and wrap long content, at this size.
const (
	minWidth  = 60
	minHeight = 16
)

// ansiSGR matches the SGR color escapes lipgloss emits, so a test can compare
// the plain text a user reads.
var ansiSGR = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiSGR.ReplaceAllString(s, "") }

// maxLineWidth is the widest rendered line in display cells (ANSI ignored).
func maxLineWidth(view string) int {
	w := 0
	for line := range strings.SplitSeq(view, "\n") {
		w = max(w, lipgloss.Width(line))
	}

	return w
}

// flatten strips ANSI and per-line trailing padding, then joins the lines with
// no separator, so a value wrapped across lines can be matched as one contiguous
// string (proving it was wrapped, not truncated).
func flatten(view string) string {
	var b strings.Builder

	for line := range strings.SplitSeq(stripANSI(view), "\n") {
		b.WriteString(strings.TrimRight(line, " "))
	}

	return b.String()
}

// TestEntryForm_TallFormFitsMinSize pins the #686 fix: at the minimum supported
// 60×16 terminal the create form — the tallest dialog — no longer clips its
// controls off the bottom. huh caps its body into a scrollable region so the
// whole box (including the submit/cancel hint) fits within the terminal height,
// and no line overflows the dialog width.
func TestEntryForm_TallFormFitsMinSize(t *testing.T) {
	t.Parallel()

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(),
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	view := m.View()

	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"the dialog body fits inside the frame, so the shell never clips it")
	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"no line overflows the dialog width at the minimum size")
	// The hint may fold across two lines at the minimum size, so match wrap-safe
	// affordance tokens rather than a phrase that could straddle the break.
	assert.Contains(t, flatten(view), "fields", "the field-nav hint stays on-screen")
	assert.Contains(t, flatten(view), "cancel", "the cancel hint stays on-screen")
}

// TestEntryForm_CompressesWhenTallerThanScreen pins that the min-size form is
// actually compressed (its body scrolled), not merely short: the same form at a
// tall terminal renders more rows and shows the full form down to the Description
// field, part of which scrolls out of view at 60×16.
func TestEntryForm_CompressesWhenTallerThanScreen(t *testing.T) {
	t.Parallel()

	build := func() Model {
		// The AWS param form is the tallest (Name + Type select with three options +
		// Value textarea + Description), so its body must scroll at the minimum size.
		m, _ := NewEntryForm(EntryFormInput{
			Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsParamCap()},
			Service: "param", Styles: styles.New(),
		})

		return m
	}

	small := build()
	small, _ = small.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	tall := build()
	tall, _ = tall.Update(tea.WindowSizeMsg{Width: minWidth, Height: 40})

	assert.Less(t, lipgloss.Height(small.View()), lipgloss.Height(tall.View()),
		"the min-size form is compressed relative to a tall terminal")
	assert.Contains(t, stripANSI(tall.View()), "Description",
		"a tall terminal shows the full form, down to the Description field")
}

// TestDeleteConfirm_LongNameWrapsMinSize pins the safety fix: a long delete
// target name wraps within the dialog width instead of clipping at the screen
// edge (which could hide the suffix that distinguishes sibling paths), while the
// action buttons and the close hint stay on-screen at the minimum size.
func TestDeleteConfirm_LongNameWrapsMinSize(t *testing.T) {
	t.Parallel()

	const name = "/prod/service/database/primary/credentials/password-rotation-key"

	m := NewDeleteConfirm(DeleteInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(), Name: name,
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	view := m.View()

	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"the wrapped name (and hint) never overflow the dialog width")
	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"the whole dialog fits within the terminal height")
	assert.Contains(t, flatten(view), name,
		"the full name is present, wrapped across lines rather than truncated")
	assert.Contains(t, stripANSI(view), "Cancel", "the Cancel button stays on-screen")
	assert.Contains(t, stripANSI(view), "esc: cancel", "the close hint stays on-screen")

	// Before a size is seeded the name is not wrapped (renders as one line): the
	// wrap is driven by the terminal size the shell fans in.
	unsized := NewDeleteConfirm(DeleteInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(), Name: name,
	})
	assert.Contains(t, unsized.View(), name, "unsized: the name renders as a single line")
}

// TestDeleteConfirm_LongErrorStaysBounded pins that a long provider error after
// a failed delete is wrapped and capped so the delete confirm still fits the
// minimum size: the Cancel button and the close hint stay on-screen instead of
// being pushed off the bottom by a tall error (the delete confirm cannot scroll
// because its controls need focus).
func TestDeleteConfirm_LongErrorStaysBounded(t *testing.T) {
	t.Parallel()

	m := NewDeleteConfirm(DeleteInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(),
		Name: "/prod/service/database/primary/credentials/password-rotation-key",
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	d, ok := m.(*deleteConfirm)
	require.True(t, ok)

	d.err = "AccessDeniedException: the caller is not authorized to perform " +
		"secretsmanager:DeleteSecret on this resource because no identity-based policy " +
		"grants the action; contact your administrator to grant the permission or assume " +
		"a role that has it before retrying the delete operation."

	view := m.View()
	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"a long error keeps the whole dialog within the terminal height")
	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"the wrapped error never overflows the dialog width")
	assert.Contains(t, stripANSI(view), "Cancel", "the Cancel button stays on-screen under a long error")
	assert.Contains(t, stripANSI(view), "esc: cancel", "the close hint stays on-screen under a long error")
}

// TestEntryForm_LongErrorStaysBounded pins that a long provider error after a
// failed create/edit keeps the huh-form dialog within the minimum size: the
// wrapped error is capped so the form body keeps at least minFormBody rows (huh
// scrolls it) and the submit/cancel hint stays on-screen instead of being pushed
// off the bottom. Driven through the real result path (onResult → rebuildForm →
// syncFormSize), which is how the error actually arrives.
func TestEntryForm_LongErrorStaysBounded(t *testing.T) {
	t.Parallel()

	m, _ := NewEntryForm(EntryFormInput{
		Ctx: context.Background(), Mutator: &fakeMutator{svcCap: awsSecretCap()},
		Service: "secret", Styles: styles.New(),
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	longErr := stringError(strings.Repeat("the create request was rejected by the provider and cannot be retried. ", 10))
	m, _ = m.Update(mutationResultMsg{err: longErr})

	view := m.View()
	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"a long error keeps the whole form within the terminal height")
	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"the wrapped error never overflows the dialog width")
	assert.Contains(t, flatten(view), "cancel", "the submit/cancel hint stays on-screen under a long error")
}

// TestErrorDialog_LongMessageWrapsAndScrollsMinSize pins that a long
// provider/key-loss error wraps to the dialog width and scrolls inside a
// viewport, so neither the message nor the close hint clips off-screen at the
// minimum size; a short message neither scrolls nor advertises scroll keys.
func TestErrorDialog_LongMessageWrapsAndScrollsMinSize(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("the staging data key could not be recovered from the keychain. ", 10)

	m := NewError(styles.New(), "Staging key lost", long)
	m, _ = m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	d, ok := m.(*errorDialog)
	require.True(t, ok)
	assert.True(t, d.scrollable, "a message taller than the box scrolls")

	view := m.View()
	assert.LessOrEqual(t, maxLineWidth(view), minWidth-dialogChrome,
		"the wrapped message never overflows the dialog width")
	assert.LessOrEqual(t, lipgloss.Height(view), minHeight-dialogFrameHeight,
		"the whole dialog fits within the terminal height")
	assert.Contains(t, stripANSI(view), "scroll", "the hint advertises scroll when the body overflows")
	assert.Contains(t, stripANSI(view), "enter/esc: close", "the close hint stays pinned")

	short := NewError(styles.New(), "Blocked", "Pick one namespace first.")
	short, _ = short.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})
	sd, ok := short.(*errorDialog)
	require.True(t, ok)
	assert.False(t, sd.scrollable, "a short message does not scroll")
	assert.NotContains(t, stripANSI(short.View()), "scroll", "no scroll keys advertised when it fits")
}
