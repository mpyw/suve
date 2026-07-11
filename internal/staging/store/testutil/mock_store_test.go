package testutil_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/file"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// assertListCopyOnRead verifies that the store's List methods return brand-new
// maps: mutating a returned map must not leak into subsequent reads. This is the
// real file store's copy-on-read contract that MockStore must also honor.
func assertListCopyOnRead(t *testing.T, rw store.ReadWriteOperator) {
	t.Helper()

	ctx := context.Background()
	key := staging.EntryKey{Name: "/a"}

	require.NoError(t, rw.StageEntry(ctx, staging.ServiceParam, key, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("v1"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, rw.StageTag(ctx, staging.ServiceParam, key, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	// Mutating the map returned by ListEntries must not affect stored state.
	entries, err := rw.ListEntries(ctx, staging.ServiceParam)
	require.NoError(t, err)
	require.Len(t, entries[staging.ServiceParam], 1)
	delete(entries[staging.ServiceParam], key)

	entries, err = rw.ListEntries(ctx, staging.ServiceParam)
	require.NoError(t, err)
	assert.Len(t, entries[staging.ServiceParam], 1, "ListEntries must return a copy, not the live map")

	// Mutating the map returned by ListTags must not affect stored state.
	tags, err := rw.ListTags(ctx, staging.ServiceParam)
	require.NoError(t, err)
	require.Len(t, tags[staging.ServiceParam], 1)
	delete(tags[staging.ServiceParam], key)

	tags, err = rw.ListTags(ctx, staging.ServiceParam)
	require.NoError(t, err)
	assert.Len(t, tags[staging.ServiceParam], 1, "ListTags must return a copy, not the live map")
}

// TestListCopyOnRead_Parity pins MockStore to the real file store's
// copy-on-read semantics for the List methods.
func TestListCopyOnRead_Parity(t *testing.T) {
	t.Parallel()

	t.Run("mock", func(t *testing.T) {
		t.Parallel()
		assertListCopyOnRead(t, testutil.NewMockStore())
	})

	t.Run("file", func(t *testing.T) {
		t.Parallel()
		assertListCopyOnRead(t, file.NewStoreWithPath(filepath.Join(t.TempDir(), "stage.json")))
	})
}
