//nolint:testpackage // white-box: exercises newApp/config, the dialog stack, and the setClipboard seam
package tui

import (
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/components"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/dialogs"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/styles"
)

// awsTargetFixture is the deterministic resolved target used in AWS goldens so
// the status bar renders without an async STS call.
func awsTargetFixture() *provider.Target {
	return new(provider.AWSTarget("dev", "123456789012", "ap-northeast-1"))
}

// keyPress builds a printable-character key press.
func keyPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// specialKey builds a special (non-text) key press such as Tab or Esc.
func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// fakeDialog is a test-only dialog that records the messages routed to it, so
// modality (input reaching the dialog, not the page) can be asserted. busyFlag
// drives the dismissal-suppression path.
type fakeDialog struct {
	got      []tea.Msg
	busyFlag bool
}

func (d *fakeDialog) Update(msg tea.Msg) (dialog, tea.Cmd) {
	d.got = append(d.got, msg)

	return d, nil
}

func (d *fakeDialog) View() string { return "fake dialog" }

// busy reports whether the dialog is mid-operation (drives dismissal suppression).
func (d *fakeDialog) busy() bool { return d.busyFlag }

// updateApp applies one message to the model and returns it as *App.
func updateApp(t *testing.T, m *App, msg tea.Msg) *App {
	t.Helper()

	next, _ := m.Update(msg)
	app, ok := next.(*App)
	require.True(t, ok, "Update must return *App")

	return app
}

// TestUpdate_TabSwitching covers keyboard tab navigation: tab/shift+tab cycle
// (wrapping) and 1/2/3 jump, on an AWS scope (Param, Secret, Staging).
func TestUpdate_TabSwitching(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	require.Len(t, m.tabs, 3, "AWS offers Param, Secret, Staging")
	assert.Equal(t, 0, m.activeTab)

	m = updateApp(t, m, m.keyForBinding(t, "tab"))
	assert.Equal(t, 1, m.activeTab, "tab advances")

	m = updateApp(t, m, keyPress('3'))
	assert.Equal(t, 2, m.activeTab, "3 jumps to the third tab")

	m = updateApp(t, m, m.keyForBinding(t, "tab"))
	assert.Equal(t, 0, m.activeTab, "tab wraps past the last tab")

	m = updateApp(t, m, m.keyForBinding(t, "shift+tab"))
	assert.Equal(t, 2, m.activeTab, "shift+tab wraps backwards")

	m = updateApp(t, m, keyPress('1'))
	assert.Equal(t, 0, m.activeTab, "1 jumps to the first tab")
}

// TestUpdate_JumpBeyondTabsIsNoop pins that a jump key past the last tab does
// nothing (rather than snapping to the last tab).
func TestUpdate_JumpBeyondTabsIsNoop(t *testing.T) {
	t.Parallel()

	// Google Cloud offers only Secret + Staging (2 tabs); "3" must be a no-op.
	m := newApp(config{scope: provider.GoogleCloudScope("proj")})
	require.Len(t, m.tabs, 2)

	m = updateApp(t, m, keyPress('3'))
	assert.Equal(t, 0, m.activeTab)
}

// TestUpdate_DialogModality pins that while a dialog is open it captures input
// (the page/tabs do not react) and Esc closes it.
func TestUpdate_DialogModality(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	fd := &fakeDialog{}
	m.dialogs = []dialog{fd}

	// A tab key must reach the dialog, not switch tabs.
	m = updateApp(t, m, m.keyForBinding(t, "tab"))
	assert.Equal(t, 0, m.activeTab, "tabs are inert while a dialog is modal")
	require.Len(t, fd.got, 1, "the dialog receives the key")

	// Esc closes the top dialog and does not reach it as a message.
	m = updateApp(t, m, specialKey(tea.KeyEscape))
	assert.Empty(t, m.dialogs, "esc closes the dialog")
	assert.Len(t, fd.got, 1, "esc is consumed by the close, not forwarded")
}

// pageOwnedMsg stands in for a page's own async result (list/detail/staged
// loads, review results) in the modal routing tests.
type pageOwnedMsg struct{}

