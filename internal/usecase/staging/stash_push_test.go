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

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})
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

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Keep: true})
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

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})
		assert.ErrorIs(t, err, stagingusecase.ErrNothingToStashPush)
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

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Service: staging.ServiceParam})
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

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})
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

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Both should exist in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret")
		require.NoError(t, err)
	})
}

func TestPersistUseCase_PersistMode(t *testing.T) {
	t.Parallel()

	t.Run("overwrite mode replaces entire file", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Existing file has both param and secret
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/existing/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-param"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceSecret, "existing-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-secret"),
			StagedAt:  time.Now(),
		})

		// Agent has new param
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/new/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-param"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Mode: stagingusecase.StashPushModeOverwrite,
		})
		require.NoError(t, err)

		// New param should exist
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/new/param")
		require.NoError(t, err)

		// Existing param should be gone (overwritten)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/existing/param")
		require.ErrorIs(t, err, staging.ErrNotStaged)

		// Existing secret should also be gone (overwritten)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret")
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("merge mode combines with existing file", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Existing file has both param and secret
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/existing/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-param"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceSecret, "existing-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-secret"),
			StagedAt:  time.Now(),
		})

		// Agent has new param
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/new/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-param"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Mode: stagingusecase.StashPushModeMerge,
		})
		require.NoError(t, err)

		// All entries should exist
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/new/param")
		require.NoError(t, err)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/existing/param")
		require.NoError(t, err)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret")
		require.NoError(t, err)
	})

	t.Run("merge mode with conflicting keys - agent wins", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Existing file has param
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("old-value"),
			StagedAt:  time.Now(),
		})

		// Agent has same param with different value
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Mode: stagingusecase.StashPushModeMerge,
		})
		require.NoError(t, err)

		// Agent value should win
		entry, err := fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
	})
}

func TestPersistUseCase_ServiceSpecific_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("push param only when secret stash exists - preserves secrets", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Existing file has only secrets
		_ = fileStore.StageEntry(t.Context(), staging.ServiceSecret, "existing-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		// Agent has params
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		// Push param only
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Service: staging.ServiceParam,
		})
		require.NoError(t, err)

		// Param should exist in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)

		// Secret should still exist in file (preserved)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret")
		require.NoError(t, err)
	})

	t.Run("push secret only when param stash exists - preserves params", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Existing file has only params
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/existing/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})

		// Agent has secrets
		_ = agentStore.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		// Push secret only
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Service: staging.ServiceSecret,
		})
		require.NoError(t, err)

		// Secret should exist in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
		require.NoError(t, err)

		// Param should still exist in file (preserved)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/existing/param")
		require.NoError(t, err)
	})

	t.Run("push param when both services stash exists - replaces only param", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Existing file has both param and secret
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/old/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("old-param"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceSecret, "existing-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		// Agent has new param
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/new/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-param"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		// Push param only
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Service: staging.ServiceParam,
		})
		require.NoError(t, err)

		// New param should exist
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/new/param")
		require.NoError(t, err)

		// Old param should be gone (replaced)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/old/param")
		require.ErrorIs(t, err, staging.ErrNotStaged)

		// Secret should still exist (preserved)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret")
		require.NoError(t, err)
	})

	t.Run("service-specific push ignores Mode flag - always merges at service level", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Existing file has secret
		_ = fileStore.StageEntry(t.Context(), staging.ServiceSecret, "existing-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		// Agent has param
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		// Even with Overwrite mode, service-specific push preserves other services
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Service: staging.ServiceParam,
			Mode:    stagingusecase.StashPushModeOverwrite, // This is ignored for service-specific
		})
		require.NoError(t, err)

		// Param should exist
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)

		// Secret should still exist (service filter forces merge behavior)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret")
		require.NoError(t, err)
	})
}

func TestPersistUseCase_Execute_Errors(t *testing.T) {
	t.Parallel()

	t.Run("error on agent load", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		agentStore.DrainErr = errors.New("read error")
		fileStore := testutil.NewMockStore()

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})

		var persistErr *stagingusecase.StashPushError
		require.ErrorAs(t, err, &persistErr)
		assert.Equal(t, "load", persistErr.Op)
	})

	t.Run("error on file write", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		})
		fileStore.WriteStateErr = errors.New("write error")

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})

		var persistErr *stagingusecase.StashPushError
		require.ErrorAs(t, err, &persistErr)
		assert.Equal(t, "write", persistErr.Op)
	})
}

