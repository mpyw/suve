//go:build e2e

//nolint:paralleltest // E2E tests run sequentially, not in parallel
package e2e_test

import (
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
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/tui"
)

// =============================================================================
// TUI E2E Tests (Azure / App Configuration + Key Vault emulators)
//
// Follow-up to #676 (the AWS/localstack TUI foundation in aws_tui_test.go).
// These tests drive the real TUI App — the same model internal/tui/run.go builds
// at launch, wired to registry-backed provider stores — through teatest against
// the Azure emulators, exercising the full read/write data path (data source →
// usecase → provider store → emulator), not mocks.
//
// Azure is the most bug-prone provider surface (two independent services + a
// namespace axis + per-version tags), so the coverage is deliberately thorough:
//
//   - Key Vault (secret): browse + explicit reveal, version history WITH
//     per-version tags (the TagsPerVersion axis), version-to-version diff, and a
//     browse → stage → apply loop over both a value update and a tag change.
//   - App Configuration (param, namespaced): browse across ≥2 namespaces with the
//     namespace badge/partitioning assertions (the EntryKey{Name,Namespace} axis),
//     a namespaced entry's detail, capability-gated hiding of the (absent) version
//     history, and a namespace-scoped stage → apply that must land under the RIGHT
//     namespace.
//
// The suite reuses the shared TUI teatest harness from aws_tui_test.go
// (tuiTermWidth/Height, tuiWaitTimeout, keyRune, waitForScreen, finalScreen) and
// the Azure CLI harness from azure_{keyvault,appconfig,stage}_test.go
// (setupAzureKeyVault/AppConfig, runAzureSecret/Param, setAzureKeyVaultStagingKey).
// The scope passed to NewE2EModel is the same globally-unique-name scope the CLI
// staging path keys on (vault/store name "suve-e2e"), so the TUI's registry-
// resolved stores and its staging bucket line up with what the CLI seeds. The
// staging tests pin SUVE_STAGING_KEY so the working store never blocks on an OS
// keychain (the CI e2e-azure job does not run inside the compose test-runner that
// otherwise injects it).
//
// Naming: every test is prefixed TestAzureKeyVault* / TestAzureAppConfig* so it
// runs under the existing coverage-uploaded e2e-azure CI job, whose filter is
// `-run 'TestAzureAppConfig|TestAzureKeyVault'` (both emulators up as services);
// the split mise e2e-azure-keyvault / e2e-azure-appconfig tasks pick them up too,
// and `mise e2e-azure-tui` runs exactly this file against both emulators.
// =============================================================================

// azureKeyVaultTUIScope is the Key Vault scope every secret-service TUI e2e
// launches with. The emulator serves a single default vault; the name only has
// to match the staging bucket the test seeds ("suve-e2e", pinned by
// setupAzureKeyVault's AZURE_KEYVAULT_NAME).
func azureKeyVaultTUIScope() provider.Scope {
	return provider.AzureKeyVaultScope("suve-e2e")
}

// azureAppConfigTUIScope is the App Configuration scope every param-service TUI
// e2e launches with (store name "suve-e2e", matching setupAzureAppConfig).
func azureAppConfigTUIScope() provider.Scope {
	return provider.AzureAppConfigScope("suve-e2e")
}

// newAzureTUIModel builds the real TUI model for a launched Azure scope and
// service, wired to registry-backed stores pointed at the emulators (via the
// provider adapters' endpoint seams the CLI harness configures).
func newAzureTUIModel(t *testing.T, scope provider.Scope, service string) tea.Model {
	t.Helper()

	model, err := tui.NewE2EModel(t.Context(), scope, service)
	require.NoError(t, err, "building the TUI model must succeed for a resolvable Azure scope")

	return model
}

