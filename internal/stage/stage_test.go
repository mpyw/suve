package stage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/stage"
)

func TestNewStore(t *testing.T) {
	t.Parallel()

	store, err := stage.NewStore()
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestStore_LoadEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	state, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, 1, state.Version)
	assert.Empty(t, state.SSM)
	assert.Empty(t, state.SM)
}

func TestStore_StageAndLoad(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now().Truncate(time.Second)

	// Stage SSM entry
	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "test-value",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Stage SM entry
	err = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Load and verify
	state, err := store.Load()
	require.NoError(t, err)

	assert.Len(t, state.SSM, 1)
	assert.Equal(t, stage.OperationSet, state.SSM["/app/config"].Operation)
	assert.Equal(t, "test-value", state.SSM["/app/config"].Value)

	assert.Len(t, state.SM, 1)
	assert.Equal(t, stage.OperationDelete, state.SM["my-secret"].Operation)
}

func TestStore_StageOverwrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()

	// Stage initial
	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "initial",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Stage overwrite
	err = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "updated",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Verify overwritten
	entry, err := store.Get(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "updated", entry.Value)
}

func TestStore_Unstage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()

	// Stage
	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "test",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Unstage
	err = store.Unstage(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)

	// Verify removed
	_, err = store.Get(stage.ServiceSSM, "/app/config")
	assert.ErrorIs(t, err, stage.ErrNotStaged)
}

func TestStore_UnstageNotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Unstage(stage.ServiceSSM, "/not/staged")
	assert.ErrorIs(t, err, stage.ErrNotStaged)

	err = store.Unstage(stage.ServiceSM, "not-staged")
	assert.ErrorIs(t, err, stage.ErrNotStaged)
}

func TestStore_UnstageAll(t *testing.T) {
	t.Parallel()

	t.Run("unstage all SSM", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		now := time.Now()

		// Stage multiple
		_ = store.Stage(stage.ServiceSSM, "/app/config1", stage.Entry{Operation: stage.OperationSet, Value: "v1", StagedAt: now})
		_ = store.Stage(stage.ServiceSSM, "/app/config2", stage.Entry{Operation: stage.OperationSet, Value: "v2", StagedAt: now})
		_ = store.Stage(stage.ServiceSM, "secret1", stage.Entry{Operation: stage.OperationSet, Value: "s1", StagedAt: now})

		// Unstage all SSM
		err := store.UnstageAll(stage.ServiceSSM)
		require.NoError(t, err)

		// Verify SSM cleared, SM intact
		state, err := store.Load()
		require.NoError(t, err)
		assert.Empty(t, state.SSM)
		assert.Len(t, state.SM, 1)
	})

	t.Run("unstage all SM", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		now := time.Now()

		_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{Operation: stage.OperationSet, Value: "v", StagedAt: now})
		_ = store.Stage(stage.ServiceSM, "secret1", stage.Entry{Operation: stage.OperationSet, Value: "s1", StagedAt: now})
		_ = store.Stage(stage.ServiceSM, "secret2", stage.Entry{Operation: stage.OperationSet, Value: "s2", StagedAt: now})

		err := store.UnstageAll(stage.ServiceSM)
		require.NoError(t, err)

		state, err := store.Load()
		require.NoError(t, err)
		assert.Len(t, state.SSM, 1)
		assert.Empty(t, state.SM)
	})

	t.Run("unstage everything", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		now := time.Now()

		_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{Operation: stage.OperationSet, Value: "v", StagedAt: now})
		_ = store.Stage(stage.ServiceSM, "secret", stage.Entry{Operation: stage.OperationSet, Value: "s", StagedAt: now})

		err := store.UnstageAll("")
		require.NoError(t, err)

		state, err := store.Load()
		require.NoError(t, err)
		assert.Empty(t, state.SSM)
		assert.Empty(t, state.SM)
	})
}

func TestStore_Get(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()

	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "test-value",
		StagedAt:  now,
	})

	t.Run("get existing", func(t *testing.T) {
		t.Parallel()
		entry, err := store.Get(stage.ServiceSSM, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "test-value", entry.Value)
	})

	t.Run("get not staged", func(t *testing.T) {
		t.Parallel()
		_, err := store.Get(stage.ServiceSSM, "/not/staged")
		assert.ErrorIs(t, err, stage.ErrNotStaged)
	})

	t.Run("get wrong service", func(t *testing.T) {
		t.Parallel()
		_, err := store.Get(stage.ServiceSM, "/app/config")
		assert.ErrorIs(t, err, stage.ErrNotStaged)
	})
}

