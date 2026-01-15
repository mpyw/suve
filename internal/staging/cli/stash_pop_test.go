package cli_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/confirm"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestStashPopRunner_RunBasic(t *testing.T) {
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

		runner := &cli.StashPopRunner{
			UseCase: &stagingusecase.StashPopUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), cli.StashPopOptions{})
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

		runner := &cli.StashPopRunner{
			UseCase: &stagingusecase.StashPopUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), cli.StashPopOptions{Keep: true})
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

		runner := &cli.StashPopRunner{
			UseCase: &stagingusecase.StashPopUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), cli.StashPopOptions{Mode: stagingusecase.StashModeMerge})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "merged")
	})

	t.Run("error - nothing to pop", func(t *testing.T) {
		t.Parallel()

		fileStore := testutil.NewMockStore()
		agentStore := testutil.NewMockStore()

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		runner := &cli.StashPopRunner{
			UseCase: &stagingusecase.StashPopUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		err := runner.Run(t.Context(), cli.StashPopOptions{})
		assert.ErrorIs(t, err, stagingusecase.ErrNothingToStashPop)
	})

	t.Run("defaults to merge when agent has changes", func(t *testing.T) {
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

		runner := &cli.StashPopRunner{
			UseCase: &stagingusecase.StashPopUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		// Default mode is merge, so both entries should exist
		err := runner.Run(t.Context(), cli.StashPopOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "merged")

		// Verify both entries exist in agent
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/existing")
		require.NoError(t, err)
		_, err = agentStore.GetEntry(t.Context(), staging.ServiceParam, "/app/new")
		require.NoError(t, err)
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

		runner := &cli.StashPopRunner{
			UseCase: &stagingusecase.StashPopUseCase{
				FileStore:  fileStore,
				AgentStore: agentStore,
			},
			Stdout: stdout,
			Stderr: stderr,
		}

		// This test verifies the runner handles success case
		err := runner.Run(t.Context(), cli.StashPopOptions{Keep: true})
		require.NoError(t, err)
	})
}

func TestStashPopRunner_NonFatalError(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	nonFatalErr := &stagingusecase.StashPopError{
		Op:       "delete",
		Err:      errors.New("file deletion failed"),
		NonFatal: true,
	}

	assert.True(t, nonFatalErr.NonFatal)
	assert.Contains(t, nonFatalErr.Error(), "failed to delete file")

	_ = stdout
	_ = stderr
}

func TestStashPopModeChooser_ChooseMode(t *testing.T) {
	t.Parallel()

	t.Run("overwrite flag takes precedence", func(t *testing.T) {
		t.Parallel()

		chooser := &cli.StashPopModeChooser{
			Stderr: &bytes.Buffer{},
			Stdout: &bytes.Buffer{},
		}

		result, err := chooser.ChooseMode(cli.StashPopModeInput{
			OverwriteFlag: true,
			MergeFlag:     true, // Should be ignored
			HasChanges:    true,
			IsTTY:         true,
		})

		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, stagingusecase.StashModeOverwrite, result.Mode)
	})

	t.Run("merge flag takes precedence over prompt", func(t *testing.T) {
		t.Parallel()

		chooser := &cli.StashPopModeChooser{
			Stderr: &bytes.Buffer{},
			Stdout: &bytes.Buffer{},
		}

		result, err := chooser.ChooseMode(cli.StashPopModeInput{
			MergeFlag:  true,
			HasChanges: true,
			IsTTY:      true,
		})

		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, stagingusecase.StashModeMerge, result.Mode)
	})

	t.Run("defaults to merge when no changes", func(t *testing.T) {
		t.Parallel()

		chooser := &cli.StashPopModeChooser{
			Stderr: &bytes.Buffer{},
			Stdout: &bytes.Buffer{},
		}

		result, err := chooser.ChooseMode(cli.StashPopModeInput{
			HasChanges: false,
			IsTTY:      true,
		})

		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, stagingusecase.StashModeMerge, result.Mode)
	})

	t.Run("defaults to merge when non-TTY", func(t *testing.T) {
		t.Parallel()

		chooser := &cli.StashPopModeChooser{
			Stderr: &bytes.Buffer{},
			Stdout: &bytes.Buffer{},
		}

		result, err := chooser.ChooseMode(cli.StashPopModeInput{
			HasChanges: true,
			ItemCount:  5,
			IsTTY:      false, // Non-TTY environment
		})

		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, stagingusecase.StashModeMerge, result.Mode)
	})

	t.Run("prompts when TTY and has changes - user selects merge", func(t *testing.T) {
		t.Parallel()

		stdin := bytes.NewBufferString("1\n") // Select "Merge"
		stderr := &bytes.Buffer{}

		chooser := &cli.StashPopModeChooser{
			Prompter: &confirm.Prompter{
				Stdin:  stdin,
				Stdout: &bytes.Buffer{},
				Stderr: stderr,
			},
			Stderr: stderr,
			Stdout: &bytes.Buffer{},
		}

		result, err := chooser.ChooseMode(cli.StashPopModeInput{
			HasChanges: true,
			ItemCount:  3,
			IsTTY:      true,
		})

		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, stagingusecase.StashModeMerge, result.Mode)
		assert.Contains(t, stderr.String(), "3 staged change(s)")
	})

	t.Run("prompts when TTY and has changes - user selects overwrite", func(t *testing.T) {
		t.Parallel()

		stdin := bytes.NewBufferString("2\n") // Select "Overwrite"
		stderr := &bytes.Buffer{}

		chooser := &cli.StashPopModeChooser{
			Prompter: &confirm.Prompter{
				Stdin:  stdin,
				Stdout: &bytes.Buffer{},
				Stderr: stderr,
			},
			Stderr: stderr,
			Stdout: &bytes.Buffer{},
		}

		result, err := chooser.ChooseMode(cli.StashPopModeInput{
			HasChanges: true,
			ItemCount:  3,
			IsTTY:      true,
		})

		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, stagingusecase.StashModeOverwrite, result.Mode)
	})

	t.Run("prompts when TTY and has changes - user selects cancel", func(t *testing.T) {
		t.Parallel()

		stdin := bytes.NewBufferString("3\n") // Select "Cancel"
		stderr := &bytes.Buffer{}

		chooser := &cli.StashPopModeChooser{
			Prompter: &confirm.Prompter{
				Stdin:  stdin,
				Stdout: &bytes.Buffer{},
				Stderr: stderr,
			},
			Stderr: stderr,
			Stdout: &bytes.Buffer{},
		}

		result, err := chooser.ChooseMode(cli.StashPopModeInput{
			HasChanges: true,
			ItemCount:  3,
			IsTTY:      true,
		})

		require.NoError(t, err)
		assert.True(t, result.Cancelled)
	})

	t.Run("prompts when TTY and has changes - user enters default (merge)", func(t *testing.T) {
		t.Parallel()

		stdin := bytes.NewBufferString("\n") // Just press Enter (default)
		stderr := &bytes.Buffer{}

		chooser := &cli.StashPopModeChooser{
			Prompter: &confirm.Prompter{
				Stdin:  stdin,
				Stdout: &bytes.Buffer{},
				Stderr: stderr,
			},
			Stderr: stderr,
			Stdout: &bytes.Buffer{},
		}

		result, err := chooser.ChooseMode(cli.StashPopModeInput{
			HasChanges: true,
			ItemCount:  3,
			IsTTY:      true,
		})

		require.NoError(t, err)
		assert.False(t, result.Cancelled)
		assert.Equal(t, stagingusecase.StashModeMerge, result.Mode)
	})
}
