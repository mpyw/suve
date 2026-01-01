package delete_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	ssmdelete "github.com/mpyw/suve/internal/cli/ssm/stage/delete"
	"github.com/mpyw/suve/internal/stage"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "ssm", "stage", "delete", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Stage a parameter for deletion")
	})

	t.Run("no argument", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "stage", "delete"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

func TestRun_StageDelete(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var stdout, stderr bytes.Buffer
	r := &ssmdelete.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err := r.Run(context.Background(), ssmdelete.Options{Name: "/app/config"})
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "✓")
	assert.Contains(t, stdout.String(), "Staged for deletion")
	assert.Contains(t, stdout.String(), "/app/config")

	// Verify staged
	entry, err := store.Get(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, stage.OperationDelete, entry.Operation)
}

func TestRun_OverwriteExisting(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a set operation first
	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "old-value",
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	r := &ssmdelete.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// Now stage delete (should overwrite)
	err = r.Run(context.Background(), ssmdelete.Options{Name: "/app/config"})
	require.NoError(t, err)

	// Verify changed to delete
	entry, err := store.Get(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, stage.OperationDelete, entry.Operation)
	assert.Empty(t, entry.Value)
}
