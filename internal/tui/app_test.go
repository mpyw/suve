//nolint:testpackage // white-box: exercises newApp/config, the dialog stack, and the setClipboard seam
package tui

import (
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/components"
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
// modality (input reaching the dialog, not the page) can be asserted.
type fakeDialog struct {
	got []tea.Msg
}

func (d *fakeDialog) Update(msg tea.Msg) (dialog, tea.Cmd) {
	d.got = append(d.got, msg)

	return d, nil
}

func (d *fakeDialog) View() string { return "fake dialog" }

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

// TestUpdate_MouseClickReducesToTabSelect pins the epic's mouse rule: a tab-bar
// click reduces to the SAME internal tab selection as the equivalent jump key,
// with the coordinate derived from the tab-bar layout helper (never hard-coded).
func TestUpdate_MouseClickReducesToTabSelect(t *testing.T) {
	t.Parallel()

	base := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})

	// Derive an x column that the layout maps to tab index 1 — no magic number.
	x, ok := columnForTab(base.tabBar(), 1)
	require.True(t, ok, "layout must expose a column for tab 1")

	clicked := updateApp(t, base, tea.MouseClickMsg{X: x, Y: base.tabBarRow(), Button: tea.MouseLeft})

	keyed := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	keyed = updateApp(t, keyed, keyPress('2'))

	assert.Equal(t, keyed.activeTab, clicked.activeTab, "click and key select the same tab")
	assert.Equal(t, 1, clicked.activeTab)
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

// TestUpdate_CopyToClipboard pins that the `y` key routes through the OSC52
// clipboard seam (asserted via a stub, never real escape bytes).
//
//nolint:paralleltest // swaps the package-level setClipboard seam; must not race other tests
func TestUpdate_CopyToClipboard(t *testing.T) {
	called := false
	orig := setClipboard
	setClipboard = func(string) tea.Cmd {
		called = true

		return nil
	}

	t.Cleanup(func() { setClipboard = orig })

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	_ = updateApp(t, m, keyPress('y'))

	assert.True(t, called, "y copies via the clipboard seam")
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

// requireShellGolden drives the model through teatest at a fixed 100x30 size and
// golden-compares the full program output. The AWS/Azure golden models have no
// async work (identity is preseeded; non-AWS scopes fetch nothing), so Bubble
// Tea's FIFO message order — the initial resize renders the shell, then the
// quit key exits — makes the captured stream deterministic without polling the
// shared output reader (which would consume the frame before FinalOutput).
func requireShellGolden(t *testing.T, m *App) {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))

	tm.Send(keyPress('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	out, err := io.ReadAll(tm.FinalOutput(t))
	require.NoError(t, err)
	teatest.RequireEqualOutput(t, out)
}
