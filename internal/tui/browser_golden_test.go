//nolint:testpackage // white-box: shares the vt harness and drives the app with providermock sources
package tui

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/timeutil"
	"github.com/mpyw/suve/internal/tui/data"
	"github.com/mpyw/suve/internal/tui/keys"
	"github.com/mpyw/suve/internal/tui/nav"
	"github.com/mpyw/suve/internal/tui/pages/diff"
	"github.com/mpyw/suve/internal/tui/styles"
)

// goldenEnv sets the deterministic environment every browser/diff golden needs:
// NO_COLOR strips ANSI so the golden is plain text (and can never carry a color
// escape), and TZ=UTC pins date columns.
func goldenEnv(t *testing.T) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TZ", "UTC")
	timeutil.ResetLocationCache()
}

// captureUntil runs a model through teatest, accumulating ALL output into a
// private buffer until marker appears (so it never races FinalOutput's shared
// reader, per the harness note), then quits and returns the full stream. The
// final visible screen is the last frame — the loaded state — because the quit
// is sent only after the marker proves the async loads landed.
// browser goldens render at a two-pane size (≥110 columns) with enough height
// for the version history; diff goldens use the default shell size.
const (
	browserTermWidth  = 120
	browserTermHeight = 34
)

func captureUntil(t *testing.T, model tea.Model, marker string, width, height int) []byte {
	t.Helper()

	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(width, height))

	var buf bytes.Buffer

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = io.Copy(&buf, tm.Output())
		if bytes.Contains(buf.Bytes(), []byte(marker)) {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Contains(t, buf.String(), marker, "loaded content never rendered")

	tm.Send(keyPress('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	_, _ = io.Copy(&buf, tm.Output())

	return buf.Bytes()
}

// TestBrowser_AWSParamGolden renders the AWS param browser (masked SecureString
// value, version history, a [staged] badge + banner from a static probe).
func TestBrowser_AWSParamGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	probe := staticProbe{keys: map[data.StagedKey]struct{}{{Name: "/app/web/BASE_URL"}: {}}}

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), probe),
	})

	browserGolden(t, m, "SecureString")
}

// TestBrowser_AWSParamHistoryFocusGolden renders the AWS param browser after
// pressing enter to move focus into the version history (#685): the history pane
// carries the active selection cursor (▸) and the `esc: list` hint, while the
// list drops to the dimmed cursor (▹) — so the focused pane is unambiguous.
func TestBrowser_AWSParamHistoryFocusGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	probe := staticProbe{keys: map[data.StagedKey]struct{}{{Name: "/app/web/BASE_URL"}: {}}}

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), probe),
	})

	// Drive to the loaded state, then press enter to focus the history pane.
	raw := captureBrowserAfterKeys(t, m, "SecureString", keyEnterMsg())
	golden.RequireEqual(t, renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight))
}

// TestBrowser_AWSParamHistoryValueRevealGolden pins #733: each version row shows
// its value, masked by default; pressing `x` reveals the current value AND the
// history values together (one shared Show toggle, GUI parity). The fixture's
// SecureString value is a secret on the value-type axis, so it is masked until
// revealed.
func TestBrowser_AWSParamHistoryValueRevealGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), nil),
	})

	// The default masked screen shows bullets in the history; press `x` to reveal.
	raw := captureBrowserKeys(t, m, "SecureString", 'x')
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	require.Contains(t, screen, "postgres://db.internal:5432/app", "x reveals the current and history values")
	golden.RequireEqual(t, screen)
}

