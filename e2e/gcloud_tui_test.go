//go:build e2e

//nolint:paralleltest // E2E tests run sequentially, not in parallel
package e2e_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/tui"
)

// =============================================================================
// TUI E2E Tests (Google Cloud / Secret Manager emulator)
//
// These tests drive the real TUI App — the same model internal/tui/run.go builds
// at launch, wired to registry-backed provider stores — through teatest against
// the gcloud secret-manager emulator (ghcr.io/blackwell-systems/
// gcp-secret-manager-emulator-dual). They exercise the full read/write data path
// (data source → usecase → provider store → emulator), not mocks, mirroring the
// AWS/localstack TUI foundation in aws_tui_test.go (#676) and the CLI gcloud e2e
// (gcloud_secret_test.go / gcloud_stage_test.go).
//
// Google Cloud offers a single service (Secret Manager), so the tab bar is
// [Secret Manager, Staging] — the Staging tab is at index 1 (jump key "2"),
// whereas the AWS suite reaches it at "3" behind param+secret.
//
// The tests share the TUI helpers from aws_tui_test.go (tuiTermWidth/Height,
// keyRune, waitForScreen, finalScreen) and the gcloud harness from the CLI e2e
// (setupGoogleCloud, runGcloud, newGoogleCloudStore, setupTempHome). Naming them
// TestGoogleCloudTUI_* folds them into the existing e2e-gcloud CI job, whose
// filter is `-run TestGoogleCloud` — no new job is needed.
// =============================================================================

// gcloudTUIProject is the project id setupGoogleCloud pins (GOOGLE_CLOUD_PROJECT)
// and the scope every gcloud TUI e2e launches with, so the TUI's registry-resolved
// stores, the staging bucket, and what the test seeds via `runGcloud` all line up.
const gcloudTUIProject = "suve-e2e"

// The Secret Manager secret names each gcloud TUI e2e seeds. Google Cloud secret
// ids are flat identifiers — no "/" (the adapter's List returns only the last
// "/"-segment as the display name, so a slash-bearing id would list under its leaf
// and its detail lookup would miss). Browse's alpha sorts before bravo so alpha is
// the default selection.
const (
	gcloudTUIBrowseAlpha = "suve-e2e-gcloud-tui-browse-alpha"
	gcloudTUIBrowseBravo = "suve-e2e-gcloud-tui-browse-bravo"
	gcloudTUIHistoryName = "suve-e2e-gcloud-tui-history-secret"
	gcloudTUIApplyName   = "suve-e2e-gcloud-tui-apply-secret"
)

// resetGoogleCloudTUISecrets deletes every secret the gcloud TUI suite may seed,
// giving each test a clean, single-secret-set list regardless of a sibling test's
// leftovers. Called at the START of each test (under the running test's live
// context, so the deletes actually commit) — a t.Cleanup delete can race the
// test-context cancellation, so the next test's list must not depend on it.
func resetGoogleCloudTUISecrets(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		gcloudTUIBrowseAlpha, gcloudTUIBrowseBravo, gcloudTUIHistoryName, gcloudTUIApplyName,
	} {
		_, _ = runGcloud(t, "secret", "delete", "--yes", name)
	}
}

// waitGcloudSecretReadable polls the CLI read path until the secret reads back the
// expected value. The TUI builds its OWN gcloud client, and the Secret Manager
// emulator does not guarantee a write committed by one client is immediately
// visible to another (nor a create immediately listable/versioned): launching the
// TUI straight after seeding can therefore race the emulator and surface a spurious
// "entry not found" under CI load. Gating on a fresh CLI read makes the seeded
// state visible before the TUI ever lists or reads it.
func waitGcloudSecretReadable(t *testing.T, name, want string) {
	t.Helper()

	deadline := time.Now().Add(tuiWaitTimeout)
	for time.Now().Before(deadline) {
		// Gate on BOTH read paths the TUI's detail load uses: the value
		// (AccessSecretVersion) AND the version list (ListSecretVersions). The
		// emulator can make the value readable before the version list is, so gating
		// on the value alone left the TUI's history load racing a not-yet-listable
		// secret ("failed to list secret versions: entry not found").
		out, valErr := runGcloud(t, "secret", "show", "--raw", name)
		_, logErr := runGcloud(t, "secret", "log", name)
		if valErr == nil && logErr == nil && strings.TrimSpace(out) == want {
			return
		}

		time.Sleep(50 * time.Millisecond) //nolint:mnd // poll interval
	}

	t.Fatalf("gcloud secret %q never became readable+listable as %q within %s", name, want, tuiWaitTimeout)
}

