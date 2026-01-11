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

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/file"
	usecasestaging "github.com/mpyw/suve/internal/usecase/staging"
)

type mockResetStrategy struct {
	*mockParser
	fetchValue        string
	versionLabel      string
	fetchErr          error
	currentValue      string
	fetchCurrentError error
}

func (m *mockResetStrategy) FetchVersion(_ context.Context, _ string) (string, string, error) {
	if m.fetchErr != nil {
		return "", "", m.fetchErr
	}
	return m.fetchValue, m.versionLabel, nil
}

func (m *mockResetStrategy) FetchCurrentValue(_ context.Context, _ string) (*staging.EditFetchResult, error) {
	if m.fetchCurrentError != nil {
		return nil, m.fetchCurrentError
	}
	return &staging.EditFetchResult{Value: m.currentValue}, nil
}

func newMockResetStrategy() *mockResetStrategy {
	return &mockResetStrategy{
		mockParser:   newMockParser(),
		fetchValue:   "version-value",
		versionLabel: "#3",
		currentValue: "current-value",
	}
}

func TestResetUseCase_Execute_Unstage(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "/app/config",
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultUnstaged, output.Type)
	assert.Equal(t, "/app/config", output.Name)

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestResetUseCase_Execute_NotStaged(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "/app/not-staged",
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultNotStaged, output.Type)
}

func TestResetUseCase_Execute_UnstageAll(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/one", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("one"),
		StagedAt:  time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, "/app/two", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("two"),
		StagedAt:  time.Now(),
	}))

	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		All: true,
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultUnstagedAll, output.Type)
	assert.Equal(t, 2, output.Count)

	// Verify all unstaged
	entries, err := store.ListEntries(t.Context(), staging.ServiceParam)
	require.NoError(t, err)
	assert.Empty(t, entries[staging.ServiceParam])
}

func TestResetUseCase_Execute_UnstageAll_Empty(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		All: true,
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultNothingStaged, output.Type)
}

func TestResetUseCase_Execute_Restore(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	parser := &mockParserWithVersion{
		mockParser: newMockParser(),
		hasVersion: true,
	}

	uc := &usecasestaging.ResetUseCase{
		Parser:  parser,
		Fetcher: newMockResetStrategy(),
		Store:   store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "/app/config#3",
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultRestored, output.Type)
	assert.Equal(t, "#3", output.VersionLabel)

	// Verify staged
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config#3")
	require.NoError(t, err)
	assert.Equal(t, staging.OperationUpdate, entry.Operation)
	assert.Equal(t, "version-value", lo.FromPtr(entry.Value))
}

func TestResetUseCase_Execute_Restore_NoFetcher(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	parser := &mockParserWithVersion{
		mockParser: newMockParser(),
		hasVersion: true,
	}

	uc := &usecasestaging.ResetUseCase{
		Parser: parser,
		Store:  store,
		// No Fetcher
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "/app/config#3",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reset strategy required")
}

func TestResetUseCase_Execute_Restore_FetchError(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	parser := &mockParserWithVersion{
		mockParser: newMockParser(),
		hasVersion: true,
	}
	fetcher := newMockResetStrategy()
	fetcher.fetchErr = errors.New("version not found")

	uc := &usecasestaging.ResetUseCase{
		Parser:  parser,
		Fetcher: fetcher,
		Store:   store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "/app/config#999",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version not found")
}

type mockParserWithVersion struct {
	*mockParser
	hasVersion bool
}

func (m *mockParserWithVersion) ParseSpec(input string) (string, bool, error) {
	return input, m.hasVersion, nil
}

func TestResetUseCase_Execute_ParseError(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	parser := &mockParserWithParseSpecErr{
		mockParser: newMockParser(),
		parseErr:   errors.New("parse error"),
	}

	uc := &usecasestaging.ResetUseCase{
		Parser: parser,
		Store:  store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "invalid",
	})
	assert.Error(t, err)
}

type mockParserWithParseSpecErr struct {
	*mockParser
	parseErr error
}

func (m *mockParserWithParseSpecErr) ParseSpec(_ string) (string, bool, error) {
	return "", false, m.parseErr
}

func TestResetUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.listErr = errors.New("list error")

	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{All: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

func TestResetUseCase_Execute_UnstageAllError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.addEntry(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
	})
	store.unstageAllErr = errors.New("unstage all error")

	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{All: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unstage all error")
}

func TestResetUseCase_Execute_GetError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.getErr = errors.New("get error")

	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{Spec: "/app/config"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get error")
}

func TestResetUseCase_Execute_UnstageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.addEntry(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
	})
	store.unstageErr = errors.New("unstage error")

	uc := &usecasestaging.ResetUseCase{
		Parser: newMockParser(),
		Store:  store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{Spec: "/app/config"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unstage error")
}

func TestResetUseCase_Execute_RestoreStageError(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	store.stageErr = errors.New("stage error")

	parser := &mockParserWithVersion{
		mockParser: newMockParser(),
		hasVersion: true,
	}

	uc := &usecasestaging.ResetUseCase{
		Parser:  parser,
		Fetcher: newMockResetStrategy(),
		Store:   store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{Spec: "/app/config#3"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage error")
}

func TestResetUseCase_Execute_RestoreSkipped_SameAsAWS(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	parser := &mockParserWithVersion{
		mockParser: newMockParser(),
		hasVersion: true,
	}

	// Fetcher returns value that matches current AWS
	fetcher := newMockResetStrategy()
	fetcher.fetchValue = "current-value"
	fetcher.currentValue = "current-value" // Same as fetched version
	fetcher.versionLabel = "#3"

	uc := &usecasestaging.ResetUseCase{
		Parser:  parser,
		Fetcher: fetcher,
		Store:   store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "/app/config#3",
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultSkipped, output.Type)
	assert.Equal(t, "#3", output.VersionLabel)

	// Verify nothing was staged (auto-skipped)
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config#3")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestResetUseCase_Execute_RestoreNotSkipped_DifferentFromAWS(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	parser := &mockParserWithVersion{
		mockParser: newMockParser(),
		hasVersion: true,
	}

	// Fetcher returns value different from current AWS
	fetcher := newMockResetStrategy()
	fetcher.fetchValue = "old-version-value"
	fetcher.currentValue = "current-value" // Different from fetched version
	fetcher.versionLabel = "#3"

	uc := &usecasestaging.ResetUseCase{
		Parser:  parser,
		Fetcher: fetcher,
		Store:   store,
	}

	output, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "/app/config#3",
	})
	require.NoError(t, err)
	assert.Equal(t, usecasestaging.ResetResultRestored, output.Type)
	assert.Equal(t, "#3", output.VersionLabel)

	// Verify entry was staged
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config#3")
	require.NoError(t, err)
	assert.Equal(t, "old-version-value", lo.FromPtr(entry.Value))
}

func TestResetUseCase_Execute_RestoreFetchCurrentError(t *testing.T) {
	t.Parallel()

	store := file.NewStoreWithPath(filepath.Join(t.TempDir(), "staging.json"))
	parser := &mockParserWithVersion{
		mockParser: newMockParser(),
		hasVersion: true,
	}

	fetcher := newMockResetStrategy()
	fetcher.fetchCurrentError = errors.New("aws error")

	uc := &usecasestaging.ResetUseCase{
		Parser:  parser,
		Fetcher: fetcher,
		Store:   store,
	}

	_, err := uc.Execute(t.Context(), usecasestaging.ResetInput{
		Spec: "/app/config#3",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aws error")
}
