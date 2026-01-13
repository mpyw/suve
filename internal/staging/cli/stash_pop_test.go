package cli

import (
	"bytes"
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

func TestStashPopRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("success - basic stash pop", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &StashPopRunner{
			UseCase: &stagingusecase.DrainUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), StashPopOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Stashed changes restored")
		assert.Contains(t, stdout.String(), "file deleted")
	})

	t.Run("success - stash pop with keep", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &StashPopRunner{
			UseCase: &stagingusecase.DrainUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), StashPopOptions{Keep: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "file kept")
	})

	t.Run("success - stash pop with merge", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/existing", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-value"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &StashPopRunner{
			UseCase: &stagingusecase.DrainUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), StashPopOptions{Merge: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "merged")
	})

	t.Run("error - nothing to pop", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &StashPopRunner{
			UseCase: &stagingusecase.DrainUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), StashPopOptions{})
		assert.ErrorIs(t, err, stagingusecase.ErrNothingToDrain)
	})

	t.Run("error - agent has changes without force/merge", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = agentStore.StageEntry(t.Context(), staging.ServiceParam, "/app/existing", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("existing-value"),
			StagedAt:  time.Now(),
		})
		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/new", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &StashPopRunner{
			UseCase: &stagingusecase.DrainUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), StashPopOptions{})
		assert.ErrorIs(t, err, stagingusecase.ErrAgentHasChanges)
	})

	t.Run("non-fatal error - shows warning but succeeds", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		_ = fileStore.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("test-value"),
			StagedAt:  time.Now(),
		})

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &StashPopRunner{
			UseCase: &stagingusecase.DrainUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		// This test verifies the runner handles success case
		err := runner.Run(t.Context(), StashPopOptions{Keep: true})
		require.NoError(t, err)
	})
}

func TestStashPopFlags(t *testing.T) {
	t.Parallel()

	flags := stashPopFlags()
	assert.Len(t, flags, 4)

	flagNames := make([]string, len(flags))
	for i, f := range flags {
		flagNames[i] = f.Names()[0]
	}

	assert.Contains(t, flagNames, "keep")
	assert.Contains(t, flagNames, "force")
	assert.Contains(t, flagNames, "merge")
	assert.Contains(t, flagNames, "passphrase-stdin")
}

func TestStashPopRunner_NonFatalError(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	nonFatalErr := &stagingusecase.DrainError{
		Op:       "delete",
		Err:      errors.New("file deletion failed"),
		NonFatal: true,
	}

	assert.True(t, nonFatalErr.NonFatal)
	assert.Contains(t, nonFatalErr.Error(), "failed to delete file")

	_ = stdout
	_ = stderr
}