// TestUpdate_DialogOpenForwardsNonInputToPage pins #987: while a dialog is open,
// a message the shell does not handle itself reaches both the top dialog and the
// active page (so the page's async results and spinner ticks still land), while
// user input such as a paste reaches the dialog only.
func TestUpdate_DialogOpenForwardsNonInputToPage(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	rp := &recordingPage{}
	m.pages = []page{rp}
	fd := &fakeDialog{}
	m.dialogs = []dialog{fd}

	m = updateApp(t, m, pageOwnedMsg{})
	m = updateApp(t, m, spinner.TickMsg{})

	require.Len(t, rp.got, 2, "the page beneath the dialog still receives non-input messages")
	assert.IsType(t, pageOwnedMsg{}, rp.got[0])
	assert.IsType(t, spinner.TickMsg{}, rp.got[1])
	require.Len(t, fd.got, 2, "the dialog receives them too")

	m = updateApp(t, m, tea.PasteMsg{Content: "typed"})
	updateApp(t, m, tea.KeyReleaseMsg{Code: 'x', Text: "x"})

	assert.Len(t, rp.got, 2, "a paste or key release never leaks to the page beneath the dialog")
	require.Len(t, fd.got, 4, "the paste and key release reach the dialog")
	assert.IsType(t, tea.PasteMsg{}, fd.got[2])
	assert.IsType(t, tea.KeyReleaseMsg{}, fd.got[3])
}

// TestUpdate_ListLoadedWhileDialogOpen replays #987 end to end: the browser's
// initial loads land while a dialog covers it, and after the dialog is canceled
// the list shows the loaded entries rather than an empty "entries (0)".
func TestUpdate_ListLoadedWhileDialogOpen(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		target:    awsTargetFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), staticProbe{keys: map[data.StagedKey]struct{}{}}),
	})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 120, Height: 34})

	// Hold back the initial loads and the spinner's first tick.
	var (
		pending []tea.Msg
		tick    tea.Msg
	)

	for _, msg := range drainBatch(m.initialPageCmd()) {
		if _, ok := msg.(spinner.TickMsg); ok {
			tick = msg

			continue
		}

		pending = append(pending, msg)
	}

	require.NotEmpty(t, pending, "the browser issues its initial loads")
	require.NotNil(t, tick, "the browser starts its spinner")

	m = updateApp(t, m, nav.OpenError{Title: "boom", Message: "a dialog opened before the list landed"})
	require.Len(t, m.dialogs, 1)

	// The spinner's tick chain survives the dialog: the page re-arms the next tick.
	next, cmd := m.Update(tick)
	m = next.(*App) //nolint:forcetypeassert // Update always returns *App
	assert.NotNil(t, cmd, "the browser re-arms its spinner tick while the dialog is open")

	for _, msg := range pending {
		m = updateApp(t, m, msg)
	}

	m = updateApp(t, m, dialogs.CanceledMsg{})
	require.Empty(t, m.dialogs)

	out := m.View().Content
	assert.Contains(t, out, "/app/", "the list loaded while the dialog was open is shown after it closes")
	assert.NotContains(t, out, "entries (0)")
}

// TestUpdate_DialogCapturesGlobalKeys pins the Step-2 carried-over fix: while a
// modal dialog is open, only ctrl+c stays global (force-quit); every other key —
// q, digits, letters — is forwarded into the dialog so a focused text field
// types normally, and does not quit or switch tabs.
func TestUpdate_DialogCapturesGlobalKeys(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	fd := &fakeDialog{}
	m.dialogs = []dialog{fd}

	// q and 1 reach the dialog; neither quits nor switches tabs.
	m = updateApp(t, m, keyPress('q'))
	m = updateApp(t, m, keyPress('1'))

	assert.Equal(t, 0, m.activeTab, "digits typed into a dialog do not switch tabs")
	require.Len(t, m.dialogs, 1, "q does not quit/close the dialog")
	assert.Len(t, fd.got, 2, "both keys were forwarded to the dialog")

	// ctrl+c still force-quits even while the dialog is modal.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd(), "ctrl+c force-quits through a modal dialog")
}

// TestUpdate_BusyDialogSuppressesDismiss pins GUI "Modal busy" parity: a busy
// dialog is not dismissed by Esc (the key is forwarded instead); an idle dialog
// is.
func TestUpdate_BusyDialogSuppressesDismiss(t *testing.T) {
	t.Parallel()

	busy := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	fdBusy := &fakeDialog{busyFlag: true}
	busy.dialogs = []dialog{fdBusy}

	busy = updateApp(t, busy, specialKey(tea.KeyEscape))
	require.Len(t, busy.dialogs, 1, "a busy dialog is not dismissed by esc")
	assert.Len(t, fdBusy.got, 1, "esc is forwarded to the busy dialog instead of closing it")

	idle := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	idle.dialogs = []dialog{&fakeDialog{}}
	idle = updateApp(t, idle, specialKey(tea.KeyEscape))
	assert.Empty(t, idle.dialogs, "an idle dialog is dismissed by esc")
}