// filterBrowser types a substring into the browser's filter field and commits it,
// isolating a single target entry so it becomes the selected row. Key Vault
// deletes are SOFT (lowkey-vault keeps a deleted secret listed until purged), so
// across the sequential suite the browser list accumulates prior tests' secrets
// and would otherwise auto-select the alphabetically-first one rather than the
// seeded target. Filtering to the target's unique name makes the selection (and
// thus the detail/history it drives) deterministic regardless of leftovers.
func filterBrowser(t *testing.T, tm *teatest.TestModel, substr string) {
	t.Helper()

	// Focus the filter field, then type the substring rune by rune.
	tm.Send(keyRune('/'))

	for _, r := range substr {
		tm.Send(keyRune(r))
	}

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit: blur + reload the list
	// The caller gates on a target-specific marker (a per-version tag line or a
	// revealed value that only renders once the isolated entry is selected AND its
	// detail/history has loaded), which is what makes the filtered selection
	// deterministic — not the return of this function.
}

// settleReload gives the browser's batched detail+history reload (issued by the
// filter's reselection) a moment to land before the test sends a reveal or
// compare keystroke. Those two loads are concurrent (tea.Batch) and each resets
// derived state on arrival — SetValue re-masks the value pane, SetRows clears
// compare picks and history reveal — so a keystroke sent while one is still in
// flight would be undone by the late message. A gate on a rendered marker cannot
// close this window: the reload re-renders the SAME content for the (now
// isolated) entry, which the terminal's cell-level diffing emits no new bytes
// for, so there is nothing new to wait on. A short settle is the reliable seam.
func settleReload() { time.Sleep(1500 * time.Millisecond) }

// -----------------------------------------------------------------------------
// Key Vault (secret)
// -----------------------------------------------------------------------------

