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

	"github.com/mpyw/suve/internal/maputil"
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

func TestApp_StagingCancelRemoveTag(t *testing.T) {
	t.Parallel()

	t.Run("cancel single remove tag", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage multiple remove tags
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Remove: maputil.NewSet("env", "team"),
		})
		require.NoError(t, err)

		// Cancel one tag removal
		result, err := app.StagingCancelRemoveTag("param", "/app/config", "env")
		require.NoError(t, err)
		assert.Equal(t, "/app/config", result.Name)

		// Verify only team remains in remove list
		tagEntry, err := app.stagingStore.GetTag(app.ctx, staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.False(t, tagEntry.Remove.Contains("env"))
		assert.True(t, tagEntry.Remove.Contains("team"))
	})

	t.Run("cancel last remove tag removes entry", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage single remove tag
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Remove: maputil.NewSet("env"),
		})
		require.NoError(t, err)

		// Cancel the only tag
		_, err = app.StagingCancelRemoveTag("param", "/app/config", "env")
		require.NoError(t, err)

		// Verify tag entry is removed
		_, err = app.stagingStore.GetTag(app.ctx, staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("cancel with both add and remove - preserves add", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage both add and remove
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:    map[string]string{"env": "prod"},
			Remove: maputil.NewSet("team"),
		})
		require.NoError(t, err)

		// Cancel the remove tag
		_, err = app.StagingCancelRemoveTag("param", "/app/config", "team")
		require.NoError(t, err)

		// Verify add tags are preserved
		tagEntry, err := app.stagingStore.GetTag(app.ctx, staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Empty(t, tagEntry.Remove)
		assert.Equal(t, "prod", tagEntry.Add["env"])
	})
}

func TestApp_StagingStatus_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("mixed services entries and tags", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage param entry
		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value1"),
		})
		require.NoError(t, err)

		// Stage secret entry
		err = app.stagingStore.StageEntry(app.ctx, staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		})
		require.NoError(t, err)

		// Stage param tag
		err = app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/other", staging.TagEntry{
			Add: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)

		// Stage secret tag
		err = app.stagingStore.StageTag(app.ctx, staging.ServiceSecret, "other-secret", staging.TagEntry{
			Remove: maputil.NewSet("deprecated"),
		})
		require.NoError(t, err)

		result, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Len(t, result.Param, 1)
		assert.Len(t, result.Secret, 1)
		assert.Len(t, result.ParamTags, 1)
		assert.Len(t, result.SecretTags, 1)
	})

	t.Run("delete operation", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/to-delete", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err)

		result, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Len(t, result.Param, 1)
		assert.Equal(t, "delete", result.Param[0].Operation)
		assert.Nil(t, result.Param[0].Value)
	})

	t.Run("tag with add and remove", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:    map[string]string{"env": "prod", "team": "backend"},
			Remove: maputil.NewSet("deprecated", "old"),
		})
		require.NoError(t, err)

		result, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Len(t, result.ParamTags, 1)
		assert.Equal(t, "prod", result.ParamTags[0].AddTags["env"])
		assert.Equal(t, "backend", result.ParamTags[0].AddTags["team"])
		assert.Contains(t, result.ParamTags[0].RemoveTags, "deprecated")
		assert.Contains(t, result.ParamTags[0].RemoveTags, "old")
	})
}

