package file_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/file"
)

func TestNewStore(t *testing.T) {
	t.Parallel()

	store, err := file.NewStore("123456789012", "ap-northeast-1")
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_LoadEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	state, err := store.Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, state.Version)
	assert.Empty(t, state.Entries[staging.ServiceParam])
	assert.Empty(t, state.Entries[staging.ServiceSecret])
}

func TestStore_StageAndLoad(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now().Truncate(time.Second)

	// Stage SSM Parameter Store entry
	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("test-value"),
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Stage Secrets Manager entry
	err = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Load and verify
	state, err := store.Load(t.Context())
	require.NoError(t, err)

	assert.Len(t, state.Entries[staging.ServiceParam], 1)
	assert.Equal(t, staging.OperationUpdate, state.Entries[staging.ServiceParam]["/app/config"].Operation)
	assert.Equal(t, "test-value", lo.FromPtr(state.Entries[staging.ServiceParam]["/app/config"].Value))

	assert.Len(t, state.Entries[staging.ServiceSecret], 1)
	assert.Equal(t, staging.OperationDelete, state.Entries[staging.ServiceSecret]["my-secret"].Operation)
}

func TestStore_StageOverwrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage initial
	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("initial"),
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Stage overwrite
	err = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("updated"),
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Verify overwritten
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "updated", lo.FromPtr(entry.Value))
}

func TestStore_Unstage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage
	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("test"),
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Unstage
	err = store.UnstageEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)

	// Verify removed
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_UnstageNotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	err := store.UnstageEntry(t.Context(), staging.ServiceParam, "/not/staged")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	err = store.UnstageEntry(t.Context(), staging.ServiceSecret, "not-staged")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_UnstageAll(t *testing.T) {
	t.Parallel()

	t.Run("unstage all SSM Parameter Store", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		// Stage multiple
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v1"), StagedAt: now})
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v2"), StagedAt: now})
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "secret1", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("s1"), StagedAt: now})

		// Unstage all SSM Parameter Store
		err := store.UnstageAll(t.Context(), staging.ServiceParam)
		require.NoError(t, err)

		// Verify SSM Parameter Store cleared, Secrets Manager intact
		state, err := store.Load(t.Context())
		require.NoError(t, err)
		assert.Empty(t, state.Entries[staging.ServiceParam])
		assert.Len(t, state.Entries[staging.ServiceSecret], 1)
	})

	t.Run("unstage all Secrets Manager", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v"), StagedAt: now})
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "secret1", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("s1"), StagedAt: now})
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "secret2", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("s2"), StagedAt: now})

		err := store.UnstageAll(t.Context(), staging.ServiceSecret)
		require.NoError(t, err)

		state, err := store.Load(t.Context())
		require.NoError(t, err)
		assert.Len(t, state.Entries[staging.ServiceParam], 1)
		assert.Empty(t, state.Entries[staging.ServiceSecret])
	})

	t.Run("unstage everything", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v"), StagedAt: now})
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "secret", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("s"), StagedAt: now})

		err := store.UnstageAll(t.Context(), "")
		require.NoError(t, err)

		state, err := store.Load(t.Context())
		require.NoError(t, err)
		assert.Empty(t, state.Entries[staging.ServiceParam])
		assert.Empty(t, state.Entries[staging.ServiceSecret])
	})
}

