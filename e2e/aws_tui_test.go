//go:build e2e

//nolint:paralleltest,dogsled,gosec // E2E tests: sequential execution, ignored cleanup output, G101 false positive
package e2e_test

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmdparam "github.com/mpyw/suve/internal/cli/commands/aws/param"
	paramcreate "github.com/mpyw/suve/internal/cli/commands/aws/param/create"
	paramdelete "github.com/mpyw/suve/internal/cli/commands/aws/param/delete"
	secretcreate "github.com/mpyw/suve/internal/cli/commands/aws/secret/create"
	secretdelete "github.com/mpyw/suve/internal/cli/commands/aws/secret/delete"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/tui"
)

// =============================================================================
// TUI E2E Tests (AWS / localstack)
//
// These tests drive the real TUI App — the same model internal/tui/run.go
// builds at launch, wired to registry-backed provider stores — through teatest
// against localstack. They exercise the full read/write data path
// (data source → usecase → provider store → emulator), not mocks, mirroring how
// the CLI e2e suites build real stores. This is the AWS/localstack foundation
// for #676; Google Cloud and Azure TUI e2e are left as follow-ups.
//
// The suite reuses the AWS localstack harness from aws_test.go (setupEnv,
// newStore) and the closed-network compose runner from compose.test.yaml, which
// sets SUVE_STAGING_KEY so the staging store never blocks on an OS keychain.
// =============================================================================

// tuiTermWidth/tuiTermHeight match the TUI's own two-pane golden size: the
// browser needs ≥110 columns to render the list and detail panes side by side.
const (
	tuiTermWidth  = 120
	tuiTermHeight = 34

	// tuiWaitTimeout bounds each async gate. Localstack round-trips (list, get,
	// apply) go over the compose network, so it is generous relative to the
	// in-process unit teatest waits.
	tuiWaitTimeout = 15 * time.Second
)

// awsTUIScope is the localstack AWS scope every TUI e2e launches with. It
// matches newStore()'s scope (account 000000000000, region us-east-1) so the
// TUI's registry-resolved stores and staging bucket line up with what the test
// seeds via the CLI and the staging store.
func awsTUIScope() provider.Scope {
	return provider.AWSScope("000000000000", "us-east-1")
}

// newTUIModel builds the real TUI model for the launched service tab, wired to
// registry-backed stores pointed at localstack (via setupEnv's AWS_ENDPOINT_URL).
func newTUIModel(t *testing.T, service string) tea.Model {
	t.Helper()

	model, err := tui.NewE2EModel(t.Context(), awsTUIScope(), service)
	require.NoError(t, err, "building the TUI model must succeed for a resolvable AWS scope")

	return model
}

// keyRune builds a printable key press (matches the tui package's own helper).
func keyRune(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// waitForScreen gates on marker appearing in the RENDERED visible screen, so
// navigation keys are only sent once the awaited async frame has rendered. It
// renders the accumulated output through a cell-grid emulator rather than matching
// the raw stream: CI emits incremental cell-updates that can split a marker across
// cursor-positioned writes, so a raw bytes.Contains misses text the user plainly
// sees (the same flake the internal/tui waitFor fixes).
func waitForScreen(t *testing.T, tm *teatest.TestModel, marker string) {
	t.Helper()

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(renderTUIScreen(t, b), marker)
	}, teatest.WithDuration(tuiWaitTimeout), teatest.WithCheckInterval(50*time.Millisecond))
}

// finalScreen quits the program and returns the settled final model's screen,
// rendered through the cell-grid emulator so the returned string is the plain
// visible screen (no interspersed styling that could split a marker mid-phrase).
// Rendering the settled View — not the live diff-frame stream — is what makes the
// assertion deterministic (the tui package's golden harness uses the same
// discipline for the same reason).
func finalScreen(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()

	tm.Send(keyRune('q'))

	fm := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second))

	vm, ok := fm.(interface{ View() tea.View })
	require.True(t, ok, "final model must render a tea.View")

	return renderTUIScreen(t, []byte(vm.View().Content))
}

