package staging_test

import (
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
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

	store := testutil.NewMockStore()
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
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/new-param")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationCreate, entry.Operation)
	assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
	assert.Equal(t, "test description", lo.FromPtr(entry.Description))
}

func TestAddUseCase_Execute_RejectsWhenResourceExists(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Resource exists on AWS
	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategy(), // Returns existing value
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/existing",
		Value: "new-value",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, transition.ErrCannotAddToExisting)
}

func TestAddUseCase_Execute_MinimalInput(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
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

	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/simple")
	require.NoError(t, err)
	assert.Nil(t, entry.Description)
}

func TestAddUseCase_Draft_NotStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
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

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/draft", staging.Entry{
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

	store := testutil.NewMockStore()
	// Update operation should not be returned as draft
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/update", staging.Entry{
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

	store := testutil.NewMockStore()
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid name")
}

func TestAddUseCase_Draft_ParseError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
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

	store := testutil.NewMockStore()
	store.StageEntryErr = errors.New("stage error")

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/config",
		Value: "value",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestAddUseCase_Draft_GetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetEntryErr = errors.New("get error")

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get error")
}

func TestAddUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.GetEntryErr = errors.New("store get error")

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.AddInput{
		Name:  "/app/config",
		Value: "value",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store get error")
}

func TestAddUseCase_Execute_RejectsWhenUpdateStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Pre-stage an UPDATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/existing", staging.Entry{
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already staged for update")
}

func TestAddUseCase_Execute_RejectsWhenDeleteStaged(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Pre-stage a DELETE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/deleted", staging.Entry{
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already staged for deletion")
}

func TestAddUseCase_Execute_AllowsReEditOfCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	// Pre-stage a CREATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
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
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationCreate, entry.Operation)
	assert.Equal(t, "updated-value", lo.FromPtr(entry.Value))
}

func TestAddUseCase_Execute_FetchError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS connection error")
}

// Ping-first pattern tests

func TestAddUseCase_Draft_PingFirst_DaemonNotRunning(t *testing.T) {
	t.Parallel()

	// When daemon is not running (Ping fails), should return "not staged" without checking store
	store := testutil.NewMockStore()
	store.PingErr = errors.New("daemon not running")

	// Pre-stage a CREATE operation (should not be found if Ping fails)
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/draft", staging.Entry{
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
	// Should return "not staged" since daemon isn't running
	assert.False(t, output.IsStaged)
	assert.Empty(t, output.Value)
}

func TestAddUseCase_Draft_PingFirst_DaemonRunning(t *testing.T) {
	t.Parallel()

	// When daemon is running (Ping succeeds), should find staged draft
	store := testutil.NewMockStore()
	store.PingErr = nil // Daemon running

	// Pre-stage a CREATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/draft", staging.Entry{
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
	// Should find the staged draft
	assert.True(t, output.IsStaged)
	assert.Equal(t, "draft-value", output.Value)
}

func TestAddUseCase_Draft_PingFirst_DaemonRunning_NotStaged(t *testing.T) {
	t.Parallel()

	// When daemon is running but nothing is staged
	store := testutil.NewMockStore()
	store.PingErr = nil // Daemon running

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	output, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "/app/not-exists"})
	require.NoError(t, err)
	assert.False(t, output.IsStaged)
	assert.Empty(t, output.Value)
}

func TestAddUseCase_Draft_PingFirst_DaemonRunning_GetEntryError(t *testing.T) {
	t.Parallel()

	// When Ping succeeds but GetEntry fails (not ErrNotStaged), error should propagate
	store := testutil.NewMockStore()
	store.PingErr = nil // Daemon running
	store.GetEntryErr = errors.New("storage unavailable")

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    store,
	}

	_, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage unavailable")
}

// nonPingerStoreWrapper is defined in edit_test.go (same package).
// It wraps a store but doesn't implement Pinger interface,
// simulating FileStore behavior where Ping check is not available.

func TestAddUseCase_Draft_NonPingerStore(t *testing.T) {
	t.Parallel()

	// When store doesn't implement Pinger (like FileStore),
	// should proceed directly to GetEntry without Ping check
	mockStore := testutil.NewMockStore()

	// Pre-stage a CREATE operation
	require.NoError(t, mockStore.StageEntry(t.Context(), staging.ServiceParam, "/app/draft", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("draft-value"),
		StagedAt:  time.Now(),
	}))

	// Wrap to hide Pinger interface
	wrappedStore := &nonPingerStoreWrapper{mockStore}

	uc := &usecasestaging.AddUseCase{
		Strategy: newMockEditStrategyNotFound(),
		Store:    wrappedStore,
	}

	// Should check staged state directly and return draft
	output, err := uc.Draft(t.Context(), usecasestaging.DraftInput{Name: "/app/draft"})
	require.NoError(t, err)
	assert.True(t, output.IsStaged)
	assert.Equal(t, "draft-value", output.Value)
}
