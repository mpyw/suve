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
	"github.com/mpyw/suve/internal/staging/file"
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

// =============================================================================
// Tag Tests
// =============================================================================

func TestTagUseCase_Tag_NewTagEntry(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	output, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"env": "prod", "team": "backend"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)

	// Verify staged as tag entry
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
	assert.Equal(t, "backend", tagEntry.Add["team"])
	assert.NotNil(t, tagEntry.BaseModifiedAt)
}

func TestTagUseCase_Tag_MergeWithExisting(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Pre-stage a tag entry
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:            map[string]string{"existing": "tag"},
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"new": "tag"},
	})
	require.NoError(t, err)

	// Verify merged
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "tag", tagEntry.Add["existing"]) // Existing tag preserved
	assert.Equal(t, "tag", tagEntry.Add["new"])      // New tag added
	assert.Equal(t, baseTime, *tagEntry.BaseModifiedAt)
}

func TestTagUseCase_Tag_AddTagRemovesFromUntagList(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with remove tags
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("env", "team"),
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Add a tag that was previously in remove list
	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
	assert.False(t, tagEntry.Remove.Contains("env")) // "env" removed from remove list
	assert.True(t, tagEntry.Remove.Contains("team")) // "team" still in remove list
}

func TestTagUseCase_Tag_ParseError(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.parseErr = errors.New("invalid name")

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "invalid",
		Tags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid name")
}

func TestTagUseCase_Tag_FetchError(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.fetchErr = errors.New("aws error")

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aws error")
}

func TestTagUseCase_Tag_StageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.stageTagErr = errors.New("stage error")

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestTagUseCase_Tag_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getTagErr = errors.New("get error")

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get error")
}

func TestTagUseCase_Tag_ZeroLastModified(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Time{}, // Zero time
	}

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Nil(t, tagEntry.BaseModifiedAt)
}

func TestTagUseCase_Tag_StagedForCreate_ResourceNotFound(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// First, stage an entry for CREATE (new resource that doesn't exist in AWS)
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))

	// Create strategy that returns not-found (resource doesn't exist on AWS)
	strategy := newMockTagStrategy()
	strategy.fetchErr = &staging.ResourceNotFoundError{Err: errors.New("resource not found")}

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Tag the staged CREATE entry - should succeed despite resource not existing on AWS
	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/new-param",
		Tags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	// Verify tag was staged
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/new-param")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
	assert.Nil(t, tagEntry.BaseModifiedAt) // No base time since resource doesn't exist
}

func TestTagUseCase_Tag_EmptyTags(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Execute with empty tags - should error
	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tags specified")
}

func TestTagUseCase_Tag_BlockedOnDelete(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage DELETE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Attempting to tag a DELETE should fail
	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"env": "prod"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staged for deletion")
}

func TestTagUseCase_Tag_GetEntryError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("get entry error")

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Tag(t.Context(), usecasestaging.TagInput{
		Name: "/app/config",
		Tags: map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get entry error")
}

// =============================================================================
// Untag Tests
// =============================================================================

func TestTagUseCase_Untag_Success(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	output, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("old-tag", "deprecated"),
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)

	// Verify staged
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.True(t, tagEntry.Remove.Contains("old-tag"))
	assert.True(t, tagEntry.Remove.Contains("deprecated"))
}

func TestTagUseCase_Untag_RemoveTagDeletesFromAddList(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with tags to add
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Remove a tag that was previously in add list
	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.NotContains(t, tagEntry.Add, "env")       // "env" removed from add list
	assert.Equal(t, "backend", tagEntry.Add["team"]) // "team" still in add list
	assert.True(t, tagEntry.Remove.Contains("env"))  // "env" added to remove list
}

func TestTagUseCase_Untag_DuplicateRemoveKeys(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage with remove key
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("env"),
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Try to remove the same key again
	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	// Set guarantees uniqueness - "env" should be in the set exactly once
	assert.True(t, tagEntry.Remove.Contains("env"))
	assert.Equal(t, 1, tagEntry.Remove.Len())
}

func TestTagUseCase_Untag_ParseError(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.parseErr = errors.New("invalid name")

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "invalid",
		TagKeys: maputil.NewSet("env"),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid name")
}

func TestTagUseCase_Untag_GetEntryError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("get entry error")

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get entry error")
}

func TestTagUseCase_Untag_GetTagError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getTagErr = errors.New("get tag error")

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get tag error")
}

func TestTagUseCase_Untag_FetchError(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.fetchErr = errors.New("aws error")

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aws error")
}

func TestTagUseCase_Untag_BlockedOnDelete(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Pre-stage DELETE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Attempting to untag a DELETE should fail
	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staged for deletion")
}

func TestTagUseCase_Untag_StagedForCreate_WithExistingTags(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage an entry for CREATE
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))

	// Also stage some tags for this CREATE entry
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/new-param", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt: time.Now(),
	}))

	// Strategy that returns not-found (resource doesn't exist on AWS)
	strategy := newMockTagStrategy()
	strategy.fetchErr = &staging.ResourceNotFoundError{Err: errors.New("resource not found")}

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Untag "env" from the staged CREATE entry - should succeed despite resource not existing
	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/new-param",
		TagKeys: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	// Verify "env" was removed from ToSet, "team" remains
	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/new-param")
	require.NoError(t, err)
	assert.NotContains(t, tagEntry.Add, "env")
	assert.Contains(t, tagEntry.Add, "team")
	// ToUnset should be empty since resource doesn't exist on AWS
	assert.Empty(t, tagEntry.Remove)
}

func TestTagUseCase_Untag_StagedForCreate_AutoSkip(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage an entry for CREATE without any pre-staged tags
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))

	// Strategy that returns not-found (resource doesn't exist on AWS)
	strategy := newMockTagStrategy()
	strategy.fetchErr = &staging.ResourceNotFoundError{Err: errors.New("resource not found")}

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Untag "env" - auto-skipped since tag doesn't exist
	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/new-param",
		TagKeys: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	// Verify nothing was staged (auto-skipped)
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/new-param")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestTagUseCase_Untag_EmptyTagKeys(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Execute with empty tag keys - should error
	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet[string](),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tag keys specified")
}

func TestTagUseCase_Untag_StageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.stageTagErr = errors.New("stage error")

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestTagUseCase_Untag_PreservesBaseModifiedAt(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Pre-stage a tag entry with BaseModifiedAt
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:            map[string]string{"existing": "tag"},
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, baseTime, *tagEntry.BaseModifiedAt)
}

func TestTagUseCase_Untag_ZeroLastModified(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockTagStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Time{}, // Zero time
	}

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Untag(t.Context(), usecasestaging.UntagInput{
		Name:    "/app/config",
		TagKeys: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Nil(t, tagEntry.BaseModifiedAt)
}
