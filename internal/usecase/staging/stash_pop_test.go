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

//nolint:funlen // Table-driven test with many cases
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{})
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{Keep: true})
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{Merge: true})
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{Force: true})
		require.NoError(t, err)

		// With force, agent should have file content
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
	})

	t.Run("drain error - nothing to drain", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{})
		assert.ErrorIs(t, err, stagingusecase.ErrNothingToStashPop)
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{})
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{Service: staging.ServiceParam})
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		output, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{})
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{})

		var drainErr *stagingusecase.StashPopError
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{})

		var drainErr *stagingusecase.StashPopError
		require.ErrorAs(t, err, &drainErr)
		assert.Equal(t, "write", drainErr.Op)
	})
}

func TestDrainError(t *testing.T) {
	t.Parallel()

	t.Run("error message - load", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.StashPopError{Op: "load", Err: errors.New("connection failed")}
		assert.Contains(t, err.Error(), "failed to load state from file")
		assert.Contains(t, err.Error(), "connection failed")
	})

	t.Run("error message - write", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.StashPopError{Op: "write", Err: errors.New("write failed")}
		assert.Contains(t, err.Error(), "failed to set state in agent")
	})

	t.Run("error message - delete", func(t *testing.T) {
		t.Parallel()

		err := &stagingusecase.StashPopError{Op: "delete", Err: errors.New("delete failed")}
		assert.Contains(t, err.Error(), "failed to delete file")
	})

	t.Run("error message - unknown op", func(t *testing.T) {
		t.Parallel()

		innerErr := errors.New("something went wrong")
		err := &stagingusecase.StashPopError{Op: "unknown", Err: innerErr}
		assert.Equal(t, "something went wrong", err.Error())
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()

		innerErr := errors.New("inner error")
		err := &stagingusecase.StashPopError{Op: "load", Err: innerErr}
		assert.ErrorIs(t, err, innerErr)
	})
}

// =============================================================================
// Service-Specific File Deletion Tests
// =============================================================================

func TestDrainUseCase_ServiceSpecific_FileDeleteErrors(t *testing.T) {
	t.Parallel()

	t.Run("write back error for service-specific drain when file has other services", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		// File has both param and secret entries
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

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		// Make WriteState fail (for writing back remaining state)
		fileStore.WriteStateErr = errors.New("write back error")

		// Execute should return output but with non-fatal error
		output, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{
			Service: staging.ServiceParam,
		})
		assert.NotNil(t, output)
		assert.Equal(t, 1, output.EntryCount)

		var drainErr *stagingusecase.StashPopError
		require.ErrorAs(t, err, &drainErr)
		assert.Equal(t, "delete", drainErr.Op)
		assert.True(t, drainErr.NonFatal)

		// Agent should still have the param entry (operation succeeded before cleanup)
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
		require.NoError(t, err)
	})
}

func TestDrainUseCase_ServiceSpecific_MergeWithAgentState(t *testing.T) {
	t.Parallel()

	t.Run("service-specific drain merges with existing agent state for other services", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		// Agent already has secret entries
		_ = agentStore.StageEntry(t.Context(), staging.ServiceSecret, "existing-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("agent-secret"),
			StagedAt:  time.Now(),
		})

		// File has param entries
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("file-param"),
			StagedAt:  time.Now(),
		})

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		_, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{
			Service: staging.ServiceParam,
		})
		require.NoError(t, err)

		// Both services should be in agent
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
		require.NoError(t, err)
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceSecret, "existing-secret")
		require.NoError(t, err)
	})
}

func TestDrainUseCase_AgentDrainError_TreatedAsEmpty(t *testing.T) {
	t.Parallel()

	fileStore := testutil.NewMockStore()
	agentStore := testutil.NewMockStore()

	// File has param entries
	_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("file-param"),
		StagedAt:  time.Now(),
	})

	// Make agent drain fail (simulates agent not running)
	agentStore.DrainErr = errors.New("agent not available")

	usecase := &stagingusecase.StashPopUseCase{
		FileStore:  fileStore,
		AgentStore: agentStore,
	}

	// Should succeed because agent drain error is treated as empty state
	output, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{})
	require.NoError(t, err)
	assert.Equal(t, 1, output.EntryCount)

	// Verify entry is in agent (write should succeed even though drain failed)
	_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
	require.NoError(t, err)
}

func TestDrainUseCase_ServiceSpecific_FileDeleteDrainError(t *testing.T) {
	t.Parallel()

	t.Run("drain error when deleting file after service removal - file becomes empty", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		// File has only param entries (will be empty after draining param service)
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})

		// Set DrainErr to fail on 2nd call (the delete call after successful initial drain)
		fileStore.DrainErr = errors.New("delete file error")
		fileStore.DrainErrOnCall = 2 // Fail on second Drain call only

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		// Execute should succeed with non-fatal error (state already transferred)
		output, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{
			Service: staging.ServiceParam,
		})
		require.NotNil(t, output)
		assert.Equal(t, 1, output.EntryCount)

		var drainErr *stagingusecase.StashPopError
		require.ErrorAs(t, err, &drainErr)
		assert.Equal(t, "delete", drainErr.Op)
		assert.True(t, drainErr.NonFatal)
	})

	t.Run("drain error when deleting file for full drain", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		// File has entries
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("param-value"),
			StagedAt:  time.Now(),
		})

		// Set DrainErr to fail on 2nd call (the delete call after successful initial drain)
		fileStore.DrainErr = errors.New("delete file error")
		fileStore.DrainErrOnCall = 2 // Fail on second Drain call only

		usecase := &stagingusecase.StashPopUseCase{
			FileStore:  fileStore,
			AgentStore: agentStore,
		}

		// Execute should succeed with non-fatal error (state already transferred)
		output, err := usecase.Execute(t.Context(), stagingusecase.StashPopInput{})
		require.NotNil(t, output)
		assert.Equal(t, 1, output.EntryCount)

		var drainErr *stagingusecase.StashPopError
		require.ErrorAs(t, err, &drainErr)
		assert.Equal(t, "delete", drainErr.Op)
		assert.True(t, drainErr.NonFatal)
	})
}
