//go:build e2e

//nolint:paralleltest // E2E subtests share state and run sequentially, not in parallel
package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
)

// newGoogleCloudStore creates a working staging store keyed by the given Google
// Cloud project, matching the scope the `gcloud stage` commands resolve.
func newGoogleCloudStore(project string) *file.Store {
	s, err := file.NewWorkingStore(provider.GoogleCloudScope(project))
	if err != nil {
		panic(err)
	}

	return s
}

// TestGoogleCloudStage_Workflow exercises the `suve gcloud stage` staging
// workflow (status / diff / apply for update, create, and delete) against a
// local Secret Manager emulator. It is skipped unless the emulator endpoint is
// configured (see setupGoogleCloud) and uses an isolated temp HOME for the
// on-disk staging state.
func TestGoogleCloudStage_Workflow(t *testing.T) {
	setupGoogleCloud(t)
	setupTempHome(t)

	const (
		project    = "suve-e2e"
		updateName = "suve-e2e-gcloud-stage/update"
		createName = "suve-e2e-gcloud-stage/create"
		deleteName = "suve-e2e-gcloud-stage/delete"
	)

	// Best-effort cleanup.
	cleanup := func() {
		_, _ = runGcloud(t, "secret", "delete", "--yes", updateName)
		_, _ = runGcloud(t, "secret", "delete", "--yes", createName)
		_, _ = runGcloud(t, "secret", "delete", "--yes", deleteName)
	}
	cleanup()
	t.Cleanup(cleanup)

	// Seed the secrets that update/delete operate on.
	_, err := runGcloud(t, "secret", "create", updateName, "original")
	require.NoError(t, err)
	_, err = runGcloud(t, "secret", "create", deleteName, "to-be-deleted")
	require.NoError(t, err)

	store := newGoogleCloudStore(project)
	now := time.Now()

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: updateName}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-value"),
		StagedAt:  now,
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: createName}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("created-value"),
		StagedAt:  now,
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: deleteName}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	}))

	t.Run("status", func(t *testing.T) {
		stdout, err := runGcloud(t, "stage", "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Secret Manager")
		assert.Contains(t, stdout, updateName)
		assert.Contains(t, stdout, createName)
		assert.Contains(t, stdout, deleteName)
	})

	t.Run("diff", func(t *testing.T) {
		stdout, err := runGcloud(t, "stage", "diff")
		require.NoError(t, err)
		assert.Contains(t, stdout, "-original")
		assert.Contains(t, stdout, "+staged-value")
		assert.Contains(t, stdout, "+created-value")
	})

	t.Run("apply", func(t *testing.T) {
		stdout, err := runGcloud(t, "stage", "apply", "--yes")
		require.NoError(t, err)
		assert.Contains(t, stdout, updateName)
		assert.Contains(t, stdout, createName)
		assert.Contains(t, stdout, deleteName)
	})

	t.Run("verify-update", func(t *testing.T) {
		stdout, err := runGcloud(t, "secret", "show", "--raw", updateName)
		require.NoError(t, err)
		assert.Equal(t, "staged-value", stdout)
	})

	t.Run("verify-create", func(t *testing.T) {
		stdout, err := runGcloud(t, "secret", "show", "--raw", createName)
		require.NoError(t, err)
		assert.Equal(t, "created-value", stdout)
	})

	t.Run("verify-delete", func(t *testing.T) {
		_, err := runGcloud(t, "secret", "show", "--raw", deleteName)
		require.Error(t, err)
	})

	t.Run("status-empty-after-apply", func(t *testing.T) {
		stdout, err := runGcloud(t, "stage", "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, updateName)
		assert.NotContains(t, stdout, createName)
		assert.NotContains(t, stdout, deleteName)
	})
}

// TestGoogleCloudStage_DescriptionApplied confirms that a staged description is
// carried through apply to the "description" annotation and shown in the read
// view — #666's fix for the previously silent no-op (the value was accepted and
// shown in status/diff, then dropped on apply for non-AWS providers).
func TestGoogleCloudStage_DescriptionApplied(t *testing.T) {
	setupGoogleCloud(t)
	setupTempHome(t)

	const (
		project = "suve-e2e"
		name    = "suve-e2e-gcloud-stage-desc/create"
	)

	cleanup := func() { _, _ = runGcloud(t, "secret", "delete", "--yes", name) }
	cleanup()
	t.Cleanup(cleanup)

	store := newGoogleCloudStore(project)
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: name}, staging.Entry{
		Operation:   staging.OperationCreate,
		Value:       lo.ToPtr("created-value"),
		Description: lo.ToPtr("staged description"),
		StagedAt:    time.Now(),
	}))

	_, err := runGcloud(t, "stage", "apply", "--yes")
	require.NoError(t, err)

	stdout, err := runGcloud(t, "secret", "show", name)
	require.NoError(t, err)
	assert.Contains(t, stdout, "created-value")
	assert.Contains(t, stdout, "Description")
	assert.Contains(t, stdout, "staged description")
}

// TestGoogleCloudStage_FlatAliasReachesEmulator confirms the flat `suve stage`
// alias, when forced to Google Cloud, resolves the project and drives the same
// emulator-backed staging store.
func TestGoogleCloudStage_FlatAliasReachesEmulator(t *testing.T) {
	setupGoogleCloud(t)
	setupTempHome(t)

	const project = "suve-e2e"

	store := newGoogleCloudStore(project)
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "suve-e2e-gcloud-flat/secret"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("v"),
		StagedAt:  time.Now(),
	}))

	stdout, err := runGcloud(t, "stage", "status")
	require.NoError(t, err)
	assert.Contains(t, stdout, "suve-e2e-gcloud-flat/secret")
}

// TestGoogleCloudStage_ExportImport exercises the service-specific
// `gcloud stage export <file>` / `import <file>` round-trip (Google Cloud is
// secret-only, so the operation lives directly under `gcloud stage`). It uses an
// isolated temp HOME so the working staging area starts empty.
func TestGoogleCloudStage_ExportImport(t *testing.T) {
	setupGoogleCloud(t)
	setupTempHome(t)

	const name = "suve-e2e-gcloud-stage-export-import/secret"

	exportPath := filepath.Join(t.TempDir(), "secret.json")

	// Stage a create in the working staging area.
	_, err := runGcloud(t, "stage", "add", name, "exported-value")
	require.NoError(t, err)

	t.Run("export", func(t *testing.T) {
		stdout, err := runGcloud(t, "stage", "export", exportPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "exported")

		_, statErr := os.Stat(exportPath)
		require.NoError(t, statErr)
	})

	t.Run("working-cleared", func(t *testing.T) {
		stdout, err := runGcloud(t, "stage", "status")
		require.NoError(t, err)
		assert.NotContains(t, stdout, name)
	})

	t.Run("import", func(t *testing.T) {
		stdout, err := runGcloud(t, "stage", "import", exportPath)
		require.NoError(t, err)
		assert.Contains(t, stdout, "imported")
	})

	t.Run("working-restored", func(t *testing.T) {
		stdout, err := runGcloud(t, "stage", "status")
		require.NoError(t, err)
		assert.Contains(t, stdout, "Secret Manager")
		assert.Contains(t, stdout, name)
	})
}
