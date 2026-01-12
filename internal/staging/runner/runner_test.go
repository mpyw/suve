package runner_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/runner"
	"github.com/mpyw/suve/internal/staging/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
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
	applyErr             error
	fetchLastModifiedVal time.Time
	fetchLastModifiedErr error
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
func (m *fullMockStrategy) FetchCurrentValue(_ context.Context, _ string) (*staging.EditFetchResult, error) {
	if m.fetchCurrentErr != nil {
		return nil, m.fetchCurrentErr
	}
	return &staging.EditFetchResult{
		Value:        m.fetchCurrentVal,
		LastModified: m.fetchLastModifiedVal,
	}, nil
}
func (m *fullMockStrategy) FetchVersion(_ context.Context, _ string) (string, string, error) {
	if m.fetchVersionErr != nil {
		return "", "", m.fetchVersionErr
	}
	return m.fetchVersionVal, m.fetchVersionLbl, nil
}
func (m *fullMockStrategy) Apply(_ context.Context, _ string, _ staging.Entry) error {
	return m.applyErr
}
func (m *fullMockStrategy) ApplyTags(_ context.Context, _ string, _ staging.TagEntry) error {
	return nil
}
func (m *fullMockStrategy) FetchLastModified(_ context.Context, _ string) (time.Time, error) {
	if m.fetchLastModifiedErr != nil {
		return time.Time{}, m.fetchLastModifiedErr
	}
	return m.fetchLastModifiedVal, nil
}

// =============================================================================
// StatusRunner Tests
// =============================================================================

func TestStatusRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("show single - staged item", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("staged-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "/app/config")
		assert.Contains(t, stdout.String(), "M") // Modified
	})

	t.Run("show single - not staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not staged")
	})

	t.Run("show single - with verbose", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("staged-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Name: "/app/config", Verbose: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "staged-value")
	})

	t.Run("show all - no items staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "No")
	})

	t.Run("show all - multiple items", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value1"),
			StagedAt:  time.Now(),
		})
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("value2"),
			StagedAt:  time.Now(),
		})
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config3", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{})
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

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
			Operation:     staging.OperationDelete,
			StagedAt:      time.Now(),
			DeleteOptions: &staging.DeleteOptions{Force: true},
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceSecret, hasDeleteOptions: true},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Verbose: true})
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

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "-old-value")
		assert.Contains(t, output, "+new-value")
	})

	t.Run("diff all items", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new1"),
			StagedAt:  time.Now(),
		})
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new2"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "/app/config1")
		assert.Contains(t, output, "/app/config2")
	})

	t.Run("diff item not staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "not staged")
	})

	t.Run("diff no items staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "no")
	})

	t.Run("diff with JSON format", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr(`{"b":2,"a":1}`),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: `{"a":1}`},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config", ParseJSON: true})
		require.NoError(t, err)
		output := stdout.String()
		// JSON should be formatted with sorted keys
		assert.Contains(t, output, `"a"`)
	})

	t.Run("diff with JSON format - non-JSON value", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("not-json"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: `{"valid": "json"}`},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config", ParseJSON: true})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "--parse-json has no effect")
	})

	t.Run("diff identical values - auto unstage", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("same-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "same-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "unstaged")
		assert.Contains(t, stderr.String(), "identical")

		// Verify unstaged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("diff delete operation", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "current-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "-current-value")
		assert.Contains(t, output, "staged for deletion")
	})

	t.Run("diff update - auto unstage when item deleted from AWS", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				// FetchCurrent returns error because item was deleted from AWS
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("parameter not found")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "unstaged")
		assert.Contains(t, stderr.String(), "no longer exists")

		// Verify unstaged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("diff create - show diff from empty when item not in AWS", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("brand-new-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				// FetchCurrent returns error because item doesn't exist yet
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("parameter not found")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/new-param"})
		require.NoError(t, err)

		output := stdout.String()
		assert.Contains(t, output, "+brand-new-value")
		assert.Contains(t, output, "not in AWS")
		assert.Contains(t, output, "staged for creation")

		// Verify still staged (not auto-unstaged)
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/new-param")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationCreate, entry.Operation)
	})

	t.Run("diff create with JSON format - item not in AWS", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-json", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr(`{"b":2,"a":1}`),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("parameter not found")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/new-json", ParseJSON: true})
		require.NoError(t, err)
		output := stdout.String()
		// JSON should be formatted
		assert.Contains(t, output, `"a"`)
		assert.Contains(t, output, `"b"`)
	})

	t.Run("diff create - auto unstage when item exists in AWS with same value", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("same-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				// Item already exists in AWS with same value
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "same-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/param"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "unstaged")
		assert.Contains(t, stderr.String(), "identical")

		// Verify unstaged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/param")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("diff create - show diff when item exists in AWS with different value", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/param", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				// Item already exists in AWS with different value
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/param"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "-old-value")
		assert.Contains(t, output, "+new-value")
	})

	t.Run("diff delete - auto unstage when already deleted in AWS", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				// FetchCurrent returns error because item doesn't exist in AWS anymore
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("parameter not found")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "unstaged")
		assert.Contains(t, stderr.String(), "already deleted")

		// Verify unstaged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
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

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "aws-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			OpenEditor: func(current string) (string, error) {
				assert.Equal(t, "aws-value", current)
				return "edited-value", nil
			},
		}

		err := r.Run(t.Context(), runner.EditOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged")

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationUpdate, entry.Operation)
		assert.Equal(t, "edited-value", lo.FromPtr(entry.Value))
	})

	t.Run("edit existing - use staged value", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("staged-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "aws-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			OpenEditor: func(current string) (string, error) {
				assert.Equal(t, "staged-value", current) // Should use staged, not AWS
				return "new-value", nil
			},
		}

		err := r.Run(t.Context(), runner.EditOptions{Name: "/app/config"})
		require.NoError(t, err)

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "new-value", lo.FromPtr(entry.Value))
	})

	t.Run("edit existing - use staged create value", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("create-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			OpenEditor: func(current string) (string, error) {
				assert.Equal(t, "create-value", current)
				return "edited-create-value", nil
			},
		}

		err := r.Run(t.Context(), runner.EditOptions{Name: "/app/config"})
		require.NoError(t, err)
	})

	t.Run("edit - no changes", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "same-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			OpenEditor: func(current string) (string, error) {
				return current, nil // Return unchanged
			},
		}

		err := r.Run(t.Context(), runner.EditOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "No changes made")
	})

	t.Run("edit - editor error", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			OpenEditor: func(_ string) (string, error) {
				return "", errors.New("editor crashed")
			},
		}

		err := r.Run(t.Context(), runner.EditOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to edit")
	})

	t.Run("edit - fetch error", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("not found")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			OpenEditor: func(_ string) (string, error) {
				return "value", nil
			},
		}

		err := r.Run(t.Context(), runner.EditOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// =============================================================================
// PushRunner Tests
// =============================================================================

func TestApplyRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("apply all - success", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("value1"),
			StagedAt:  time.Now(),
		})
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value2"),
			StagedAt:  time.Now(),
		})
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config3", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Created")
		assert.Contains(t, output, "Updated")
		assert.Contains(t, output, "Deleted")

		// Verify all unstaged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config1")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config2")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config3")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("apply single - success", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value1"),
			StagedAt:  time.Now(),
		})
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value2"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{Name: "/app/config1"})
		require.NoError(t, err)

		// Only config1 should be unstaged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config1")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config2")
		assert.NoError(t, err) // Still staged
	})

	t.Run("apply single - not staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		// Stage a different item so we can test "specific item not staged"
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/other", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{Name: "/app/config"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not staged")
	})

	t.Run("apply - nothing staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "No")
	})

	t.Run("apply - with failures", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, applyErr: errors.New("apply failed")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed 1")
		assert.Contains(t, stderr.String(), "apply failed")
	})

	t.Run("conflict - create operation (resource already exists)", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
			// No BaseModifiedAt for Create (didn't exist at staging time)
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				// FetchLastModified returns non-zero time = resource exists now
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchLastModifiedVal: time.Now()},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "conflict")
		assert.Contains(t, stderr.String(), "conflict detected")
	})

	t.Run("conflict - update operation (resource modified after staging)", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		baseTime := time.Now().Add(-time.Hour)
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation:      staging.OperationUpdate,
			Value:          lo.ToPtr("updated-value"),
			StagedAt:       time.Now(),
			BaseModifiedAt: &baseTime, // AWS was at this time when we fetched
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				// FetchLastModified returns time AFTER BaseModifiedAt = conflict
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchLastModifiedVal: time.Now()},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "conflict")
		assert.Contains(t, stderr.String(), "conflict detected")
	})

	t.Run("conflict - delete operation (resource modified after staging)", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		baseTime := time.Now().Add(-time.Hour)
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation:      staging.OperationDelete,
			StagedAt:       time.Now(),
			BaseModifiedAt: &baseTime,
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchLastModifiedVal: time.Now()},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "conflict")
	})

	t.Run("no conflict - update with same time", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		baseTime := time.Now()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation:      staging.OperationUpdate,
			Value:          lo.ToPtr("updated-value"),
			StagedAt:       time.Now(),
			BaseModifiedAt: &baseTime,
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				// FetchLastModified returns same time as BaseModifiedAt = no conflict
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchLastModifiedVal: baseTime},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Updated")
	})

	t.Run("conflict ignored with --ignore-conflicts", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		baseTime := time.Now().Add(-time.Hour)
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation:      staging.OperationUpdate,
			Value:          lo.ToPtr("updated-value"),
			StagedAt:       time.Now(),
			BaseModifiedAt: &baseTime,
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchLastModifiedVal: time.Now()},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		// With IgnoreConflicts, apply should proceed despite conflict
		err := r.Run(t.Context(), runner.ApplyOptions{IgnoreConflicts: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Updated")
	})

	t.Run("no conflict - create when resource doesn't exist", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				// FetchLastModified returns zero time = resource doesn't exist = no conflict
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchLastModifiedVal: time.Time{}},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Created")
	})
}

