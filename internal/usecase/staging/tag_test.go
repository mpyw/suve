package staging_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	usecasestaging "github.com/mpyw/suve/internal/usecase/staging"
)

type mockTagStrategy struct {
	*mockParser
	fetchResult *staging.EditFetchResult
	fetchErr    error
}

func (m *mockTagStrategy) FetchCurrentValue(_ context.Context, _ string) (*staging.EditFetchResult, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	return m.fetchResult, nil
}

func newMockTagStrategy() *mockTagStrategy {
	return &mockTagStrategy{
		mockParser: newMockParser(),
		fetchResult: &staging.EditFetchResult{
			Value:        "aws-value",
			LastModified: time.Now(),
		},
	}
}

func TestTagUseCase_Execute_NewEntry(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"env": "prod", "team": "backend"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)

	// Verify staged
	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Nil(t, entry.Value) // Value should be nil for tag-only changes
	assert.Equal(t, "prod", entry.Tags["env"])
	assert.Equal(t, "backend", entry.Tags["team"])
	assert.NotNil(t, entry.BaseModifiedAt)
}

func TestTagUseCase_Execute_RemoveTags(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:       "/app/config",
		RemoveTags: maputil.NewSet("old-tag", "deprecated"),
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)

	// Verify staged
	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.True(t, entry.UntagKeys.Contains("old-tag"))
	assert.True(t, entry.UntagKeys.Contains("deprecated"))
}

func TestTagUseCase_Execute_MergeWithExisting(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Pre-stage an entry with a value
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("existing-value"),
		Tags:           map[string]string{"existing": "tag"},
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"new": "tag"},
	})
	require.NoError(t, err)

	// Verify merged
	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "existing-value", lo.FromPtr(entry.Value)) // Value preserved
	assert.Equal(t, "tag", entry.Tags["existing"])             // Existing tag preserved
	assert.Equal(t, "tag", entry.Tags["new"])                  // New tag added
	assert.Equal(t, baseTime, *entry.BaseModifiedAt)           // BaseModifiedAt preserved
}

func TestTagUseCase_Execute_AddTagRemovesFromUntagList(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with untag keys
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		UntagKeys: maputil.NewSet("env", "team"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Add a tag that was previously in untag list
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", entry.Tags["env"])
	assert.False(t, entry.UntagKeys.Contains("env")) // "env" removed from untag list
	assert.True(t, entry.UntagKeys.Contains("team")) // "team" still in untag list
}

func TestTagUseCase_Execute_RemoveTagDeletesFromAddList(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with tags to add
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Tags:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Remove a tag that was previously in add list
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:       "/app/config",
		RemoveTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.NotContains(t, entry.Tags, "env")        // "env" removed from tags
	assert.Equal(t, "backend", entry.Tags["team"])  // "team" still in tags
	assert.True(t, entry.UntagKeys.Contains("env")) // "env" added to untag list
}

func TestTagUseCase_Execute_ParseError(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.parseErr = errors.New("invalid name")

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "invalid",
		AddTags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid name")
}

func TestTagUseCase_Execute_FetchError(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.fetchErr = errors.New("aws error")

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aws error")
}

func TestTagUseCase_Execute_StageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.stageErr = errors.New("stage error")

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestTagUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("get error")

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get error")
}

func TestTagUseCase_Execute_ZeroLastModified(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Time{}, // Zero time
	}

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Nil(t, entry.BaseModifiedAt)
}

func TestTagUseCase_Execute_DuplicateUntagKeys(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with untag key
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		UntagKeys: maputil.NewSet("env"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Try to remove the same key again
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:       "/app/config",
		RemoveTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	// Set guarantees uniqueness - "env" should be in the set exactly once
	assert.True(t, entry.UntagKeys.Contains("env"))
	assert.Equal(t, 1, entry.UntagKeys.Len())
}

func TestTagUseCase_Execute_CancelAddTags(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with tags to add
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Tags:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Cancel a staged tag addition
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:          "/app/config",
		CancelAddTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.NotContains(t, entry.Tags, "env")         // "env" removed from tags
	assert.Equal(t, "backend", entry.Tags["team"])   // "team" still in tags
	assert.False(t, entry.UntagKeys.Contains("env")) // "env" NOT added to untag list (cancel, not remove)
}

func TestTagUseCase_Execute_CancelRemoveTags(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with untag keys
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		UntagKeys: maputil.NewSet("env", "team"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Cancel a staged tag removal
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:             "/app/config",
		CancelRemoveTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.False(t, entry.UntagKeys.Contains("env")) // "env" removed from untag list
	assert.True(t, entry.UntagKeys.Contains("team")) // "team" still in untag list
	assert.NotContains(t, entry.Tags, "env")         // "env" NOT added to tags (cancel, not add)
}

func TestTagUseCase_Execute_CancelAllTags_UnstagesEntry(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with only a tag (no value, no description)
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Tags:      map[string]string{"env": "prod"},
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Cancel the only staged tag
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:          "/app/config",
		CancelAddTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	// Entry should be completely unstaged since there's nothing left
	_, err = store.Get(staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestTagUseCase_Execute_CancelAllUntags_UnstagesEntry(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with only untag keys (no value, no description, no tags)
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		UntagKeys: maputil.NewSet("env"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Cancel the only staged untag
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:             "/app/config",
		CancelRemoveTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	// Entry should be completely unstaged since there's nothing left
	_, err = store.Get(staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestTagUseCase_Execute_CancelWithValueRemains(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with value AND tag
	require.NoError(t, store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("some-value"),
		Tags:      map[string]string{"env": "prod"},
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Cancel the tag, but value should keep entry staged
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:          "/app/config",
		CancelAddTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	// Entry should still be staged because value exists
	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "some-value", lo.FromPtr(entry.Value))
	assert.Empty(t, entry.Tags)
}