// TestUpdate_DirtyFormEscGuard pins the #790 guard end-to-end through the shell:
// a create/edit form that has unsaved input owns Esc (the shell forwards it via
// the EscInterceptor seam instead of bare-popping), so the first Esc arms a
// discard confirmation (the dialog stays on the stack) and only the second
// consecutive Esc closes it. A clean form still closes on the first Esc.
func TestUpdate_DirtyFormEscGuard(t *testing.T) {
	t.Parallel()

	newModel := func() *App {
		m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
		ef, _ := dialogs.NewEntryForm(dialogs.EntryFormInput{
			Ctx: t.Context(), Mutator: capMutator{cap: goldenCap("aws", "secret")},
			Service: "secret", Styles: styles.New(),
		})
		m.dialogs = []dialog{dialogAdapter{m: ef}}
		m = updateApp(t, m, tea.WindowSizeMsg{Width: goldenTermWidth, Height: goldenTermHeight})

		return m
	}

	// Clean form: the first Esc pops it (via the CanceledMsg the dialog emits).
	clean := newModel()
	next, cmd := clean.Update(specialKey(tea.KeyEscape))
	clean, ok := next.(*App)
	require.True(t, ok)
	require.NotNil(t, cmd, "a clean form emits CanceledMsg on esc")
	clean = updateApp(t, clean, cmd())
	assert.Empty(t, clean.dialogs, "esc closes a clean form")

	// Dirty form: typing a character makes it dirty.
	dirty := newModel()
	dirty = updateApp(t, dirty, keyPress('x'))

	// First Esc arms — the dialog stays on the stack (not bare-popped).
	dirty = updateApp(t, dirty, specialKey(tea.KeyEscape))
	require.Len(t, dirty.dialogs, 1, "first esc on a dirty form arms, does not close")

	// Second consecutive Esc discards: the dialog emits CanceledMsg, which pops it.
	next, cmd = dirty.Update(specialKey(tea.KeyEscape))
	dirty, ok = next.(*App)
	require.True(t, ok)
	require.NotNil(t, cmd, "the second esc emits CanceledMsg")
	dirty = updateApp(t, dirty, cmd())
	assert.Empty(t, dirty.dialogs, "a second consecutive esc discards")
}

// TestUpdate_StagedCountBadge pins that a staged-count report updates the Staging
// tab's count badge, and zero clears it.
func TestUpdate_StagedCountBadge(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})

	m = updateApp(t, m, nav.StagedCount{Service: "param", Count: 2})
	m = updateApp(t, m, nav.StagedCount{Service: "secret", Count: 1})
	assert.Equal(t, "Staging(3)", stagingTabTitle(m), "the badge totals both services")

	m = updateApp(t, m, nav.StagedCount{Service: "param", Count: 0})
	m = updateApp(t, m, nav.StagedCount{Service: "secret", Count: 0})
	assert.Equal(t, "Staging", stagingTabTitle(m), "zero clears the badge")
}

// TestUpdate_MutationDoneClosesDialog pins that a completed mutation pops the
// dialog and voices its status.
func TestUpdate_MutationDoneClosesDialog(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	m.dialogs = []dialog{&fakeDialog{}}

	m = updateApp(t, m, dialogs.MutationDoneMsg{Service: "param", Status: "Staged create."})
	assert.Empty(t, m.dialogs, "a completed mutation closes the dialog")
	assert.Equal(t, "Staged create.", m.status, "the outcome is voiced in the status line")
}

// TestUpdate_DialogOpenGuardPreventsStacking pins #697: dialog-open requests
// arrive as async commands, so a rapid double-press of a dialog-open key can
// emit two Open* commands before the first dialog lands. The app must push
// exactly one dialog — the second (stale, in-flight) Open* is dropped rather
// than stacking an identical duplicate that Esc would reveal underneath.
func TestUpdate_DialogOpenGuardPreventsStacking(t *testing.T) {
	t.Parallel()

	mut := capMutator{cap: goldenCap("aws", "param")}
	m := newApp(config{
		scope:      provider.Scope{Provider: provider.ProviderAWS},
		target:     awsTargetFixture(),
		mutatorFor: func(string) data.Mutator { return mut },
	})

	open := nav.OpenEntryForm{Service: "param"}

	m = updateApp(t, m, open)
	require.Len(t, m.dialogs, 1, "the first open pushes the dialog")

	m = updateApp(t, m, open)
	assert.Len(t, m.dialogs, 1, "a second open while a dialog is already on the stack is dropped, not stacked")
}

