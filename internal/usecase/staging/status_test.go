package staging_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/file"
	usecasestaging "github.com/mpyw/suve/internal/usecase/staging"
)

type mockServiceStrategy struct {
	service          staging.Service
	serviceName      string
	itemName         string
	hasDeleteOptions bool
}

func (m *mockServiceStrategy) Service() staging.Service { return m.service }
func (m *mockServiceStrategy) ServiceName() string      { return m.serviceName }
func (m *mockServiceStrategy) ItemName() string         { return m.itemName }
func (m *mockServiceStrategy) HasDeleteOptions() bool   { return m.hasDeleteOptions }

func newParamStrategy() *mockServiceStrategy {
	return &mockServiceStrategy{
		service:          staging.ServiceParam,
		serviceName:      "SSM Parameter Store",
		itemName:         "parameter",
		hasDeleteOptions: false,
	}
}

func newSecretStrategy() *mockServiceStrategy {
	return &mockServiceStrategy{
		service:          staging.ServiceSecret,
		serviceName:      "Secrets Manager",
		itemName:         "secret",
		hasDeleteOptions: true,
	}
}

func TestStatusUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.StatusInput{})
	require.NoError(t, err)
	assert.Equal(t, staging.ServiceParam, output.Service)
	assert.Equal(t, "SSM Parameter Store", output.ServiceName)
	assert.Equal(t, "parameter", output.ItemName)
	assert.Empty(t, output.Entries)
}

func TestStatusUseCase_Execute_WithEntries(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	now := time.Now().Truncate(time.Second)

	// Stage some entries
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  now,
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	}))

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.StatusInput{})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestStatusUseCase_Execute_FilterByName(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	now := time.Now()

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value"),
		StagedAt:  now,
	}))

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	// Existing entry
	output, err := uc.Execute(t.Context(), usecasestaging.StatusInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Equal(t, "/app/config", output.Entries[0].Name)

	// Non-existent entry
	_, err = uc.Execute(t.Context(), usecasestaging.StatusInput{Name: "/app/other"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not staged")
}

func TestStatusUseCase_Execute_SecretWithDeleteOptions(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	now := time.Now()

	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
		DeleteOptions: &staging.DeleteOptions{
			Force:          false,
			RecoveryWindow: 14,
		},
	}))

	uc := &usecasestaging.StatusUseCase{
		Strategy: newSecretStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.StatusInput{})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.True(t, output.Entries[0].ShowDeleteOptions)
	assert.NotNil(t, output.Entries[0].DeleteOptions)
	assert.Equal(t, 14, output.Entries[0].DeleteOptions.RecoveryWindow)
}

func TestStatusUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("store error")

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.StatusInput{Name: "/app/config"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store error")
}

func TestStatusUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.listErr = errors.New("list error")

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.StatusInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

func TestStatusUseCase_Execute_WithTagEntries(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	now := time.Now().Truncate(time.Second)

	// Stage tag entries
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "backend"},
		StagedAt: now,
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/secret", staging.TagEntry{
		Remove:   map[string]struct{}{"deprecated": {}},
		StagedAt: now,
	}))

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.StatusInput{})
	require.NoError(t, err)
	assert.Len(t, output.TagEntries, 2)

	// Find the specific entries
	var configEntry, secretEntry *usecasestaging.StatusTagEntry
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

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	now := time.Now()

	// Stage only tag entry (no regular entry)
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	}))

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	// Existing tag entry
	output, err := uc.Execute(t.Context(), usecasestaging.StatusInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
	assert.Len(t, output.TagEntries, 1)
	assert.Equal(t, "/app/config", output.TagEntries[0].Name)
	assert.Equal(t, "prod", output.TagEntries[0].Add["env"])
}

func TestStatusUseCase_Execute_FilterByName_BothEntryAndTag(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	now := time.Now()

	// Stage both regular entry and tag entry
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  now,
	}))
	require.NoError(t, store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	}))

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.StatusInput{Name: "/app/config"})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 1)
	assert.Len(t, output.TagEntries, 1)
	assert.Equal(t, "/app/config", output.Entries[0].Name)
	assert.Equal(t, "/app/config", output.TagEntries[0].Name)
}

func TestStatusUseCase_Execute_GetTagError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getTagErr = errors.New("get tag error")

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.StatusInput{Name: "/app/config"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get tag error")
}

func TestStatusUseCase_Execute_ListTagsError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.listTagsErr = errors.New("list tags error")

	uc := &usecasestaging.StatusUseCase{
		Strategy: newParamStrategy(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.StatusInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list tags error")
}
