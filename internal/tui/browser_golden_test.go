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

func TestDiff_AWSSecretGolden(t *testing.T) { //nolint:paralleltest // goldenEnv calls t.Setenv (NO_COLOR/TZ), which forbids t.Parallel
	goldenEnv(t)

	host := newDiffHost(diffReq(awsSecretSource(), "prod/api/key", "e5f6a7b8-9999-8888-7777-666655554444", "a1b2c3d4-1111-2222-3333-444455556666"))

	diffGolden(t, host, "diff:")
}

// TestDiff_GCloudSecretGolden pins a SECRET diff: the two versions differ, so the
// diff renders +/- lines — and every one is a run of mask bullets, never a
// revealed secret value (the fixture values, e.g. "googlecloud-secret-value-…",
// must not appear in the golden).
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

// diffGolden drives a diff host to its loaded state at the shell size and
// golden-compares the rendered visible screen.
func diffGolden(t *testing.T, host *diffHost, marker string) {
	t.Helper()

	raw := captureUntil(t, host, marker, goldenTermWidth, goldenTermHeight)
	golden.RequireEqual(t, renderVisibleScreenSize(t, raw, goldenTermWidth, goldenTermHeight))
}