// stagingTabTitle returns the current Staging tab title.
func stagingTabTitle(m *App) string {
	for _, t := range m.tabs {
		if t.Service == stagingService {
			return t.Title
		}
	}

	return ""
}

// TestUpdate_MouseClickReducesToTabSelect pins the epic's mouse rule: a tab-bar
// click reduces to the SAME internal tab selection as the equivalent jump key,
// with the coordinate derived from the tab-bar layout helper (never hard-coded).
func TestUpdate_MouseClickReducesToTabSelect(t *testing.T) {
	t.Parallel()

	base := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})

	// Size the shell above the minimum so the tab bar is actually rendered and
	// mouse tab selection is live (below the minimum a click is inert).
	base = updateApp(t, base, tea.WindowSizeMsg{Width: 100, Height: 30})

	// Derive an x column that the layout maps to tab index 1 — no magic number.
	x, ok := columnForTab(base.tabBar(), 1)
	require.True(t, ok, "layout must expose a column for tab 1")

	clicked := updateApp(t, base, tea.MouseClickMsg{X: x, Y: base.tabBarRow(), Button: tea.MouseLeft})

	keyed := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	keyed = updateApp(t, keyed, keyPress('2'))

	assert.Equal(t, keyed.activeTab, clicked.activeTab, "click and key select the same tab")
	assert.Equal(t, 1, clicked.activeTab)
}

// TestUpdate_MouseClickInertBelowMinSize pins the guard: below the minimum
// terminal size the tab bar is not rendered, so a left click at the tab-bar row
// must not hit-test (and switch) an invisible tab.
func TestUpdate_MouseClickInertBelowMinSize(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})

	// A column that WOULD map to tab 1 at full size, then shrink below the minimum.
	x, ok := columnForTab(m.tabBar(), 1)
	require.True(t, ok, "layout must expose a column for tab 1")

	m = updateApp(t, m, tea.WindowSizeMsg{Width: minWidth - 1, Height: minHeight - 1})
	m = updateApp(t, m, tea.MouseClickMsg{X: x, Y: m.tabBarRow(), Button: tea.MouseLeft})

	assert.Equal(t, 0, m.activeTab, "a click is inert while the tab bar is hidden")
}

// columnForTab walks the tab bar's own hit-test to find a column inside tab i,
// so a click test never encodes a fixed coordinate.
func columnForTab(tb components.TabBar, target int) (int, bool) {
	for x := range 200 {
		if i, ok := tb.TabAtX(x); ok && i == target {
			return x, true
		}
	}

	return 0, false
}

// TestUpdate_DialogClickTranslatedToContentLocal pins the #663 dialog-offset fix:
// a click inside a centered dialog box reaches the dialog translated into its own
// content-local coordinates — the box origin (from the overlay hit region) and
// the frame inset subtracted — so a dialog hit-tests its controls without knowing
// the shell's centering math. The screen coordinate is derived from the drawn
// overlay region, never hard-coded.
func TestUpdate_DialogClickTranslatedToContentLocal(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	fd := &fakeDialog{}
	m.dialogs = []dialog{fd}
	_ = m.View() // composes the overlay and builds the dialog hit map

	bx, by, ok := m.dialogHits.Origin("0")
	require.True(t, ok, "the top dialog's box region is drawn")

	left, top := m.dialogFrameOffset()

	// Click a point 3 columns into the dialog content; the dialog must see (3, 0).
	const dx, dy = 3, 0

	_ = updateApp(t, m, tea.MouseClickMsg{X: bx + left + dx, Y: by + top + dy, Button: tea.MouseLeft})

	var click *tea.MouseClickMsg

	for _, msg := range fd.got {
		if c, isClick := msg.(tea.MouseClickMsg); isClick {
			c := c
			click = &c
		}
	}

	require.NotNil(t, click, "the click reached the dialog")
	assert.Equal(t, dx, click.X, "X is translated to content-local (box origin + frame subtracted)")
	assert.Equal(t, dy, click.Y, "Y is translated to content-local")
}

