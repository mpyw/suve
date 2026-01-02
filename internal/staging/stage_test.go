package staging_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
)

func TestNewStore(t *testing.T) {
	t.Parallel()

	store, err := staging.NewStore()
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_LoadEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	state, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, 1, state.Version)
	assert.Empty(t, state.Param)
	assert.Empty(t, state.Secret)
}

func TestStore_StageAndLoad(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now().Truncate(time.Second)

	// Stage SSM Parameter Store entry
	err := store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "test-value",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Stage Secrets Manager entry
	err = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Load and verify
	state, err := store.Load()
	require.NoError(t, err)

	assert.Len(t, state.Param, 1)
	assert.Equal(t, staging.OperationUpdate, state.Param["/app/config"].Operation)
	assert.Equal(t, "test-value", state.Param["/app/config"].Value)

	assert.Len(t, state.Secret, 1)
	assert.Equal(t, staging.OperationDelete, state.Secret["my-secret"].Operation)
}

func TestStore_StageOverwrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage initial
	err := store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "initial",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Stage overwrite
	err = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "updated",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Verify overwritten
	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "updated", entry.Value)
}

func TestStore_Unstage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage
	err := store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "test",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Unstage
	err = store.Unstage(staging.ServiceParam, "/app/config")
	require.NoError(t, err)

	// Verify removed
	_, err = store.Get(staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_UnstageNotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	err := store.Unstage(staging.ServiceParam, "/not/staged")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	err = store.Unstage(staging.ServiceSecret, "not-staged")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_UnstageAll(t *testing.T) {
	t.Parallel()

	t.Run("unstage all SSM Parameter Store", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		// Stage multiple
		_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{Operation: staging.OperationUpdate, Value: "v1", StagedAt: now})
		_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{Operation: staging.OperationUpdate, Value: "v2", StagedAt: now})
		_ = store.Stage(staging.ServiceSecret, "secret1", staging.Entry{Operation: staging.OperationUpdate, Value: "s1", StagedAt: now})

		// Unstage all SSM Parameter Store
		err := store.UnstageAll(staging.ServiceParam)
		require.NoError(t, err)

		// Verify SSM Parameter Store cleared, Secrets Manager intact
		state, err := store.Load()
		require.NoError(t, err)
		assert.Empty(t, state.Param)
		assert.Len(t, state.Secret, 1)
	})

	t.Run("unstage all Secrets Manager", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{Operation: staging.OperationUpdate, Value: "v", StagedAt: now})
		_ = store.Stage(staging.ServiceSecret, "secret1", staging.Entry{Operation: staging.OperationUpdate, Value: "s1", StagedAt: now})
		_ = store.Stage(staging.ServiceSecret, "secret2", staging.Entry{Operation: staging.OperationUpdate, Value: "s2", StagedAt: now})

		err := store.UnstageAll(staging.ServiceSecret)
		require.NoError(t, err)

		state, err := store.Load()
		require.NoError(t, err)
		assert.Len(t, state.Param, 1)
		assert.Empty(t, state.Secret)
	})

	t.Run("unstage everything", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

		now := time.Now()

		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{Operation: staging.OperationUpdate, Value: "v", StagedAt: now})
		_ = store.Stage(staging.ServiceSecret, "secret", staging.Entry{Operation: staging.OperationUpdate, Value: "s", StagedAt: now})

		err := store.UnstageAll("")
		require.NoError(t, err)

		state, err := store.Load()
		require.NoError(t, err)
		assert.Empty(t, state.Param)
		assert.Empty(t, state.Secret)
	})
}

