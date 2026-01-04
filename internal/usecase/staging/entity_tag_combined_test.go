package staging_test

import (
	"context"
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

// =============================================================================
// Tag + Entity State Combination Tests
// =============================================================================

func TestTagUseCase_Execute_StagedForUpdate_FetchesAWS(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// First, stage an entry for UPDATE (existing resource in AWS)
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/existing", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("updated-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	// Create strategy that tracks if FetchCurrentValue is called
	strategy := newMockTagStrategy()
	awsTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: awsTime,
	}

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Tag the staged UPDATE entry - SHOULD call FetchCurrentValue
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/existing",
		AddTags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	// Verify tag was staged with AWS time (not entry's BaseModifiedAt)
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/existing")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
	require.NotNil(t, tagEntry.BaseModifiedAt)
	assert.Equal(t, awsTime, *tagEntry.BaseModifiedAt)
}

func TestTagUseCase_Execute_StagedForDelete_FetchesAWS(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// First, stage an entry for DELETE
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/to-delete", staging.Entry{
		Operation:      staging.OperationDelete,
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))

	// Create strategy
	strategy := newMockTagStrategy()
	awsTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: awsTime,
	}

	uc := &usecasestaging.TagUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Tag the staged DELETE entry - SHOULD call FetchCurrentValue
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/to-delete",
		AddTags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	// Verify both entry (DELETE) and tag are staged
	entry, err := store.GetEntry(staging.ServiceParam, "/app/to-delete")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)

	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/to-delete")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
}

// =============================================================================
// Delete Entity with Tags Tests
// =============================================================================

func TestDeleteUseCase_Execute_EntityWithStagedTags_TagsRemain(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage an UPDATE entry with tags
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("updated-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	// Delete the entity
	output, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/config",
	})
	require.NoError(t, err)
	assert.False(t, output.Unstaged)

	// Entity should be staged as DELETE
	entry, err := store.GetEntry(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationDelete, entry.Operation)

	// Tags should STILL be staged (independent of entry)
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
}

func TestDeleteUseCase_Execute_UnstageCreateWithTags_TagsRemain(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage a CREATE entry with tags
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/new", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.DeleteUseCase{
		Strategy: newMockDeleteStrategy(false),
		Store:    store,
	}

	// Delete (unstage) the CREATE entry
	output, err := uc.Execute(context.Background(), usecasestaging.DeleteInput{
		Name: "/app/new",
	})
	require.NoError(t, err)
	assert.True(t, output.Unstaged)

	// Entry should be unstaged
	_, err = store.GetEntry(staging.ServiceParam, "/app/new")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	// Tags should STILL be staged (independent of entry - might be intentional for re-create)
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/new")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
}

// =============================================================================
// Edit Entity with Tags Tests
// =============================================================================

