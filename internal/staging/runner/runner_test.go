package runner_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/runner"
)

// =============================================================================
// Mock Implementations
// =============================================================================

// fullMockStrategy implements all staging interfaces for testing.
type fullMockStrategy struct {
	service              staging.Service
	hasDeleteOptions     bool
	parseNameErr         error
	parseSpecVersion     bool
	parseSpecErr         error
	fetchCurrentErr      error
	fetchCurrentVal      string
	fetchVersionErr      error
	fetchVersionVal      string
	fetchVersionLbl      string
	pushErr              error
	fetchLastModifiedVal time.Time
}

func (m *fullMockStrategy) Service() staging.Service { return m.service }
func (m *fullMockStrategy) ServiceName() string      { return string(m.service) }
func (m *fullMockStrategy) ItemName() string         { return "item" }
func (m *fullMockStrategy) HasDeleteOptions() bool   { return m.hasDeleteOptions }
func (m *fullMockStrategy) ParseName(input string) (string, error) {
	if m.parseNameErr != nil {
		return "", m.parseNameErr
	}
	return input, nil
}
func (m *fullMockStrategy) ParseSpec(input string) (string, bool, error) {
	if m.parseSpecErr != nil {
		return "", false, m.parseSpecErr
	}
	return input, m.parseSpecVersion, nil
}
func (m *fullMockStrategy) FetchCurrent(_ context.Context, name string) (*staging.FetchResult, error) {
	if m.fetchCurrentErr != nil {
		return nil, m.fetchCurrentErr
	}
	return &staging.FetchResult{
		Value:      m.fetchCurrentVal,
		Identifier: "#1",
	}, nil
}
func (m *fullMockStrategy) FetchCurrentValue(_ context.Context, _ string) (string, error) {
	if m.fetchCurrentErr != nil {
		return "", m.fetchCurrentErr
	}
	return m.fetchCurrentVal, nil
}
func (m *fullMockStrategy) FetchVersion(_ context.Context, _ string) (string, string, error) {
	if m.fetchVersionErr != nil {
		return "", "", m.fetchVersionErr
	}
	return m.fetchVersionVal, m.fetchVersionLbl, nil
}
func (m *fullMockStrategy) Push(_ context.Context, _ string, _ staging.Entry) error {
	return m.pushErr
}
func (m *fullMockStrategy) FetchLastModified(_ context.Context, _ string) (time.Time, error) {
	return m.fetchLastModifiedVal, nil
}

// =============================================================================
// StatusRunner Tests
// =============================================================================

func TestStatusRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("show single - staged item", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "staged-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.StatusOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "M") // Modified
	})

	t.Run("show single - not staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.StatusOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not staged")
	})

	t.Run("show single - with verbose", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "staged-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.StatusOptions{Name: "/app/config", Verbose: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "staged-value")
	})

	t.Run("show all - no items staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.StatusOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "No")
	})

	t.Run("show all - multiple items", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value1",
			StagedAt:  time.Now(),
		})
		_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "value2",
			StagedAt:  time.Now(),
		})
		_ = store.Stage(staging.ServiceParam, "/app/config3", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.StatusOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Staged")
		assert.Contains(t, output, "(3)")
		assert.Contains(t, output, "/app/config1")
		assert.Contains(t, output, "/app/config2")
		assert.Contains(t, output, "/app/config3")
	})

	t.Run("show all - Secrets Manager service with delete options", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
			Operation:     staging.OperationDelete,
			StagedAt:      time.Now(),
			DeleteOptions: &staging.DeleteOptions{Force: true},
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceSecret, hasDeleteOptions: true},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.StatusOptions{Verbose: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "my-secret")
	})
}

// =============================================================================
// DiffRunner Tests
// =============================================================================

func TestDiffRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("diff single item", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "new-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "-old-value")
		assert.Contains(t, output, "+new-value")
	})

	t.Run("diff all items", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "new1",
			StagedAt:  time.Now(),
		})
		_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "new2",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "/app/config1")
		assert.Contains(t, output, "/app/config2")
	})

	t.Run("diff item not staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "not staged")
	})

	t.Run("diff no items staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "no")
	})

	t.Run("diff with JSON format", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     `{"b":2,"a":1}`,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: `{"a":1}`},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config", JSONFormat: true})
		require.NoError(t, err)
		output := stdout.String()
		// JSON should be formatted with sorted keys
		assert.Contains(t, output, `"a"`)
	})

	t.Run("diff with JSON format - non-JSON value", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "not-json",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: `{"valid": "json"}`},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config", JSONFormat: true})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "--json has no effect")
	})

	t.Run("diff identical values - auto unstage", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "same-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "same-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "unstaged")
		assert.Contains(t, stderr.String(), "identical")

		// Verify unstaged
		_, err = store.Get(staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("diff delete operation", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "current-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "-current-value")
		assert.Contains(t, output, "staged for deletion")
	})

	t.Run("diff update - auto unstage when item deleted from AWS", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "new-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			// FetchCurrent returns error because item was deleted from AWS
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("parameter not found")},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "unstaged")
		assert.Contains(t, stderr.String(), "no longer exists")

		// Verify unstaged
		_, err = store.Get(staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("diff create - show diff from empty when item not in AWS", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/new-param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "brand-new-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			// FetchCurrent returns error because item doesn't exist yet
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("parameter not found")},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/new-param"})
		require.NoError(t, err)

		output := stdout.String()
		assert.Contains(t, output, "+brand-new-value")
		assert.Contains(t, output, "not in AWS")
		assert.Contains(t, output, "staged for creation")

		// Verify still staged (not auto-unstaged)
		entry, err := store.Get(staging.ServiceParam, "/app/new-param")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
	})

	t.Run("diff create with JSON format - item not in AWS", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/new-json", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     `{"b":2,"a":1}`,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("parameter not found")},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/new-json", JSONFormat: true})
		require.NoError(t, err)
		output := stdout.String()
		// JSON should be formatted
		assert.Contains(t, output, `"a"`)
		assert.Contains(t, output, `"b"`)
	})

	t.Run("diff create - auto unstage when item exists in AWS with same value", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "same-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			// Item already exists in AWS with same value
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "same-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/param"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "unstaged")
		assert.Contains(t, stderr.String(), "identical")

		// Verify unstaged
		_, err = store.Get(staging.ServiceParam, "/app/param")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("diff create - show diff when item exists in AWS with different value", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "new-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			// Item already exists in AWS with different value
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/param"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "-old-value")
		assert.Contains(t, output, "+new-value")
	})

	t.Run("diff delete - auto unstage when already deleted in AWS", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			// FetchCurrent returns error because item doesn't exist in AWS anymore
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("parameter not found")},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "unstaged")
		assert.Contains(t, stderr.String(), "already deleted")

		// Verify unstaged
		_, err = store.Get(staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

// =============================================================================
// EditRunner Tests
// =============================================================================

func TestEditRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("edit existing - fetch from AWS", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "aws-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
			OpenEditor: func(current string) (string, error) {
				assert.Equal(t, "aws-value", current)
				return "edited-value", nil
			},
		}

		err := r.Run(context.Background(), runner.EditOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged")

		entry, err := store.Get(staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationUpdate, entry.Operation)
		assert.Equal(t, "edited-value", entry.Value)
	})

	t.Run("edit existing - use staged value", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "staged-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "aws-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
			OpenEditor: func(current string) (string, error) {
				assert.Equal(t, "staged-value", current) // Should use staged, not AWS
				return "new-value", nil
			},
		}

		err := r.Run(context.Background(), runner.EditOptions{Name: "/app/config"})
		require.NoError(t, err)

		entry, err := store.Get(staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "new-value", entry.Value)
	})

	t.Run("edit existing - use staged create value", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "create-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
			OpenEditor: func(current string) (string, error) {
				assert.Equal(t, "create-value", current)
				return "edited-create-value", nil
			},
		}

		err := r.Run(context.Background(), runner.EditOptions{Name: "/app/config"})
		require.NoError(t, err)
	})

	t.Run("edit - no changes", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "same-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
			OpenEditor: func(current string) (string, error) {
				return current, nil // Return unchanged
			},
		}

		err := r.Run(context.Background(), runner.EditOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "No changes made")
	})

	t.Run("edit - editor error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
			OpenEditor: func(_ string) (string, error) {
				return "", errors.New("editor crashed")
			},
		}

		err := r.Run(context.Background(), runner.EditOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to edit")
	})

	t.Run("edit - fetch error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("not found")},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
			OpenEditor: func(_ string) (string, error) {
				return "value", nil
			},
		}

		err := r.Run(context.Background(), runner.EditOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// =============================================================================
// PushRunner Tests
// =============================================================================

func TestPushRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("push all - success", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     "value1",
			StagedAt:  time.Now(),
		})
		_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value2",
			StagedAt:  time.Now(),
		})
		_ = store.Stage(staging.ServiceParam, "/app/config3", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.PushRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.PushOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Created")
		assert.Contains(t, output, "Updated")
		assert.Contains(t, output, "Deleted")

		// Verify all unstaged
		_, err = store.Get(staging.ServiceParam, "/app/config1")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = store.Get(staging.ServiceParam, "/app/config2")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = store.Get(staging.ServiceParam, "/app/config3")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("push single - success", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value1",
			StagedAt:  time.Now(),
		})
		_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value2",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.PushRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.PushOptions{Name: "/app/config1"})
		require.NoError(t, err)

		// Only config1 should be unstaged
		_, err = store.Get(staging.ServiceParam, "/app/config1")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = store.Get(staging.ServiceParam, "/app/config2")
		assert.NoError(t, err) // Still staged
	})

	t.Run("push single - not staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		// Stage a different item so we can test "specific item not staged"
		_ = store.Stage(staging.ServiceParam, "/app/other", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.PushRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.PushOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not staged")
	})

	t.Run("push - nothing staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.PushRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.PushOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "No")
	})

	t.Run("push - with failures", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.PushRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, pushErr: errors.New("push failed")},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.PushOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed 1")
		assert.Contains(t, stderr.String(), "push failed")
	})
}

// =============================================================================
// ResetRunner Tests
// =============================================================================

func TestResetRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("unstage all - success", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value1",
			StagedAt:  time.Now(),
		})
		_ = store.Stage(staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value2",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			Parser: &fullMockStrategy{service: staging.ServiceParam},
			Store:  store,
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(context.Background(), runner.ResetOptions{All: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Unstaged all")
		assert.Contains(t, stdout.String(), "(2)")

		// Verify all unstaged
		_, err = store.Get(staging.ServiceParam, "/app/config1")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("unstage all - nothing staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			Parser: &fullMockStrategy{service: staging.ServiceParam},
			Store:  store,
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(context.Background(), runner.ResetOptions{All: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "No")
	})

	t.Run("unstage single - success", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			Parser: &fullMockStrategy{service: staging.ServiceParam},
			Store:  store,
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(context.Background(), runner.ResetOptions{Spec: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Unstaged")

		_, err = store.Get(staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("unstage single - not staged", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			Parser: &fullMockStrategy{service: staging.ServiceParam},
			Store:  store,
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(context.Background(), runner.ResetOptions{Spec: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "not staged")
	})

	t.Run("restore version - success", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		fetcher := &fullMockStrategy{
			service:          staging.ServiceParam,
			parseSpecVersion: true,
			fetchVersionVal:  "old-value",
			fetchVersionLbl:  "#1",
		}
		r := &runner.ResetRunner{
			Parser:  fetcher,
			Fetcher: fetcher,
			Store:   store,
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		err := r.Run(context.Background(), runner.ResetOptions{Spec: "/app/config#1"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Restored")
		assert.Contains(t, stdout.String(), "#1")

		entry, err := store.Get(staging.ServiceParam, "/app/config#1")
		require.NoError(t, err)
		assert.Equal(t, "old-value", entry.Value)
	})

	t.Run("restore version - no fetcher", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			Parser:  &fullMockStrategy{service: staging.ServiceParam, parseSpecVersion: true},
			Fetcher: nil, // No fetcher
			Store:   store,
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		err := r.Run(context.Background(), runner.ResetOptions{Spec: "/app/config#1"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version fetcher required")
	})

	t.Run("restore version - fetch error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		fetcher := &fullMockStrategy{
			service:          staging.ServiceParam,
			parseSpecVersion: true,
			fetchVersionErr:  errors.New("version not found"),
		}
		r := &runner.ResetRunner{
			Parser:  fetcher,
			Fetcher: fetcher,
			Store:   store,
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		err := r.Run(context.Background(), runner.ResetOptions{Spec: "/app/config#999"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version not found")
	})

	t.Run("parse spec error", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			Parser: &fullMockStrategy{
				service:      staging.ServiceParam,
				parseSpecErr: errors.New("invalid spec"),
			},
			Store:  store,
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(context.Background(), runner.ResetOptions{Spec: "invalid"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid spec")
	})
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestRunners_SMService(t *testing.T) {
	t.Parallel()

	t.Run("status runner with Secrets Manager", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "secret-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceSecret},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.StatusOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "my-secret")
	})

	t.Run("push runner with Secrets Manager", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "secret-value",
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.PushRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceSecret},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.PushOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Updated")
	})
}

func TestRunners_DeleteOptions(t *testing.T) {
	t.Parallel()

	t.Run("status with delete options verbose", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
			Operation:     staging.OperationDelete,
			StagedAt:      time.Now(),
			DeleteOptions: &staging.DeleteOptions{RecoveryWindow: 14},
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceSecret, hasDeleteOptions: true},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.StatusOptions{Name: "my-secret", Verbose: true})
		require.NoError(t, err)
		// With verbose, should show delete options
		output := stdout.String()
		assert.Contains(t, output, "my-secret")
	})
}

func TestDiffRunner_OutputMetadata(t *testing.T) {
	t.Parallel()

	t.Run("diff with description", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		desc := "Updated config description"
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation:   staging.OperationUpdate,
			Value:       "new-value",
			Description: &desc,
			StagedAt:    time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Description:")
		assert.Contains(t, output, "Updated config description")
	})

	t.Run("diff with tags", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     "new-value",
			Tags:      map[string]string{"env": "prod", "team": "platform"},
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old-value"},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Tags:")
		assert.Contains(t, output, "env=prod")
		assert.Contains(t, output, "team=platform")
	})

	t.Run("diff create with metadata", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
		desc := "New parameter"
		_ = store.Stage(staging.ServiceParam, "/app/new-param", staging.Entry{
			Operation:   staging.OperationCreate,
			Value:       "brand-new",
			Description: &desc,
			Tags:        map[string]string{"env": "staging"},
			StagedAt:    time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("not found")},
			Store:    store,
			Stdout:   &stdout,
			Stderr:   &stderr,
		}

		err := r.Run(context.Background(), runner.DiffOptions{Name: "/app/new-param"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "+brand-new")
		assert.Contains(t, output, "staged for creation")
		assert.Contains(t, output, "Description:")
		assert.Contains(t, output, "New parameter")
		assert.Contains(t, output, "Tags:")
		assert.Contains(t, output, "env=staging")
	})
}