// newGoogleCloudTUIModel builds the real TUI model launched on the Secret Manager
// tab (Google Cloud's only service), wired to registry-backed stores pointed at
// the emulator (via the EmulatorEnvVar endpoint set by the CI job / mise task).
func newGoogleCloudTUIModel(t *testing.T) tea.Model {
	t.Helper()

	model, err := tui.NewE2EModel(t.Context(), provider.GoogleCloudScope(gcloudTUIProject), string(staging.ServiceSecret))
	require.NoError(t, err, "building the TUI model must succeed for a resolvable Google Cloud scope")

	return model
}

// TestGoogleCloudTUI_SecretBrowse seeds two Secret Manager secrets and drives the
// TUI secret browser: it lists both real secrets and, on the explicit-reveal
// surface (the `x` key, GUI parity), fetches and shows the selected secret's value
// from the emulator. The value is masked until that explicit reveal, and the
// unselected secret's value is never fetched (so it stays absent from the screen).
func TestGoogleCloudTUI_SecretBrowse(t *testing.T) {
	setupGoogleCloud(t)

	// alpha sorts before bravo, so alpha is the default selection whose value the
	// `x` reveal fetches.
	const (
		alphaName  = gcloudTUIBrowseAlpha
		bravoName  = gcloudTUIBrowseBravo
		alphaValue = "gcloud-alpha-value-1"
		bravoValue = "gcloud-bravo-value-2"
	)

	resetGoogleCloudTUISecrets(t)
	t.Cleanup(func() { resetGoogleCloudTUISecrets(t) })

	_, err := runGcloud(t, "secret", "create", alphaName, alphaValue)
	require.NoError(t, err)
	_, err = runGcloud(t, "secret", "create", bravoName, bravoValue)
	require.NoError(t, err)

	// Ensure both seeded secrets are visible to a fresh client before the TUI (its
	// own client) launches and lists them.
	waitGcloudSecretReadable(t, alphaName, alphaValue)
	waitGcloudSecretReadable(t, bravoName, bravoValue)

	tm := teatest.NewTestModel(t, newGoogleCloudTUIModel(t),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Gate on the async list landing, then explicitly reveal the value with `x`
	// and wait for the async fetch+reveal to render the value before capturing —
	// the reveal triggers a fresh emulator read, so gating on the list alone
	// would race the masked → revealed transition.
	waitForScreen(t, tm, alphaName)
	tm.Send(keyRune('x'))
	waitForScreen(t, tm, alphaValue)

	screen := finalScreen(t, tm)

	assert.Contains(t, screen, alphaName, "the secret browser lists the first seeded secret")
	assert.Contains(t, screen, bravoName, "the secret browser lists the second seeded secret")
	assert.Contains(t, screen, alphaValue,
		"pressing x explicitly reveals the selected secret's value fetched from the emulator")
	assert.NotContains(t, screen, bravoValue,
		"the unselected secret's value is never fetched, so it stays masked/absent")
}

// TestGoogleCloudTUI_SecretHistory seeds one secret with three versions and drives
// the TUI secret browser's detail history: it asserts the version history renders
// (the current-version marker plus the Version ID meta), that every per-version
// value is masked by default, and that pressing `x` reveals the current value and
// all history values together (one shared Show toggle, GUI parity, #733). The
// history fetch is capped at 10 rows (#747); three versions render in full.
func TestGoogleCloudTUI_SecretHistory(t *testing.T) {
	setupGoogleCloud(t)

	const (
		name  = gcloudTUIHistoryName
		v1Val = "gcloud-history-value-1"
		v2Val = "gcloud-history-value-2"
		v3Val = "gcloud-history-value-3"
	)

	resetGoogleCloudTUISecrets(t)
	t.Cleanup(func() { resetGoogleCloudTUISecrets(t) })

	// Seed three versions: create (v1) + two updates (v2, v3). v3 is current.
	_, err := runGcloud(t, "secret", "create", name, v1Val)
	require.NoError(t, err)
	_, err = runGcloud(t, "secret", "update", "--yes", name, v2Val)
	require.NoError(t, err)
	_, err = runGcloud(t, "secret", "update", "--yes", name, v3Val)
	require.NoError(t, err)

	// Ensure the final seeded version is visible to a fresh client before the TUI
	// (its own client) launches and lists the secret's versions.
	waitGcloudSecretReadable(t, name, v3Val)

	// Masked capture: drive to the loaded detail/history (the current marker only
	// renders once the history lands), then read the settled screen without any
	// reveal — no per-version value must be visible.
	masked := func() string {
		tm := teatest.NewTestModel(t, newGoogleCloudTUIModel(t),
			teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))
		waitForScreen(t, tm, "current")

		return finalScreen(t, tm)
	}()

	assert.Contains(t, masked, name, "the secret browser lists the seeded secret")
	assert.Contains(t, masked, "Version ID", "the detail pane shows the current version's id")
	assert.Contains(t, masked, "current", "the version history renders with the current-version marker")
	assert.NotContains(t, masked, v1Val, "history values are masked until an explicit reveal")
	assert.NotContains(t, masked, v2Val, "history values are masked until an explicit reveal")
	assert.NotContains(t, masked, v3Val, "the current value is masked until an explicit reveal")

	// Revealed capture: the same load, then `x` reveals the current value AND every
	// history value together (GUI parity, #733).
	revealed := func() string {
		tm := teatest.NewTestModel(t, newGoogleCloudTUIModel(t),
			teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))
		waitForScreen(t, tm, "current")
		tm.Send(keyRune('x'))
		// Wait for the reveal's async fetch to render the deepest (oldest) history
		// value: v1 rendering proves the whole reveal — current value plus every
		// history row — landed before we capture, not just the list/detail frame.
		waitForScreen(t, tm, v1Val)

		return finalScreen(t, tm)
	}()

	assert.Contains(t, revealed, v3Val, "x reveals the current version's value")
	assert.Contains(t, revealed, v2Val, "x reveals the second version's value in the history")
	assert.Contains(t, revealed, v1Val, "x reveals the first version's value in the history")
}