func TestStore_Get(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("test-value"),
		StagedAt:  now,
	})

	t.Run("get existing", func(t *testing.T) {
		t.Parallel()
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "test-value", lo.FromPtr(entry.Value))
	})

	t.Run("get not staged", func(t *testing.T) {
		t.Parallel()
		_, err := store.GetEntry(t.Context(), staging.ServiceParam, "/not/staged")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("get wrong service", func(t *testing.T) {
		t.Parallel()
		_, err := store.GetEntry(t.Context(), staging.ServiceSecret, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

func TestStore_List(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v1"), StagedAt: now})
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{Operation: staging.OperationDelete, StagedAt: now})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "secret1", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("s1"), StagedAt: now})

	t.Run("list SSM Parameter Store only", func(t *testing.T) {
		t.Parallel()
		result, err := store.ListEntries(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Len(t, result[staging.ServiceParam], 2)
	})

	t.Run("list Secrets Manager only", func(t *testing.T) {
		t.Parallel()
		result, err := store.ListEntries(t.Context(), staging.ServiceSecret)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Len(t, result[staging.ServiceSecret], 1)
	})

	t.Run("list all", func(t *testing.T) {
		t.Parallel()
		result, err := store.ListEntries(t.Context(), "")
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Len(t, result[staging.ServiceParam], 2)
		assert.Len(t, result[staging.ServiceSecret], 1)
	})
}

func TestStore_ListEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	result, err := store.ListEntries(t.Context(), "")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestStore_HasChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	// Initially no changes
	has, err := store.HasChanges(t.Context(), "")
	require.NoError(t, err)
	assert.False(t, has)

	has, err = store.HasChanges(t.Context(), staging.ServiceParam)
	require.NoError(t, err)
	assert.False(t, has)

	// Add SSM Parameter Store change
	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v"), StagedAt: now})

	has, err = store.HasChanges(t.Context(), "")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = store.HasChanges(t.Context(), staging.ServiceParam)
	require.NoError(t, err)
	assert.True(t, has)

	has, err = store.HasChanges(t.Context(), staging.ServiceSecret)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestStore_Count(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v1"), StagedAt: now})
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v2"), StagedAt: now})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "secret1", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("s1"), StagedAt: now})

	count, err := store.Count(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	count, err = store.Count(t.Context(), staging.ServiceParam)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	count, err = store.Count(t.Context(), staging.ServiceSecret)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestStore_UnknownService(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	unknownService := staging.Service("unknown")

	err := store.StageEntry(t.Context(), unknownService, "test", staging.Entry{Operation: staging.OperationUpdate})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	err = store.UnstageEntry(t.Context(), unknownService, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	err = store.UnstageAll(t.Context(), unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	_, err = store.GetEntry(t.Context(), unknownService, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	_, err = store.ListEntries(t.Context(), unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	_, err = store.HasChanges(t.Context(), unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	_, err = store.Count(t.Context(), unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")
}

func TestStore_CorruptedFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	err := os.WriteFile(path, []byte("not valid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	_, err = store.Load(t.Context())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_SaveRemovesEmptyFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")
	store := file.NewStoreWithPath(path)

	now := time.Now()

	// Stage and then unstage
	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr("v"), StagedAt: now})
	require.NoError(t, err)

	// File should exist
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Unstage
	err = store.UnstageEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)

	// File should be removed
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestStore_NilMapsInLoadedState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Write an old v1 state with null maps
	err := os.WriteFile(path, []byte(`{"version":1,"param":null,"secret":null}`), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	state, err := store.Load(t.Context())
	require.NoError(t, err)

	// Maps should be initialized (migrated to v2)
	assert.NotNil(t, state.Entries)
	assert.NotNil(t, state.Tags)
}

func TestStore_Save(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")
	store := file.NewStoreWithPath(path)

	state := &staging.State{
		Version: 2,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/app/config": {
					Operation: staging.OperationUpdate,
					Value:     lo.ToPtr("test"),
					StagedAt:  time.Now(),
				},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	err := store.Save(state)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Verify contents
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, state.Version, loaded.Version)
	assert.Equal(t, lo.FromPtr(state.Entries[staging.ServiceParam]["/app/config"].Value), lo.FromPtr(loaded.Entries[staging.ServiceParam]["/app/config"].Value))
}

func TestStore_DirectoryCreation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "nested", "staging.json")
	store := file.NewStoreWithPath(path)

	now := time.Now()
	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("test"),
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Verify directories were created
	_, err = os.Stat(filepath.Dir(path))
	require.NoError(t, err)
}

func TestStore_UnstageFromSecret(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage Secrets Manager entry
	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Unstage
	err = store.UnstageEntry(t.Context(), staging.ServiceSecret, "my-secret")
	require.NoError(t, err)

	// Verify removed
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_LoadReadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create a directory with the same name as the file
	err := os.Mkdir(path, 0o755)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	_, err = store.Load(t.Context())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read state file")
}

func TestStore_StageLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	err = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("test"),
		StagedAt:  time.Now(),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_UnstageLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	err = store.UnstageEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_UnstageAllLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	err = store.UnstageAll(t.Context(), staging.ServiceParam)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_GetLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_ListLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	_, err = store.ListEntries(t.Context(), staging.ServiceParam)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_HasChangesLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	_, err = store.HasChanges(t.Context(), staging.ServiceParam)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_CountLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	_, err = store.Count(t.Context(), staging.ServiceParam)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_GetSecret(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  now,
	})

	entry, err := store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", lo.FromPtr(entry.Value))
}

func TestStore_SaveWriteError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// Create a read-only directory to trigger write error
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.Mkdir(readOnlyDir, 0o500)
	require.NoError(t, err)

	path := filepath.Join(readOnlyDir, "subdir", "staging.json")
	store := file.NewStoreWithPath(path)

	state := &staging.State{
		Version: 2,
		Entries: map[staging.Service]map[string]staging.Entry{
			staging.ServiceParam: {
				"/app/config": {
					Operation: staging.OperationUpdate,
					Value:     lo.ToPtr("test"),
					StagedAt:  time.Now(),
				},
			},
			staging.ServiceSecret: {},
		},
		Tags: map[staging.Service]map[string]staging.TagEntry{
			staging.ServiceParam:  {},
			staging.ServiceSecret: {},
		},
	}

	err = store.Save(state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create state directory")
}

// =============================================================================
// Tag-related Tests
// =============================================================================

func TestStore_StageTag(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now().Truncate(time.Second)

	// Stage tag for SSM Parameter Store
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt: now,
	})
	require.NoError(t, err)

	// Stage tag for Secrets Manager
	err = store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Add:      map[string]string{"owner": "platform"},
		Remove:   map[string]struct{}{"deprecated": {}},
		StagedAt: now,
	})
	require.NoError(t, err)

	// Load and verify
	state, err := store.Load(t.Context())
	require.NoError(t, err)

	assert.Len(t, state.Tags[staging.ServiceParam], 1)
	assert.Equal(t, "prod", state.Tags[staging.ServiceParam]["/app/config"].Add["env"])
	assert.Equal(t, "backend", state.Tags[staging.ServiceParam]["/app/config"].Add["team"])

	assert.Len(t, state.Tags[staging.ServiceSecret], 1)
	assert.Equal(t, "platform", state.Tags[staging.ServiceSecret]["my-secret"].Add["owner"])
	assert.True(t, state.Tags[staging.ServiceSecret]["my-secret"].Remove.Contains("deprecated"))
}

