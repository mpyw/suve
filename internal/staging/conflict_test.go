package staging_test

import (
	"context"
	"errors"
	"sync"
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

// resolverFor wraps a single strategy as a namespace-agnostic resolver, matching
// the behavior of a provider without a namespace axis (AWS/GCloud/Key Vault):
// every namespace resolves to the same strategy.
func resolverFor(s staging.ApplyStrategy) staging.ApplyStrategyResolver {
	return func(string) (staging.ApplyStrategy, error) {
		return s, nil
	}
}

func TestCheckConflicts(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	laterTime := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)

	t.Run("empty entries returns empty conflicts", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), map[staging.EntryKey]staging.Entry{})
		assert.Empty(t, conflicts)
	})

	t.Run("entries without create or base time return empty", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{}
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "item1"}: {Operation: staging.OperationUpdate},
			{Name: "item2"}: {Operation: staging.OperationDelete},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Empty(t, conflicts)
	})

	t.Run("create conflict - resource now exists", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return baseTime, nil // Resource exists (non-zero time)
			},
		}
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "new-item"}: {Operation: staging.OperationCreate, Value: lo.ToPtr("value")},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Contains(t, conflicts, staging.EntryKey{Name: "new-item"})
	})

	t.Run("create no conflict - resource does not exist", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, nil // Resource doesn't exist (zero time)
			},
		}

		entries := map[staging.EntryKey]staging.Entry{
			{Name: "new-item"}: {Operation: staging.OperationCreate, Value: lo.ToPtr("value")},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Empty(t, conflicts)
	})

	t.Run("create fetch error - no conflict assumed", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, errors.New("access denied")
			},
		}
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "new-item"}: {Operation: staging.OperationCreate, Value: lo.ToPtr("value")},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Empty(t, conflicts)
	})

	t.Run("update conflict - AWS modified after base", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return laterTime, nil
			},
		}
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "existing-item"}: {
				Operation:      staging.OperationUpdate,
				Value:          lo.ToPtr("value"),
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Contains(t, conflicts, staging.EntryKey{Name: "existing-item"})
	})

	t.Run("update no conflict - AWS not modified after base", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return baseTime, nil // Same time as base
			},
		}
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "existing-item"}: {
				Operation:      staging.OperationUpdate,
				Value:          lo.ToPtr("value"),
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Empty(t, conflicts)
	})

	t.Run("update fetch error - no conflict assumed", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, errors.New("access denied")
			},
		}
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "existing-item"}: {
				Operation:      staging.OperationUpdate,
				Value:          lo.ToPtr("value"),
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Empty(t, conflicts)
	})

	t.Run("delete conflict - resource was modified after base", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return laterTime, nil
			},
		}
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "delete-item"}: {
				Operation:      staging.OperationDelete,
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Contains(t, conflicts, staging.EntryKey{Name: "delete-item"})
	})

	t.Run("delete no conflict - resource already deleted", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, nil // Resource doesn't exist
			},
		}
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "delete-item"}: {
				Operation:      staging.OperationDelete,
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
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
		entries := map[staging.EntryKey]staging.Entry{
			{Name: "create-item"}:        {Operation: staging.OperationCreate, Value: lo.ToPtr("v")},
			{Name: "update-item"}:        {Operation: staging.OperationUpdate, Value: lo.ToPtr("v"), BaseModifiedAt: &baseTime},
			{Name: "delete-item"}:        {Operation: staging.OperationDelete, BaseModifiedAt: &baseTime},
			{Name: "update-no-conflict"}: {Operation: staging.OperationUpdate, Value: lo.ToPtr("v"), BaseModifiedAt: &baseTime},
		}
		conflicts := staging.CheckConflicts(t.Context(), resolverFor(strategy), entries)
		assert.Len(t, conflicts, 2)
		assert.Contains(t, conflicts, staging.EntryKey{Name: "create-item"})
		assert.Contains(t, conflicts, staging.EntryKey{Name: "update-item"})
	})
}

