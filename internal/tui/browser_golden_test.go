//nolint:testpackage // white-box: shares the vt harness and drives the app with providermock sources
package tui

import (
	"context"
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

// browser goldens render at a two-pane size (≥110 columns) with enough height
// for the version history; diff goldens use the default shell size.
const (
	browserTermWidth  = 120
	browserTermHeight = 34
)

// captureUntil drives a model to its loaded state (waiting for marker to render),
// quits via `q`, and renders the SETTLED final model's screen at the given size.
//
// Like the staging/dialog helpers (#766), the golden is taken from the final
// View().Content — a single coherent full render of the settled model after the
// async load, the WindowSizeMsg, and the quit are all processed — not from the
// live teatest frame stream. Bubble Tea emits diff frames, and under CI's
// parallel -race those settle at timing-dependent points, so replaying the raw
// stream through the vt intermittently corrupts the final screen (#764).
func captureUntil(t *testing.T, model tea.Model, marker string, width, height int) string {
	t.Helper()

	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(width, height))

	waitFor(t, tm, marker)

	tm.Send(keyPress('q'))

	return settledScreen(t, tm, width, height)
}

// settledScreen waits for the program to finish, then renders the settled final
// model's full-screen View().Content through the vt at the given size. It works
// for any host that renders a tea.View (the browser *App and the diff/dialog
// hosts alike), so the browser and diff goldens share one deterministic settle.
func settledScreen(t *testing.T, tm *teatest.TestModel, width, height int) string {
	t.Helper()

	fm := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))

	vm, ok := fm.(interface{ View() tea.View })
	require.True(t, ok, "final model must render a tea.View")

	return renderVisibleScreenSize(t, []byte(vm.View().Content), width, height)
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

// TestBrowser_AWSParamValuesOnGolden pins #734: with values:on the list renders
// each entry's value on its OWN indented second line (mirroring the version
// history layout), so the value never collides with the right-aligned [staged]
// badge on the name line. And because values:on is an EXPLICIT reveal (GUI
// parity, the same policy as Compare/diff), a SecureString (secret) value is
// SHOWN in the preview rather than masked.
func TestBrowser_AWSParamValuesOnGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	probe := staticProbe{keys: map[data.StagedKey]struct{}{{Name: "/app/web/BASE_URL"}: {}}}

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamSource(), probe),
	})

	// Press `v` to turn values on; the list reloads with the value under each name.
	screen := captureBrowserKeys(t, m, "SecureString", 'v')

	require.Contains(t, screen, "postgres://db.internal:5432/app",
		"values:on reveals the SecureString value in the list preview (explicit reveal, GUI parity)")
	require.Contains(t, screen, "[staged]", "the staged badge still renders on the name line")
	golden.RequireEqual(t, screen)
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
	golden.RequireEqual(t, captureBrowserAfterKeys(t, m, "SecureString", keyEnterMsg()))
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
	screen := captureBrowserKeys(t, m, "SecureString", 'x')

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

	screen := captureBrowserAfterKeys(t, m, "Version ID", keyPress('t'))

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

	screen := captureUntil(t, awsParamStagedBannerApp(true, false), "staged value changes", browserTermWidth, browserTermHeight)

	require.Contains(t, screen, "⚠ staged value changes — S: staging", "value-only shows the value-change banner")
	require.NotContains(t, screen, "tag changes", "value-only must not mention tag changes")
	golden.RequireEqual(t, screen)
}

// TestBrowser_StagedTagBannerGolden pins the tag-only staged banner: the selected
// entry has a staged tag change and no value change.
func TestBrowser_StagedTagBannerGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureUntil(t, awsParamStagedBannerApp(false, true), "staged tag changes", browserTermWidth, browserTermHeight)

	require.Contains(t, screen, "⚠ staged tag changes — S: staging", "tag-only shows the tag-change banner")
	require.NotContains(t, screen, "value and tag", "tag-only must not use the combined wording")
	golden.RequireEqual(t, screen)
}

// TestBrowser_StagedValueAndTagBannerGolden pins the combined staged banner: the
// selected entry has both a staged value change and a staged tag change.
func TestBrowser_StagedValueAndTagBannerGolden(t *testing.T) { //nolint:paralleltest // goldenEnv sets NO_COLOR/TZ
	goldenEnv(t)

	screen := captureUntil(t, awsParamStagedBannerApp(true, true), "staged value and tag changes", browserTermWidth, browserTermHeight)

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

	screen := captureBrowserKeys(t, m, "SecureString", 'y')

	assert.Contains(t, screen, "copied (value stays masked)", "the copy status shows")
	assert.NotContains(t, screen, "postgres://db.internal:5432/app", "the copied secret is NOT revealed on screen")
	golden.RequireEqual(t, screen)
}

// TestBrowser_JSONFormattedGolden pins #732: a JSON value in the browser detail
// value pane is pretty-printed BY DEFAULT (parity with the GUI, which formats
// every JSON value) — no manual toggle needed.
func TestBrowser_JSONFormattedGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	m := newApp(config{
		scope:     provider.Scope{Provider: provider.ProviderAWS},
		identity:  awsIdentityFixture(),
		sourceFor: sourceForShape("param", awsParamJSONSource(), nil),
	})

	screen := captureUntil(t, m, "db.internal", browserTermWidth, browserTermHeight)

	assert.Contains(t, screen, `"host": "db.internal"`, "the JSON value is pretty-printed onto indented lines by default")
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

	screen := captureUntil(t, host, "diff:", goldenTermWidth, goldenTermHeight)

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

	golden.RequireEqual(t, captureUntil(t, m, marker, browserTermWidth, browserTermHeight))
}

// captureBrowserKeys drives a browser app to its loaded state (marker), sends the
// given rune keys, quits, and renders the SETTLED final model's screen — so a
// golden can capture the screen after a key toggle (copy, parse-json, values).
func captureBrowserKeys(t *testing.T, m *App, marker string, keys ...rune) string {
	t.Helper()

	presses := make([]tea.KeyPressMsg, len(keys))
	for i, k := range keys {
		presses[i] = keyPress(k)
	}

	return captureBrowserAfterKeys(t, m, marker, presses...)
}

// captureBrowserAfterKeys drives a browser app to its loaded state (waiting for
// marker), sends follow-up key presses (e.g. enter to focus the history or `v` to
// toggle values), quits, and renders the settled final model's screen.
func captureBrowserAfterKeys(t *testing.T, m *App, marker string, keys ...tea.KeyPressMsg) string {
	t.Helper()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(browserTermWidth, browserTermHeight))

	waitFor(t, tm, marker)

	for _, k := range keys {
		tm.Send(k)
	}

	tm.Send(keyPress('q'))

	return settledScreen(t, tm, browserTermWidth, browserTermHeight)
}

// diffGolden drives a diff host to its loaded state at the shell size and
// golden-compares the rendered visible screen.
func diffGolden(t *testing.T, host *diffHost, marker string) {
	t.Helper()

	golden.RequireEqual(t, captureUntil(t, host, marker, goldenTermWidth, goldenTermHeight))
}
