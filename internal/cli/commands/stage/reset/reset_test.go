package reset_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
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

	// Stage SSM Parameter Store parameters
	_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value1"),
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value2"),
		StagedAt:  time.Now(),
	})

	// Stage Secrets Manager secrets
	_ = store.Stage(staging.ServiceSecret, "secret1", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value1"),
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
	assert.Contains(t, buf.String(), "Unstaged all changes (2 SSM Parameter Store, 1 Secrets Manager)")

	// Verify all unstaged
	_, err = store.Get(staging.ServiceParam, "/app/config1")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.Get(staging.ServiceParam, "/app/config2")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.Get(staging.ServiceSecret, "secret1")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_UnstageParamOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only SSM Parameter Store parameters
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
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
	assert.Contains(t, buf.String(), "Unstaged all changes (1 SSM Parameter Store, 0 Secrets Manager)")
}

func TestRun_UnstageSecretOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only Secrets Manager secrets
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
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
	assert.Contains(t, buf.String(), "Unstaged all changes (0 SSM Parameter Store, 1 Secrets Manager)")
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