// TestGoogleCloudTUI_StageApply exercises the browse → stage → apply loop
// end-to-end: a secret is pre-staged for update through the shared staging store,
// then the TUI applies it via the Staging tab's apply-all dialog. The applied
// value is verified through the CLI `secret show` path, proving the write reached
// the emulator through the real TUI apply path.
func TestGoogleCloudTUI_StageApply(t *testing.T) {
	setupGoogleCloud(t)
	setupTempHome(t)

	// The staging store is keychain-encrypted; pin a deterministic data key so the
	// pre-stage store and the TUI's staging store agree without an OS keychain
	// (matches compose.test.yaml / the workflow-level SUVE_STAGING_KEY).
	t.Setenv("SUVE_STAGING_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))

	const (
		name        = gcloudTUIApplyName
		originalVal = "gcloud-original-value"
		stagedVal   = "gcloud-applied-via-tui"
	)

	resetGoogleCloudTUISecrets(t)
	t.Cleanup(func() { resetGoogleCloudTUISecrets(t) })

	_, err := runGcloud(t, "secret", "create", name, originalVal)
	require.NoError(t, err)

	// Pre-stage an update through the same staging store the TUI reads (matched
	// scope), mirroring the CLI staging e2e's seeding.
	store := newGoogleCloudStore(gcloudTUIProject)
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret,
		staging.EntryKey{Name: name}, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr(stagedVal),
			StagedAt:  time.Now(),
		}))

	tm := teatest.NewTestModel(t, newGoogleCloudTUIModel(t),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Jump to the Staging tab (global key "2": [Secret Manager, Staging]) and wait
	// for the staged entry.
	tm.Send(keyRune('2'))
	waitForScreen(t, tm, name)

	// Open the apply-all confirmation dialog ("A"), then focus the Apply action
	// (Down) and confirm (Enter) — the same choreography the AWS TUI apply drives.
	tm.Send(keyRune('A'))
	waitForScreen(t, tm, "Ignore conflicts")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	// The results view reports the applied entry — the apply has committed to the
	// emulator by the time this status renders.
	waitForScreen(t, tm, "updated")

	// Quit the program directly: the results dialog captures input first (`q` would
	// go to the dialog, not the shell), and the assertion below is the CLI-verified
	// side effect, not a final screen.
	require.NoError(t, tm.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	// The staged update must have reached the emulator through the TUI apply path.
	stdout, err := runGcloud(t, "secret", "show", "--raw", name)
	require.NoError(t, err)
	assert.Equal(t, stagedVal, stdout, "the TUI apply wrote the staged value to the emulator")
}
