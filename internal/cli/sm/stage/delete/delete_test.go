package delete_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	smdelete "github.com/mpyw/suve/internal/cli/sm/stage/delete"
	"github.com/mpyw/suve/internal/stage"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "delete", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Stage a secret for deletion")
	})

	t.Run("no argument", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "delete"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("invalid recovery window too low", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "delete", "--recovery-window", "5", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recovery window must be between 7 and 30")
	})

	t.Run("invalid recovery window too high", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "delete", "--recovery-window", "31", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recovery window must be between 7 and 30")
	})
}

func TestRun_StageDelete(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var stdout, stderr bytes.Buffer
	r := &smdelete.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err := r.Run(context.Background(), smdelete.Options{
		Name:           "my-secret",
		RecoveryWindow: 30,
	})
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "✓")
	assert.Contains(t, stdout.String(), "Staged for deletion")
	assert.Contains(t, stdout.String(), "30-day recovery")
	assert.Contains(t, stdout.String(), "my-secret")

	// Verify staged
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, stage.OperationDelete, entry.Operation)
	require.NotNil(t, entry.DeleteOptions)
	assert.False(t, entry.DeleteOptions.Force)
	assert.Equal(t, 30, entry.DeleteOptions.RecoveryWindow)
}

func TestRun_StageDeleteWithForce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var stdout, stderr bytes.Buffer
	r := &smdelete.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err := r.Run(context.Background(), smdelete.Options{
		Name:  "my-secret",
		Force: true,
	})
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "✓")
	assert.Contains(t, stdout.String(), "immediate deletion")
	assert.Contains(t, stdout.String(), "my-secret")

	// Verify staged with force option
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, stage.OperationDelete, entry.Operation)
	require.NotNil(t, entry.DeleteOptions)
	assert.True(t, entry.DeleteOptions.Force)
}

func TestRun_StageDeleteWithCustomRecoveryWindow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var stdout, stderr bytes.Buffer
	r := &smdelete.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err := r.Run(context.Background(), smdelete.Options{
		Name:           "my-secret",
		RecoveryWindow: 7,
	})
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "7-day recovery")

	// Verify staged with custom recovery window
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	require.NotNil(t, entry.DeleteOptions)
	assert.Equal(t, 7, entry.DeleteOptions.RecoveryWindow)
}

func TestRun_OverwriteExisting(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a set operation first
	err := store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "old-value",
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	r := &smdelete.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// Now stage delete (should overwrite)
	err = r.Run(context.Background(), smdelete.Options{
		Name:           "my-secret",
		RecoveryWindow: 30,
	})
	require.NoError(t, err)

	// Verify changed to delete
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, stage.OperationDelete, entry.Operation)
	assert.Empty(t, entry.Value)
}