func TestCheckTagConflicts(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	laterTime := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)

	t.Run("empty tags returns empty conflicts", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{}
		conflicts := staging.CheckTagConflicts(t.Context(), resolverFor(strategy), map[staging.EntryKey]staging.TagEntry{})
		assert.Empty(t, conflicts)
	})

	t.Run("tag without base time is never a conflict", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return laterTime, nil // modified, but no base to compare against
			},
		}
		tags := map[staging.EntryKey]staging.TagEntry{
			{Name: "item1"}: {Add: map[string]string{"env": "prod"}},
		}
		conflicts := staging.CheckTagConflicts(t.Context(), resolverFor(strategy), tags)
		assert.Empty(t, conflicts)
	})

	t.Run("conflict - remote modified after base", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return laterTime, nil
			},
		}
		tags := map[staging.EntryKey]staging.TagEntry{
			{Name: "existing-item"}: {
				Add:            map[string]string{"env": "prod"},
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckTagConflicts(t.Context(), resolverFor(strategy), tags)
		assert.Contains(t, conflicts, staging.EntryKey{Name: "existing-item"})
	})

	t.Run("no conflict - remote unchanged since base", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return baseTime, nil // same time as base
			},
		}
		tags := map[staging.EntryKey]staging.TagEntry{
			{Name: "existing-item"}: {
				Add:            map[string]string{"env": "prod"},
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckTagConflicts(t.Context(), resolverFor(strategy), tags)
		assert.Empty(t, conflicts)
	})

	t.Run("no conflict - remote no longer exists", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, nil // zero time: resource gone
			},
		}
		tags := map[staging.EntryKey]staging.TagEntry{
			{Name: "existing-item"}: {
				Add:            map[string]string{"env": "prod"},
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckTagConflicts(t.Context(), resolverFor(strategy), tags)
		assert.Empty(t, conflicts)
	})

	t.Run("fetch error - no conflict assumed", func(t *testing.T) {
		t.Parallel()

		strategy := &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
				return time.Time{}, errors.New("access denied")
			},
		}
		tags := map[staging.EntryKey]staging.TagEntry{
			{Name: "existing-item"}: {
				Add:            map[string]string{"env": "prod"},
				BaseModifiedAt: &baseTime,
			},
		}
		conflicts := staging.CheckTagConflicts(t.Context(), resolverFor(strategy), tags)
		assert.Empty(t, conflicts)
	})

	t.Run("per-namespace - each probed against its own remote", func(t *testing.T) {
		t.Parallel()

		remoteByNamespace := map[string]time.Time{
			"":    baseTime,  // unchanged since base -> no conflict
			"dev": laterTime, // modified after base -> conflict
		}

		resolve := func(namespace string) (staging.ApplyStrategy, error) {
			return &mockApplyStrategy{
				fetchLastModifiedFunc: func(_ context.Context, _ string) (time.Time, error) {
					return remoteByNamespace[namespace], nil
				},
			}, nil
		}

		tags := map[staging.EntryKey]staging.TagEntry{
			{Name: "k", Namespace: ""}:    {Add: map[string]string{"a": "1"}, BaseModifiedAt: &baseTime},
			{Name: "k", Namespace: "dev"}: {Add: map[string]string{"a": "1"}, BaseModifiedAt: &baseTime},
		}

		conflicts := staging.CheckTagConflicts(t.Context(), resolve, tags)
		assert.Len(t, conflicts, 1)
		assert.Contains(t, conflicts, staging.EntryKey{Name: "k", Namespace: "dev"})
		assert.NotContains(t, conflicts, staging.EntryKey{Name: "k", Namespace: ""})
	})
}

// TestCheckConflicts_ResolverError verifies that a per-namespace resolver which
// fails to resolve a strategy is treated like a fetch error: the entry is
// skipped (no conflict) and the failure surfaces later on the apply attempt.
func TestCheckConflicts_ResolverError(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	resolve := func(string) (staging.ApplyStrategy, error) {
		return nil, errors.New("cannot resolve strategy")
	}
	entries := map[staging.EntryKey]staging.Entry{
		{Name: "create-item"}: {Operation: staging.OperationCreate, Value: lo.ToPtr("value")},
		{Name: "update-item"}: {
			Operation:      staging.OperationUpdate,
			Value:          lo.ToPtr("value"),
			BaseModifiedAt: &baseTime,
		},
	}

	conflicts := staging.CheckConflicts(t.Context(), resolve, entries)
	assert.Empty(t, conflicts)
}

// TestCheckConflicts_PerNamespace is a regression for #441: two same-named
// entries under different namespaces must each be probed against their OWN
// namespace's remote state, and the reported conflict must carry the namespace.
// With the namespace dropped, a single bare-name probe would compare both
// entries against one namespace's time and falsely flag or clear the other.
func TestCheckConflicts_PerNamespace(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	laterTime := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)

	// The "dev" namespace was modified after base (conflict); the default
	// namespace was not (no conflict). Same name "k" in both.
	remoteByNamespace := map[string]time.Time{
		"":    baseTime,  // unchanged since base -> no conflict
		"dev": laterTime, // modified after base -> conflict
	}

	var mu sync.Mutex

	probed := map[string]string{} // namespace -> name probed against its strategy

	resolve := func(namespace string) (staging.ApplyStrategy, error) {
		return &mockApplyStrategy{
			fetchLastModifiedFunc: func(_ context.Context, name string) (time.Time, error) {
				mu.Lock()
				probed[namespace] = name
				mu.Unlock()

				return remoteByNamespace[namespace], nil
			},
		}, nil
	}

	entries := map[staging.EntryKey]staging.Entry{
		{Name: "k", Namespace: ""}: {
			Operation:      staging.OperationUpdate,
			Value:          lo.ToPtr("v"),
			BaseModifiedAt: &baseTime,
		},
		{Name: "k", Namespace: "dev"}: {
			Operation:      staging.OperationUpdate,
			Value:          lo.ToPtr("v"),
			BaseModifiedAt: &baseTime,
		},
	}

	conflicts := staging.CheckConflicts(t.Context(), resolve, entries)

	// Only the "dev" entry conflicts, and the report carries its namespace.
	assert.Len(t, conflicts, 1)
	assert.Contains(t, conflicts, staging.EntryKey{Name: "k", Namespace: "dev"})
	assert.NotContains(t, conflicts, staging.EntryKey{Name: "k", Namespace: ""})

	// Each namespace's strategy was probed with the entry's own name — proving
	// the namespace is threaded through to the probe, not dropped.
	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, map[string]string{"": "k", "dev": "k"}, probed)
}
