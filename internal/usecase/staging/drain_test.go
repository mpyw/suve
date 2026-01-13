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

func TestDrainUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("drain file to agent - success", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("file-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)
		assert.False(t, output.Merged)

		// Verify entry moved to agent
		entry, err := agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "file-value", lo.FromPtr(entry.Value))

		// Verify file is cleared
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("drain with keep - file preserved", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("file-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{Keep: true})
		require.NoError(t, err)

		// Verify entry exists in both
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
	})

	t.Run("drain with merge - combines states", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/existing", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("file-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{Merge: true})
		require.NoError(t, err)
		assert.True(t, output.Merged)

		// Verify both entries exist in agent
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/existing")
		require.NoError(t, err)
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
		require.NoError(t, err)
	})

	t.Run("drain with force - overwrites agent", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/existing", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("file-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{Force: true})
		require.NoError(t, err)

		// With force, agent should have file content
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
	})

	t.Run("drain error - nothing to drain", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{})
		assert.ErrorIs(t, err, stagingusecase.ErrNothingToDrain)
	})

	t.Run("drain error - agent has changes", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/existing", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("file-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{})
		assert.ErrorIs(t, err, stagingusecase.ErrAgentHasChanges)
	})

	t.Run("drain with service filter", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Param should be in agent
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
		require.NoError(t, err)

		// Secret should still be in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
		require.NoError(t, err)
	})

	t.Run("drain with tags", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = fileStore.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		})

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{})
		require.NoError(t, err)
		assert.Equal(t, 1, output.TagCount)

		// Verify tag moved to agent
		tag, err := agentStore.GetTag(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "prod", tag.Add["env"])
	})
}

func TestDrainUseCase_Execute_Errors(t *testing.T) {
	t.Parallel()

	t.Run("error on file load", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		fileStore.DrainErr = errors.New("read error")
		agentStore := testutil.NewMockStore()

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{})
		var drainErr *stagingusecase.DrainError
		require.ErrorAs(t, err, &drainErr)
		assert.Equal(t, "load", drainErr.Op)
	})

	t.Run("error on agent write", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		})
		agentStore.WriteStateErr = errors.New("write error")

		usecase := &stagingusecase.DrainUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.DrainInput{})
		var drainErr *stagingusecase.DrainError
		require.ErrorAs(t, err, &drainErr)
		assert.Equal(t, "write", drainErr.Op)
	})
}

func TestDrainError(t *testing.T) {
	t.Parallel()

	t.Run("error message - load", func(t *testing.T) {
		t.Parallel()
		err := &stagingusecase.DrainError{Op: "load", Err: errors.New("connection failed")}
		assert.Contains(t, err.Error(), "failed to load state from file")
		assert.Contains(t, err.Error(), "connection failed")
	})

	t.Run("error message - write", func(t *testing.T) {
		t.Parallel()
		err := &stagingusecase.DrainError{Op: "write", Err: errors.New("write failed")}
		assert.Contains(t, err.Error(), "failed to set state in agent")
	})

	t.Run("error message - delete", func(t *testing.T) {
		t.Parallel()
		err := &stagingusecase.DrainError{Op: "delete", Err: errors.New("delete failed")}
		assert.Contains(t, err.Error(), "failed to delete file")
	})

	t.Run("error message - unknown op", func(t *testing.T) {
		t.Parallel()
		innerErr := errors.New("something went wrong")
		err := &stagingusecase.DrainError{Op: "unknown", Err: innerErr}
		assert.Equal(t, "something went wrong", err.Error())
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()
		innerErr := errors.New("inner error")
		err := &stagingusecase.DrainError{Op: "load", Err: innerErr}
		assert.ErrorIs(t, err, innerErr)
	})
}