// TestAzureKeyVault_TUI_Browse seeds a Key Vault secret and drives the TUI secret
// browser: it lists the real secret, shows the value MASKED by default in the
// detail pane, and only reveals it over the explicit-reveal surface (the `x` key,
// GUI parity), fetching the value from the emulator.
func TestAzureKeyVault_TUI_Browse(t *testing.T) {
	setupAzureKeyVault(t)

	const (
		name  = "suve-e2e-tui-kv-browse"
		value = "kv-browse-secret-value"
	)

	cleanup := func() { _, _ = runAzureSecret(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", name, value)
	require.NoError(t, err)

	tm := teatest.NewTestModel(t, newAzureTUIModel(t, azureKeyVaultTUIScope(), string(staging.ServiceSecret)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Gate on the async list landing (the seeded secret is listed), isolate it so
	// it is the selected row (Key Vault's soft-deleted leftovers otherwise share
	// the list), and let the filter's reselection detail load settle so the value
	// pane holds the real value before we reveal it.
	waitForScreen(t, tm, name)
	filterBrowser(t, tm, "kv-browse")
	settleReload()

	// Explicitly reveal with `x`, then gate on the DETAIL content (the plaintext,
	// masked by default until this reveal) so the assertion runs only after the
	// async detail read from the emulator has landed and unmasked.
	tm.Send(keyRune('x'))
	waitForScreen(t, tm, value)

	screen := finalScreen(t, tm)

	assert.Contains(t, screen, name, "the secret browser lists the seeded secret")
	assert.Contains(t, screen, value,
		"pressing x explicitly reveals the secret value fetched from the emulator")
}

// TestAzureKeyVault_TUI_VersionHistoryPerVersionTags seeds a secret with two
// versions and a tag on the current version, then asserts the detail pane's
// version history renders BOTH versions AND — the Key Vault-specific axis — the
// per-version tag line (`tags: …`, only emitted when the capability's
// TagsPerVersion is set). The history value lines are masked until the shared `x`
// reveal, after which each version's value shows, proving both versions rendered.
func TestAzureKeyVault_TUI_VersionHistoryPerVersionTags(t *testing.T) {
	setupAzureKeyVault(t)

	const (
		name    = "suve-e2e-tui-kv-history"
		v1Value = "kv-history-initial"
		v2Value = "kv-history-updated"
	)

	cleanup := func() { _, _ = runAzureSecret(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", name, v1Value)
	require.NoError(t, err)

	// Key Vault version timestamps are second-granular and ordered only by creation
	// time (ids are opaque); sleep past a second boundary so the two versions land
	// in distinct seconds and history order is deterministic.
	time.Sleep(1100 * time.Millisecond)

	_, err = runAzureSecret(t, "update", "--yes", name, v2Value)
	require.NoError(t, err)

	// Tag the CURRENT (v2) version. Key Vault tags are per-version, so only the
	// current version carries this tag — the axis the history renders specially.
	_, err = runAzureSecret(t, "tag", name, "env=prod")
	require.NoError(t, err)

	tm := teatest.NewTestModel(t, newAzureTUIModel(t, azureKeyVaultTUIScope(), string(staging.ServiceSecret)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Gate on the list, isolate this test's secret so it is selected (soft-deleted
	// leftovers otherwise share the list), then gate on the per-version tag line
	// rendering — a target-specific, unmasked marker proving THIS secret's history
	// (its tagged current version) has loaded.
	waitForScreen(t, tm, name)
	filterBrowser(t, tm, "kv-history")
	settleReload()

	// Reveal so the per-version value lines show (the shared `x` toggle drives both
	// the detail value pane and the history value lines), then gate on the DETAIL
	// content: the per-version tag line and the older version's revealed value both
	// prove THIS secret's two-version history has loaded and rendered.
	tm.Send(keyRune('x'))
	waitForScreen(t, tm, "tags: env=prod")
	waitForScreen(t, tm, v1Value)

	screen := finalScreen(t, tm)

	assert.Contains(t, screen, "History", "the detail pane renders the version-history section")
	assert.Contains(t, screen, "current", "the current version is marked in the history")
	assert.Contains(t, screen, "tags: env=prod",
		"the current version's per-version tag renders on its history row (TagsPerVersion)")
	assert.GreaterOrEqual(t, strings.Count(screen, "[enabled]"), 2,
		"both versions render as rows in the history")
	assert.Contains(t, screen, v2Value, "the revealed current-version value shows in the history")
	assert.Contains(t, screen, v1Value,
		"the revealed older-version value shows too, proving both versions rendered")
}

// TestAzureKeyVault_TUI_Diff seeds two versions and drives the compare→diff flow:
// enter compare mode (`c`), pick the two history rows (`space` on each), and open
// the diff (`enter`). The diff page is an explicit-reveal surface, so a secret
// diff shows both versions' values (revealed by default) rather than masking them.
func TestAzureKeyVault_TUI_Diff(t *testing.T) {
	setupAzureKeyVault(t)

	const (
		name    = "suve-e2e-tui-kv-diff"
		v1Value = "kv-diff-initial"
		v2Value = "kv-diff-updated"
	)

	cleanup := func() { _, _ = runAzureSecret(t, "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", name, v1Value)
	require.NoError(t, err)

	time.Sleep(1100 * time.Millisecond)

	_, err = runAzureSecret(t, "update", "--yes", name, v2Value)
	require.NoError(t, err)

	tm := teatest.NewTestModel(t, newAzureTUIModel(t, azureKeyVaultTUIScope(), string(staging.ServiceSecret)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Gate on the list, isolate this test's secret (soft-deleted leftovers
	// otherwise share the list), and let the filter's reselection detail/history
	// reload settle so kv-diff's two-version history is loaded before we compare.
	waitForScreen(t, tm, name)
	filterBrowser(t, tm, "kv-diff")
	settleReload()

	// Enter compare mode, pick the current row (index 0), move down to the older
	// row (index 1), pick it, then open the diff. Let the diff page fetch both
	// versions and render before capturing.
	tm.Send(keyRune('c'))
	tm.Send(keyRune(' '))
	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(keyRune(' '))
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	settleReload()

	// Gate on the diff page's fetched content landing (the newer version's value)
	// before capturing, so the assertion never races the diff page's async
	// two-version fetch on a slow CI runner.
	waitForScreen(t, tm, v2Value)

	// The settled diff page renders both versions' values (revealed by default on
	// this explicit-compare surface); its side-by-side layout key confirms we are on
	// the diff page, not the browser.
	screen := finalScreen(t, tm)

	assert.Contains(t, screen, "side-by-side", "the compare flow opened the diff page")
	assert.Contains(t, screen, v1Value, "the diff shows the older version's value (explicit-reveal surface)")
	assert.Contains(t, screen, v2Value, "the diff shows the newer version's value (explicit-reveal surface)")
}

// TestAzureKeyVault_TUI_StageApply exercises the browse → stage → apply loop for
// BOTH a value update and a tag change: two secrets are pre-staged through the
// shared staging store (one value update, one tag add), then the TUI applies them
// via the Staging tab's apply-all dialog. Both writes are verified through the CLI
// show/tag read path, proving they reached the emulator through the real TUI apply
// path.
func TestAzureKeyVault_TUI_StageApply(t *testing.T) {
	setupAzureKeyVault(t)
	setAzureKeyVaultStagingKey(t)
	setupTempHome(t)

	const (
		updateName  = "suve-e2e-tui-kv-stage-update"
		tagName     = "suve-e2e-tui-kv-stage-tag"
		originalVal = "kv-stage-original"
		stagedVal   = "kv-stage-applied-via-tui"
	)

	cleanup := func() {
		_, _ = runAzureSecret(t, "delete", "--yes", updateName)
		_, _ = runAzureSecret(t, "delete", "--yes", tagName)
	}
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureSecret(t, "create", updateName, originalVal)
	require.NoError(t, err)
	_, err = runAzureSecret(t, "create", tagName, "kv-stage-tag-value")
	require.NoError(t, err)

	// Pre-stage a value update and a tag add through the same working store the TUI
	// reads (matched vault scope), mirroring the CLI staging e2e's seeding.
	store, err := file.NewWorkingStore(azureKeyVaultTUIScope())
	require.NoError(t, err)

	now := time.Now()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret,
		staging.EntryKey{Name: updateName}, staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr(stagedVal),
			StagedAt:  now,
		}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceSecret,
		staging.EntryKey{Name: tagName}, staging.TagEntry{
			Add:      map[string]string{"env": "staged"},
			StagedAt: now,
		}))

	tm := teatest.NewTestModel(t, newAzureTUIModel(t, azureKeyVaultTUIScope(), string(staging.ServiceSecret)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Jump to the Staging tab. Key Vault tabs are [Key Vault, Staging], so the
	// Staging tab is the second tab (global key "2").
	tm.Send(keyRune('2'))
	waitForScreen(t, tm, updateName)

	// Open the apply-all confirmation dialog ("A"), focus the Apply action (Down),
	// and confirm (Enter) — the same choreography the AWS TUI apply e2e drives.
	tm.Send(keyRune('A'))
	waitForScreen(t, tm, "Ignore conflicts")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	// The results view renders once the apply has committed to the emulator. Gate
	// on the value-update entry's "updated" status line (a results-phase-only,
	// contiguous marker), the same discipline the AWS TUI apply e2e uses.
	waitForScreen(t, tm, "updated")

	// Quit directly: the results dialog captures input first (`q` would go to the
	// dialog), and the assertions below are the CLI-verified side effects.
	require.NoError(t, tm.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	// The staged value update reached the emulator through the TUI apply path.
	got, err := runAzureSecret(t, "show", "--raw", updateName)
	require.NoError(t, err)
	assert.Equal(t, stagedVal, got, "the TUI apply wrote the staged value to the emulator")

	// The staged tag change reached the emulator too (per-version tag on the
	// current version).
	tagShow, err := runAzureSecret(t, "show", tagName)
	require.NoError(t, err)
	assert.Contains(t, tagShow, "env", "the TUI apply wrote the staged tag key")
	assert.Contains(t, tagShow, "staged", "the TUI apply wrote the staged tag value")
}

// -----------------------------------------------------------------------------
// App Configuration (param, namespaced)
// -----------------------------------------------------------------------------

// TestAzureAppConfig_TUI_Namespaces seeds App Configuration settings across the
// null namespace and a "dev" namespace, then asserts the namespace axis
// (EntryKey{Name,Namespace}) is honored in the TUI browser: the namespace badge
// column renders, and entries are correctly PARTITIONED by namespace.
//
//   - The null-namespace view (the launch default) shows the null setting badged
//     "(NULL)" and HIDES the dev-only setting.
//   - Advancing the namespace filter one step (space) reveals a view that includes
//     the dev entries, each badged "dev".
//
// Together these prove both the "(NULL)" and "dev" badges render and that the null
// view partitions out the dev-only key.
// waitAzureParamReadable polls the CLI read path until the App Configuration
// setting reads back the expected value. The TUI builds its OWN App Config client,
// and the emulator does not guarantee a write committed by one client is
// immediately visible to another, so launching the TUI straight after seeding can
// race and surface an empty/still-loading list under CI load. Gating on a fresh CLI
// read makes the seed visible before the TUI lists it. The trailing args address
// the setting (name, plus --namespace when the setting is namespaced).
func waitAzureParamReadable(t *testing.T, want string, args ...string) {
	t.Helper()

	deadline := time.Now().Add(tuiWaitTimeout)
	for time.Now().Before(deadline) {
		out, err := runAzureParam(t, append([]string{"show", "--raw"}, args...)...)
		if err == nil && strings.TrimSpace(out) == want {
			return
		}

		time.Sleep(50 * time.Millisecond) //nolint:mnd // poll interval
	}

	t.Fatalf("azure app config setting %v never became readable as %q within %s", args, want, tuiWaitTimeout)
}

func TestAzureAppConfig_TUI_Namespaces(t *testing.T) {
	setupAzureAppConfig(t)

	const (
		shared  = "suve/tui/ac/ns/shared"
		devOnly = "suve/tui/ac/ns/devonly"
	)

	cleanup := func() {
		_, _ = runAzureParam(t, "delete", "--yes", shared)
		_, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", shared)
		_, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", devOnly)
	}
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureParam(t, "create", shared, "null-value")
	require.NoError(t, err)
	_, err = runAzureParam(t, "create", "--namespace", "dev", shared, "dev-value")
	require.NoError(t, err)
	_, err = runAzureParam(t, "create", "--namespace", "dev", devOnly, "dev-only-value")
	require.NoError(t, err)

	// Ensure all three seeds are visible to a fresh client before the TUI launches.
	waitAzureParamReadable(t, "null-value", shared)
	waitAzureParamReadable(t, "dev-value", "--namespace", "dev", shared)
	waitAzureParamReadable(t, "dev-only-value", "--namespace", "dev", devOnly)

	t.Run("null-namespace-view-partitions", func(t *testing.T) {
		tm := teatest.NewTestModel(t, newAzureTUIModel(t, azureAppConfigTUIScope(), string(staging.ServiceParam)),
			teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

		waitForScreen(t, tm, shared)

		screen := finalScreen(t, tm)

		assert.Contains(t, screen, shared, "the null-namespace view lists the null setting")
		assert.Contains(t, screen, "[(NULL)]",
			"the namespace badge column renders the null badge for the null setting")
		assert.NotContains(t, screen, devOnly,
			"the null-namespace view partitions OUT the dev-only setting")
	})

	t.Run("dev-namespace-view-shows-dev-entries", func(t *testing.T) {
		// The dev entries are read by the TUI's own client, which the emulator may
		// not immediately show a just-seeded namespace to (a CI-only cross-client
		// race, #796). Retry with a fresh launch until the dev view renders: wait for
		// the launch (null) view, advance the namespace filter one step (space) — the
		// next option always includes the dev entries (either "dev" or the
		// all-namespaces "*") — then wait for the dev-only setting.
		screen := retryTUIScreen(t,
			func() tea.Model {
				return newAzureTUIModel(t, azureAppConfigTUIScope(), string(staging.ServiceParam))
			},
			func(tm *teatest.TestModel) bool {
				if !awaitScreen(t, tm, shared, tuiAttemptWait) {
					return false
				}
				tm.Send(keyRune(' '))

				return awaitScreen(t, tm, devOnly, tuiAttemptWait)
			})

		assert.Contains(t, screen, devOnly, "advancing the namespace filter reveals the dev-only setting")
		assert.Contains(t, screen, "[dev]",
			"the namespace badge column renders the dev badge for dev entries")
	})
}

// TestAzureAppConfig_TUI_DetailNoHistory seeds a single setting under the "dev"
// namespace, navigates to it, and asserts (a) its detail renders — value shown
// (App Configuration values are plaintext, never masked) and the namespace
// surfaced — and (b) the version-history controls are HIDDEN, because App
// Configuration is unversioned and the browser gates the history section on the
// capability (HasVersionHistory=false).
func TestAzureAppConfig_TUI_DetailNoHistory(t *testing.T) {
	setupAzureAppConfig(t)

	const (
		name  = "suve/tui/ac/detail"
		value = "ac-detail-value"
	)

	cleanup := func() { _, _ = runAzureParam(t, "delete", "--yes", "--namespace", "dev", name) }
	cleanup()
	t.Cleanup(cleanup)

	_, err := runAzureParam(t, "create", "--namespace", "dev", name, value)
	require.NoError(t, err)

	// Ensure the seed is visible to a fresh client before the TUI launches.
	waitAzureParamReadable(t, value, "--namespace", "dev", name)

	tm := teatest.NewTestModel(t, newAzureTUIModel(t, azureAppConfigTUIScope(), string(staging.ServiceParam)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// The setting is under "dev", so the launch (null) view is empty; gate on the
	// namespace header rendering, advance one step to a view that includes the dev
	// entries, wait for the setting to be listed (the sole seeded entry, so it is
	// selected), then let the selection's async detail load settle so the value has
	// landed before we capture — so the assert never races the detail read on a slow
	// CI runner.
	waitForScreen(t, tm, "ns:")
	tm.Send(keyRune(' '))
	waitForScreen(t, tm, name)
	settleReload()

	screen := finalScreen(t, tm)

	assert.Contains(t, screen, name, "the browser lists the namespaced setting")
	assert.Contains(t, screen, value,
		"the detail pane shows the namespaced setting's plaintext value over the real read path")
	assert.Contains(t, screen, "ns:", "the namespace filter control renders (namespace-aware service)")
	assert.NotContains(t, screen, "History",
		"App Configuration is unversioned: the version-history section is capability-gated OFF")
}

// TestAzureAppConfig_TUI_StageNamespace guards namespace threading through the TUI
// apply path: a create is pre-staged under a SPECIFIC namespace ("dev"), the TUI
// applies it, and the CLI confirms the write landed under "dev" — and NOT under
// the null namespace. This is the end-to-end guard for the namespace axis
// surviving stage → apply through the TUI (a historically bug-prone area).
func TestAzureAppConfig_TUI_StageNamespace(t *testing.T) {
	setupAzureAppConfig(t)
	setAzureKeyVaultStagingKey(t)
	setupTempHome(t)

	const (
		name      = "suve/tui/ac/stage/ns"
		namespace = "dev"
		stagedVal = "ac-stage-ns-value"
	)

	cleanup := func() {
		_, _ = runAzureParam(t, "delete", "--yes", name)
		_, _ = runAzureParam(t, "delete", "--yes", "--namespace", namespace, name)
	}
	cleanup()
	t.Cleanup(cleanup)

	// Pre-stage a create under the "dev" namespace through the same per-store
	// working store the TUI reads. App Configuration staging is per-store; the
	// namespace is part of the entry's identity (EntryKey.Namespace), not the path.
	store, err := file.NewWorkingStore(azureAppConfigTUIScope())
	require.NoError(t, err)

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam,
		staging.EntryKey{Name: name, Namespace: namespace}, staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr(stagedVal),
			StagedAt:  time.Now(),
		}))

	tm := teatest.NewTestModel(t, newAzureTUIModel(t, azureAppConfigTUIScope(), string(staging.ServiceParam)),
		teatest.WithInitialTermSize(tuiTermWidth, tuiTermHeight))

	// Jump to the Staging tab. App Configuration tabs are [App Configuration,
	// Staging], so the Staging tab is the second tab (global key "2").
	tm.Send(keyRune('2'))
	waitForScreen(t, tm, name)

	tm.Send(keyRune('A'))
	waitForScreen(t, tm, "Ignore conflicts")

	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Gate on the create entry's "created" status line in the results view (a
	// results-phase-only, contiguous marker).
	waitForScreen(t, tm, "created")

	require.NoError(t, tm.Quit())
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	// The staged create landed under the RIGHT namespace…
	got, err := runAzureParam(t, "show", "--raw", "--namespace", namespace, name)
	require.NoError(t, err)
	assert.Equal(t, stagedVal, got, "the TUI apply wrote the staged value under the dev namespace")

	// …and NOT under the null namespace (namespace threading was preserved).
	_, err = runAzureParam(t, "show", "--raw", name)
	require.Error(t, err, "the setting must not exist under the null namespace")
}
