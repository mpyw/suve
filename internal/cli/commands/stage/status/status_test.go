package status_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/stage/status"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/file"
)

func TestCommand_NoStagedChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

func TestCommand_ShowParamChangesOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value1"),
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes")
	assert.Contains(t, output, "/app/config")
	assert.NotContains(t, output, "Staged Secrets Manager changes")
}

func TestCommand_ShowSecretChangesOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged Secrets Manager changes")
	assert.Contains(t, output, "my-secret")
	assert.NotContains(t, output, "Staged SSM Parameter Store changes")
}

func TestCommand_ShowBothParamAndSecretChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  now,
	})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "M")
	assert.Contains(t, output, "Staged Secrets Manager changes")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "D")
}

func TestCommand_VerboseOutput(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("test-value"),
		StagedAt:  now,
	})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{Verbose: true})
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
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "Staged:")
	assert.NotContains(t, output, "Value:")
}

func TestCommand_VerboseTruncatesLongValue(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	longValue := "this is a very long value that exceeds one hundred characters and should be truncated in verbose mode output display"
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(longValue),
		StagedAt:  now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{Verbose: true})
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
	err := app.Run(t.Context(), []string{"suve", "status", "--help"})
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

	store := file.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err = r.Run(t.Context(), status.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestCommand_ShowParamTagChangesOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		StagedAt: now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "T")
	assert.Contains(t, output, "+2 tag(s)")
	assert.NotContains(t, output, "Staged Secrets Manager changes")
}

func TestCommand_ShowSecretTagChangesOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged Secrets Manager changes")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "T")
	assert.Contains(t, output, "+1 tag(s)")
	assert.Contains(t, output, "-1 tag(s)")
	assert.NotContains(t, output, "Staged SSM Parameter Store changes")
}

func TestCommand_ShowMixedEntryAndTagChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	// Entry change
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  now,
	})
	// Tag change (different resource)
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/other", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes (2)")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "M")
	assert.Contains(t, output, "/app/other")
	assert.Contains(t, output, "T")
}

func TestCommand_TagChangesVerbose(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		Remove:   maputil.NewSet("deprecated", "old"),
		StagedAt: now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{Verbose: true})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "T")
	assert.Contains(t, output, "/app/config")
	// Verbose output should show individual tags
	assert.Contains(t, output, "+ env=prod")
	assert.Contains(t, output, "+ team=api")
	assert.Contains(t, output, "- deprecated")
	assert.Contains(t, output, "- old")
}

func TestCommand_TagOnlyChangesNoEntries(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	now := time.Now()
	// Only tag changes, no entry changes
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/param", staging.TagEntry{
		Add:      map[string]string{"key": "value"},
		StagedAt: now,
	})
	_ = store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Remove:   maputil.NewSet("old-tag"),
		StagedAt: now,
	})

	var buf bytes.Buffer
	r := &status.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(t.Context(), status.Options{})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Staged SSM Parameter Store changes (1)")
	assert.Contains(t, output, "/app/param")
	assert.Contains(t, output, "Staged Secrets Manager changes (1)")
	assert.Contains(t, output, "my-secret")
	assert.NotContains(t, output, "No changes staged")
}