// TestBrowser_DeleteStagedGateStatusGolden pins #692: on an entry staged for
// deletion, pressing `t` (edit/delete/tag are all dead-end transitions there)
// does not open the tag dialog but surfaces a one-line status message instead —
// matching the GUI, which hides those controls. The delete-staged entry is the
// default selection (index 0), so the gate fires on the first `t`.
func TestBrowser_DeleteStagedGateStatusGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	doomed := data.StagedKey{Name: "prod/api/key"}
	probe := staticProbe{
		keys:       map[data.StagedKey]struct{}{doomed: {}},
		deleteKeys: map[data.StagedKey]struct{}{doomed: {}},
		entryCount: 1,
	}

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		service:   "secret",
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("secret", awsSecretSource(), probe),
	})

	raw := captureBrowserAfterKeys(t, m, "Version ID", keyPress('t'))
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	require.Contains(t, screen, "cannot tag: staged for deletion", "the gate surfaces a status message rather than the tag dialog")
	golden.RequireEqual(t, screen)
}

// The default selection is index 0 (/app/api/DATABASE_URL); staging that entry
// makes its detail-pane banner render, so the three staged-kind goldens below
// pin the value-only / tag-only / both wording (#701).
//
//nolint:gochecknoglobals // immutable test fixture
var stagedSelectedKey = data.StagedKey{Name: "/app/api/DATABASE_URL"}

// awsParamStagedBannerApp builds the AWS param browser with the default-selected
// entry staged for the given change kinds.
func awsParamStagedBannerApp(entry, tags bool) *App {
	keySet := map[data.StagedKey]struct{}{stagedSelectedKey: {}}
	probe := staticProbe{keys: keySet}

	if entry {
		probe.entryKeys = keySet
	}

	if tags {
		probe.tagKeys = keySet
	}

	return newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), probe),
	})
}

// TestBrowser_StagedValueBannerGolden pins the value-only staged banner: the
// selected entry has a staged value change and no tag change.
func TestBrowser_StagedValueBannerGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	raw := captureUntil(t, awsParamStagedBannerApp(true, false), "staged value changes", browserTermWidth, browserTermHeight)
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	require.Contains(t, screen, "⚠ staged value changes — S: staging", "value-only shows the value-change banner")
	require.NotContains(t, screen, "tag changes", "value-only must not mention tag changes")
	golden.RequireEqual(t, screen)
}

// TestBrowser_StagedTagBannerGolden pins the tag-only staged banner: the selected
// entry has a staged tag change and no value change.
func TestBrowser_StagedTagBannerGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	raw := captureUntil(t, awsParamStagedBannerApp(false, true), "staged tag changes", browserTermWidth, browserTermHeight)
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	require.Contains(t, screen, "⚠ staged tag changes — S: staging", "tag-only shows the tag-change banner")
	require.NotContains(t, screen, "value and tag", "tag-only must not use the combined wording")
	golden.RequireEqual(t, screen)
}

// TestBrowser_StagedValueAndTagBannerGolden pins the combined staged banner: the
// selected entry has both a staged value change and a staged tag change.
func TestBrowser_StagedValueAndTagBannerGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	raw := captureUntil(t, awsParamStagedBannerApp(true, true), "staged value and tag changes", browserTermWidth, browserTermHeight)
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	require.Contains(t, screen, "⚠ staged value and tag changes — S: staging", "both shows the combined banner")
	golden.RequireEqual(t, screen)
}

// TestBrowser_CopyKeepsMaskGolden pins #689: after `y` copies the masked
// SecureString value, the detail value pane stays MASKED (bullets, no plaintext)
// and a transient "copied (value stays masked)" status shows — a copy is never a
// standing on-screen disclosure.
func TestBrowser_CopyKeepsMaskGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), nil),
	})

	raw := captureBrowserKeys(t, m, "SecureString", 'y')
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	assert.Contains(t, screen, "copied (value stays masked)", "the copy status shows")
	assert.NotContains(t, screen, "postgres://db.internal:5432/app", "the copied secret is NOT revealed on screen")
	golden.RequireEqual(t, screen)
}

