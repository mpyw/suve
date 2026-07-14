package azure_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/azure"
)

func TestShowUseCase(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Equal(t, "#abc", spec)

			return provider.NewVersionRef("abc"), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			assert.Equal(t, "abc", ref.ID())

			return &domain.Entry{
				Name:    name,
				Value:   "hello",
				Version: domain.Version{ID: "abc", State: "enabled"},
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	uc := &azure.ShowUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), azure.ShowInput{Name: "my-secret", Suffix: "#abc"})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", out.Name)
	assert.Equal(t, "hello", out.Value)
	assert.Equal(t, "abc", out.Version)
	assert.Equal(t, "enabled", out.State)
	assert.Equal(t, []azure.ShowTag{{Key: "env", Value: "prod"}}, out.Tags)
}

// TestLogUseCase_HistoryErrorPropagates is the acid-test at the use-case layer:
// an unversioned store returns an error from History, and the log use case
// propagates it (rather than crashing or returning an empty history).
func TestLogUseCase_HistoryErrorPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("Azure App Configuration does not support versions")

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return nil, sentinel
		},
	}

	uc := &azure.LogUseCase{Reader: store}
	_, err := uc.Execute(t.Context(), azure.LogInput{Name: "my-key"})
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
}

// TestLogUseCase_FilterBeforeCount asserts date filters run BEFORE the count
// cap: -n must yield up to N versions that match the filter, not N newest then
// filtered down to fewer (#351). Only the two oldest versions match --until;
// the old count-first order would have truncated them all away.
func TestLogUseCase_FilterBeforeCount(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "newest", Created: &now},
		{ID: "middle", Created: lo.ToPtr(now.Add(-2 * time.Hour))},
		{ID: "oldest", Created: lo.ToPtr(now.Add(-3 * time.Hour))},
	}

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return versions, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: ref.ID(), Version: domain.Version{ID: ref.ID()}}, nil
		},
	}

	uc := &azure.LogUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), azure.LogInput{
		Name:       "my-key",
		MaxResults: 1,
		Until:      lo.ToPtr(now.Add(-time.Hour)),
	})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	assert.Equal(t, "middle", out.Entries[0].Version)
}

func TestDiffUseCase(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, name, _ string) (provider.VersionRef, error) {
			return provider.NewVersionRef(name), nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "val-" + name}, nil
		},
	}

	uc := &azure.DiffUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), azure.DiffInput{Name1: "key-a", Name2: "key-b"})
	require.NoError(t, err)
	assert.Equal(t, "key-a", out.OldName)
	assert.Equal(t, "key-b", out.NewName)
	assert.Equal(t, "val-key-a", out.OldValue)
	assert.Equal(t, "val-key-b", out.NewValue)
}

func TestUpdateUseCase_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
	}

	uc := &azure.UpdateUseCase{Store: store}
	_, err := uc.Execute(t.Context(), azure.UpdateInput{Name: "missing", Value: "v", ValueType: domain.ValueTypePlaintext})
	require.ErrorIs(t, err, azure.ErrEntryNotFound)
}

// namespaceListerMock is a minimal fake for the App-Config-specific
// NamespaceLister the namespace-aware list use case depends on.
type namespaceListerMock struct {
	rows []appconfig.KeyNamespace
	err  error
}

func (m *namespaceListerMock) ListWithNamespacesScoped(_ context.Context) ([]appconfig.KeyNamespace, error) {
	return m.rows, m.err
}