func TestStore_StageTagOverwrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage initial
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "dev"},
		StagedAt: now,
	})
	require.NoError(t, err)

	// Stage overwrite
	err = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	})
	require.NoError(t, err)

	// Verify overwritten
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
}

func TestStore_StageTagLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	err = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_StageTagUnknownService(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	unknownService := staging.Service("unknown")

	err := store.StageTag(t.Context(), unknownService, "test", staging.TagEntry{
		Add:      map[string]string{"key": "value"},
		StagedAt: time.Now(),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")
}

func TestStore_GetTag(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	})

	t.Run("get existing", func(t *testing.T) {
		t.Parallel()
		tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "prod", tagEntry.Add["env"])
	})

	t.Run("get not staged", func(t *testing.T) {
		t.Parallel()
		_, err := store.GetTag(t.Context(), staging.ServiceParam, "/not/staged")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("get wrong service", func(t *testing.T) {
		t.Parallel()
		_, err := store.GetTag(t.Context(), staging.ServiceSecret, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

func TestStore_GetTagLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_GetTagUnknownService(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	unknownService := staging.Service("unknown")

	_, err := store.GetTag(t.Context(), unknownService, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")
}

func TestStore_UnstageTag(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	})
	require.NoError(t, err)

	// Unstage
	err = store.UnstageTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)

	// Verify removed
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_UnstageTagNotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	err := store.UnstageTag(t.Context(), staging.ServiceParam, "/not/staged")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	err = store.UnstageTag(t.Context(), staging.ServiceSecret, "not-staged")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_UnstageTagLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	err = store.UnstageTag(t.Context(), staging.ServiceParam, "/app/config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_UnstageTagUnknownService(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	unknownService := staging.Service("unknown")

	err := store.UnstageTag(t.Context(), unknownService, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")
}

func TestStore_UnstageTagFromSecret(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage Secrets Manager tag
	err := store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	})
	require.NoError(t, err)

	// Unstage
	err = store.UnstageTag(t.Context(), staging.ServiceSecret, "my-secret")
	require.NoError(t, err)

	// Verify removed
	_, err = store.GetTag(t.Context(), staging.ServiceSecret, "my-secret")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_ListTags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config1", staging.TagEntry{Add: map[string]string{"env": "prod"}, StagedAt: now})
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config2", staging.TagEntry{Add: map[string]string{"env": "dev"}, StagedAt: now})
	_ = store.StageTag(t.Context(), staging.ServiceSecret, "secret1", staging.TagEntry{Add: map[string]string{"owner": "platform"}, StagedAt: now})

	t.Run("list SSM Parameter Store only", func(t *testing.T) {
		t.Parallel()
		result, err := store.ListTags(t.Context(), staging.ServiceParam)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Len(t, result[staging.ServiceParam], 2)
	})

	t.Run("list Secrets Manager only", func(t *testing.T) {
		t.Parallel()
		result, err := store.ListTags(t.Context(), staging.ServiceSecret)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Len(t, result[staging.ServiceSecret], 1)
	})

	t.Run("list all", func(t *testing.T) {
		t.Parallel()
		result, err := store.ListTags(t.Context(), "")
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Len(t, result[staging.ServiceParam], 2)
		assert.Len(t, result[staging.ServiceSecret], 1)
	})
}

func TestStore_ListTagsEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	result, err := store.ListTags(t.Context(), "")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestStore_ListTagsLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := file.NewStoreWithPath(path)
	_, err = store.ListTags(t.Context(), staging.ServiceParam)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_ListTagsUnknownService(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	unknownService := staging.Service("unknown")

	_, err := store.ListTags(t.Context(), unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")
}

func TestStore_UnstageAllClearsTags(t *testing.T) {
	t.Parallel()

	t.Run("unstage all SSM Parameter Store clears tags too", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		// Stage multiple tags
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config1", staging.TagEntry{Add: map[string]string{"env": "prod"}, StagedAt: now})
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config2", staging.TagEntry{Add: map[string]string{"env": "dev"}, StagedAt: now})
		_ = store.StageTag(t.Context(), staging.ServiceSecret, "secret1", staging.TagEntry{Add: map[string]string{"owner": "platform"}, StagedAt: now})

		// Unstage all SSM Parameter Store (entries AND tags)
		err := store.UnstageAll(t.Context(), staging.ServiceParam)
		require.NoError(t, err)

		// Verify SSM Parameter Store tags cleared, Secrets Manager intact
		state, err := store.Load(t.Context())
		require.NoError(t, err)
		assert.Empty(t, state.Tags[staging.ServiceParam])
		assert.Len(t, state.Tags[staging.ServiceSecret], 1)
	})

	t.Run("unstage all Secrets Manager clears tags too", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{Add: map[string]string{"env": "prod"}, StagedAt: now})
		_ = store.StageTag(t.Context(), staging.ServiceSecret, "secret1", staging.TagEntry{Add: map[string]string{"owner": "platform"}, StagedAt: now})
		_ = store.StageTag(t.Context(), staging.ServiceSecret, "secret2", staging.TagEntry{Add: map[string]string{"owner": "backend"}, StagedAt: now})

		err := store.UnstageAll(t.Context(), staging.ServiceSecret)
		require.NoError(t, err)

		state, err := store.Load(t.Context())
		require.NoError(t, err)
		assert.Len(t, state.Tags[staging.ServiceParam], 1)
		assert.Empty(t, state.Tags[staging.ServiceSecret])
	})

	t.Run("unstage all clears all tags", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := file.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{Add: map[string]string{"env": "prod"}, StagedAt: now})
		_ = store.StageTag(t.Context(), staging.ServiceSecret, "secret", staging.TagEntry{Add: map[string]string{"owner": "platform"}, StagedAt: now})

		err := store.UnstageAll(t.Context(), "")
		require.NoError(t, err)

		state, err := store.Load(t.Context())
		require.NoError(t, err)
		assert.Empty(t, state.Tags[staging.ServiceParam])
		assert.Empty(t, state.Tags[staging.ServiceSecret])
	})
}

func TestStore_SaveRemovesEmptyFileWithOnlyTags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")
	store := file.NewStoreWithPath(path)

	now := time.Now()

	// Stage tag and then unstage
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	})
	require.NoError(t, err)

	// File should exist
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Unstage
	err = store.UnstageTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)

	// File should be removed
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}