// TestBrowser_ParseJSONGolden pins #690: `J` pretty-prints a JSON value in the
// browser detail value pane (parity with the diff page and the GUI).
func TestBrowser_ParseJSONGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamJSONSource(), nil),
	})

	raw := captureBrowserKeys(t, m, "db.internal", 'J')
	screen := renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight)

	assert.Contains(t, screen, `"host": "db.internal"`, "J pretty-prints the JSON value onto indented lines")
	golden.RequireEqual(t, screen)
}

// TestBrowser_AWSSecretGolden renders the AWS secret browser (staging labels +
// ARN, masked value).
func TestBrowser_AWSSecretGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		service:   "secret",
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("secret", awsSecretSource(), nil),
	})

	browserGolden(t, m, "Version ID")
}

// TestBrowser_GCloudSecretGolden renders the Google Cloud secret browser
// (per-version State).
func TestBrowser_GCloudSecretGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.GoogleCloudScope("my-project"),
		sourceFor: sourceForShape("secret", gcloudSecretSource(), nil),
	})

	browserGolden(t, m, "enabled")
}

// TestBrowser_AzureKVGolden renders the Azure Key Vault browser (per-version
// tags inline in history).
func TestBrowser_AzureKVGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.AzureKeyVaultScope("myvault"),
		sourceFor: sourceForShape("secret", azureKVSource(), nil),
	})

	browserGolden(t, m, "rotation")
}

// TestBrowser_AzureAppConfigGolden renders the Azure App Configuration browser
// (namespace badges, no history/version meta).
func TestBrowser_AzureAppConfigGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.AzureAppConfigScope("myapp-config"),
		sourceFor: sourceForShape("param", azureAppConfigSource(), nil),
	})

	browserGolden(t, m, "Namespace")
}

// ---------------------------------------------------------------------------
// Diff page goldens (versioned shapes; App Configuration has no history/diff)
// ---------------------------------------------------------------------------

// diffHost hosts a diff page full-screen for a golden, quitting on `q` or a
// PopPage request.
type diffHost struct {
	m *diff.Model
	w int
	h int
}

func newDiffHost(req nav.OpenDiff) *diffHost {
	return &diffHost{m: diff.New(context.Background(), req, styles.New(), keys.Default())}
}

func (h *diffHost) Init() tea.Cmd { return h.m.Init() }

func (h *diffHost) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.w, h.h = msg.Width, msg.Height
		m, cmd := h.m.Update(msg)
		h.m = m

		return h, cmd
	case tea.KeyPressMsg:
		if msg.Text == "q" {
			return h, tea.Quit
		}

		m, cmd := h.m.Update(msg)
		h.m = m

		return h, cmd
	case nav.PopPage:
		return h, tea.Quit
	default:
		m, cmd := h.m.Update(msg)
		h.m = m

		return h, cmd
	}
}

func (h *diffHost) View() tea.View {
	v := tea.NewView(h.m.View(h.w, h.h))
	v.AltScreen = true

	return v
}

// diffReq builds an OpenDiff request for a shape. Secret-ness is not carried
// here — the diff page learns it from the source's DiffContent.
func diffReq(src data.Source, name, oldV, newV string) nav.OpenDiff {
	return nav.OpenDiff{Source: src, Name: name, OldVersion: oldV, NewVersion: newV}
}

func TestDiff_AWSParamGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	host := newDiffHost(diffReq(awsParamDiffSource(), "/app/api/DATABASE_URL", "13", "14"))

	diffGolden(t, host, "@@")
}

// TestDiff_AWSParamSecureStringGolden pins a SecureString PARAM diff: the
// Compare/diff view is a surface the user explicitly opened to inspect the
// change, so its values are REVEALED by default (#702/#735) — a SecureString is
// a secret on the value-type axis, so `x` can still hide it, but the default
// shows the real +/- change. The fixture's cleartext values appear in the golden.
func TestDiff_AWSParamSecureStringGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	host := newDiffHost(diffReq(awsParamSecureStringDiffSource(), "/app/api/DATABASE_URL", "13", "14"))

	raw := captureUntil(t, host, "diff:", goldenTermWidth, goldenTermHeight)
	screen := renderVisibleScreenSize(t, raw, goldenTermWidth, goldenTermHeight)

	require.Contains(t, screen, secureStringDiffValue, "the SecureString diff is revealed by default (#702/#735)")
	require.Contains(t, screen, secureStringDiffOldValue, "both SecureString versions are shown")
	golden.RequireEqual(t, screen)
}

