package staging_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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

func TestTagUseCase_Execute_NewTagEntry(t *testing.T) {
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

	// Verify staged as tag entry
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
	assert.Equal(t, "backend", tagEntry.Add["team"])
	assert.NotNil(t, tagEntry.BaseModifiedAt)
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
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.True(t, tagEntry.Remove.Contains("old-tag"))
	assert.True(t, tagEntry.Remove.Contains("deprecated"))
}

func TestTagUseCase_Execute_MergeWithExisting(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Pre-stage a tag entry
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:            map[string]string{"existing": "tag"},
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
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "tag", tagEntry.Add["existing"]) // Existing tag preserved
	assert.Equal(t, "tag", tagEntry.Add["new"])      // New tag added
	assert.Equal(t, baseTime, *tagEntry.BaseModifiedAt)
}

func TestTagUseCase_Execute_AddTagRemovesFromUntagList(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with remove tags
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("env", "team"),
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Add a tag that was previously in remove list
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
	assert.False(t, tagEntry.Remove.Contains("env")) // "env" removed from remove list
	assert.True(t, tagEntry.Remove.Contains("team")) // "team" still in remove list
}

func TestTagUseCase_Execute_RemoveTagDeletesFromAddList(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with tags to add
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt: time.Now(),
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

	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.NotContains(t, tagEntry.Add, "env")       // "env" removed from add list
	assert.Equal(t, "backend", tagEntry.Add["team"]) // "team" still in add list
	assert.True(t, tagEntry.Remove.Contains("env"))  // "env" added to remove list
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
	store.stageTagErr = errors.New("stage error")

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
	store.getTagErr = errors.New("get error")

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

	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Nil(t, tagEntry.BaseModifiedAt)
}

func TestTagUseCase_Execute_DuplicateRemoveKeys(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with remove key
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("env"),
		StagedAt: time.Now(),
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

	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	// Set guarantees uniqueness - "env" should be in the set exactly once
	assert.True(t, tagEntry.Remove.Contains("env"))
	assert.Equal(t, 1, tagEntry.Remove.Len())
}

func TestTagUseCase_Execute_AddAndRemoveSameKey(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Add and remove the same key - add wins
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:       "/app/config",
		AddTags:    map[string]string{"env": "prod"},
		RemoveTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
	assert.False(t, tagEntry.Remove.Contains("env"))
}