// =============================================================================
// ResetRunner Tests
// =============================================================================

func TestResetRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("unstage all - success", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config1", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value1"),
			StagedAt:  time.Now(),
		})
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config2", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value2"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser: &fullMockStrategy{service: staging.ServiceParam},
				Store:  store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{All: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Unstaged all")
		assert.Contains(t, stdout.String(), "(2)")

		// Verify all unstaged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config1")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("unstage all - nothing staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser: &fullMockStrategy{service: staging.ServiceParam},
				Store:  store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{All: true})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "No")
	})

	t.Run("unstage single - success", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser: &fullMockStrategy{service: staging.ServiceParam},
				Store:  store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{Spec: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Unstaged")

		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("unstage single - not staged", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser: &fullMockStrategy{service: staging.ServiceParam},
				Store:  store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{Spec: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "not staged")
	})

	t.Run("restore version - success", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		fetcher := &fullMockStrategy{
			service:          staging.ServiceParam,
			parseSpecVersion: true,
			fetchVersionVal:  "old-value",
			fetchVersionLbl:  "#1",
		}
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser:  fetcher,
				Fetcher: fetcher,
				Store:   store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{Spec: "/app/config#1"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Restored")
		assert.Contains(t, stdout.String(), "#1")

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config#1")
		require.NoError(t, err)
		assert.Equal(t, "old-value", lo.FromPtr(entry.Value))
	})

	t.Run("restore version - no fetcher", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser:  &fullMockStrategy{service: staging.ServiceParam, parseSpecVersion: true},
				Fetcher: nil, // No fetcher
				Store:   store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{Spec: "/app/config#1"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reset strategy required")
	})

	t.Run("restore version - fetch error", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		fetcher := &fullMockStrategy{
			service:          staging.ServiceParam,
			parseSpecVersion: true,
			fetchVersionErr:  errors.New("version not found"),
		}
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser:  fetcher,
				Fetcher: fetcher,
				Store:   store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{Spec: "/app/config#999"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version not found")
	})

	t.Run("parse spec error", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser: &fullMockStrategy{
					service:      staging.ServiceParam,
					parseSpecErr: errors.New("invalid spec"),
				},
				Store: store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{Spec: "invalid"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid spec")
	})
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestRunners_SecretService(t *testing.T) {
	t.Parallel()

	t.Run("status runner with Secrets Manager", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceSecret},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "my-secret")
	})

	t.Run("apply runner with Secrets Manager", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("secret-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceSecret},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Updated")
	})
}

func TestRunners_DeleteOptions(t *testing.T) {
	t.Parallel()

	t.Run("status with delete options verbose", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
			Operation:     staging.OperationDelete,
			StagedAt:      time.Now(),
			DeleteOptions: &staging.DeleteOptions{RecoveryWindow: 14},
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceSecret, hasDeleteOptions: true},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Name: "my-secret", Verbose: true})
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

		store := testutil.NewMockStore()
		desc := "Updated config description"
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation:   staging.OperationUpdate,
			Value:       lo.ToPtr("new-value"),
			Description: &desc,
			StagedAt:    time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Description:")
		assert.Contains(t, output, "Updated config description")
	})

	t.Run("diff with tags", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod", "team": "platform"},
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "old-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Tags:")
		assert.Contains(t, output, "env=prod")
		assert.Contains(t, output, "team=platform")
	})

	t.Run("diff create with metadata", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		desc := "New parameter"
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
			Operation:   staging.OperationCreate,
			Value:       lo.ToPtr("brand-new"),
			Description: &desc,
			StagedAt:    time.Now(),
		})
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/new-param", staging.TagEntry{
			Add:      map[string]string{"env": "staging"},
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DiffRunner{
			UseCase: &stagingusecase.DiffUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: errors.New("not found")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DiffOptions{Name: "/app/new-param"})
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

