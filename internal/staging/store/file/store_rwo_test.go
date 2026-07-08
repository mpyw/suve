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
	"github.com/mpyw/suve/internal/staging/store/file"
)

// newTempStore returns a file store backed by a fresh temp file path.
func newTempStore(t *testing.T) *file.Store {
	t.Helper()

	return file.NewStoreWithPath(filepath.Join(t.TempDir(), "stage.json"))
}

func sampleEntry(value string) staging.Entry {
	return staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(value),
		StagedAt:  time.Now(),
	}
}

func TestStore_GetEntry_NotStaged(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	_, err := store.GetEntry(t.Context(), staging.ServiceParam, "/missing", "")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_GetTag_NotStaged(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	_, err := store.GetTag(t.Context(), staging.ServiceParam, "/missing")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_StageAndGetEntry(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/a", sampleEntry("v1")))

	got, err := store.GetEntry(t.Context(), staging.ServiceParam, "/a", "")
	require.NoError(t, err)
	assert.Equal(t, "v1", lo.FromPtr(got.Value))

	// Read-modify-write: update the same entry, other entries preserved.
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/b", sampleEntry("v2")))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/a", sampleEntry("v1-updated")))

	got, err = store.GetEntry(t.Context(), staging.ServiceParam, "/a", "")
	require.NoError(t, err)
	assert.Equal(t, "v1-updated", lo.FromPtr(got.Value))

	got, err = store.GetEntry(t.Context(), staging.ServiceParam, "/b", "")
	require.NoError(t, err)
	assert.Equal(t, "v2", lo.FromPtr(got.Value))
}

func TestStore_StageAndGetTag(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	tag := staging.TagEntry{Add: map[string]string{"env": "prod"}, StagedAt: time.Now()}
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceSecret, "s1", tag))

	got, err := store.GetTag(t.Context(), staging.ServiceSecret, "s1")
	require.NoError(t, err)
	assert.Equal(t, "prod", got.Add["env"])
}

func TestStore_ListEntries_Filtering(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/p", sampleEntry("p")))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, "s", sampleEntry("s")))

	// Filter to param only: secret map omitted.
	entries, err := store.ListEntries(t.Context(), staging.ServiceParam)
	require.NoError(t, err)
	assert.Len(t, entries[staging.ServiceParam], 1)
	assert.NotContains(t, entries, staging.ServiceSecret)

	// All services.
	all, err := store.ListEntries(t.Context(), "")
	require.NoError(t, err)
	assert.Contains(t, all, staging.ServiceParam)
	assert.Contains(t, all, staging.ServiceSecret)

	// Empty service maps are omitted entirely.
	empty := newTempStore(t)
	got, err := empty.ListEntries(t.Context(), staging.ServiceParam)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestStore_ListTags_Filtering(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/p", staging.TagEntry{
		Add:      map[string]string{"k": "v"},
		StagedAt: time.Now(),
	}))

	tags, err := store.ListTags(t.Context(), staging.ServiceSecret)
	require.NoError(t, err)
	assert.Empty(t, tags)

	tags, err = store.ListTags(t.Context(), staging.ServiceParam)
	require.NoError(t, err)
	assert.Contains(t, tags[staging.ServiceParam], "/p")
}

func TestStore_UnstageEntry(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	// Unstage a missing entry returns ErrNotStaged.
	require.ErrorIs(t, store.UnstageEntry(t.Context(), staging.ServiceParam, "/missing", ""), staging.ErrNotStaged)

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/a", sampleEntry("v")))
	require.NoError(t, store.UnstageEntry(t.Context(), staging.ServiceParam, "/a", ""))

	_, err := store.GetEntry(t.Context(), staging.ServiceParam, "/a", "")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_UnstageTag(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	require.ErrorIs(t, store.UnstageTag(t.Context(), staging.ServiceParam, "/missing"), staging.ErrNotStaged)

	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/a", staging.TagEntry{
		Add:      map[string]string{"k": "v"},
		StagedAt: time.Now(),
	}))
	require.NoError(t, store.UnstageTag(t.Context(), staging.ServiceParam, "/a"))

	_, err := store.GetTag(t.Context(), staging.ServiceParam, "/a")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestStore_UnstageAll_RemovesFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "stage.json")
	store := file.NewStoreWithPath(path)

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/p", sampleEntry("p")))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, "s", sampleEntry("s")))

	// File exists after staging.
	_, err := os.Stat(path)
	require.NoError(t, err)

	// UnstageAll with a specific service clears only that service.
	require.NoError(t, store.UnstageAll(t.Context(), staging.ServiceParam))

	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/p", "")
	require.ErrorIs(t, err, staging.ErrNotStaged)

	got, err := store.GetEntry(t.Context(), staging.ServiceSecret, "s", "")
	require.NoError(t, err)
	assert.Equal(t, "s", lo.FromPtr(got.Value))

	// UnstageAll with empty service clears everything and removes the file.
	require.NoError(t, store.UnstageAll(t.Context(), ""))

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "file should be removed when state becomes empty")
}

func TestStore_UnstageAll_Empty(t *testing.T) {
	t.Parallel()

	store := newTempStore(t)

	// UnstageAll on an empty store is a no-op and succeeds.
	require.NoError(t, store.UnstageAll(t.Context(), ""))
}
