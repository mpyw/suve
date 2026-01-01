package runner_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/runner"
)

func TestAddRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("create new item", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var buf bytes.Buffer
		r := &runner.AddRunner{
			Strategy:   &mockStrategy{service: staging.ServiceSSM},
			Store:      store,
			Stdout:     &buf,
			Stderr:     &bytes.Buffer{},
			OpenEditor: func(_ string) (string, error) { return "new-value", nil },
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Staged for creation")
		assert.Contains(t, buf.String(), "/app/config")

		// Verify staged with OperationCreate
		entry, err := store.Get(staging.ServiceSSM, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "new-value", entry.Value)
	})

	t.Run("edit already staged create", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		// Pre-stage as create
		_ = store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "original-value",
		})

		var buf bytes.Buffer
		r := &runner.AddRunner{
			Strategy: &mockStrategy{service: staging.ServiceSSM},
			Store:    store,
			Stdout:   &buf,
			Stderr:   &bytes.Buffer{},
			OpenEditor: func(current string) (string, error) {
				assert.Equal(t, "original-value", current)
				return "updated-value", nil
			},
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Staged for creation")

		// Verify updated
		entry, err := store.Get(staging.ServiceSSM, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "updated-value", entry.Value)
	})

	t.Run("empty value not staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var buf bytes.Buffer
		r := &runner.AddRunner{
			Strategy:   &mockStrategy{service: staging.ServiceSSM},
			Store:      store,
			Stdout:     &buf,
			Stderr:     &bytes.Buffer{},
			OpenEditor: func(_ string) (string, error) { return "", nil },
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Empty value")

		// Verify not staged
		_, err = store.Get(staging.ServiceSSM, "/app/config")
		assert.Equal(t, staging.ErrNotStaged, err)
	})

	t.Run("no changes made", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		// Pre-stage as create
		_ = store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "same-value",
		})

		var buf bytes.Buffer
		r := &runner.AddRunner{
			Strategy: &mockStrategy{service: staging.ServiceSSM},
			Store:    store,
			Stdout:   &buf,
			Stderr:   &bytes.Buffer{},
			OpenEditor: func(_ string) (string, error) {
				return "same-value", nil
			},
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No changes made")
	})

	t.Run("SM service", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var buf bytes.Buffer
		r := &runner.AddRunner{
			Strategy:   &mockStrategy{service: staging.ServiceSM},
			Store:      store,
			Stdout:     &buf,
			Stderr:     &bytes.Buffer{},
			OpenEditor: func(_ string) (string, error) { return "secret-value", nil },
		}

		err := r.Run(context.Background(), runner.AddOptions{Name: "my-secret"})
		require.NoError(t, err)

		// Verify staged with correct service
		entry, err := store.Get(staging.ServiceSM, "my-secret")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
		assert.Equal(t, "secret-value", entry.Value)
	})
}

// mockStrategy implements staging.Parser for testing.
type mockStrategy struct {
	service staging.Service
}

func (m *mockStrategy) Service() staging.Service { return m.service }
func (m *mockStrategy) ServiceName() string      { return string(m.service) }
func (m *mockStrategy) ItemName() string         { return "item" }
func (m *mockStrategy) HasDeleteOptions() bool   { return false }
func (m *mockStrategy) ParseName(input string) (string, error) {
	return input, nil
}
func (m *mockStrategy) ParseSpec(input string) (string, bool, error) {
	return input, false, nil
}
