package runner_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/runner"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestAddRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("create new item", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var buf bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout:     &buf,
			Stderr:     &bytes.Buffer{},
			OpenEditor: func(_ string) (string, error) { return "new-value", nil },
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Staged for creation")
		assert.Contains(t, buf.String(), "/app/config")

		// Verify staged with OperationCreate
		entry, err := store.Get(staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
	})

	t.Run("edit already staged create", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		// Pre-stage as create
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("original-value"),
		})

		var buf bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &buf,
			Stderr: &bytes.Buffer{},
			OpenEditor: func(current string) (string, error) {
				assert.Equal(t, "original-value", current)
				return "updated-value", nil
			},
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Staged for creation")

		// Verify updated
		entry, err := store.Get(staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "updated-value", lo.FromPtr(entry.Value))
	})

	t.Run("empty value not staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var buf bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout:     &buf,
			Stderr:     &bytes.Buffer{},
			OpenEditor: func(_ string) (string, error) { return "", nil },
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Empty value")

		// Verify not staged
		_, err = store.Get(staging.ServiceParam, "/app/config")
		assert.Equal(t, staging.ErrNotStaged, err)
	})

	t.Run("no changes made", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		// Pre-stage as create
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("same-value"),
		})

		var buf bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &buf,
			Stderr: &bytes.Buffer{},
			OpenEditor: func(_ string) (string, error) {
				return "same-value", nil
			},
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No changes made")
	})

	t.Run("Secrets Manager service", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var buf bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceSecret},
				Store:    store,
			},
			Stdout:     &buf,
			Stderr:     &bytes.Buffer{},
			OpenEditor: func(_ string) (string, error) { return "secret-value", nil },
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "my-secret"})
		require.NoError(t, err)

		// Verify staged with correct service
		entry, err := store.Get(staging.ServiceSecret, "my-secret")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "secret-value", lo.FromPtr(entry.Value))
	})
}

// mockStrategy implements staging.Parser for testing.
type mockStrategy struct {
	service      staging.Service
	parseNameErr error
}

func (m *mockStrategy) Service() staging.Service { return m.service }
func (m *mockStrategy) ServiceName() string      { return string(m.service) }
func (m *mockStrategy) ItemName() string         { return "item" }
func (m *mockStrategy) HasDeleteOptions() bool   { return false }
func (m *mockStrategy) ParseName(input string) (string, error) {
	if m.parseNameErr != nil {
		return "", m.parseNameErr
	}
	return input, nil
}
func (m *mockStrategy) ParseSpec(input string) (string, bool, error) {
	return input, false, nil
}

func TestAddRunner_ErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("parse name error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceParam, parseNameErr: errors.New("invalid name")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "invalid"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid name")
	})

	t.Run("editor error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			OpenEditor: func(_ string) (string, error) {
				return "", errors.New("editor crashed")
			},
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to edit")
	})
}

func TestAddRunner_WithOptions(t *testing.T) {
	t.Parallel()

	t.Run("with provided value (skip editor)", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			// No OpenEditor set - with Value provided, editor should not be called
		}

		err := r.Run(context.Background(), runner.AddOptions{
			Name:  "/app/new-config",
			Value: "direct-value",
		})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged for creation")

		entry, err := store.Get(staging.ServiceParam, "/app/new-config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "direct-value", lo.FromPtr(entry.Value))
	})

	t.Run("with description", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.AddRunner{
			UseCase: &stagingusecase.AddUseCase{
				Strategy: &mockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(context.Background(), runner.AddOptions{
			Name:        "/app/new-config",
			Value:       "test-value",
			Description: "Test description",
		})
		require.NoError(t, err)

		entry, err := store.Get(staging.ServiceParam, "/app/new-config")
		require.NoError(t, err)
		assert.Equal(t, "Test description", lo.FromPtr(entry.Description))
	})
}
