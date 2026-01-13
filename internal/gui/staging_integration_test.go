//go:build production || dev

package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// setupTestApp creates a test App with a memory-based staging store.
func setupTestApp(t *testing.T) *App {
	t.Helper()

	app := NewApp()
	app.Startup(t.Context())
	app.stagingStore = testutil.NewMockStore()

	return app
}

func TestApp_StagingStatus(t *testing.T) {
	t.Parallel()

	t.Run("empty staging", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		result, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Empty(t, result.Param)
		assert.Empty(t, result.Secret)
		assert.Empty(t, result.ParamTags)
		assert.Empty(t, result.SecretTags)
	})

	t.Run("with staged entries", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage a param entry
		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
		})
		require.NoError(t, err)

		result, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Len(t, result.Param, 1)
		assert.Equal(t, "/app/config", result.Param[0].Name)
		assert.Equal(t, "update", result.Param[0].Operation)
	})

	t.Run("with staged tags", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage tag changes
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceSecret, "my-secret", staging.TagEntry{
			Add: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)

		result, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Len(t, result.SecretTags, 1)
		assert.Equal(t, "my-secret", result.SecretTags[0].Name)
		assert.Equal(t, "prod", result.SecretTags[0].AddTags["env"])
	})
}

func TestApp_StagingUnstage(t *testing.T) {
	t.Parallel()

	t.Run("unstage entry", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage an entry
		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.NoError(t, err)

		// Unstage it
		result, err := app.StagingUnstage("param", "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "/app/config", result.Name)

		// Verify it's gone
		status, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Empty(t, status.Param)
	})

	t.Run("unstage nonexistent - no error", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Should not error even if not staged
		result, err := app.StagingUnstage("param", "/nonexistent")
		require.NoError(t, err)
		assert.Equal(t, "/nonexistent", result.Name)
	})
}

func TestApp_StagingCancelAddTag(t *testing.T) {
	t.Parallel()

	t.Run("cancel single tag", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage multiple tags
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Add: map[string]string{"env": "prod", "team": "backend"},
		})
		require.NoError(t, err)

		// Cancel one tag
		result, err := app.StagingCancelAddTag("param", "/app/config", "env")
		require.NoError(t, err)
		assert.Equal(t, "/app/config", result.Name)

		// Verify only team remains
		tagEntry, err := app.stagingStore.GetTag(app.ctx, staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.NotContains(t, tagEntry.Add, "env")
		assert.Contains(t, tagEntry.Add, "team")
	})

	t.Run("cancel last tag removes entry", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage single tag
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Add: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)

		// Cancel the only tag
		_, err = app.StagingCancelAddTag("param", "/app/config", "env")
		require.NoError(t, err)

		// Verify tag entry is removed
		_, err = app.stagingStore.GetTag(app.ctx, staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

func TestApp_StagingCheckStatus(t *testing.T) {
	t.Parallel()

	t.Run("no staged changes", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		result, err := app.StagingCheckStatus("param", "/app/config")
		require.NoError(t, err)
		assert.False(t, result.HasEntry)
		assert.False(t, result.HasTags)
	})

	t.Run("has entry only", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.NoError(t, err)

		result, err := app.StagingCheckStatus("param", "/app/config")
		require.NoError(t, err)
		assert.True(t, result.HasEntry)
		assert.False(t, result.HasTags)
	})

	t.Run("has tags only", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Add: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)

		result, err := app.StagingCheckStatus("param", "/app/config")
		require.NoError(t, err)
		assert.False(t, result.HasEntry)
		assert.True(t, result.HasTags)
	})

	t.Run("has both entry and tags", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.NoError(t, err)

		err = app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Add: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)

		result, err := app.StagingCheckStatus("param", "/app/config")
		require.NoError(t, err)
		assert.True(t, result.HasEntry)
		assert.True(t, result.HasTags)
	})
}

