package staging_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/staging"
)

type mockApplyStrategy struct {
	fetchLastModifiedFunc func(ctx context.Context, name string) (time.Time, error)
}

func (m *mockApplyStrategy) FetchLastModified(ctx context.Context, name string) (time.Time, error) {
	if m.fetchLastModifiedFunc != nil {
		return m.fetchLastModifiedFunc(ctx, name)
	}

	return time.Time{}, nil
}

func (m *mockApplyStrategy) Service() staging.Service {
	return staging.ServiceParam
}

func (m *mockApplyStrategy) ServiceName() string {
	return "Test"
}

func (m *mockApplyStrategy) ItemName() string {
	return "item"
}

func (m *mockApplyStrategy) HasDeleteOptions() bool {
	return false
}

func (m *mockApplyStrategy) Apply(_ context.Context, _ string, _ staging.Entry) error {
	return nil
}

func (m *mockApplyStrategy) ApplyTags(_ context.Context, _ string, _ staging.TagEntry) error {
	return nil
}

func TestCheckConflicts(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	laterTime := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)

	t.Run("empty entries returns empty conflicts", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{}
		conflicts := staging.CheckConflicts(t.Context(), strategy, map[string]staging.Entry{})
		assert.Empty(t, conflicts)
	})

	t.Run("entries without create or base time return empty", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{}
		entries := map[string]staging.Entry{
			"item1": {Operation: staging.OperationUpdate},
			"item2": {Operation: staging.OperationDelete},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Empty(t, conflicts)
	})

	t.Run("create conflict - resource now exists", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return baseTime, nil // Resource exists (non-zero time)
			},
		}
		entries := map[string]staging.Entry{
			"new-item": {Operation: staging.OperationCreate, Value: lo.ToPtr("value")},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Contains(t, conflicts, "new-item")
	})

	t.Run("create no conflict - resource does not exist", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, nil // Resource doesn't exist (zero time)
			},
		}

		entries := map[string]staging.Entry{
			"new-item": {Operation: staging.OperationCreate, Value: lo.ToPtr("value")},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Empty(t, conflicts)
	})

	t.Run("create fetch error - no conflict assumed", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, errors.New("access denied")
			},
		}
		entries := map[string]staging.Entry{
			"new-item": {Operation: staging.OperationCreate, Value: lo.ToPtr("value")},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Empty(t, conflicts)
	})

	t.Run("update conflict - AWS modified after base", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return laterTime, nil
			},
		}
		entries := map[string]staging.Entry{
			"existing-item": {
				Operation:      staging.OperationUpdate,
				Value:          lo.ToPtr("value"),
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Contains(t, conflicts, "existing-item")
	})

	t.Run("update no conflict - AWS not modified after base", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return baseTime, nil // Same time as base
			},
		}
		entries := map[string]staging.Entry{
			"existing-item": {
				Operation:      staging.OperationUpdate,
				Value:          lo.ToPtr("value"),
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Empty(t, conflicts)
	})

	t.Run("update fetch error - no conflict assumed", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, errors.New("access denied")
			},
		}
		entries := map[string]staging.Entry{
			"existing-item": {
				Operation:      staging.OperationUpdate,
				Value:          lo.ToPtr("value"),
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Empty(t, conflicts)
	})

	t.Run("delete conflict - resource was modified after base", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return laterTime, nil
			},
		}
		entries := map[string]staging.Entry{
			"delete-item": {
				Operation:      staging.OperationDelete,
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Contains(t, conflicts, "delete-item")
	})

	t.Run("delete no conflict - resource already deleted", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, nil // Resource doesn't exist
			},
		}
		entries := map[string]staging.Entry{
			"delete-item": {
				Operation:      staging.OperationDelete,
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Empty(t, conflicts)
	})

	t.Run("mixed entries - multiple conflicts", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, name string) (time.Time, error) {
				switch name {
				case "create-item":
					return baseTime, nil // Exists - conflict for create
				case "update-item":
					return laterTime, nil // Modified after base - conflict
				case "delete-item":
					return time.Time{}, nil // Doesn't exist - no conflict
				case "update-no-conflict":
					return baseTime, nil // Same as base - no conflict
				default:
					return time.Time{}, nil
				}
			},
		}
		entries := map[string]staging.Entry{
			"create-item":        {Operation: staging.OperationCreate, Value: lo.ToPtr("v")},
			"update-item":        {Operation: staging.OperationUpdate, Value: lo.ToPtr("v"), BaseModifiedAt: &baseTime},
			"delete-item":        {Operation: staging.OperationDelete, BaseModifiedAt: &baseTime},
			"update-no-conflict": {Operation: staging.OperationUpdate, Value: lo.ToPtr("v"), BaseModifiedAt: &baseTime},
		}
		conflicts := staging.CheckConflicts(t.Context(), strategy, entries)
		assert.Len(t, conflicts, 2)
		assert.Contains(t, conflicts, "create-item")
		assert.Contains(t, conflicts, "update-item")
	})
}
