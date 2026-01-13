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
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestPersistUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("persist agent to file - success", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.PersistUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.PersistInput{})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)

		// Verify entry moved to file
		entry, err := fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "agent-value", lo.FromPtr(entry.Value))

		// Verify agent is cleared
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("persist with keep - agent preserved", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.PersistUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.PersistInput{Keep: true})
		require.NoError(t, err)

		// Verify entry exists in both
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
	})

	t.Run("persist error - nothing to persist", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		usecase := &stagingusecase.PersistUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.PersistInput{})
		assert.ErrorIs(t, err, stagingusecase.ErrNothingToPersist)
	})

	t.Run("persist with service filter", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})
		_ = agentStore.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.PersistUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.PersistInput{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Param should be in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
		require.NoError(t, err)

		// Secret should still be in agent
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
		require.NoError(t, err)

		// Param should be cleared from agent (not kept)
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("persist with tags", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		})

		usecase := &stagingusecase.PersistUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.PersistInput{})
		require.NoError(t, err)
		assert.Equal(t, 1, output.TagCount)

		// Verify tag moved to file
		tag, err := fileStore.GetTag(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "prod", tag.Add["env"])
	})

	t.Run("persist merges with existing file state for service filter", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Existing file has secret entry
		_ = fileStore.StageEntry(t.Context(), staging.ServiceSecret, "existing-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-value"),
			StagedAt:  time.Now(),
		})

		// Agent has param entry
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.PersistUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.PersistInput{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Both should exist in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret")
		require.NoError(t, err)
	})
}

func TestPersistError(t *testing.T) {
	t.Parallel()

	t.Run("error message - load", func(t *testing.T) {
		t.Parallel()
		err := &stagingusecase.PersistError{Op: "load", Err: errors.New("connection failed")}
		assert.Contains(t, err.Error(), "failed to get state from agent")
		assert.Contains(t, err.Error(), "connection failed")
	})

	t.Run("error message - write", func(t *testing.T) {
		t.Parallel()
		err := &stagingusecase.PersistError{Op: "write", Err: errors.New("write failed")}
		assert.Contains(t, err.Error(), "failed to save state to file")
	})

	t.Run("error message - clear", func(t *testing.T) {
		t.Parallel()
		err := &stagingusecase.PersistError{Op: "clear", Err: errors.New("clear failed")}
		assert.Contains(t, err.Error(), "failed to clear agent memory")
	})

	t.Run("error message - unknown op", func(t *testing.T) {
		t.Parallel()
		innerErr := errors.New("something went wrong")
		err := &stagingusecase.PersistError{Op: "unknown", Err: innerErr}
		assert.Equal(t, "something went wrong", err.Error())
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()
		innerErr := errors.New("inner error")
		err := &stagingusecase.PersistError{Op: "load", Err: innerErr}
		assert.ErrorIs(t, err, innerErr)
	})
}