func TestApp_getService(t *testing.T) {
	t.Parallel()

	app := &App{}

	t.Run("param", func(t *testing.T) {
		t.Parallel()
		svc, err := app.getService("param")
		require.NoError(t, err)
		assert.Equal(t, staging.ServiceParam, svc)
	})

	t.Run("secret", func(t *testing.T) {
		t.Parallel()
		svc, err := app.getService("secret")
		require.NoError(t, err)
		assert.Equal(t, staging.ServiceSecret, svc)
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		_, err := app.getService("invalid")
		assert.ErrorIs(t, err, errInvalidService)
	})
}

// =============================================================================
// File-based Integration Tests for Drain/Persist
// =============================================================================

func TestApp_StagingFileStatus(t *testing.T) {
	t.Parallel()

	// Skip if AWS credentials are not available
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		t.Skip("Skipping test: AWS credentials not configured")
	}

	t.Run("file not exists", func(t *testing.T) {
		t.Parallel()

		app := &App{ctx: context.Background()}

		result, err := app.StagingFileStatus()
		if err != nil {
			t.Skipf("Skipping: %v", err)
		}

		// Result depends on whether file exists in user's environment
		assert.NotNil(t, result)
	})
}

// TestFileDrainPersist tests the drain/persist cycle using actual file stores.
// This test doesn't require AWS credentials as it uses file stores directly.
func TestFileDrainPersist(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "stage.json")

	t.Run("persist and drain cycle - unencrypted", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()
		localFilePath := filepath.Join(localTmpDir, "stage.json")

		// Create file store
		fileStore := file.NewStoreWithPath(localFilePath)

		// Create state with entries
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
		}

		// Write state (persist)
		err := fileStore.WriteState(context.Background(), state)
		require.NoError(t, err)

		// Verify file exists
		exists, err := fileStore.Exists()
		require.NoError(t, err)
		assert.True(t, exists)

		// Verify not encrypted
		isEnc, err := fileStore.IsEncrypted()
		require.NoError(t, err)
		assert.False(t, isEnc)

		// Drain (read back)
		drainedState, err := fileStore.Drain(context.Background(), true)
		require.NoError(t, err)
		assert.Equal(t, "test-value", lo.FromPtr(drainedState.Entries[staging.ServiceParam]["/app/config"].Value))
	})

	t.Run("persist and drain cycle - encrypted", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()
		localFilePath := filepath.Join(localTmpDir, "stage.json")

		// Create file store with passphrase
		fileStore := file.NewStoreWithPath(localFilePath)
		fileStore.SetPassphrase("test-passphrase")

		// Create state
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		}

		// Write encrypted state
		err := fileStore.WriteState(context.Background(), state)
		require.NoError(t, err)

		// Verify encrypted
		isEnc, err := fileStore.IsEncrypted()
		require.NoError(t, err)
		assert.True(t, isEnc)

		// Drain with correct passphrase
		drainedState, err := fileStore.Drain(context.Background(), true)
		require.NoError(t, err)
		assert.Equal(t, "secret-value", lo.FromPtr(drainedState.Entries[staging.ServiceSecret]["my-secret"].Value))

		// Drain with wrong passphrase should fail
		wrongStore := file.NewStoreWithPath(localFilePath)
		wrongStore.SetPassphrase("wrong-passphrase")
		_, err = wrongStore.Drain(context.Background(), true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decryption")
	})

	t.Run("drain with keep=false removes file", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()
		localFilePath := filepath.Join(localTmpDir, "stage.json")

		fileStore := file.NewStoreWithPath(localFilePath)

		// Create and write state
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/test"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		}
		err := fileStore.WriteState(context.Background(), state)
		require.NoError(t, err)

		// Drain with keep=false
		_, err = fileStore.Drain(context.Background(), false)
		require.NoError(t, err)

		// File should be removed
		exists, err := fileStore.Exists()
		require.NoError(t, err)
		assert.False(t, exists)
	})

	_ = filePath // suppress unused variable warning
}