func TestPersistError(t *testing.T) {
	t.Parallel()

	t.Run("error message - load", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.StashPushError{Op: "load", Err: errors.New("connection failed")}
		assert.Contains(t, err.Error(), "failed to get state from agent")
		assert.Contains(t, err.Error(), "connection failed")
	})

	t.Run("error message - write", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.StashPushError{Op: "write", Err: errors.New("write failed")}
		assert.Contains(t, err.Error(), "failed to save state to file")
	})

	t.Run("error message - clear", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.StashPushError{Op: "clear", Err: errors.New("clear failed")}
		assert.Contains(t, err.Error(), "failed to clear agent memory")
	})

	t.Run("error message - unknown op", func(t *testing.T) {
		t.Parallel()

		innerErr := errors.New("something went wrong")
		err := &stagingusecase.StashPushError{Op: "unknown", Err: innerErr}
		assert.Equal(t, "something went wrong", err.Error())
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()

		innerErr := errors.New("inner error")
		err := &stagingusecase.StashPushError{Op: "load", Err: innerErr}
		assert.ErrorIs(t, err, innerErr)
	})
}

// =============================================================================
// HintedUnstager Tests
// =============================================================================

func TestPersistUseCase_WithHintedUnstager(t *testing.T) {
	t.Parallel()

	t.Run("uses persist hint for UnstageAll", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewHintedMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)

		// Verify persist hint was used
		assert.Equal(t, "persist", agentStore.LastHint)

		// Verify agent is cleared
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("hinted unstage error is non-fatal", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewHintedMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})
		agentStore.UnstageAllWithHintErr = errors.New("hinted unstage error")

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})
		// Should return output even on error since it's non-fatal
		assert.NotNil(t, output)
		assert.Equal(t, 1, output.EntryCount)

		// Error should be returned but as non-fatal
		var persistErr *stagingusecase.StashPushError
		require.ErrorAs(t, err, &persistErr)
		assert.Equal(t, "clear", persistErr.Op)
		assert.True(t, persistErr.NonFatal)

		// File should still have the entry (write succeeded)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
	})

	t.Run("non-hinted agent UnstageAll error is non-fatal", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})
		agentStore.UnstageAllErr = errors.New("unstage all error")

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})
		// Should return output even on error since it's non-fatal
		assert.NotNil(t, output)
		assert.Equal(t, 1, output.EntryCount)

		// Error should be returned but as non-fatal
		var persistErr *stagingusecase.StashPushError
		require.ErrorAs(t, err, &persistErr)
		assert.Equal(t, "clear", persistErr.Op)
		assert.True(t, persistErr.NonFatal)
	})
}

func TestPersistUseCase_ServiceSpecific_ClearError(t *testing.T) {
	t.Parallel()

	t.Run("service-specific WriteState error is non-fatal", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Agent has entries for both services
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

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		// First push succeeds
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Now make WriteState fail for the clearing step
		agentStore.WriteStateErr = errors.New("write state error")

		// Push secret service (param should already be cleared from previous push)
		// Re-add param to agent store since it was cleared
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value2"),
			StagedAt:  time.Now(),
		})

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Service: staging.ServiceParam})
		// Should return output even on error since it's non-fatal
		assert.NotNil(t, output)
		assert.Equal(t, 1, output.EntryCount)

		// Error should be returned but as non-fatal
		var persistErr *stagingusecase.StashPushError
		require.ErrorAs(t, err, &persistErr)
		assert.Equal(t, "clear", persistErr.Op)
		assert.True(t, persistErr.NonFatal)
	})
}

func TestPersistUseCase_FileDrainError_TreatedAsFresh(t *testing.T) {
	t.Parallel()

	t.Run("merge mode with file drain error starts fresh", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Agent has entries
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})

		// Make file drain fail (simulates file doesn't exist)
		fileStore.DrainErr = errors.New("file not found")

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		// Should succeed because file drain error is treated as empty state (start fresh)
		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Mode: stagingusecase.StashPushModeMerge})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)

		// Verify entry is in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
		require.NoError(t, err)
	})

	t.Run("service-specific push with file drain error starts fresh", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Agent has entries
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})

		// Make file drain fail (simulates file doesn't exist)
		fileStore.DrainErr = errors.New("file not found")

		usecase := &stagingusecase.StashPushUseCase{
			AgentStore: agentStore,
			FileStore:  fileStore,
		}

		// Should succeed because file drain error is treated as empty state (start fresh)
		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Service: staging.ServiceParam})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)

		// Verify entry is in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
		require.NoError(t, err)
	})
}
