//nolint:testpackage // white-box: exercises app.go pure helpers, mouse routing, and renderTooSmall
package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/tui/data"
)

// recordingPage is a test-only page that records every message routed to it, so
// mouse forwarding (and its swallow-while-modal counterpart) can be asserted from
// the page's side.
type recordingPage struct {
	got []tea.Msg
}

func (p *recordingPage) Update(msg tea.Msg) (page, tea.Cmd) {
	p.got = append(p.got, msg)

	return p, nil
}

func (p *recordingPage) View(int, int) string { return "recording page" }
func (p *recordingPage) capturesInput() bool  { return false }

// TestAppendKV pins the "key value" segment builder: a non-empty value is
// appended as "key value", an empty value leaves the slice untouched.
func TestAppendKV(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"aws", "account 123"},
		appendKV([]string{"aws"}, "account", "123"), "a non-empty value is appended as \"key value\"")
	assert.Equal(t, []string{"aws"},
		appendKV([]string{"aws"}, "region", ""), "an empty value is skipped")
}

// TestTargetTitle pins the apply/reset target label: a global fan-out or a
// multi-target set is "all", while a single target voices its own label.
func TestTargetTitle(t *testing.T) {
	t.Parallel()

	one := &goldenStaging{service: "secret", label: "Secret"}
	two := &goldenStaging{service: "param", label: "Param"}

	assert.Equal(t, "all", targetTitle(true, []data.StagingService{one}), "a global fan-out is \"all\"")
	assert.Equal(t, "all", targetTitle(false, []data.StagingService{one, two}), "a multi-target set is \"all\"")
	assert.Equal(t, "all", targetTitle(false, nil), "an empty target set is \"all\"")
	assert.Equal(t, "Secret", targetTitle(false, []data.StagingService{one}), "a single target voices its label")
}

// TestApplyResetTitle pins the apply/reset confirmation titles compose the fixed
// prefix with the target title.
func TestApplyResetTitle(t *testing.T) {
	t.Parallel()

	one := &goldenStaging{service: "secret", label: "Secret"}

	assert.Equal(t, "Apply staged changes — Secret", applyTitle(false, []data.StagingService{one}))
	assert.Equal(t, "Apply staged changes — all", applyTitle(true, []data.StagingService{one}))
	assert.Equal(t, "Reset staged changes — Secret", resetTitle(false, []data.StagingService{one}))
	assert.Equal(t, "Reset staged changes — all", resetTitle(true, []data.StagingService{one}))
}

// TestClipStatus pins the status-line width clamp: a status that fits is returned
// verbatim, a status wider than the terminal is truncated to width−1, and a
// degenerate width (≤1) returns the input unchanged (no room to clamp).
func TestClipStatus(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Staged create.", clipStatus("Staged create.", 80), "a status that fits is unchanged")
	assert.Equal(t, "hi", clipStatus("hi", 1), "a width of 1 is too small to clamp; the input is returned")

	clipped := clipStatus("this is a long status line that must be clipped", 10)
	assert.LessOrEqual(t, len([]rune(clipped)), 9, "a wide status is clamped below the terminal width")
}

// TestApplyTargetLine pins the apply target identity line per provider: AWS shows
// account/region only when the identity is resolved, Google Cloud shows the
// project, Azure shows vault/store, and an unknown provider falls back to its bare
// name.
func TestApplyTargetLine(t *testing.T) {
	t.Parallel()

	awsWith := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	assert.Equal(t, "aws · account 123456789012 · region ap-northeast-1", awsWith.applyTargetLine(),
		"AWS voices account and region from the resolved identity")

	awsNo := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}})
	assert.Equal(t, "aws", awsNo.applyTargetLine(), "AWS with no identity is just the provider name")

	gcloud := newApp(config{scope: provider.GoogleCloudScope("proj")})
	assert.Equal(t, "googlecloud · project proj", gcloud.applyTargetLine(), "Google Cloud voices the project")

	azure := newApp(config{scope: provider.Scope{Provider: provider.ProviderAzure, VaultName: "v", StoreName: "s"}})
	assert.Equal(t, "azure · vault v · store s", azure.applyTargetLine(), "Azure voices the vault and store")

	unknown := newApp(config{scope: provider.Scope{Provider: provider.Provider("mystery")}})
	assert.Equal(t, "mystery", unknown.applyTargetLine(), "an unknown provider falls back to its bare name")
}

