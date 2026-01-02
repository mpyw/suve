package status_test

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
	"github.com/mpyw/suve/internal/cli/commands/stage/status"
	"github.com/mpyw/suve/internal/staging"
)

func TestCommand_NoStagedChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), status.Options{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

func TestCommand_ShowSSMChangesOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "value1",
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM changes")
	assert.Contains(t, output, "/app/config")
	assert.NotContains(t, output, "Staged SM changes")
}

func TestCommand_ShowSMChangesOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "secret-value",
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SM changes")
	assert.Contains(t, output, "my-secret")
	assert.NotContains(t, output, "Staged SSM changes")
}

func TestCommand_ShowBothSSMAndSMChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "param-value",
		StagedAt:  now,
	})
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM changes")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "M")
	assert.Contains(t, output, "Staged SM changes")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "D")
}

func TestCommand_VerboseOutput(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "test-value",
		StagedAt:  now,
	})
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "secret-value",
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged:")
	assert.Contains(t, output, "Value:")
	assert.Contains(t, output, "test-value")
	assert.Contains(t, output, "secret-value")
}

func TestCommand_VerboseWithDelete(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "Staged:")
	assert.NotContains(t, output, "Value:")
}

func TestCommand_VerboseTruncatesLongValue(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	longValue := "this is a very long value that exceeds one hundred characters and should be truncated in verbose mode output display"
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     longValue,
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "...")
	assert.NotContains(t, output, "display")
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	app := appcli.MakeApp()
	var buf bytes.Buffer
	app.Writer = &buf

	// Test that the command exists and works
	err := app.Run(context.Background(), []string{"suve", "status", "--help"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "staged changes")
}

func TestCommand_StoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	err := os.WriteFile(path, []byte("invalid json"), 0o600)
	require.NoError(t, err)

	store := staging.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err = r.Run(context.Background(), status.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