func TestDiff_AWSSecretGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	host := newDiffHost(diffReq(awsSecretSource(), "prod/api/key", "e5f6a7b8-9999-8888-7777-666655554444", "a1b2c3d4-1111-2222-3333-444455556666"))

	diffGolden(t, host, "diff:")
}

// TestDiff_GCloudSecretGolden pins a SECRET diff: the Compare/diff view is
// explicitly opened to inspect the change, so the two differing versions are
// REVEALED by default (#735) — the real +/- content shows (the `x` toggle can
// still hide it).
func TestDiff_GCloudSecretGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	host := newDiffHost(diffReq(gcloudSecretSource(), "api-key", "2", "3"))

	diffGolden(t, host, "diff:")
}

func TestDiff_AzureKVGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	host := newDiffHost(diffReq(azureKVSource(), "vault-secret", "4c3b2a1908f7", "9f8e7d6c5b4a"))

	diffGolden(t, host, "diff:")
}

// browserGolden drives a browser app to its loaded state at the two-pane size
// and golden-compares the rendered visible screen.
func browserGolden(t *testing.T, m *App, marker string) {
	t.Helper()

	raw := captureUntil(t, m, marker, browserTermWidth, browserTermHeight)
	golden.RequireEqual(t, renderVisibleScreenSize(t, raw, browserTermWidth, browserTermHeight))
}

// captureBrowserKeys drives a browser app to its loaded state (marker), then
// sends keys (each followed by a short settle) before quitting — so a golden can
// capture the screen after a key toggle (copy, parse-json).
func captureBrowserKeys(t *testing.T, m *App, marker string, keys ...rune) []byte {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(browserTermWidth, browserTermHeight))

	var buf bytes.Buffer

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = io.Copy(&buf, tm.Output())
		if bytes.Contains(buf.Bytes(), []byte(marker)) {
			break
		}

		time.Sleep(20 * time.Millisecond)
	}

	require.Contains(t, buf.String(), marker, "loaded content never rendered")

	for _, k := range keys {
		tm.Send(keyPress(k))
		time.Sleep(100 * time.Millisecond)

		_, _ = io.Copy(&buf, tm.Output())
	}

	tm.Send(keyPress('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	_, _ = io.Copy(&buf, tm.Output())

	return buf.Bytes()
}

// captureBrowserAfterKeys drives a browser app to its loaded state (waiting for
// marker), sends follow-up key presses (e.g. enter to focus the history), lets
// the frame settle, then quits and returns the full captured stream.
func captureBrowserAfterKeys(t *testing.T, m *App, marker string, keys ...tea.KeyPressMsg) []byte {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(browserTermWidth, browserTermHeight))

	var buf bytes.Buffer

	waitFor(t, tm, &buf, marker)

	for _, k := range keys {
		tm.Send(k)
		time.Sleep(100 * time.Millisecond)

		_, _ = io.Copy(&buf, tm.Output())
	}

	tm.Send(keyPress('q'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	_, _ = io.Copy(&buf, tm.Output())

	return buf.Bytes()
}

// diffGolden drives a diff host to its loaded state at the shell size and
// golden-compares the rendered visible screen.
func diffGolden(t *testing.T, host *diffHost, marker string) {
	t.Helper()

	raw := captureUntil(t, host, marker, goldenTermWidth, goldenTermHeight)
	golden.RequireEqual(t, renderVisibleScreenSize(t, raw, goldenTermWidth, goldenTermHeight))
}