func TestListNamespacesUseCase(t *testing.T) {
	t.Parallel()

	// The store (service) already sorts by (key, namespace); the use case only
	// layers the client-side key filters on top and preserves order.
	rows := []appconfig.KeyNamespace{
		{Key: "app/a", Namespace: "", Value: "a-null"},
		{Key: "app/a", Namespace: "dev", Value: "a-dev"},
		{Key: "app/b", Namespace: "prd", Value: "b-prd"},
		{Key: "other", Namespace: "", Value: "o"},
	}

	t.Run("without value carries namespace and drops value", func(t *testing.T) {
		t.Parallel()

		uc := &azure.ListNamespacesUseCase{Lister: &namespaceListerMock{rows: rows}}
		out, err := uc.Execute(t.Context(), azure.ListNamespacesInput{})
		require.NoError(t, err)

		assert.Equal(t, []azure.ListNamespacesEntry{
			{Namespace: "", Name: "app/a"},
			{Namespace: "dev", Name: "app/a"},
			{Namespace: "prd", Name: "app/b"},
			{Namespace: "", Name: "other"},
		}, out.Entries)
	})

	t.Run("prefix filters on the key, keeping every namespace of a match", func(t *testing.T) {
		t.Parallel()

		uc := &azure.ListNamespacesUseCase{Lister: &namespaceListerMock{rows: rows}}
		out, err := uc.Execute(t.Context(), azure.ListNamespacesInput{Prefix: "app/", WithValue: true})
		require.NoError(t, err)

		assert.Equal(t, []azure.ListNamespacesEntry{
			{Namespace: "", Name: "app/a", Value: lo.ToPtr("a-null")},
			{Namespace: "dev", Name: "app/a", Value: lo.ToPtr("a-dev")},
			{Namespace: "prd", Name: "app/b", Value: lo.ToPtr("b-prd")},
		}, out.Entries)
	})

	t.Run("regex filters on the key", func(t *testing.T) {
		t.Parallel()

		uc := &azure.ListNamespacesUseCase{Lister: &namespaceListerMock{rows: rows}}
		out, err := uc.Execute(t.Context(), azure.ListNamespacesInput{Filter: "^other$"})
		require.NoError(t, err)

		assert.Equal(t, []azure.ListNamespacesEntry{{Namespace: "", Name: "other"}}, out.Entries)
	})

	t.Run("invalid regex is a usage error", func(t *testing.T) {
		t.Parallel()

		uc := &azure.ListNamespacesUseCase{Lister: &namespaceListerMock{rows: rows}}
		_, err := uc.Execute(t.Context(), azure.ListNamespacesInput{Filter: "["})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid filter regex")
	})

	t.Run("lister error propagates", func(t *testing.T) {
		t.Parallel()

		uc := &azure.ListNamespacesUseCase{Lister: &namespaceListerMock{err: errors.New("boom")}}
		_, err := uc.Execute(t.Context(), azure.ListNamespacesInput{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list entries")
	})
}

// TestListUseCase_SortsNames verifies the list use case emits names in a stable
// alphabetical order regardless of the provider's native ordering. This covers
// Key Vault, whose API returns names in an unspecified order (#480).
func TestListUseCase_SortsNames(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return []string{"charlie", "alpha", "bravo"}, nil
		},
	}

	uc := &azure.ListUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), azure.ListInput{})
	require.NoError(t, err)

	names := make([]string, len(out.Entries))
	for i, e := range out.Entries {
		names[i] = e.Name
	}

	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, names)
}

// TestListUseCase_WithValue exercises the --with-value parallel fetch path
// (Execute + buildOutput + fetchValues): every name's value is fetched, and a
// per-name Get failure is surfaced on that entry's Error field.
func TestListUseCase_WithValue(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return []string{"alpha", "bravo"}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			if name == "bravo" {
				return nil, assert.AnError
			}

			return &domain.Entry{Name: name, Value: "val-" + name}, nil
		},
	}

	uc := &azure.ListUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), azure.ListInput{WithValue: true})
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)

	byName := lo.SliceToMap(out.Entries, func(e azure.ListEntry) (string, azure.ListEntry) {
		return e.Name, e
	})

	require.NotNil(t, byName["alpha"].Value)
	assert.Equal(t, "val-alpha", *byName["alpha"].Value)
	require.NoError(t, byName["alpha"].Error)

	assert.Nil(t, byName["bravo"].Value)
	require.ErrorIs(t, byName["bravo"].Error, assert.AnError)
}

// TestListUseCase_Error asserts a List failure is wrapped and propagated.
func TestListUseCase_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return nil, assert.AnError
		},
	}

	uc := &azure.ListUseCase{Reader: store}
	_, err := uc.Execute(t.Context(), azure.ListInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list entries")
}

// TestListUseCase_InvalidFilter asserts a bad regex is a usage error.
func TestListUseCase_InvalidFilter(t *testing.T) {
	t.Parallel()

	uc := &azure.ListUseCase{Reader: &providermock.Store{}}
	_, err := uc.Execute(t.Context(), azure.ListInput{Filter: "["})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter regex")
}

func TestDeleteUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		getErr    error
		wantValue string
		wantErr   error
	}{
		{name: "found", wantValue: "current"},
		{name: "not found yields empty", getErr: provider.ErrNotFound},
		{name: "other error propagates", getErr: assert.AnError, wantErr: assert.AnError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &providermock.Store{
				GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
					if tt.getErr != nil {
						return nil, tt.getErr
					}

					return &domain.Entry{Name: name, Value: "current"}, nil
				},
			}

			uc := &azure.DeleteUseCase{Store: store}
			val, err := uc.GetCurrentValue(t.Context(), "my-entry")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantValue, val)
		})
	}
}

func TestDeleteUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var deleted string

		store := &providermock.Store{
			DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
				deleted = name

				return nil
			},
		}

		uc := &azure.DeleteUseCase{Store: store}
		out, err := uc.Execute(t.Context(), azure.DeleteInput{Name: "my-entry"})
		require.NoError(t, err)
		assert.Equal(t, "my-entry", out.Name)
		assert.Equal(t, "my-entry", deleted)
	})

	t.Run("error is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
				return assert.AnError
			},
		}

		uc := &azure.DeleteUseCase{Store: store}
		_, err := uc.Execute(t.Context(), azure.DeleteInput{Name: "my-entry"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete entry")
	})
}

// TestRestoreUseCase_Execute covers the Key Vault soft-delete recovery path
// (delete.go RestoreUseCase.Execute), both success and wrapped-error.
func TestRestoreUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var restored string

		store := &providermock.Store{
			RestoreFunc: func(_ context.Context, name string) error {
				restored = name

				return nil
			},
		}

		uc := &azure.RestoreUseCase{Restorer: store}
		out, err := uc.Execute(t.Context(), azure.RestoreInput{Name: "my-entry"})
		require.NoError(t, err)
		assert.Equal(t, "my-entry", out.Name)
		assert.Equal(t, "my-entry", restored)
	})

	t.Run("error is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			RestoreFunc: func(_ context.Context, _ string) error {
				return assert.AnError
			},
		}

		uc := &azure.RestoreUseCase{Restorer: store}
		_, err := uc.Execute(t.Context(), azure.RestoreInput{Name: "my-entry"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to restore entry")
	})
}

func TestUpdateUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		getErr    error
		wantValue string
		wantErr   error
	}{
		{name: "found", wantValue: "current"},
		{name: "not found yields empty", getErr: provider.ErrNotFound},
		{name: "other error propagates", getErr: assert.AnError, wantErr: assert.AnError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &providermock.Store{
				GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
					if tt.getErr != nil {
						return nil, tt.getErr
					}

					return &domain.Entry{Name: name, Value: "current"}, nil
				},
			}

			uc := &azure.UpdateUseCase{Store: store}
			val, err := uc.GetCurrentValue(t.Context(), "my-entry")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantValue, val)
		})
	}
}

// TestUpdateUseCase_Execute covers the success path plus the non-not-found read
// failure and Put failure branches (the not-found branch is TestUpdateUseCase_NotFound).
func TestUpdateUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var gotType domain.ValueType

		store := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Name: name, Value: "old"}, nil
			},
			PutFunc: func(_ context.Context, _, _ string, valueType domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				gotType = valueType

				return domain.Version{ID: "abc"}, nil
			},
		}

		uc := &azure.UpdateUseCase{Store: store}
		out, err := uc.Execute(t.Context(), azure.UpdateInput{Name: "my-entry", Value: "v", ValueType: domain.ValueTypeSecret})
		require.NoError(t, err)
		assert.Equal(t, "abc", out.Version)
		assert.Equal(t, domain.ValueTypeSecret, gotType)
	})

	t.Run("read failure propagates unchanged", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, assert.AnError
			},
		}

		uc := &azure.UpdateUseCase{Store: store}
		_, err := uc.Execute(t.Context(), azure.UpdateInput{Name: "my-entry", Value: "v", ValueType: domain.ValueTypePlaintext})
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("put failure is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				return &domain.Entry{Name: name, Value: "old"}, nil
			},
			PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
				return domain.Version{}, assert.AnError
			},
		}

		uc := &azure.UpdateUseCase{Store: store}
		_, err := uc.Execute(t.Context(), azure.UpdateInput{Name: "my-entry", Value: "v", ValueType: domain.ValueTypePlaintext})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update entry")
	})
}