func TestStore_List(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()

	_ = store.Stage(stage.ServiceSSM, "/app/config1", stage.Entry{Operation: stage.OperationSet, Value: "v1", StagedAt: now})
	_ = store.Stage(stage.ServiceSSM, "/app/config2", stage.Entry{Operation: stage.OperationDelete, StagedAt: now})
	_ = store.Stage(stage.ServiceSM, "secret1", stage.Entry{Operation: stage.OperationSet, Value: "s1", StagedAt: now})

	t.Run("list SSM only", func(t *testing.T) {
		t.Parallel()
		result, err := store.List(stage.ServiceSSM)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Len(t, result[stage.ServiceSSM], 2)
	})

	t.Run("list SM only", func(t *testing.T) {
		t.Parallel()
		result, err := store.List(stage.ServiceSM)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Len(t, result[stage.ServiceSM], 1)
	})

	t.Run("list all", func(t *testing.T) {
		t.Parallel()
		result, err := store.List("")
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Len(t, result[stage.ServiceSSM], 2)
		assert.Len(t, result[stage.ServiceSM], 1)
	})
}

func TestStore_ListEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	result, err := store.List("")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestStore_HasChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Initially no changes
	has, err := store.HasChanges("")
	require.NoError(t, err)
	assert.False(t, has)

	has, err = store.HasChanges(stage.ServiceSSM)
	require.NoError(t, err)
	assert.False(t, has)

	// Add SSM change
	now := time.Now()
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{Operation: stage.OperationSet, Value: "v", StagedAt: now})

	has, err = store.HasChanges("")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = store.HasChanges(stage.ServiceSSM)
	require.NoError(t, err)
	assert.True(t, has)

	has, err = store.HasChanges(stage.ServiceSM)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestStore_Count(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()

	_ = store.Stage(stage.ServiceSSM, "/app/config1", stage.Entry{Operation: stage.OperationSet, Value: "v1", StagedAt: now})
	_ = store.Stage(stage.ServiceSSM, "/app/config2", stage.Entry{Operation: stage.OperationSet, Value: "v2", StagedAt: now})
	_ = store.Stage(stage.ServiceSM, "secret1", stage.Entry{Operation: stage.OperationSet, Value: "s1", StagedAt: now})

	count, err := store.Count("")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	count, err = store.Count(stage.ServiceSSM)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	count, err = store.Count(stage.ServiceSM)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestStore_UnknownService(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	unknownService := stage.Service("unknown")

	err := store.Stage(unknownService, "test", stage.Entry{Operation: stage.OperationSet})
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
	path := filepath.Join(tmpDir, "stage.json")

	err := os.WriteFile(path, []byte("not valid json"), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	_, err = store.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_SaveRemovesEmptyFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")
	store := stage.NewStoreWithPath(path)

	now := time.Now()

	// Stage and then unstage
	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{Operation: stage.OperationSet, Value: "v", StagedAt: now})
	require.NoError(t, err)

	// File should exist
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Unstage
	err = store.Unstage(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)

	// File should be removed
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestStore_NilMapsInLoadedState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Write a state with null maps
	err := os.WriteFile(path, []byte(`{"version":1,"ssm":null,"sm":null}`), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	state, err := store.Load()
	require.NoError(t, err)

	// Maps should be initialized
	assert.NotNil(t, state.SSM)
	assert.NotNil(t, state.SM)
}

func TestStore_Save(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")
	store := stage.NewStoreWithPath(path)

	state := &stage.State{
		Version: 1,
		SSM: map[string]stage.Entry{
			"/app/config": {
				Operation: stage.OperationSet,
				Value:     "test",
				StagedAt:  time.Now(),
			},
		},
		SM: make(map[string]stage.Entry),
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
	assert.Equal(t, state.SSM["/app/config"].Value, loaded.SSM["/app/config"].Value)
}

func TestStore_DirectoryCreation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "nested", "stage.json")
	store := stage.NewStoreWithPath(path)

	now := time.Now()
	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
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
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()

	// Stage SM entry
	err := store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "secret-value",
		StagedAt:  now,
	})
	require.NoError(t, err)

	// Unstage
	err = store.Unstage(stage.ServiceSM, "my-secret")
	require.NoError(t, err)

	// Verify removed
	_, err = store.Get(stage.ServiceSM, "my-secret")
	assert.ErrorIs(t, err, stage.ErrNotStaged)
}

func TestStore_LoadReadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create a directory with the same name as the file
	err := os.Mkdir(path, 0o755)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	_, err = store.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read state file")
}

func TestStore_StageLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	err = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "test",
		StagedAt:  time.Now(),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_UnstageLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	err = store.Unstage(stage.ServiceSSM, "/app/config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_UnstageAllLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	err = store.UnstageAll(stage.ServiceSSM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_GetLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	_, err = store.Get(stage.ServiceSSM, "/app/config")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_ListLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	_, err = store.List(stage.ServiceSSM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_HasChangesLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	_, err = store.HasChanges(stage.ServiceSSM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_CountLoadError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := stage.NewStoreWithPath(path)
	_, err = store.Count(stage.ServiceSSM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse state file")
}

func TestStore_GetSM(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()

	_ = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "secret-value",
		StagedAt:  now,
	})

	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", entry.Value)
}