// TestUpdate_DialogClickOutsideBoxSwallowed pins that a modal click landing
// outside the dialog box is swallowed — it never reaches the dialog and never
// leaks to the page beneath (modality is preserved).
func TestUpdate_DialogClickOutsideBoxSwallowed(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	fd := &fakeDialog{}
	m.dialogs = []dialog{fd}
	_ = m.View()

	// (0,0) is the top-left corner, well outside the centered box.
	_ = updateApp(t, m, tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})

	for _, msg := range fd.got {
		_, isClick := msg.(tea.MouseClickMsg)
		assert.False(t, isClick, "a click outside the box is swallowed, not forwarded to the dialog")
	}
}

// TestUpdate_HelpBarClickTogglesHelp pins that a click on the bottom help bar
// toggles the short/full help, reducing to the same toggle `?` performs.
func TestUpdate_HelpBarClickTogglesHelp(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	require.False(t, m.help.ShowAll, "help starts collapsed")

	m = updateApp(t, m, tea.MouseClickMsg{X: 1, Y: m.helpBarTop(), Button: tea.MouseLeft})
	assert.True(t, m.help.ShowAll, "clicking the help bar expands the help (like ?)")

	m = updateApp(t, m, tea.MouseClickMsg{X: 1, Y: m.helpBarTop(), Button: tea.MouseLeft})
	assert.False(t, m.help.ShowAll, "clicking again collapses it")
}

// TestUpdate_CopyToClipboard pins that the `y` key routes a non-empty focused
// value through the OSC52 clipboard seam (asserted via a stub, never real escape
// bytes), and — the guard — that with no value it does NOT touch the clipboard:
// copying "" would emit an OSC52 that clears the user's system clipboard.
//
//nolint:paralleltest // swaps the package-level setClipboard seam; must not race other tests
func TestUpdate_CopyToClipboard(t *testing.T) {
	called := false
	copied := ""
	orig := setClipboard
	setClipboard = func(s string) tea.Cmd {
		called = true
		copied = s

		return nil
	}

	t.Cleanup(func() { setClipboard = orig })

	// A focused value is present: y copies it through the seam.
	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	m.copyValue = "s3cr3t"
	_ = updateApp(t, m, keyPress('y'))

	assert.True(t, called, "y copies via the clipboard seam")
	assert.Equal(t, "s3cr3t", copied, "y copies the focused value verbatim")

	// No focused value: y must be a no-op so it never clears the clipboard.
	called = false
	empty := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	_ = updateApp(t, empty, keyPress('y'))

	assert.False(t, called, "y with no value does not clear the clipboard")
}

// TestUpdate_FocusedFilterCapturesGlobalKeys pins the headline fix: while the
// browser's filter input is focused, the global key map must NOT steal
// keystrokes — typing `q` and `1` types text (never quits, never switches tabs).
// Only ctrl+c stays global as a force-quit escape.
func TestUpdate_FocusedFilterCapturesGlobalKeys(t *testing.T) {
	t.Parallel()

	// Control: with nothing focused, `q` quits (proves q IS normally a global key).
	control := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		target:    awsTargetFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), nil),
	})
	control = updateApp(t, control, tea.WindowSizeMsg{Width: browserTermWidth, Height: browserTermHeight})
	require.False(t, control.activePageCapturesInput(), "the list, not an input, is focused at first")

	_, quitCmd := control.Update(keyPress('q'))
	require.NotNil(t, quitCmd, "q emits a command when unfocused")
	assert.IsType(t, tea.QuitMsg{}, quitCmd(), "q quits while no input is focused")

	// Now focus the filter and type q then 1.
	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		target:    awsTargetFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), nil),
	})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: browserTermWidth, Height: browserTermHeight})

	m = updateApp(t, m, keyPress('/')) // browser: focus the filter input
	require.True(t, m.activePageCapturesInput(), "the filter input is now focused")

	// q must NOT quit: the returned command is text-input machinery, never Quit.
	_, qCmd := m.Update(keyPress('q'))
	if qCmd != nil {
		assert.NotEqual(t, tea.QuitMsg{}, qCmd(), "q typed into the filter must not quit")
	}

	m = updateApp(t, m, keyPress('q'))
	m = updateApp(t, m, keyPress('1'))

	assert.Equal(t, 0, m.activeTab, "1 typed into the filter must not switch tabs")

	// The characters reached the input: the rendered header echoes the filter value.
	assert.Contains(t, m.render(), "q1", "q and 1 were typed into the filter")

	// ctrl+c still force-quits even while the input is focused.
	_, escCmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.NotNil(t, escCmd, "ctrl+c emits a command")
	assert.IsType(t, tea.QuitMsg{}, escCmd(), "ctrl+c force-quits even while an input is focused")
}

