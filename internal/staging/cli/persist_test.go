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

func TestPersistRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("success - basic persist unencrypted", func(t *testing.T) {
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

		runner := &PersistRunner{
			UseCase: &stagingusecase.PersistUseCase{
				AgentStore: agentStore,
				FileStore:  fileStore,
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), PersistOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged changes persisted to file")
		assert.Contains(t, stdout.String(), "cleared from memory")
		// Should warn about plain text
		assert.Contains(t, stderr.String(), "plain text")
	})

	t.Run("success - persist encrypted", func(t *testing.T) {
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

		runner := &PersistRunner{
			UseCase: &stagingusecase.PersistUseCase{
				AgentStore: agentStore,
				FileStore:  fileStore,
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: true,
		}

		err := runner.Run(t.Context(), PersistOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "encrypted")
		// Should NOT warn about plain text when encrypted
		assert.NotContains(t, stderr.String(), "plain text")
	})

	t.Run("success - persist with keep", func(t *testing.T) {
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

		runner := &PersistRunner{
			UseCase: &stagingusecase.PersistUseCase{
				AgentStore: agentStore,
				FileStore:  fileStore,
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), PersistOptions{Keep: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "kept in memory")
	})

	t.Run("success - persist encrypted with keep", func(t *testing.T) {
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

		runner := &PersistRunner{
			UseCase: &stagingusecase.PersistUseCase{
				AgentStore: agentStore,
				FileStore:  fileStore,
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: true,
		}

		err := runner.Run(t.Context(), PersistOptions{Keep: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "encrypted")
		assert.Contains(t, stdout.String(), "kept in memory")
	})

	t.Run("error - nothing to persist", func(t *testing.T) {
		t.Parallel()

		agentStore := testutil.NewMockStore()
		fileStore := testutil.NewMockStore()

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &PersistRunner{
			UseCase: &stagingusecase.PersistUseCase{
				AgentStore: agentStore,
				FileStore:  fileStore,
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), PersistOptions{})
		assert.ErrorIs(t, err, stagingusecase.ErrNothingToPersist)
	})

	t.Run("success - persist with service filter", func(t *testing.T) {
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

		runner := &PersistRunner{
			UseCase: &stagingusecase.PersistUseCase{
				AgentStore: agentStore,
				FileStore:  fileStore,
			},
			Stdout:    stdout,
			Stderr:    stderr,
			Encrypted: false,
		}

		err := runner.Run(t.Context(), PersistOptions{Service: staging.ServiceParam})
		require.NoError(t, err)

		// Param should be in file
		_, err = fileStore.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)

		// Secret should still be in agent
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
		require.NoError(t, err)
	})
}

func TestPersistFlags(t *testing.T) {
	t.Parallel()

	flags := persistFlags()
	assert.Len(t, flags, 2)

	flagNames := make([]string, len(flags))
	for i, f := range flags {
		flagNames[i] = f.Names()[0]
	}

	assert.Contains(t, flagNames, "keep")
	assert.Contains(t, flagNames, "passphrase-stdin")
}

func TestNewGlobalPersistCommand(t *testing.T) {
	t.Parallel()

	cmd := NewGlobalPersistCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "persist", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.Action)
	assert.Len(t, cmd.Flags, 2)
}

func TestPersistRunner_NonFatalError(t *testing.T) {
	t.Parallel()

	// Test that non-fatal errors are properly identified
	nonFatalErr := &stagingusecase.PersistError{
		Op:       "clear",
		Err:      errors.New("agent clear failed"),
		NonFatal: true,
	}

	assert.True(t, nonFatalErr.NonFatal)
	assert.Contains(t, nonFatalErr.Error(), "failed to clear agent memory")
}
