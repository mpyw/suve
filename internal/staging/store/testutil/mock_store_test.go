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

// assertDeepCloneOnRead verifies that mutating the pointer-typed fields of a
// value returned from GetEntry/GetTag does not leak into stored state. The file
// store reparses JSON on every read, so MockStore must deep-clone to match.
func assertDeepCloneOnRead(t *testing.T, rw store.ReadWriteOperator) {
	t.Helper()

	ctx := context.Background()
	key := staging.EntryKey{Name: "/a"}

	require.NoError(t, rw.StageEntry(ctx, staging.ServiceParam, key, staging.Entry{
		Operation:   staging.OperationUpdate,
		Value:       lo.ToPtr("v1"),
		Description: lo.ToPtr("d1"),
		StagedAt:    time.Now(),
	}))
	require.NoError(t, rw.StageTag(ctx, staging.ServiceParam, key, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	}))

	got, err := rw.GetEntry(ctx, staging.ServiceParam, key)
	require.NoError(t, err)

	*got.Value = "hacked"
	*got.Description = "hacked"

	gotTag, err := rw.GetTag(ctx, staging.ServiceParam, key)
	require.NoError(t, err)

	gotTag.Add["env"] = "hacked"

	fresh, err := rw.GetEntry(ctx, staging.ServiceParam, key)
	require.NoError(t, err)
	assert.Equal(t, "v1", lo.FromPtr(fresh.Value), "GetEntry must return a deep copy")
	assert.Equal(t, "d1", lo.FromPtr(fresh.Description), "GetEntry must return a deep copy")

	freshTag, err := rw.GetTag(ctx, staging.ServiceParam, key)
	require.NoError(t, err)
	assert.Equal(t, "prod", freshTag.Add["env"], "GetTag must return a deep copy")
}

// TestDeepCloneOnRead_Parity pins MockStore to the file store's deep-copy
// semantics for GetEntry/GetTag.
func TestDeepCloneOnRead_Parity(t *testing.T) {
	t.Parallel()

	t.Run("mock", func(t *testing.T) {
		t.Parallel()
		assertDeepCloneOnRead(t, testutil.NewMockStore())
	})

	t.Run("file", func(t *testing.T) {
		t.Parallel()
		assertDeepCloneOnRead(t, file.NewStoreWithPath(filepath.Join(t.TempDir(), "stage.json")))
	})
}

// TestServiceScopedClear verifies that a service-specific Drain or WriteState
// touches only that service, leaving the other service's staged changes intact —
// mirroring the split-mode file store, which backs each service with its own
// file. Previously the mock cleared every tracked service regardless of the
// service filter.
func TestServiceScopedClear(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paramKey := staging.EntryKey{Name: "/p"}
	secretKey := staging.EntryKey{Name: "/s"}

	entry := func(v string) staging.Entry {
		return staging.Entry{Operation: staging.OperationUpdate, Value: lo.ToPtr(v), StagedAt: time.Now()}
	}

	t.Run("drain", func(t *testing.T) {
		t.Parallel()

		rw := testutil.NewMockStore()
		require.NoError(t, rw.StageEntry(ctx, staging.ServiceParam, paramKey, entry("p")))
		require.NoError(t, rw.StageEntry(ctx, staging.ServiceSecret, secretKey, entry("s")))

		_, err := rw.Drain(ctx, staging.ServiceParam, false)
		require.NoError(t, err)

		secret, err := rw.ListEntries(ctx, staging.ServiceSecret)
		require.NoError(t, err)
		assert.Len(t, secret[staging.ServiceSecret], 1, "draining param must not clear secret")

		param, err := rw.ListEntries(ctx, staging.ServiceParam)
		require.NoError(t, err)
		assert.Empty(t, param[staging.ServiceParam], "draining param must clear param")
	})

	t.Run("write", func(t *testing.T) {
		t.Parallel()

		rw := testutil.NewMockStore()
		require.NoError(t, rw.StageEntry(ctx, staging.ServiceSecret, secretKey, entry("s")))

		state := staging.NewEmptyState()
		state.Entries[staging.ServiceParam][paramKey] = entry("p")
		require.NoError(t, rw.WriteState(ctx, staging.ServiceParam, state))

		secret, err := rw.ListEntries(ctx, staging.ServiceSecret)
		require.NoError(t, err)
		assert.Len(t, secret[staging.ServiceSecret], 1, "writing param must not clear secret")

		param, err := rw.ListEntries(ctx, staging.ServiceParam)
		require.NoError(t, err)
		assert.Len(t, param[staging.ServiceParam], 1, "writing param must persist param")
	})
}