// keyForBinding returns a key press whose String() matches keystroke, resolving
// special keys (tab/shift+tab/esc) that are not single printable runes.
func (m *App) keyForBinding(t *testing.T, keystroke string) tea.KeyPressMsg {
	t.Helper()

	switch keystroke {
	case "tab":
		return specialKey(tea.KeyTab)
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "esc":
		return specialKey(tea.KeyEscape)
	default:
		require.Len(t, keystroke, 1, "keyForBinding only handles named or single-rune keys")

		return keyPress(rune(keystroke[0]))
	}
}

// TestShell_AWSGolden renders the full app shell for an AWS scope through
// teatest and compares it to the golden. The AWS status bar shows
// profile/account/region and all three tabs (Param, Secret, Staging).
func TestShell_AWSGolden(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})

	requireShellGolden(t, m)
}

// TestShell_AzureVaultOnlyGolden pins Azure scope gating: a vault-only scope
// shows the Key Vault (secret) tab and Staging, but NOT App Configuration.
func TestShell_AzureVaultOnlyGolden(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newApp(config{scope: provider.AzureKeyVaultScope("myvault")})

	requireShellGolden(t, m)
}

// TestShell_AzureStoreOnlyGolden pins the complementary gate: a store-only Azure
// scope shows App Configuration (param) and Staging, but NOT Key Vault.
func TestShell_AzureStoreOnlyGolden(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	m := newApp(config{scope: provider.AzureAppConfigScope("mystore")})

	requireShellGolden(t, m)
}