func TestApp_StagingReset(t *testing.T) {
	t.Parallel()

	t.Run("reset param service", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage entries in both services
		_ = app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		_ = app.stagingStore.StageEntry(app.ctx, staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret"),
		})

		// Reset param only
		result, err := app.StagingReset("param")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "unstagedAll", result.Type)

		// Verify param cleared, secret remains
		status, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Empty(t, status.Param)
		assert.Len(t, status.Secret, 1)
	})

	t.Run("reset secret service", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage entries in both services
		_ = app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		_ = app.stagingStore.StageEntry(app.ctx, staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret"),
		})

		// Reset secret only
		result, err := app.StagingReset("secret")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "unstagedAll", result.Type)

		// Verify secret cleared, param remains
		status, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Len(t, status.Param, 1)
		assert.Empty(t, status.Secret)
	})

	t.Run("reset nothing staged", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Reset when nothing staged
		result, err := app.StagingReset("param")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "nothingStaged", result.Type)
	})

	t.Run("reset invalid service", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		_, err := app.StagingReset("invalid")
		assert.ErrorIs(t, err, errInvalidService)
	})

	t.Run("reset empty service", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		_, err := app.StagingReset("")
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

		// Create file store
		fileStore := file.NewStoreWithDir(localTmpDir)

		// Create state with entries
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
		}

		// Write state (persist)
		err := fileStore.WriteState(context.Background(), "", state)
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
		drainedState, err := fileStore.Drain(context.Background(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "test-value", lo.FromPtr(drainedState.Entries[staging.ServiceParam]["/app/config"].Value))
	})

	t.Run("persist and drain cycle - encrypted", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()

		// Create file store with passphrase
		fileStore := file.NewStoreWithDir(localTmpDir)
		fileStore.SetPassphrase("test-passphrase")

		// Create state
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		}

		// Write encrypted state
		err := fileStore.WriteState(context.Background(), "", state)
		require.NoError(t, err)

		// Verify encrypted
		isEnc, err := fileStore.IsEncrypted()
		require.NoError(t, err)
		assert.True(t, isEnc)

		// Drain with correct passphrase
		drainedState, err := fileStore.Drain(context.Background(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "secret-value", lo.FromPtr(drainedState.Entries[staging.ServiceSecret]["my-secret"].Value))

		// Drain with wrong passphrase should fail
		wrongStore := file.NewStoreWithDir(localTmpDir)
		wrongStore.SetPassphrase("wrong-passphrase")
		_, err = wrongStore.Drain(context.Background(), "", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decryption")
	})

	t.Run("drain with keep=false removes file", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()

		fileStore := file.NewStoreWithDir(localTmpDir)

		// Create and write state
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/test"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		}
		err := fileStore.WriteState(context.Background(), "", state)
		require.NoError(t, err)

		// Drain with keep=false
		_, err = fileStore.Drain(context.Background(), "", false)
		require.NoError(t, err)

		// File should be removed
		exists, err := fileStore.Exists()
		require.NoError(t, err)
		assert.False(t, exists)
	})

	_ = filePath // suppress unused variable warning

	t.Run("persist with tags only", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()

		fileStore := file.NewStoreWithDir(localTmpDir)

		// Create state with tags only (no entries)
		state := staging.NewEmptyState()
		state.Tags[staging.ServiceParam]["/app/config"] = staging.TagEntry{
			Add:    map[string]string{"env": "prod"},
			Remove: maputil.NewSet("deprecated"),
		}

		err := fileStore.WriteState(context.Background(), "", state)
		require.NoError(t, err)

		// Drain and verify
		drainedState, err := fileStore.Drain(context.Background(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "prod", drainedState.Tags[staging.ServiceParam]["/app/config"].Add["env"])
		assert.True(t, drainedState.Tags[staging.ServiceParam]["/app/config"].Remove.Contains("deprecated"))
	})

	t.Run("persist multiple services", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()

		fileStore := file.NewStoreWithDir(localTmpDir)

		// Create state with both services
		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam]["/app/config"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
		}
		state.Entries[staging.ServiceSecret]["my-secret"] = staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("secret-value"),
		}

		err := fileStore.WriteState(context.Background(), "", state)
		require.NoError(t, err)

		// Drain and verify both services
		drainedState, err := fileStore.Drain(context.Background(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "param-value", lo.FromPtr(drainedState.Entries[staging.ServiceParam]["/app/config"].Value))
		assert.Equal(t, "secret-value", lo.FromPtr(drainedState.Entries[staging.ServiceSecret]["my-secret"].Value))
	})

	t.Run("drain nonexistent file returns empty state", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()

		fileStore := file.NewStoreWithDir(localTmpDir)

		exists, err := fileStore.Exists()
		require.NoError(t, err)
		assert.False(t, exists)

		// Draining nonexistent file returns empty state (not an error)
		state, err := fileStore.Drain(context.Background(), "", true)
		require.NoError(t, err)
		assert.Empty(t, state.Entries[staging.ServiceParam])
		assert.Empty(t, state.Entries[staging.ServiceSecret])
	})

	t.Run("overwrite existing file on persist", func(t *testing.T) {
		t.Parallel()

		localTmpDir := t.TempDir()

		fileStore := file.NewStoreWithDir(localTmpDir)

		// Write first state
		state1 := staging.NewEmptyState()
		state1.Entries[staging.ServiceParam]["/first"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("first-value"),
		}
		err := fileStore.WriteState(context.Background(), "", state1)
		require.NoError(t, err)

		// Overwrite with second state
		state2 := staging.NewEmptyState()
		state2.Entries[staging.ServiceParam]["/second"] = staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("second-value"),
		}
		err = fileStore.WriteState(context.Background(), "", state2)
		require.NoError(t, err)

		// Drain should only have second state
		drainedState, err := fileStore.Drain(context.Background(), "", true)
		require.NoError(t, err)
		assert.NotContains(t, drainedState.Entries[staging.ServiceParam], "/first")
		assert.Contains(t, drainedState.Entries[staging.ServiceParam], "/second")
	})
}

