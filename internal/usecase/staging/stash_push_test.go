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
			Working: agentStore,
			Stash:   fileStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})
		require.NoError(t, err)
		assert.Equal(t, 1, output.EntryCount)

		// Verify entry moved to file
		entry, err := fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
		require.NoError(t, err)
		assert.Equal(t, "agent-value", lo.FromPtr(entry.Value))

		// Verify agent is cleared
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
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
			Working: agentStore,
			Stash:   fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Keep: true})
		require.NoError(t, err)

		// Verify entry exists in both
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
		require.NoError(t, err)
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
		require.NoError(t, err)
	})

	t.Run("persist error - nothing to persist", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		usecase := &stagingusecase.StashPushUseCase{
			Working: agentStore,
			Stash:   fileStore,
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
			Working: agentStore,
			Stash:   fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Param should be in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param", "")
		require.NoError(t, err)

		// Secret should still be in agent
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceSecret, "my-secret", "")
		require.NoError(t, err)

		// Param should be cleared from the working staging area (not kept)
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param", "")
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
			Working: agentStore,
			Stash:   fileStore,
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
			Working: agentStore,
			Stash:   fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Both should exist in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
		require.NoError(t, err)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret", "")
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
			Working: agentStore,
			Stash:   fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Mode: stagingusecase.StashModeOverwrite,
		})
		require.NoError(t, err)

		// New param should exist
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/new/param", "")
		require.NoError(t, err)

		// Existing param should be gone (overwritten)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/existing/param", "")
		require.ErrorIs(t, err, staging.ErrNotStaged)

		// Existing secret should also be gone (overwritten)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret", "")
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
			Working: agentStore,
			Stash:   fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Mode: stagingusecase.StashModeMerge,
		})
		require.NoError(t, err)

		// All entries should exist
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/new/param", "")
		require.NoError(t, err)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/existing/param", "")
		require.NoError(t, err)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret", "")
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
			Working: agentStore,
			Stash:   fileStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Mode: stagingusecase.StashModeMerge,
		})
		require.NoError(t, err)

		// Agent value should win
		entry, err := fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
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
			Working: agentStore,
			Stash:   fileStore,
		}

		// Push param only
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Service: staging.ServiceParam,
		})
		require.NoError(t, err)

		// Param should exist in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
		require.NoError(t, err)

		// Secret should still exist in file (preserved)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret", "")
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
			Working: agentStore,
			Stash:   fileStore,
		}

		// Push secret only
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Service: staging.ServiceSecret,
		})
		require.NoError(t, err)

		// Secret should exist in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "my-secret", "")
		require.NoError(t, err)

		// Param should still exist in file (preserved)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/existing/param", "")
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
			Working: agentStore,
			Stash:   fileStore,
		}

		// Push param only with overwrite (replaces existing param entries)
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Service: staging.ServiceParam,
			Mode:    stagingusecase.StashModeOverwrite,
		})
		require.NoError(t, err)

		// New param should exist
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/new/param", "")
		require.NoError(t, err)

		// Old param should be gone (replaced due to overwrite mode)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/old/param", "")
		require.ErrorIs(t, err, staging.ErrNotStaged)

		// Secret should still exist (preserved)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret", "")
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
			Working: agentStore,
			Stash:   fileStore,
		}

		// Even with Overwrite mode, service-specific push preserves other services
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{
			Service: staging.ServiceParam,
			Mode:    stagingusecase.StashModeOverwrite, // This is ignored for service-specific
		})
		require.NoError(t, err)

		// Param should exist
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
		require.NoError(t, err)

		// Secret should still exist (service filter forces merge behavior)
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret", "")
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
			Working: agentStore,
			Stash:   fileStore,
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
			Working: agentStore,
			Stash:   fileStore,
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
		assert.Contains(t, err.Error(), "failed to read the working staging area")
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
		assert.Contains(t, err.Error(), "failed to clear the working staging area")
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

func TestPersistUseCase_GlobalClearError(t *testing.T) {
	t.Parallel()

	t.Run("global clear (working WriteState) error is non-fatal", func(t *testing.T) {
		t.Parallel()

		workingStore := testutil.NewMockStore()
		stashStore := testutil.NewMockStore()

		_ = workingStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPushUseCase{
			Working: workingStore,
			Stash:   stashStore,
		}

		// Make the working-area clear step fail. The stash write happens first
		// (and succeeds), so this is a non-fatal error.
		workingStore.WriteStateErr = errors.New("clear error")

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{})
		// Should return output even on error since it's non-fatal
		assert.NotNil(t, output)
		assert.Equal(t, 1, output.EntryCount)

		// Error should be returned but as non-fatal
		var persistErr *stagingusecase.StashPushError
		require.ErrorAs(t, err, &persistErr)
		assert.Equal(t, "clear", persistErr.Op)
		assert.True(t, persistErr.NonFatal)

		// Stash should still have the entry (write succeeded)
		_, err = stashStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config", "")
		require.NoError(t, err)
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
			Working: agentStore,
			Stash:   fileStore,
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

// TestPersistUseCase_StashDrainError_Propagated: a real stash-read failure
// (wrong passphrase, corrupt/unreadable file — a missing file returns an empty
// state with a nil error) must NOT be swallowed and treated as "start fresh",
// which would overwrite the existing stash with only the working data (#320).
func TestPersistUseCase_StashDrainError_Propagated(t *testing.T) {
	t.Parallel()

	newCase := func(t *testing.T) (*testutil.MockStore, *stagingusecase.StashPushUseCase) {
		t.Helper()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-value"),
			StagedAt:  time.Now(),
		})

		fileStore.DrainErr = errors.New("decryption failed: wrong passphrase or corrupted data")

		return fileStore, &stagingusecase.StashPushUseCase{Working: agentStore, Stash: fileStore}
	}

	assertNotOverwritten := func(t *testing.T, fileStore *testutil.MockStore, err error) {
		t.Helper()

		require.Error(t, err)

		var pushErr *stagingusecase.StashPushError

		require.ErrorAs(t, err, &pushErr)
		assert.Contains(t, err.Error(), "stash file")

		// The stash must not have been overwritten (WriteState never reached).
		_, getErr := fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param", "")
		require.ErrorIs(t, getErr, staging.ErrNotStaged)
	}

	t.Run("merge mode propagates the stash read error", func(t *testing.T) {
		t.Parallel()

		fileStore, usecase := newCase(t)
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Mode: stagingusecase.StashModeMerge})
		assertNotOverwritten(t, fileStore, err)
	})

	t.Run("service-specific push propagates the stash read error", func(t *testing.T) {
		t.Parallel()

		fileStore, usecase := newCase(t)
		_, err := usecase.Execute(t.Context(), stagingusecase.StashPushInput{Service: staging.ServiceParam})
		assertNotOverwritten(t, fileStore, err)
	})
}
