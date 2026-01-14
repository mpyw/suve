package cli_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestStashPushRunner_RunBasic(t *testing.T) {
	t.Parallel()

	t.Run("success - basic stash push unencrypted", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPushRunner{
			UseCase: &stagingusecase.StashPushUseCase{
				AgentStore: agentStore.Global(),
				FileStore:  fileStore.Global(),
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), cli.StashPushOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged changes stashed to file")
		assert.Contains(t, stdout.String(), "cleared from memory")
		// Should warn about plain text
		assert.Contains(t, stderr.String(), "plain text")
	})

	t.Run("success - stash push encrypted", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPushRunner{
			UseCase: &stagingusecase.StashPushUseCase{
				AgentStore: agentStore.Global(),
				FileStore:  fileStore.Global(),
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: true,
		}

		err := runner.Run(t.Context(), cli.StashPushOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "encrypted")
		// Should NOT warn about plain text when encrypted
		assert.NotContains(t, stderr.String(), "plain text")
	})

	t.Run("success - stash push with keep", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPushRunner{
			UseCase: &stagingusecase.StashPushUseCase{
				AgentStore: agentStore.Global(),
				FileStore:  fileStore.Global(),
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), cli.StashPushOptions{Keep: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "kept in memory")
	})

	t.Run("success - stash push encrypted with keep", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPushRunner{
			UseCase: &stagingusecase.StashPushUseCase{
				AgentStore: agentStore.Global(),
				FileStore:  fileStore.Global(),
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: true,
		}

		err := runner.Run(t.Context(), cli.StashPushOptions{Keep: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "encrypted")
		assert.Contains(t, stdout.String(), "kept in memory")
	})

	t.Run("error - nothing to stash", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPushRunner{
			UseCase: &stagingusecase.StashPushUseCase{
				AgentStore: agentStore.Global(),
				FileStore:  fileStore.Global(),
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), cli.StashPushOptions{})
		assert.ErrorIs(t, err, stagingusecase.ErrNothingToStashPush)
	})

	t.Run("success - stash push with service filter", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		})
		_ = agentStore.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPushRunner{
			UseCase: &stagingusecase.StashPushUseCase{
				AgentStore: agentStore.Global(),
				FileStore:  fileStore.Global(),
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), cli.StashPushOptions{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Param should be in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)

		// Secret should still be in agent
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
		require.NoError(t, err)
	})
}

func TestStashPushRunner_NonFatalError(t *testing.T) {
	t.Parallel()

	// Test that non-fatal errors are properly identified
	nonFatalErr := &stagingusecase.StashPushError{
		Op:       "clear",
		Err:      errors.New("agent clear failed"),
		NonFatal: true,
	}

	assert.True(t, nonFatalErr.NonFatal)
	assert.Contains(t, nonFatalErr.Error(), "failed to clear agent memory")
}

func TestStashPushRunner_Run_NonFatalErrorContinues(t *testing.T) {
	t.Parallel()

	agentStore := testutil.NewMockStore()
	fileStore := testutil.NewMockStore()

	// Add entries to both services
	_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("test-value"),
		StagedAt:  time.Now(),
	})
	_ = agentStore.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	// Simulate agent store error during WriteState (agent cleanup path for service-specific persist)
	// This triggers the non-fatal error path at line 105-106 in persist.go
	agentStore.WriteStateErr = errors.New("agent unavailable")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	runner := &cli.StashPushRunner{
		UseCase: &stagingusecase.StashPushUseCase{
			AgentStore: agentStore.Global(),
			FileStore:  fileStore.Global(),
		},
		Stdout:    stdout,
		Stderr:    stderr,
		Encrypted: false,
	}

	// Use service filter to trigger the WriteState path in the usecase
	err := runner.Run(t.Context(), cli.StashPushOptions{Service: staging.ServiceParam})
	// Should succeed because the state was written (agent clear is non-fatal)
	require.NoError(t, err)

	// Should show warning about the error
	assert.Contains(t, stderr.String(), "Warning")

	// Should still show success message
	assert.Contains(t, stdout.String(), "Staged changes stashed to file")
}

func TestStashPushRunner_Run_WithModes(t *testing.T) {
	t.Parallel()

	t.Run("mode overwrite", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Pre-populate file store with existing data
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/existing", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-value"),
			StagedAt:  time.Now(),
		})

		// Add new data to agent
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/new", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPushRunner{
			UseCase: &stagingusecase.StashPushUseCase{
				AgentStore: agentStore.Global(),
				FileStore:  fileStore.Global(),
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), cli.StashPushOptions{Mode: stagingusecase.StashPushModeOverwrite})
		require.NoError(t, err)

		// New data should be in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/new")
		require.NoError(t, err)

		// Existing data should be removed
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/existing")
		require.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("mode merge", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		// Pre-populate file store with existing data
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/existing", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-value"),
			StagedAt:  time.Now(),
		})

		// Add new data to agent
		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/new", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPushRunner{
			UseCase: &stagingusecase.StashPushUseCase{
				AgentStore: agentStore.Global(),
				FileStore:  fileStore.Global(),
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), cli.StashPushOptions{Mode: stagingusecase.StashPushModeMerge})
		require.NoError(t, err)

		// New data should be in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/new")
		require.NoError(t, err)

		// Existing data should be preserved
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/existing")
		require.NoError(t, err)
	})
}