// =============================================================================
// Error Path Tests
// =============================================================================

func TestApp_StagingStatus_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("list entries error for param", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper
		mockStore.ListEntriesErr = staging.ErrNotStaged

		_, err := app.StagingStatus()
		assert.Error(t, err)
	})

	t.Run("list tags error for param", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper
		mockStore.ListTagsErr = staging.ErrNotStaged

		_, err := app.StagingStatus()
		assert.Error(t, err)
	})
}

func TestApp_StagingUnstage_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid service", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		_, err := app.StagingUnstage("invalid", "/test")
		assert.ErrorIs(t, err, errInvalidService)
	})

	t.Run("unstage entry error (not ErrNotStaged)", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper

		// Stage an entry first
		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/test", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.NoError(t, err)

		// Inject error
		mockStore.UnstageEntryErr = context.DeadlineExceeded

		_, err = app.StagingUnstage("param", "/test")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("unstage tag error (not ErrNotStaged)", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper

		// Stage a tag first
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/test", staging.TagEntry{
			Add: map[string]string{"key": "value"},
		})
		require.NoError(t, err)

		// Inject error
		mockStore.UnstageTagErr = context.DeadlineExceeded

		_, err = app.StagingUnstage("param", "/test")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestApp_StagingCancelAddTag_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid service", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		_, err := app.StagingCancelAddTag("invalid", "/test", "key")
		assert.ErrorIs(t, err, errInvalidService)
	})

	t.Run("tag not staged", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		_, err := app.StagingCancelAddTag("param", "/nonexistent", "key")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("stage tag error when updating", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper

		// Stage multiple add tags
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/test", staging.TagEntry{
			Add: map[string]string{"key1": "val1", "key2": "val2"},
		})
		require.NoError(t, err)

		// Inject error for restaging
		mockStore.StageTagErr = context.DeadlineExceeded

		// Cancel one tag (should fail when restaging)
		_, err = app.StagingCancelAddTag("param", "/test", "key1")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("unstage tag error when clearing last tag", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper

		// Stage single add tag
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/test", staging.TagEntry{
			Add: map[string]string{"key": "value"},
		})
		require.NoError(t, err)

		// Inject error for unstaging
		mockStore.UnstageTagErr = context.DeadlineExceeded

		// Cancel the only tag (should fail when trying to unstage)
		_, err = app.StagingCancelAddTag("param", "/test", "key")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestApp_StagingCancelRemoveTag_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid service", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		_, err := app.StagingCancelRemoveTag("invalid", "/test", "key")
		assert.ErrorIs(t, err, errInvalidService)
	})

	t.Run("tag not staged", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		_, err := app.StagingCancelRemoveTag("param", "/nonexistent", "key")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("stage tag error when updating", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper

		// Stage multiple remove tags
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/test", staging.TagEntry{
			Remove: maputil.NewSet("key1", "key2"),
		})
		require.NoError(t, err)

		// Inject error for restaging
		mockStore.StageTagErr = context.DeadlineExceeded

		// Cancel one tag (should fail when restaging)
		_, err = app.StagingCancelRemoveTag("param", "/test", "key1")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("unstage tag error when clearing last remove tag", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper

		// Stage single remove tag
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/test", staging.TagEntry{
			Remove: maputil.NewSet("key"),
		})
		require.NoError(t, err)

		// Inject error for unstaging
		mockStore.UnstageTagErr = context.DeadlineExceeded

		// Cancel the only tag (should fail when trying to unstage)
		_, err = app.StagingCancelRemoveTag("param", "/test", "key")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func TestApp_StagingCheckStatus_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid service", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		_, err := app.StagingCheckStatus("invalid", "/test")
		assert.ErrorIs(t, err, errInvalidService)
	})
}

