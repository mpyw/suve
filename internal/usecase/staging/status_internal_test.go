// White-box tests of status.go.
//declscope:namespace status

package staging

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

func TestStatusUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), StatusInput{})
	require.NoError(t, err)
	assert.Equal(t, staging.ServiceParam, output.service)
	assert.Equal(t, "Parameter Store", output.ServiceName)
	assert.Equal(t, "parameter", output.itemName)
	assert.Empty(t, output.Entries)
}

func TestStatusUseCase_Execute_WithEntries(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	now := time.Now().Truncate(time.Second)

	// Stage some entries
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("new-value"),
		StagedAt:  now,
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	}))

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), StatusInput{})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestStatusUseCase_Execute_FilterByName(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	now := time.Now()

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("value"),
		StagedAt:  now,
	}))

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	// Existing entry
	output, err := uc.Execute(t.Context(), StatusInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, "/app/config", output.Entries[0].Name)

	// Non-existent entry
	_, err = uc.Execute(t.Context(), StatusInput{Name: "/app/other"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not staged")
}

func TestStatusUseCase_Execute_SecretWithDeleteOptions(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	now := time.Now()

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
		DeleteOptions: &staging.DeleteOptions{
			Force:          false,
			RecoveryWindow: 14,
		},
	}))

	uc := &StatusUseCase{
		Strategy: doublesNewSecretStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), StatusInput{})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.True(t, output.Entries[0].ShowDeleteOptions)
	assert.NotNil(t, output.Entries[0].DeleteOptions)
	assert.Equal(t, 14, output.Entries[0].DeleteOptions.RecoveryWindow)
}

func TestStatusUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// The name-filtered path lists entries (keyed by the (name, namespace)
	// composite) and filters by the decoded bare name, so a list error surfaces.
	store.ListEntriesErr = errors.New("store error")

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), StatusInput{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store error")
}

func TestStatusUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListEntriesErr = errors.New("list error")

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), StatusInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

func TestStatusUseCase_Execute_WithTagEntries(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	now := time.Now().Truncate(time.Second)

	// Stage tag entries
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt: now,
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/secret"}, staging.TagEntry{
		Remove:   map[string]struct{}{"deprecated": {}},
		StagedAt: now,
	}))

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), StatusInput{})
	require.NoError(t, err)
	assert.Len(t, output.TagEntries, 2)

	// Find the specific entries
	var configEntry, secretEntry *StatusTagEntry

	for i := range output.TagEntries {
		if output.TagEntries[i].Name == "/app/config" {
			configEntry = &output.TagEntries[i]
		}

		if output.TagEntries[i].Name == "/app/secret" {
			secretEntry = &output.TagEntries[i]
		}
	}

	require.NotNil(t, configEntry)
	assert.Equal(t, "prod", configEntry.Add["env"])
	assert.Equal(t, "backend", configEntry.Add["team"])

	require.NotNil(t, secretEntry)
	assert.True(t, secretEntry.Remove.Contains("deprecated"))
}

func TestStatusUseCase_Execute_FilterByName_TagEntry(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	now := time.Now()

	// Stage only tag entry (no regular entry)
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	}))

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	// Existing tag entry
	output, err := uc.Execute(t.Context(), StatusInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
	assert.Len(t, output.TagEntries, 1)
	assert.Equal(t, "/app/config", output.TagEntries[0].Name)
	assert.Equal(t, "prod", output.TagEntries[0].Add["env"])
}

func TestStatusUseCase_Execute_FilterByName_BothEntryAndTag(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	now := time.Now()

	// Stage both regular entry and tag entry
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("new-value"),
		StagedAt:  now,
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	}))

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), StatusInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Len(t, output.TagEntries, 1)
	assert.Equal(t, "/app/config", output.Entries[0].Name)
	assert.Equal(t, "/app/config", output.TagEntries[0].Name)
}

func TestStatusUseCase_Execute_GetTagError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// The name-filtered path also lists tags, so a list-tags error surfaces.
	store.ListTagsErr = errors.New("get tag error")

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), StatusInput{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get tag error")
}

func TestStatusUseCase_Execute_ListTagsError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListTagsErr = errors.New("list tags error")

	uc := &StatusUseCase{
		Strategy: doublesNewParamStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), StatusInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list tags error")
}