// requireShellGolden drives the model through teatest at a fixed size and
// golden-compares the VISIBLE SCREEN — the captured byte stream replayed through
// a virtual terminal (see renderVisibleScreen) — rather than the raw stream. The
// raw stream carries the terminal's capability handshake, which differs between
// CI and a local run even when the drawn frame is byte-identical; goldening the
// rendered cell grid absorbs that divergence and yields a human-readable golden.
//
// The AWS/Azure golden models have no async work (identity is preseeded; non-AWS
// scopes fetch nothing), so Bubble Tea's FIFO message order — the initial resize
// renders the shell, then the quit key exits — makes the captured stream
// deterministic without polling the shared output reader (which would consume
// the frame before FinalOutput).
func requireShellGolden(t *testing.T, m *App) {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(goldenTermWidth, goldenTermHeight))

	tm.Send(keyPress('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))

	out, err := io.ReadAll(tm.FinalOutput(t))
	require.NoError(t, err)
	golden.RequireEqual(t, renderVisibleScreen(t, out))
}

// TestUpdate_FailedTargetRetriesAfterStagedCount pins #1005: a target lookup
// that fails at launch (a transient STS failure) is retried once the next staged
// count is reported, so the status bar and the apply line recover instead of
// staying blank for the session. Each failure arms exactly one retry.
func TestUpdate_FailedTargetRetriesAfterStagedCount(t *testing.T) {
	t.Parallel()

	resolved := provider.AWSTarget("dev", "123456789012", "ap-northeast-1")
	calls := 0
	m := newApp(config{
		scope: provider.Scope{Provider: provider.ProviderAWS},
		fetchTarget: func() (provider.Target, error) {
			calls++
			if calls == 1 {
				return provider.Target{}, errors.New("sts unreachable")
			}

			return resolved, nil
		},
	})

	m = updateApp(t, m, m.fetchTargetCmd()())
	assert.Equal(t, "AWS", m.applyTargetLine(), "the failed lookup leaves the target blank")

	// A staged count arrives (the staging probe has just resolved the identity):
	// the lookup is retried and the target appears.
	next, cmd := m.Update(nav.StagedCount{Service: "param", Count: 0})
	m = next.(*App) //nolint:forcetypeassert // Update always returns *App
	require.NotNil(t, cmd, "a staged count after a failed lookup retries it")
	m = updateApp(t, m, cmd())
	assert.Equal(t, "AWS · profile dev · account 123456789012 · region ap-northeast-1", m.applyTargetLine())
	assert.Equal(t, 2, calls)

	// Once resolved, later staged counts do not look the target up again.
	_, cmd = m.Update(nav.StagedCount{Service: "param", Count: 1})
	assert.Nil(t, cmd)
	assert.Equal(t, 2, calls)
}

// TestUpdate_FailedTargetRetryBounds pins the retry's limits (#1005): a failed
// retry arms the next one, several staged counts start only one lookup, and a
// staged count that arrived while the lookup was in flight retries at once.
func TestUpdate_FailedTargetRetryBounds(t *testing.T) {
	t.Parallel()

	calls := 0
	m := newApp(config{
		scope: provider.Scope{Provider: provider.ProviderAWS},
		fetchTarget: func() (provider.Target, error) {
			calls++

			return provider.Target{}, errors.New("sts unreachable")
		},
	})

	fetch := m.fetchTargetCmd()
	m = updateApp(t, m, fetch())

	// Two staged counts (a two-section review) start one lookup, not two.
	next, retry := m.Update(nav.StagedCount{Service: "param", Count: 0})
	m = next.(*App) //nolint:forcetypeassert // Update always returns *App
	require.NotNil(t, retry)
	next, second := m.Update(nav.StagedCount{Service: "secret", Count: 0})
	m = next.(*App) //nolint:forcetypeassert // Update always returns *App
	assert.Nil(t, second, "a retry already in flight is not started again")

	// That count arrived while the retry was in flight, so its failure retries
	// at once rather than waiting for another count.
	next, again := m.Update(retry())
	m = next.(*App) //nolint:forcetypeassert // Update always returns *App
	require.NotNil(t, again, "a count seen during the lookup retries at once")

	// With no count during this lookup, its failure only arms the next retry.
	next, cmd := m.Update(again())
	m = next.(*App) //nolint:forcetypeassert // Update always returns *App
	assert.Nil(t, cmd)
	assert.True(t, m.retryTarget, "the failed retry arms the next one")
	assert.Equal(t, 3, calls)
}

// TestUpdate_PendingTargetResolves pins the async target flow: a pending AWS
// target starts a fetch on Init, the resolved target replaces it, and a failed
// fetch only clears the pending flag. A target that describes itself (Google
// Cloud) never fetches.
func TestUpdate_PendingTargetResolves(t *testing.T) {
	t.Parallel()

	resolved := provider.AWSTarget("dev", "123456789012", "ap-northeast-1")
	m := newApp(config{
		scope:       provider.Scope{Provider: provider.ProviderAWS},
		fetchTarget: func() (provider.Target, error) { return resolved, nil },
	})
	require.True(t, m.statusBar().Target.Pending, "the AWS target is pending until STS answers")
	assert.Equal(t, "AWS", m.applyTargetLine())

	msg := m.fetchTargetCmd()()
	require.Equal(t, targetMsg{target: resolved}, msg, "the fetch command reports the resolved target")
	require.NotNil(t, m.Init(), "Init starts the fetch")

	m.Update(msg)
	assert.False(t, m.statusBar().Target.Pending)
	assert.Equal(t, "AWS · profile dev · account 123456789012 · region ap-northeast-1", m.applyTargetLine())

	failed := newApp(config{
		scope:       provider.Scope{Provider: provider.ProviderAWS},
		fetchTarget: func() (provider.Target, error) { return provider.Target{}, errors.New("no credentials") },
	})
	errMsg := failed.fetchTargetCmd()()
	require.IsType(t, targetErrMsg{}, errMsg, "the fetch command reports a lookup failure")
	failed.Update(errMsg)
	assert.False(t, failed.statusBar().Target.Pending, "a failed lookup stops the loading placeholder")

	gcloud := newApp(config{
		scope: provider.GoogleCloudScope("proj"),
		fetchTarget: func() (provider.Target, error) {
			t.Error("a self-describing target must not fetch")

			return provider.Target{}, nil
		},
	})
	assert.False(t, gcloud.statusBar().Target.Pending)
	assert.Equal(t, "Google Cloud · project proj", gcloud.applyTargetLine())
}

// fakeCopyPage is a page that supplies a copy value (and records that CopyText
// was consulted), so the app-level `y` wiring can be asserted without a real
// browser page or an async load.
type fakeCopyPage struct {
	text       string
	copyCalled bool
}

func (p *fakeCopyPage) Update(tea.Msg) (page, tea.Cmd) { return p, nil }
func (p *fakeCopyPage) View(int, int) string           { return "" }
func (p *fakeCopyPage) capturesInput() bool            { return false }

func (p *fakeCopyPage) CopyText() (string, bool) {
	p.copyCalled = true
	if p.text == "" {
		return "", false
	}

	return p.text, true
}

// TestApp_CopyWritesActivePageValue pins that `y` copies the active page's
// revealed value through the OSC52 seam (asserted via a stub, never real escape
// bytes), and that an empty value is a guarded no-op so it never clears the
// clipboard.
//
//nolint:paralleltest // swaps the package-level setClipboard seam; must not race other tests
func TestApp_CopyWritesActivePageValue(t *testing.T) {
	copied := ""
	called := false
	orig := setClipboard
	setClipboard = func(s string) tea.Cmd {
		called = true
		copied = s

		return nil
	}

	t.Cleanup(func() { setClipboard = orig })

	app := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, target: awsTargetFixture()})
	fp := &fakeCopyPage{text: "s3cr3t"}
	app.pages = []page{fp}

	_ = updateApp(t, app, keyPress('y'))

	assert.True(t, called, "y copies the active page's value")
	assert.Equal(t, "s3cr3t", copied)
	assert.True(t, fp.copyCalled, "the app consults the active page's CopyText for the `y` copy")

	// An empty value must not reach the clipboard (an OSC52 with "" clears it).
	called = false
	empty := &fakeCopyPage{text: ""}
	app.pages = []page{empty}

	_ = updateApp(t, app, keyPress('y'))
	assert.False(t, called, "an empty copy is a guarded no-op")
}