func TestApp_StagingReset_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("unstage all error", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper

		// Stage an entry
		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/test", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.NoError(t, err)

		// Inject error
		mockStore.UnstageAllErr = context.DeadlineExceeded

		_, err = app.StagingReset("param")
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestApp_StagingStatus_AllOperations(t *testing.T) {
	t.Parallel()

	t.Run("all operations covered", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Create operation
		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/create", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-value"),
		})
		require.NoError(t, err)

		// Update operation
		err = app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/update", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("updated-value"),
		})
		require.NoError(t, err)

		// Delete operation
		err = app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/delete", staging.Entry{
			Operation: staging.OperationDelete,
		})
		require.NoError(t, err)

		result, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Len(t, result.Param, 3)

		// Verify operations are correctly mapped
		operations := make(map[string]string)
		for _, entry := range result.Param {
			operations[entry.Name] = entry.Operation
		}

		assert.Equal(t, "create", operations["/create"])
		assert.Equal(t, "update", operations["/update"])
		assert.Equal(t, "delete", operations["/delete"])
	})
}

func TestApp_StagingCheckStatus_BothEntryAndTags(t *testing.T) {
	t.Parallel()

	t.Run("handles get entry error gracefully", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)
		mockStore := app.stagingStore.(*testutil.MockStore) //nolint:forcetypeassert // test helper

		// Stage a tag (no entry)
		err := app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/test", staging.TagEntry{
			Add: map[string]string{"key": "value"},
		})
		require.NoError(t, err)

		// GetEntryErr is not staging.ErrNotStaged, but GetEntry returns error for non-existent
		// The behavior depends on the mock - let's verify the positive case
		result, err := app.StagingCheckStatus("param", "/test")
		require.NoError(t, err)
		assert.False(t, result.HasEntry)
		assert.True(t, result.HasTags)

		// Now set error and verify handling
		mockStore.GetEntryErr = context.DeadlineExceeded
		result, err = app.StagingCheckStatus("param", "/test")
		require.NoError(t, err) // GetEntry error is swallowed, HasEntry is just false
		assert.False(t, result.HasEntry)
	})
}

func TestApp_StagingUnstage_BothEntryAndTag(t *testing.T) {
	t.Parallel()

	t.Run("unstage both entry and tag", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage both entry and tag for the same item
		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
		})
		require.NoError(t, err)

		err = app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/app/config", staging.TagEntry{
			Add: map[string]string{"env": "prod"},
		})
		require.NoError(t, err)

		// Verify both are staged
		status, err := app.StagingCheckStatus("param", "/app/config")
		require.NoError(t, err)
		assert.True(t, status.HasEntry)
		assert.True(t, status.HasTags)

		// Unstage both
		result, err := app.StagingUnstage("param", "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "/app/config", result.Name)

		// Verify both are unstaged
		status, err = app.StagingCheckStatus("param", "/app/config")
		require.NoError(t, err)
		assert.False(t, status.HasEntry)
		assert.False(t, status.HasTags)
	})
}

func TestApp_StagingReset_ResetBothEntriesAndTags(t *testing.T) {
	t.Parallel()

	t.Run("reset clears both entries and tags", func(t *testing.T) {
		t.Parallel()
		app := setupTestApp(t)

		// Stage entries
		err := app.stagingStore.StageEntry(app.ctx, staging.ServiceParam, "/entry1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value1"),
		})
		require.NoError(t, err)

		// Stage tags
		err = app.stagingStore.StageTag(app.ctx, staging.ServiceParam, "/tag1", staging.TagEntry{
			Add: map[string]string{"key": "value"},
		})
		require.NoError(t, err)

		// Verify staged
		status, err := app.StagingStatus()
		require.NoError(t, err)
		assert.Len(t, status.Param, 1)
		assert.Len(t, status.ParamTags, 1)

		// Reset
		result, err := app.StagingReset("param")
		require.NoError(t, err)
		assert.Equal(t, "unstagedAll", result.Type)

		// Verify cleared
		status, err = app.StagingStatus()
		require.NoError(t, err)
		assert.Empty(t, status.Param)
		assert.Empty(t, status.ParamTags)
	})
}