func TestEditUseCase_Execute_DeleteToUpdate_TagsUnaffected(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage DELETE entry with tags
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/config", staging.Entry{
		Operation:      staging.OperationDelete,
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	strategy := newMockEditStrategy()

	uc := &usecasestaging.EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	// Edit converts DELETE -> UPDATE
	_, err := uc.Execute(context.Background(), usecasestaging.EditInput{
		Name:  "/app/config",
		Value: "new-value",
	})
	require.NoError(t, err)

	// Entry should be UPDATE now
	entry, err := store.GetEntry(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Equal(t, "new-value", *entry.Value)

	// Tags should be unaffected
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
}

// =============================================================================
// Apply Mixed Entry and Tag Results Tests
// =============================================================================

func TestApplyUseCase_Execute_EntryFailsTagSucceeds(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage entry and tag for same resource
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	strategy := newMockApplyTagStrategy()
	strategy.applyErrors["/app/config"] = assert.AnError // Entry apply fails

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	assert.Error(t, err) // Overall error due to entry failure

	// Entry failed, tag succeeded
	assert.Equal(t, 0, output.EntrySucceeded)
	assert.Equal(t, 1, output.EntryFailed)
	assert.Equal(t, 1, output.TagSucceeded)
	assert.Equal(t, 0, output.TagFailed)

	// Entry should still be staged (failed)
	_, err = store.GetEntry(staging.ServiceParam, "/app/config")
	require.NoError(t, err)

	// Tag should be unstaged (succeeded)
	_, err = store.GetTag(staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestApplyUseCase_Execute_EntrySucceedsTagFails(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage entry and tag for same resource
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	strategy := newMockApplyTagStrategy()
	strategy.applyTagsErrors["/app/config"] = assert.AnError // Tag apply fails

	uc := &usecasestaging.ApplyUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	assert.Error(t, err) // Overall error due to tag failure

	// Entry succeeded, tag failed
	assert.Equal(t, 1, output.EntrySucceeded)
	assert.Equal(t, 0, output.EntryFailed)
	assert.Equal(t, 0, output.TagSucceeded)
	assert.Equal(t, 1, output.TagFailed)

	// Entry should be unstaged (succeeded)
	_, err = store.GetEntry(staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	// Tag should still be staged (failed)
	_, err = store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
}

// =============================================================================
// Reset with Tags Tests
// =============================================================================

func TestResetUseCase_Execute_EntryWithTags_OnlyEntryReset(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage entry and tag
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("staged-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	// Reset the entry
	output, err := uc.Execute(context.Background(), usecasestaging.ResetInput{
		Spec: "/app/config",
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultUnstaged, output.Type)

	// Entry should be unstaged
	_, err = store.GetEntry(staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	// Tags should STILL be staged (reset only affects entries)
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])
}

func TestResetUseCase_Execute_AllWithTags_BothEntriesAndTagsReset(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage multiple entries and tags
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/one", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value1"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/two", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value2"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/one", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	// Reset all
	output, err := uc.Execute(context.Background(), usecasestaging.ResetInput{
		All: true,
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultUnstagedAll, output.Type)
	assert.Equal(t, 2, output.Count)

	// Both entries should be unstaged
	_, err = store.GetEntry(staging.ServiceParam, "/app/one")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
	_, err = store.GetEntry(staging.ServiceParam, "/app/two")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	// Tags are ALSO cleared by reset --all (UnstageAll clears both entries and tags)
	_, err = store.GetTag(staging.ServiceParam, "/app/one")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

// =============================================================================
// Tag Merge Behavior Tests
// =============================================================================

func TestTagUseCase_Execute_AddThenRemoveSameKey_BecomesRemove(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage an add tag
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Remove the same tag key - this converts ADD to REMOVE (not cancellation)
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:       "/app/config",
		RemoveTags: maputil.NewSet("env"),
	})
	require.NoError(t, err)

	// Tag entry should still be staged, but now as REMOVE
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Empty(t, tagEntry.Add)                   // "env" removed from Add
	assert.True(t, tagEntry.Remove.Contains("env")) // "env" now in Remove
}

func TestTagUseCase_Execute_RemoveThenAddSameKey_BecomesAdd(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Stage a remove tag
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("env"),
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.TagUseCase{
		Strategy: newMockTagStrategy(),
		Store:    store,
	}

	// Add the same tag key - this converts REMOVE to ADD
	_, err := uc.Execute(context.Background(), usecasestaging.TagInput{
		Name:    "/app/config",
		AddTags: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)

	// Tag entry should still be staged, but now as ADD
	tagEntry, err := store.GetTag(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "prod", tagEntry.Add["env"])     // "env" now in Add
	assert.False(t, tagEntry.Remove.Contains("env")) // "env" removed from Remove
}

// =============================================================================
// Multiple Resources with Mixed Entity/Tag Staging
// =============================================================================

func TestApplyUseCase_Execute_MultipleResourcesMixedStaging(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Resource 1: Entry only
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/entry-only", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("value1"),
		StagedAt:  time.Now(),
	}))

	// Resource 2: Tag only
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/tag-only", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	// Resource 3: Both entry and tag
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/both", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value3"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/both", staging.TagEntry{
		Add:      map[string]string{"team": "backend"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyTagStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		IgnoreConflicts: true,
	})
	require.NoError(t, err)

	// 2 entries (entry-only + both), 2 tags (tag-only + both)
	assert.Equal(t, 2, output.EntrySucceeded)
	assert.Equal(t, 2, output.TagSucceeded)

	// All should be unstaged
	_, err = store.GetEntry(staging.ServiceParam, "/app/entry-only")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
	_, err = store.GetTag(staging.ServiceParam, "/app/tag-only")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
	_, err = store.GetEntry(staging.ServiceParam, "/app/both")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
	_, err = store.GetTag(staging.ServiceParam, "/app/both")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

// =============================================================================
// Filter By Name with Both Entry and Tag
// =============================================================================

func TestApplyUseCase_Execute_FilterByName_BothEntryAndTag(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))

	// Target resource: both entry and tag
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/target", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("target-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/target", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	// Other resource: both entry and tag (should not be affected)
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/other", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("other-value"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageTag(staging.ServiceParam, "/app/other", staging.TagEntry{
		Add:      map[string]string{"env": "dev"},
		StagedAt: time.Now(),
	}))

	uc := &usecasestaging.ApplyUseCase{
		Strategy: newMockApplyTagStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(context.Background(), usecasestaging.ApplyInput{
		Name:            "/app/target",
		IgnoreConflicts: true,
	})
	require.NoError(t, err)

	// Only target should be applied
	assert.Equal(t, 1, output.EntrySucceeded)
	assert.Equal(t, 1, output.TagSucceeded)

	// Target should be unstaged
	_, err = store.GetEntry(staging.ServiceParam, "/app/target")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
	_, err = store.GetTag(staging.ServiceParam, "/app/target")
	assert.ErrorIs(t, err, staging.ErrNotStaged)

	// Other should still be staged
	_, err = store.GetEntry(staging.ServiceParam, "/app/other")
	require.NoError(t, err)
	_, err = store.GetTag(staging.ServiceParam, "/app/other")
	require.NoError(t, err)
}