// tuiRetryBudget/tuiAttemptWait bound the fresh-client retry: the overall time a
// retried TUI interaction may take, and how long each attempt waits for a marker.
const (
	tuiRetryBudget = 30 * time.Second
	tuiAttemptWait = 5 * time.Second
)

// awaitScreen is a non-fatal waitForScreen for retry loops: it drains the live
// output and reports whether marker rendered within timeout (false on timeout,
// never t.Fatal), matching against the vt-rendered screen like waitForScreen.
func awaitScreen(t *testing.T, tm *teatest.TestModel, marker string, timeout time.Duration) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)

	var buf bytes.Buffer

	for time.Now().Before(deadline) {
		_, _ = io.Copy(&buf, tm.Output())
		if strings.Contains(renderTUIScreen(t, buf.Bytes()), marker) {
			return true
		}

		time.Sleep(50 * time.Millisecond) //nolint:mnd // poll interval
	}

	return false
}

// retryTUIScreen builds a fresh TUI model and drives it, rebuilding (a fresh
// provider client) until drive reports the awaited content rendered — then returns
// the settled final screen. The emulators do not guarantee a write from one client
// is visible to a brand-new client immediately, so the first launch can read
// stale/empty state ("entry not found") under CI load; relaunching with a fresh
// client eventually sees it. drive must use awaitScreen (non-fatal) and do its own
// key sends; it must not quit the model.
func retryTUIScreen(t *testing.T, build func() tea.Model, drive func(tm *teatest.TestModel) bool) string {
	t.Helper()

	deadline := time.Now().Add(tuiRetryBudget)

	for attempt := 1; ; attempt++ {
		tm := teatest.NewTestModel(t, build(), teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))
		if drive(tm) {
			return finalScreen(t, tm)
		}

		tm.Send(keyRune('q'))
		tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second)) //nolint:mnd // teardown wait

		if time.Now().After(deadline) {
			t.Fatalf("TUI content never rendered after %d attempt(s) within %s", attempt, tuiRetryBudget)
		}

		time.Sleep(200 * time.Millisecond) //nolint:mnd // brief backoff before a fresh client
	}
}

