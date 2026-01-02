package reset_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/stage/reset"
	"github.com/mpyw/suve/internal/staging"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "stage", "reset", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Unstage all changes")
	})
}

func TestRun_NoChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

func TestRun_UnstageAll(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage SSM parameters
	_ = store.Stage(staging.ServiceSSM, "/app/config1", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "ssm-value1",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceSSM, "/app/config2", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "ssm-value2",
		StagedAt:  time.Now(),
	})

	// Stage SM secrets
	_ = store.Stage(staging.ServiceSM, "secret1", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "sm-value1",
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (2 SSM, 1 SM)")

	// Verify all unstaged
	_, err = store.Get(staging.ServiceSSM, "/app/config1")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.Get(staging.ServiceSSM, "/app/config2")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.Get(staging.ServiceSM, "secret1")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_UnstageSSMOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only SSM parameters
	_ = store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "ssm-value",
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (1 SSM, 0 SM)")
}

func TestRun_UnstageSMOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only SM secrets
	_ = store.Stage(staging.ServiceSM, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "sm-value",
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all changes (0 SSM, 1 SM)")
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	require.NoError(t, os.WriteFile(path, []byte("invalid json"), 0o644))

	store := staging.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