// drainBatch runs cmd (recursing into batches) and returns every leaf message.
// It blocks on timer commands (e.g. tea.Tick), so callers accept that latency.
func drainBatch(cmd tea.Cmd) []tea.Msg {
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
			out = append(out, drainBatch(c)...)
		}

		return out
	}

	return []tea.Msg{msg}
}

// clearScreenType is the reflected type of tea's (unexported) clear-screen msg.
//
//nolint:gochecknoglobals // test-only type sentinel
var clearScreenType = reflect.TypeOf(tea.ClearScreen())

// TestCloudShellRepaintCmd_ProducesRepaintMsg pins that the repaint ticker fires
// a cloudShellRepaintMsg (so the loop keeps rescheduling itself).
func TestCloudShellRepaintCmd_ProducesRepaintMsg(t *testing.T) {
	t.Parallel()

	msg := cloudShellRepaintCmd()()
	assert.IsType(t, cloudShellRepaintMsg{}, msg)
}

// TestUpdate_CloudShellRepaint_ClearsAndReschedules pins that handling a repaint
// tick both forces a full repaint (tea.ClearScreen) and arms the next tick.
func TestUpdate_CloudShellRepaint_ClearsAndReschedules(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}})

	_, cmd := m.Update(cloudShellRepaintMsg{})
	require.NotNil(t, cmd)

	msgs := drainBatch(cmd)

	var sawClear, sawReschedule bool

	for _, msg := range msgs {
		if reflect.TypeOf(msg) == clearScreenType {
			sawClear = true
		}

		if _, ok := msg.(cloudShellRepaintMsg); ok {
			sawReschedule = true
		}
	}

	assert.True(t, sawClear, "a repaint tick must force a full ClearScreen")
	assert.True(t, sawReschedule, "a repaint tick must arm the next tick")
}

// TestNewApp_LaunchNamespaceSeedsBrowser pins #992 at the shell: the launch App
// Configuration namespace reaches the browser, whose first list shows only that
// namespace's settings.
func TestNewApp_LaunchNamespaceSeedsBrowser(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	scope := provider.AzureAppConfigScope("myapp-config")
	scope.AppConfigNamespace = "staging"

	m := newApp(config{scope: scope, sourceFor: sourceForShape("param", azureAppConfigSource(), nil)})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: 120, Height: 34})

	for _, msg := range drainBatch(m.initialPageCmd()) {
		if _, tick := msg.(spinner.TickMsg); !tick {
			m = updateApp(t, m, msg)
		}
	}

	out := m.View().Content
	assert.Contains(t, out, "entries (1)", "only the launch namespace's setting is listed")
	assert.Contains(t, out, "[staging]", "the listed setting is in the launch namespace")
	assert.Contains(t, out, "app/FeatureX", "the launch namespace's setting is listed")
	assert.NotContains(t, out, "app/Timeout", "a null-namespace setting is filtered out")
}