// TestTUIAWS_ParamBrowse seeds two SSM parameters and drives the TUI param
// browser: it launches, lists the real entries, and shows the selected entry's
// detail (name, value, type) over the real read path. The seeded params are
// String type — plaintext on the value-type axis — so their values render
// unmasked in the detail pane (no masking-policy concern).
func TestTUIAWS_ParamBrowse(t *testing.T) {
	setupEnv(t)

	const (
		alphaName  = "/suve-e2e-tui/alpha"
		bravoName  = "/suve-e2e-tui/bravo"
		alphaValue = "alpha-value-1"
		bravoValue = "bravo-value-2"
	)

	for _, name := range []string{alphaName, bravoName} {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", name)
	}

	t.Cleanup(func() {
		for _, name := range []string{alphaName, bravoName} {
			_, _, _ = runCommand(t, paramdelete.Command(), "--yes", name)
		}
	})

	_, _, err := runCommand(t, paramcreate.Command(), alphaName, alphaValue)
	require.NoError(t, err)
	_, _, err = runCommand(t, paramcreate.Command(), bravoName, bravoValue)
	require.NoError(t, err)

	tm := teatest.NewTestModel(t, newTUIModel(t, string(staging.ServiceParam)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Gate on the detail pane's async Show completing by waiting for the selected
	// entry's value — not just the list name. The detail loads AFTER the list, so
	// the value appearing proves both the list read and the async detail read
	// landed before we quit, and the assertion never races the "select an entry"
	// placeholder frame. A single gate (not list-then-value) is deliberate:
	// waitForScreen consumes the output buffer, so when the list+detail frames
	// batch a first wait on the name would swallow the value and a second wait
	// would block forever.
	waitForScreen(t, tm, alphaValue)

	screen := finalScreen(t, tm)

	assert.Contains(t, screen, alphaName, "the param browser lists the first seeded entry")
	assert.Contains(t, screen, bravoName, "the param browser lists the second seeded entry")
	assert.Contains(t, screen, alphaValue,
		"the detail pane shows the selected String param's value over the real read path")
	assert.Contains(t, screen, "String", "the detail pane shows the entry's value type")
}

// TestTUIAWS_SecretBrowse seeds a Secrets Manager secret and drives the TUI
// secret browser: it lists the real secret and, on the explicit-reveal surface
// (the `x` key, GUI parity), fetches and shows the secret value from localstack.
// The value is masked until that explicit reveal.
func TestTUIAWS_SecretBrowse(t *testing.T) {
	setupEnv(t)

	const (
		secretName  = "suve-e2e-tui/token"
		secretValue = "s3cr3t-token-value"
	)

	_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, secretdelete.Command(), "--yes", "--force", secretName)
	})

	_, _, err := runCommand(t, secretcreate.Command(), secretName, secretValue)
	require.NoError(t, err)

	tm := teatest.NewTestModel(t, newTUIModel(t, string(staging.ServiceSecret)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Gate on the async list landing, then explicitly reveal the value with `x`
	// and wait for the async fetch+reveal to render the value before capturing —
	// the reveal triggers a fresh emulator read, so gating on the list alone
	// would race the masked → revealed transition.
	waitForScreen(t, tm, secretName)
	tm.Send(keyRune('x'))
	waitForScreen(t, tm, secretValue)

	screen := finalScreen(t, tm)

	assert.Contains(t, screen, secretName, "the secret browser lists the seeded secret")
	assert.Contains(t, screen, secretValue,
		"pressing x explicitly reveals the secret value fetched from the emulator")
}

// TestTUIAWS_StageApply exercises the browse → stage → apply loop end-to-end: a
// param is pre-staged for update through the shared staging store, then the TUI
// applies it via the Staging tab's apply-all dialog. The applied value is
// verified through the CLI show path, proving the write reached localstack
// through the real TUI apply path.
func TestTUIAWS_StageApply(t *testing.T) {
	setupEnv(t)
	setupTempHome(t)

	const (
		paramName   = "/suve-e2e-tui-apply/param"
		originalVal = "original-value"
		stagedVal   = "applied-via-tui"
	)

	_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	t.Cleanup(func() {
		_, _, _ = runCommand(t, paramdelete.Command(), "--yes", paramName)
	})

	_, _, err := runCommand(t, paramcreate.Command(), paramName, originalVal)
	require.NoError(t, err)

	// Pre-stage an update through the same staging store the TUI reads (matched
	// scope), mirroring the CLI staging e2e's seeding.
	store := newStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam,
		staging.EntryKey{Name: paramName}, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr(stagedVal),
			StagedAt:  time.Now(),
		}))

	tm := teatest.NewTestModel(t, newTUIModel(t, string(staging.ServiceParam)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Jump to the Staging tab (global key "3") and wait for the staged entry.
	tm.Send(keyRune('3'))
	waitForScreen(t, tm, paramName)

	// Open the apply-all confirmation dialog ("A"), then focus the Apply action
	// (Down) and confirm (Enter) — the same choreography the apply-dialog golden
	// drives.
	tm.Send(keyRune('A'))
	waitForScreen(t, tm, "Ignore conflicts")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	// The results view reports the applied entry — the apply has committed to the
	// emulator by the time this status renders.
	waitForScreen(t, tm, "updated")

	// Quit the program directly: the results dialog captures input first (`q`
	// would go to the dialog, not the shell), and the assertion below is the
	// CLI-verified side effect, not a final screen.
	require.NoError(t, tm.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	// The staged update must have reached localstack through the TUI apply path.
	stdout, _, err := runCommand(t, cmdparam.ShowCommand(), "--raw", paramName)
	require.NoError(t, err)
	assert.Equal(t, stagedVal, stdout, "the TUI apply wrote the staged value to the emulator")
}