// =============================================================================
// EditRunner Additional Tests
// =============================================================================

func TestEditRunner_WithMetadata(t *testing.T) {
	t.Parallel()

	t.Run("edit with description", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "current"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.EditOptions{
			Name:        "/app/config",
			Value:       "new-value",
			Description: "Updated description",
		})
		require.NoError(t, err)

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, "Updated description", lo.FromPtr(entry.Description))
	})

	t.Run("edit preserves BaseModifiedAt from AWS", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		awsTime := time.Now().Add(-time.Hour).Truncate(time.Second)

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{
					service:              staging.ServiceParam,
					fetchCurrentVal:      "current",
					fetchLastModifiedVal: awsTime,
				},
				Store: store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.EditOptions{
			Name:  "/app/config",
			Value: "new-value",
		})
		require.NoError(t, err)

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		require.NotNil(t, entry.BaseModifiedAt)
		assert.WithinDuration(t, awsTime, *entry.BaseModifiedAt, time.Second)
	})

	t.Run("edit staged delete operation is blocked", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		// Stage a delete operation
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationDelete,
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "aws-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
			OpenEditor: func(current string) (string, error) {
				t.Fatal("OpenEditor should not be called when delete is staged")
				return "", nil
			},
		}

		err := r.Run(t.Context(), runner.EditOptions{Name: "/app/config"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "staged for deletion")

		// Entry should still be DELETE
		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationDelete, entry.Operation)
	})
}

func TestApplyRunner_WithTags(t *testing.T) {
	t.Parallel()

	t.Run("apply with tags only", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod", "team": "backend"},
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Tagged")
		assert.Contains(t, output, "+2")
	})

	t.Run("apply with untag keys only", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		removeKeys := make(map[string]struct{})
		removeKeys["deprecated"] = struct{}{}
		removeKeys["old"] = struct{}{}
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Remove:   removeKeys,
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Tagged")
		assert.Contains(t, output, "-2")
	})

	t.Run("apply with both tags and untag keys", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		removeKeys := make(map[string]struct{})
		removeKeys["deprecated"] = struct{}{}
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			Remove:   removeKeys,
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.ApplyRunner{
			UseCase: &stagingusecase.ApplyUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ApplyOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Updated")
		assert.Contains(t, output, "Tagged")
		assert.Contains(t, output, "+1")
		assert.Contains(t, output, "-1")
	})
}

// =============================================================================
// StatusRunner Tag Tests
// =============================================================================

func TestStatusRunner_WithTagEntries(t *testing.T) {
	t.Parallel()

	t.Run("show single - tag entry only", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod", "team": "backend"},
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "/app/config")
		assert.Contains(t, output, "T") // Tag indicator
		assert.Contains(t, output, "+2 tag(s)")
	})

	t.Run("show single - tag entry with remove", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		removeKeys := map[string]struct{}{"deprecated": {}, "old": {}}
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			Remove:   removeKeys,
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Name: "/app/config"})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "+1 tag(s)")
		assert.Contains(t, output, "-2 tag(s)")
	})

	t.Run("show single - tag entry with verbose", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		removeKeys := map[string]struct{}{"deprecated": {}}
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			Remove:   removeKeys,
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Name: "/app/config", Verbose: true})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "+ env=prod")
		assert.Contains(t, output, "- deprecated")
	})

	t.Run("show all - multiple tag entries", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config1", staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		})
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config2", staging.TagEntry{
			Add:      map[string]string{"env": "dev"},
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "Staged")
		assert.Contains(t, output, "(2)")
		assert.Contains(t, output, "/app/config1")
		assert.Contains(t, output, "/app/config2")
	})

	t.Run("show all - mixed entries and tags", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/value", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/tags", staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "(2)") // 1 entry + 1 tag
		assert.Contains(t, output, "/app/value")
		assert.Contains(t, output, "/app/tags")
	})

	t.Run("show all - tag entries with verbose", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()
		removeKeys := map[string]struct{}{"old": {}}
		_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
			Add:      map[string]string{"env": "prod"},
			Remove:   removeKeys,
			StagedAt: time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.StatusRunner{
			UseCase: &stagingusecase.StatusUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.StatusOptions{Verbose: true})
		require.NoError(t, err)
		output := stdout.String()
		assert.Contains(t, output, "+ env=prod")
		assert.Contains(t, output, "- old")
	})
}

func TestDeleteRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("delete SSM parameter", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.DeleteRunner{
			UseCase: &stagingusecase.DeleteUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchLastModifiedVal: time.Now()},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DeleteOptions{Name: "/app/config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged for deletion: /app/config")

		entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationDelete, entry.Operation)
		assert.Nil(t, entry.DeleteOptions) // SSM has no delete options
	})

	t.Run("delete secret with recovery window", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.DeleteRunner{
			UseCase: &stagingusecase.DeleteUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceSecret, hasDeleteOptions: true, fetchLastModifiedVal: time.Now()},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DeleteOptions{
			Name:           "my-secret",
			RecoveryWindow: 14,
		})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged for deletion (14-day recovery): my-secret")

		entry, err := store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationDelete, entry.Operation)
		require.NotNil(t, entry.DeleteOptions)
		assert.Equal(t, 14, entry.DeleteOptions.RecoveryWindow)
		assert.False(t, entry.DeleteOptions.Force)
	})

	t.Run("delete secret with force", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.DeleteRunner{
			UseCase: &stagingusecase.DeleteUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceSecret, hasDeleteOptions: true, fetchLastModifiedVal: time.Now()},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DeleteOptions{
			Name:  "my-secret",
			Force: true,
		})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Staged for immediate deletion: my-secret")

		entry, err := store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
		require.NoError(t, err)
		assert.Equal(t, staging.OperationDelete, entry.Operation)
		require.NotNil(t, entry.DeleteOptions)
		assert.True(t, entry.DeleteOptions.Force)
	})

	t.Run("delete with fetch error", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.DeleteRunner{
			UseCase: &stagingusecase.DeleteUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchLastModifiedErr: errors.New("not found")},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DeleteOptions{Name: "/app/config"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("delete secret with invalid recovery window", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.DeleteRunner{
			UseCase: &stagingusecase.DeleteUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceSecret, hasDeleteOptions: true, fetchLastModifiedVal: time.Now()},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DeleteOptions{
			Name:           "my-secret",
			RecoveryWindow: 5, // Invalid: must be 7-30
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recovery window must be between 7 and 30 days")
	})

	t.Run("delete staged CREATE - unstages instead of delete", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		// Pre-stage a CREATE operation
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-config", staging.Entry{
			Operation: staging.OperationCreate,
			Value:     lo.ToPtr("new-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.DeleteRunner{
			UseCase: &stagingusecase.DeleteUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.DeleteOptions{Name: "/app/new-config"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Unstaged creation: /app/new-config")

		// Verify entry was unstaged (not converted to delete)
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/new-config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

// =============================================================================
// EditRunner Skipped/Unstaged Tests
// =============================================================================

func TestEditRunner_Skipped_Unstaged(t *testing.T) {
	t.Parallel()

	t.Run("edit skipped - same as AWS", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "aws-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		// Edit with value that matches AWS - should be skipped
		err := r.Run(t.Context(), runner.EditOptions{
			Name:  "/app/config",
			Value: "aws-value", // Same as current AWS value
		})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Skipped /app/config (same as AWS)")

		// Verify nothing was staged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})

	t.Run("edit unstaged - reverted to AWS", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		// Pre-stage an UPDATE operation
		_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
			Operation: staging.OperationUpdate,
			Value:     lo.ToPtr("staged-value"),
			StagedAt:  time.Now(),
		})

		var stdout, stderr bytes.Buffer
		r := &runner.EditRunner{
			UseCase: &stagingusecase.EditUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "aws-value"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		// Edit back to AWS value - should auto-unstage
		err := r.Run(t.Context(), runner.EditOptions{
			Name:  "/app/config",
			Value: "aws-value", // Reverted to AWS value
		})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Unstaged /app/config (reverted to AWS)")

		// Verify entry was unstaged
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}

// =============================================================================
// ResetRunner Skipped Tests
// =============================================================================

func TestResetRunner_Skipped(t *testing.T) {
	t.Parallel()

	t.Run("restore version skipped - same as current AWS", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		// Fetcher returns version value that matches current AWS value
		fetcher := &fullMockStrategy{
			service:          staging.ServiceParam,
			parseSpecVersion: true,
			fetchVersionVal:  "current-value", // Version value
			fetchVersionLbl:  "#3",
			fetchCurrentVal:  "current-value", // Same as version - triggers skip
		}
		r := &runner.ResetRunner{
			UseCase: &stagingusecase.ResetUseCase{
				Parser:  fetcher,
				Fetcher: fetcher,
				Store:   store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), runner.ResetOptions{Spec: "/app/config#3"})
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Skipped /app/config#3 (version #3 matches current value)")

		// Verify nothing was staged (auto-skipped)
		_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config#3")
		assert.ErrorIs(t, err, staging.ErrNotStaged)
	})
}
