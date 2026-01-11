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
	"github.com/mpyw/suve/internal/staging/transition"
	usecasestaging "github.com/mpyw/suve/internal/usecase/staging"
)

type mockParser struct {
	*mockServiceStrategy
	parsedName string
	parseErr   error
}

func (m *mockParser) ParseName(input string) (string, error) {
	if m.parseErr != nil {
		return "", m.parseErr
	}
	if m.parsedName != "" {
		return m.parsedName, nil
	}
	return input, nil
}

func (m *mockParser) ParseSpec(input string) (string, bool, error) {
	return input, false, nil
}

func newMockParser() *mockParser {
	return &mockParser{
		mockServiceStrategy: newParamStrategy(),
	}
}

func TestAddUseCase_Execute(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(), // Resource doesn't exist - expected for add
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:        "/app/new-param",
		Value:       "new-value",
		Description: "test description",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new-param", output.Name)

	// Verify staged
	entry, err := store.GetEntry(staging.ServiceParam, "/app/new-param")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationCreate, entry.Operation)
	assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
	assert.Equal(t, "test description", lo.FromPtr(entry.Description))
}

func TestAddUseCase_Execute_RejectsWhenResourceExists(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Resource exists on AWS
	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategy(), // Returns existing value
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/existing",
		Value: "new-value",
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, transition.ErrCannotAddToExisting)
}

func TestAddUseCase_Execute_MinimalInput(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/simple",
		Value: "simple-value",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/simple", output.Name)

	entry, err := store.GetEntry(staging.ServiceParam, "/app/simple")
	require.NoError(t, err)
	assert.Nil(t, entry.Description)
}

func TestAddUseCase_Draft_NotStaged(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	output, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "/app/not-exists"})
	require.NoError(t, err)
	assert.False(t, output.IsStaged)
	assert.Empty(t, output.Value)
}

func TestAddUseCase_Draft_StagedCreate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/draft", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("draft-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	output, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "/app/draft"})
	require.NoError(t, err)
	assert.True(t, output.IsStaged)
	assert.Equal(t, "draft-value", output.Value)
}

func TestAddUseCase_Draft_StagedUpdate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Update operation should not be returned as draft
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/update", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("update-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	output, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "/app/update"})
	require.NoError(t, err)
	assert.False(t, output.IsStaged) // Update is not a draft
}

func TestAddUseCase_Execute_ParseError(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockEditStrategyNotFound()
	strategy.parseErr = errors.New("invalid name")

	uc := &usecasestaging.AddUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "invalid",
		Value: "value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid name")
}

func TestAddUseCase_Draft_ParseError(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockEditStrategyNotFound()
	strategy.parseErr = errors.New("invalid name")

	uc := &usecasestaging.AddUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "invalid"})
	assert.Error(t, err)
}

func TestAddUseCase_Execute_StageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.stageErr = errors.New("stage error")

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestAddUseCase_Draft_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("get error")

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "/app/config"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get error")
}

func TestAddUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("store get error")

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store get error")
}

func TestAddUseCase_Execute_RejectsWhenUpdateStaged(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Pre-stage an UPDATE operation
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/existing", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("update-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/existing",
		Value: "new-value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already staged for update")
}

func TestAddUseCase_Execute_RejectsWhenDeleteStaged(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Pre-stage a DELETE operation
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/deleted", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/deleted",
		Value: "new-value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already staged for deletion")
}

func TestAddUseCase_Execute_AllowsReEditOfCreate(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	// Pre-stage a CREATE operation
	require.NoError(t, store.StageEntry(staging.ServiceParam, "/app/new", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("initial-value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	// Should allow updating the value
	output, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/new",
		Value: "updated-value",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)

	// Verify the value was updated but operation remains CREATE
	entry, err := store.GetEntry(staging.ServiceParam, "/app/new")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationCreate, entry.Operation)
	assert.Equal(t, "updated-value", lo.FromPtr(entry.Value))
}

func TestAddUseCase_Execute_FetchError(t *testing.T) {
	t.Parallel()

	store := staging.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	strategy := newMockEditStrategy()
	strategy.fetchErr = errors.New("AWS connection error")

	uc := &usecasestaging.AddUseCase{
		Strategy: strategy,
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AWS connection error")
}