func TestStore_Get(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "test-value",
		StagedAt:  now,
	})

	t.Run("get existing", func(t *testing.T) {
		t.Parallel()
		entry, err := store.Get(staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "test-value", entry.Value)
	})

	t.Run("get not staged", func(t *testing.T) {
		t.Parallel()
		_, err := store.Get(staging.ServiceParam, "/not/staged")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("get wrong service", func(t *testing.T) {
		t.Parallel()
		_, err := store.Get(staging.ServiceSecret, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

func TestStore_List(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{Operation: staging.OperationUpdate, Value: "v1", StagedAt: now})
	_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{Operation: staging.OperationDelete, StagedAt: now})
	_ = store.Stage(staging.ServiceSecret, "secret1", staging.Entry{Operation: staging.OperationUpdate, Value: "s1", StagedAt: now})

	t.Run("list SSM Parameter Store only", func(t *testing.T) {
		t.Parallel()
		result, err := store.List(staging.ServiceParam)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Len(t, result[staging.ServiceParam], 2)
	})

	t.Run("list Secrets Manager only", func(t *testing.T) {
		t.Parallel()
		result, err := store.List(staging.ServiceSecret)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Len(t, result[staging.ServiceSecret], 1)
	})

	t.Run("list all", func(t *testing.T) {
		t.Parallel()
		result, err := store.List("")
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Len(t, result[staging.ServiceParam], 2)
		assert.Len(t, result[staging.ServiceSecret], 1)
	})
}

func TestStore_ListEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	result, err := store.List("")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestStore_HasChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	// Initially no changes
	has, err := store.HasChanges("")
	require.NoError(t, err)
	assert.False(t, has)

	has, err = store.HasChanges(staging.ServiceParam)
	require.NoError(t, err)
	assert.False(t, has)

	// Add SSM Parameter Store change
	now := time.Now()
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{Operation: staging.OperationUpdate, Value: "v", StagedAt: now})

	has, err = store.HasChanges("")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = store.HasChanges(staging.ServiceParam)
	require.NoError(t, err)
	assert.True(t, has)

	has, err = store.HasChanges(staging.ServiceSecret)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestStore_Count(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{Operation: staging.OperationUpdate, Value: "v1", StagedAt: now})
	_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{Operation: staging.OperationUpdate, Value: "v2", StagedAt: now})
	_ = store.Stage(staging.ServiceSecret, "secret1", staging.Entry{Operation: staging.OperationUpdate, Value: "s1", StagedAt: now})

	count, err := store.Count("")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	count, err = store.Count(staging.ServiceParam)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	count, err = store.Count(staging.ServiceSecret)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestStore_UnknownService(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	unknownService := staging.Service("unknown")

	err := store.Stage(unknownService, "test", staging.Entry{Operation: staging.OperationUpdate})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	err = store.Unstage(unknownService, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	err = store.UnstageAll(unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	_, err = store.Get(unknownService, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	_, err = store.List(unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	_, err = store.HasChanges(unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")

	_, err = store.Count(unknownService)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown service")
}

func TestStore_CorruptedFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	err := os.WriteFile(path, []byte("not valid json"), 0o600)
	require.NoError(t, err)

	store := staging.NewStoreWithPath(path)
	_, err = store.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_SaveRemovesEmptyFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")
	store := staging.NewStoreWithPath(path)

	now := time.Now()

	// Stage and then unstage
	err := store.Stage(staging.ServiceParam, "/app/config", staging.Entry{Operation: staging.OperationUpdate, Value: "v", StagedAt: now})
	require.NoError(t, err)

	// File should exist
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Unstage
	err = store.Unstage(staging.ServiceParam, "/app/config")
	require.NoError(t, err)

	// File should be removed
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestStore_NilMapsInLoadedState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Write a state with null maps
	err := os.WriteFile(path, []byte(`{"version":1,"param":null,"secret":null}`), 0o600)
	require.NoError(t, err)

	store := staging.NewStoreWithPath(path)
	state, err := store.Load()
	require.NoError(t, err)

	// Maps should be initialized
	assert.NotNil(t, state.Param)
	assert.NotNil(t, state.Secret)
}

func TestStore_Save(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")
	store := staging.NewStoreWithPath(path)

	state := &staging.State{
		Version: 1,
		Param: map[string]staging.Entry{
			"/app/config": {
				Operation: staging.OperationUpdate,
				Value:     "test",
				StagedAt:  time.Now(),
			},
		},
		Secret: make(map[string]staging.Entry),
	}

	err := store.Save(state)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Verify contents
	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, state.Version, loaded.Version)
	assert.Equal(t, state.Param["/app/config"].Value, loaded.Param["/app/config"].Value)
}

func TestStore_DirectoryCreation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "nested", "staging.json")
	store := staging.NewStoreWithPath(path)

	now := time.Now()
	err := store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "test",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Verify directories were created
	_, err = os.Stat(filepath.Dir(path))
	require.NoError(t, err)
}

func TestStore_UnstageFromSM(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	// Stage Secrets Manager entry
	err := store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "secret-value",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Unstage
	err = store.Unstage(staging.ServiceSecret, "my-secret")
	require.NoError(t, err)

	// Verify removed
	_, err = store.Get(staging.ServiceSecret, "my-secret")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_LoadReadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "staging.json")

	// Create a directory with the same name as the file
	err := os.Mkdir(path, 0o755)
	require.NoError(t, err)

	store := staging.NewStoreWithPath(path)
	_, err = store.Load()
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

	store := staging.NewStoreWithPath(path)
	err = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "test",
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

	store := staging.NewStoreWithPath(path)
	err = store.Unstage(staging.ServiceParam, "/app/config")
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

	store := staging.NewStoreWithPath(path)
	err = store.UnstageAll(staging.ServiceParam)
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

	store := staging.NewStoreWithPath(path)
	_, err = store.Get(staging.ServiceParam, "/app/config")
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

	store := staging.NewStoreWithPath(path)
	_, err = store.List(staging.ServiceParam)
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

	store := staging.NewStoreWithPath(path)
	_, err = store.HasChanges(staging.ServiceParam)
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

	store := staging.NewStoreWithPath(path)
	_, err = store.Count(staging.ServiceParam)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_GetSM(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "staging.json"))

	now := time.Now()

	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "secret-value",
		StagedAt:  now,
	})

	entry, err := store.Get(staging.ServiceSecret, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", entry.Value)
}

func TestStore_SaveWriteError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// Create a read-only directory to trigger write error
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.Mkdir(readOnlyDir, 0o500)
	require.NoError(t, err)

	path := filepath.Join(readOnlyDir, "subdir", "staging.json")
	store := staging.NewStoreWithPath(path)

	state := &staging.State{
		Version: 1,
		Param: map[string]staging.Entry{
			"/app/config": {
				Operation: staging.OperationUpdate,
				Value:     "test",
				StagedAt:  time.Now(),
			},
		},
		Secret: make(map[string]staging.Entry),
	}

	err = store.Save(state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create state directory")
}
