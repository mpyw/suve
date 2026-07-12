//nolint:testpackage // white-box: exercises newApp/config, the dialog stack, and the setClipboard seam
package tui

import (
	"io"
	"testing"
	"time"

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
)

// awsIdentityFixture is the deterministic identity used in AWS goldens so the
// status bar renders without an async STS call.
func awsIdentityFixture() *components.AWSIdentity {
	return &components.AWSIdentity{
		Account: "123456789012",
		Region:  "ap-northeast-1",
		Profile: "dev",
	}
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

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
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

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
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

// TestUpdate_DialogCapturesGlobalKeys pins the Step-2 carried-over fix: while a
// modal dialog is open, only ctrl+c stays global (force-quit); every other key —
// q, digits, letters — is forwarded into the dialog so a focused text field
// types normally, and does not quit or switch tabs.
func TestUpdate_DialogCapturesGlobalKeys(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
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

	busy := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	fdBusy := &fakeDialog{busyFlag: true}
	busy.dialogs = []dialog{fdBusy}

	busy = updateApp(t, busy, specialKey(tea.KeyEscape))
	require.Len(t, busy.dialogs, 1, "a busy dialog is not dismissed by esc")
	assert.Len(t, fdBusy.got, 1, "esc is forwarded to the busy dialog instead of closing it")

	idle := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	idle.dialogs = []dialog{&fakeDialog{}}
	idle = updateApp(t, idle, specialKey(tea.KeyEscape))
	assert.Empty(t, idle.dialogs, "an idle dialog is dismissed by esc")
}

// TestUpdate_StagedCountBadge pins that a staged-count report updates the Staging
// tab's count badge, and zero clears it.
func TestUpdate_StagedCountBadge(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})

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

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
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
		identity:   awsIdentityFixture(),
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

	base := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})

	// Size the shell above the minimum so the tab bar is actually rendered and
	// mouse tab selection is live (below the minimum a click is inert).
	base = updateApp(t, base, tea.WindowSizeMsg{Width: 100, Height: 30})

	// Derive an x column that the layout maps to tab index 1 — no magic number.
	x, ok := columnForTab(base.tabBar(), 1)
	require.True(t, ok, "layout must expose a column for tab 1")

	clicked := updateApp(t, base, tea.MouseClickMsg{X: x, Y: base.tabBarRow(), Button: tea.MouseLeft})

	keyed := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	keyed = updateApp(t, keyed, keyPress('2'))

	assert.Equal(t, keyed.activeTab, clicked.activeTab, "click and key select the same tab")
	assert.Equal(t, 1, clicked.activeTab)
}

// TestUpdate_MouseClickInertBelowMinSize pins the guard: below the minimum
// terminal size the tab bar is not rendered, so a left click at the tab-bar row
// must not hit-test (and switch) an invisible tab.
func TestUpdate_MouseClickInertBelowMinSize(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})

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
	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	m.copyValue = "s3cr3t"
	_ = updateApp(t, m, keyPress('y'))

	assert.True(t, called, "y copies via the clipboard seam")
	assert.Equal(t, "s3cr3t", copied, "y copies the focused value verbatim")

	// No focused value: y must be a no-op so it never clears the clipboard.
	called = false
	empty := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
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
		identity:  awsIdentityFixture(),
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
		identity:  awsIdentityFixture(),
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

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})

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
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	out, err := io.ReadAll(tm.FinalOutput(t))
	require.NoError(t, err)
	golden.RequireEqual(t, renderVisibleScreen(t, out))
}