// TestRenderTooSmall pins the minimum-size notice: below the minimum terminal
// size View() draws the "terminal too small" notice instead of the shell chrome.
func TestRenderTooSmall(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	m = updateApp(t, m, tea.WindowSizeMsg{Width: minWidth - 20, Height: minHeight - 6})

	// View() takes the too-small branch (it calls renderTooSmall for the content).
	_ = m.View()

	assert.Contains(t, m.renderTooSmall(), "terminal too small", "the too-small notice names the reason")
}

// TestUpdate_MouseWheelForwardedToPage pins that a wheel event with no dialog open
// reaches the active page, translated into page-local coordinates (its Y shifted
// down by the fixed chrome above the page body).
func TestUpdate_MouseWheelForwardedToPage(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	rp := &recordingPage{}
	m.pages = []page{rp}

	m = updateApp(t, m, tea.MouseWheelMsg{X: 5, Y: 10, Button: tea.MouseWheelDown})

	require.Len(t, rp.got, 1, "the wheel reaches the page")
	wheel, ok := rp.got[0].(tea.MouseWheelMsg)
	require.True(t, ok, "the page received a wheel message")
	assert.Equal(t, 10-m.pageBodyTop(), wheel.Y, "the wheel Y is translated into page-local coordinates")
}

// TestUpdate_MouseWheelSwallowedByDialog pins that while a dialog is modal the
// wheel is forwarded into the dialog (its viewport scrolls), never to the page.
func TestUpdate_MouseWheelSwallowedByDialog(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	rp := &recordingPage{}
	m.pages = []page{rp}
	fd := &fakeDialog{}
	m.dialogs = []dialog{fd}

	updateApp(t, m, tea.MouseWheelMsg{X: 5, Y: 10, Button: tea.MouseWheelDown})

	require.Len(t, fd.got, 1, "the wheel reaches the modal dialog")
	assert.IsType(t, tea.MouseWheelMsg{}, fd.got[0])
	assert.Empty(t, rp.got, "the wheel never leaks to the page beneath the dialog")
}

// TestUpdate_MouseMotionForwardedToPage pins that a motion event with no dialog
// open reaches the active page in page-local coordinates (a page follows a drag).
func TestUpdate_MouseMotionForwardedToPage(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	rp := &recordingPage{}
	m.pages = []page{rp}

	m = updateApp(t, m, tea.MouseMotionMsg{X: 4, Y: 12})

	require.Len(t, rp.got, 1, "the motion reaches the page")
	motion, ok := rp.got[0].(tea.MouseMotionMsg)
	require.True(t, ok, "the page received a motion message")
	assert.Equal(t, 12-m.pageBodyTop(), motion.Y, "the motion Y is translated into page-local coordinates")
}

// TestUpdate_MouseMotionSwallowedByDialog pins that while a dialog is modal a
// motion (drag) is swallowed — it never leaks to the page beneath the overlay.
func TestUpdate_MouseMotionSwallowedByDialog(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	rp := &recordingPage{}
	m.pages = []page{rp}
	m.dialogs = []dialog{&fakeDialog{}}

	updateApp(t, m, tea.MouseMotionMsg{X: 4, Y: 12})

	assert.Empty(t, rp.got, "a motion is swallowed while a dialog is modal")
}

// TestUpdate_MouseReleaseForwardedToPage pins that a button-release with no dialog
// open reaches the active page in page-local coordinates (a page ends a drag).
func TestUpdate_MouseReleaseForwardedToPage(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	rp := &recordingPage{}
	m.pages = []page{rp}

	m = updateApp(t, m, tea.MouseReleaseMsg{X: 4, Y: 12, Button: tea.MouseLeft})

	require.Len(t, rp.got, 1, "the release reaches the page")
	release, ok := rp.got[0].(tea.MouseReleaseMsg)
	require.True(t, ok, "the page received a release message")
	assert.Equal(t, 12-m.pageBodyTop(), release.Y, "the release Y is translated into page-local coordinates")
}

// TestUpdate_MouseReleaseSwallowedByDialog pins that while a dialog is modal a
// button-release is swallowed rather than forwarded to the page beneath it.
func TestUpdate_MouseReleaseSwallowedByDialog(t *testing.T) {
	t.Parallel()

	m := newApp(config{scope: provider.Scope{Provider: provider.ProviderAWS}, identity: awsIdentityFixture()})
	rp := &recordingPage{}
	m.pages = []page{rp}
	m.dialogs = []dialog{&fakeDialog{}}

	updateApp(t, m, tea.MouseReleaseMsg{X: 4, Y: 12, Button: tea.MouseLeft})

	assert.Empty(t, rp.got, "a release is swallowed while a dialog is modal")
}
