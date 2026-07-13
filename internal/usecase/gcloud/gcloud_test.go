package gcloud_test

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
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/gcloud"
	"github.com/mpyw/suve/internal/version/gcloudversion"
)

// TestLogUseCase_FilterBeforeCount asserts date filters run BEFORE the count
// cap: -n must yield up to N versions that match the filter, not N newest then
// filtered down to fewer (#351). Only the two oldest versions match --until;
// the old count-first order would have truncated them all away.
func TestLogUseCase_FilterBeforeCount(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "3", Created: &now},
		{ID: "2", Created: lo.ToPtr(now.Add(-2 * time.Hour))},
		{ID: "1", Created: lo.ToPtr(now.Add(-3 * time.Hour))},
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

	uc := &gcloud.LogUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.LogInput{
		Name:       "my-secret",
		MaxResults: 1,
		Until:      lo.ToPtr(now.Add(-time.Hour)),
	})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	assert.Equal(t, "2", out.Entries[0].Version)
}

func TestShowUseCase(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			assert.Equal(t, "#3", spec)

			return provider.NewVersionRef("3"), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			assert.Equal(t, "3", ref.ID())

			return &domain.Entry{
				Name:        name,
				Value:       "hello",
				Version:     domain.Version{ID: "3", State: "enabled"},
				Description: "app credentials",
				Tags:        []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	spec, err := gcloudversion.Parse("my-secret#3")
	require.NoError(t, err)

	uc := &gcloud.ShowUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.ShowInput{Spec: spec})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", out.Name)
	assert.Equal(t, "hello", out.Value)
	assert.Equal(t, "3", out.Version)
	assert.Equal(t, "enabled", out.State)
	assert.Equal(t, "app credentials", out.Description)
	assert.Equal(t, []gcloud.ShowTag{{Key: "env", Value: "prod"}}, out.Tags)
}

// TestCreateUseCase_ThreadsDescription asserts the immediate create use case
// forwards the description to the writer (stored as the "description"
// annotation by the Google Cloud adapter) — the immediate axis of #666.
func TestCreateUseCase_ThreadsDescription(t *testing.T) {
	t.Parallel()

	var gotDescription string

	store := &providermock.Store{
		CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption) (domain.Version, error) {
			gotDescription = description

			return domain.Version{ID: "1"}, nil
		},
	}

	uc := &gcloud.CreateUseCase{Writer: store}
	out, err := uc.Execute(t.Context(), gcloud.CreateInput{Name: "my-secret", Value: "v1", Description: "app credentials"})
	require.NoError(t, err)
	assert.Equal(t, "1", out.Version)
	assert.Equal(t, "app credentials", gotDescription)
}

// TestUpdateUseCase_ThreadsDescription asserts the immediate update use case
// forwards the description to the writer on Put.
func TestUpdateUseCase_ThreadsDescription(t *testing.T) {
	t.Parallel()

	var gotDescription string

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "old"}, nil
		},
		PutFunc: func(_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption) (domain.Version, error) {
			gotDescription = description

			return domain.Version{ID: "2"}, nil
		},
	}

	uc := &gcloud.UpdateUseCase{Store: store}
	out, err := uc.Execute(t.Context(), gcloud.UpdateInput{Name: "my-secret", Value: "v2", Description: "rotated key"})
	require.NoError(t, err)
	assert.Equal(t, "2", out.Version)
	assert.Equal(t, "rotated key", gotDescription)
}

func TestDiffUseCase(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			switch spec {
			case "#1":
				return provider.NewVersionRef("1"), nil
			case "":
				return provider.VersionRef{}, nil
			default:
				return provider.VersionRef{}, assert.AnError
			}
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			id := ref.ID()
			if id == "" {
				id = "2"
			}

			return &domain.Entry{Name: name, Value: "val" + id, Version: domain.Version{ID: id}}, nil
		},
	}

	spec1, err := gcloudversion.Parse("my-secret#1")
	require.NoError(t, err)

	spec2, err := gcloudversion.Parse("my-secret")
	require.NoError(t, err)

	uc := &gcloud.DiffUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.DiffInput{Spec1: spec1, Spec2: spec2})
	require.NoError(t, err)
	assert.Equal(t, "1", out.OldVersion)
	assert.Equal(t, "val1", out.OldValue)
	assert.Equal(t, "2", out.NewVersion)
	assert.Equal(t, "val2", out.NewValue)
}

func TestListUseCase_PrefixFilter(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return []string{"prod-a", "prod-b", "dev-c"}, nil
		},
	}

	uc := &gcloud.ListUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.ListInput{Prefix: "prod"})
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)
	assert.Equal(t, "prod-a", out.Entries[0].Name)
	assert.Equal(t, "prod-b", out.Entries[1].Name)
}

// TestListUseCase_SortsNames verifies the list use case emits names in a stable
// alphabetical order regardless of the provider's native ordering (#480).
func TestListUseCase_SortsNames(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: func(_ context.Context) ([]string, error) {
			return []string{"charlie", "alpha", "bravo"}, nil
		},
	}

	uc := &gcloud.ListUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.ListInput{})
	require.NoError(t, err)

	names := make([]string, len(out.Entries))
	for i, e := range out.Entries {
		names[i] = e.Name
	}

	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, names)
}

// TestListUseCase_WithValue exercises the --with-value parallel fetch path
// (buildOutput + fetchValues): every name's current value is fetched, and a
// per-name Get failure is surfaced on that entry's Error field rather than
// aborting the whole listing.
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

	uc := &gcloud.ListUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.ListInput{WithValue: true})
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)

	byName := lo.SliceToMap(out.Entries, func(e gcloud.ListEntry) (string, gcloud.ListEntry) {
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

	uc := &gcloud.ListUseCase{Reader: store}
	_, err := uc.Execute(t.Context(), gcloud.ListInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secrets")
}

// TestListUseCase_InvalidFilter asserts a bad regex is a usage error.
func TestListUseCase_InvalidFilter(t *testing.T) {
	t.Parallel()

	uc := &gcloud.ListUseCase{Reader: &providermock.Store{}}
	_, err := uc.Execute(t.Context(), gcloud.ListInput{Filter: "["})
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

			uc := &gcloud.DeleteUseCase{Store: store}
			val, err := uc.GetCurrentValue(t.Context(), "my-secret")

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

		uc := &gcloud.DeleteUseCase{Store: store}
		out, err := uc.Execute(t.Context(), gcloud.DeleteInput{Name: "my-secret"})
		require.NoError(t, err)
		assert.Equal(t, "my-secret", out.Name)
		assert.Equal(t, "my-secret", deleted)
	})

	t.Run("error is wrapped", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			DeleteFunc: func(_ context.Context, _ string, _ ...provider.DeleteOption) error {
				return assert.AnError
			},
		}

		uc := &gcloud.DeleteUseCase{Store: store}
		_, err := uc.Execute(t.Context(), gcloud.DeleteInput{Name: "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete secret")
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

			uc := &gcloud.UpdateUseCase{Store: store}
			val, err := uc.GetCurrentValue(t.Context(), "my-secret")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantValue, val)
		})
	}
}

// TestUpdateUseCase_Execute_ErrorBranches covers the not-found, non-not-found
// read failure, and Put failure branches of the update Execute path.
func TestUpdateUseCase_Execute_ErrorBranches(t *testing.T) {
	t.Parallel()

	t.Run("not found maps to ErrSecretNotFound", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, provider.ErrNotFound
			},
		}

		uc := &gcloud.UpdateUseCase{Store: store}
		_, err := uc.Execute(t.Context(), gcloud.UpdateInput{Name: "missing", Value: "v"})
		require.ErrorIs(t, err, gcloud.ErrSecretNotFound)
	})

	t.Run("read failure propagates unchanged", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, assert.AnError
			},
		}

		uc := &gcloud.UpdateUseCase{Store: store}
		_, err := uc.Execute(t.Context(), gcloud.UpdateInput{Name: "my-secret", Value: "v"})
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

		uc := &gcloud.UpdateUseCase{Store: store}
		_, err := uc.Execute(t.Context(), gcloud.UpdateInput{Name: "my-secret", Value: "v"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update secret")
	})
}

// TestCreateUseCase_Error asserts a Create failure is wrapped.
func TestCreateUseCase_Error(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(_ context.Context, _, _ string, _ domain.ValueType, _ string, _ ...provider.WriteOption) (domain.Version, error) {
			return domain.Version{}, provider.ErrAlreadyExists
		},
	}

	uc := &gcloud.CreateUseCase{Writer: store}
	_, err := uc.Execute(t.Context(), gcloud.CreateInput{Name: "my-secret", Value: "v"})
	require.Error(t, err)
	require.ErrorIs(t, err, provider.ErrAlreadyExists)
	assert.Contains(t, err.Error(), "failed to create secret")
}

// TestShowUseCase_Errors covers the Resolve-error and Get-error branches.
func TestShowUseCase_Errors(t *testing.T) {
	t.Parallel()

	spec, err := gcloudversion.Parse("my-secret#3")
	require.NoError(t, err)

	t.Run("resolve error", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
				return provider.VersionRef{}, assert.AnError
			},
		}

		uc := &gcloud.ShowUseCase{Reader: store}
		_, err := uc.Execute(t.Context(), gcloud.ShowInput{Spec: spec})
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("get error", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
				return provider.NewVersionRef("3"), nil
			},
			GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
				return nil, assert.AnError
			},
		}

		uc := &gcloud.ShowUseCase{Reader: store}
		_, err := uc.Execute(t.Context(), gcloud.ShowInput{Spec: spec})
		require.ErrorIs(t, err, assert.AnError)
	})
}

// TestDiffUseCase_Errors covers a resolve failure on the first spec and a get
// failure on the second.
func TestDiffUseCase_Errors(t *testing.T) {
	t.Parallel()

	spec1, err := gcloudversion.Parse("my-secret#1")
	require.NoError(t, err)

	spec2, err := gcloudversion.Parse("my-secret")
	require.NoError(t, err)

	t.Run("resolve failure on first spec", func(t *testing.T) {
		t.Parallel()

		store := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
				return provider.VersionRef{}, assert.AnError
			},
		}

		uc := &gcloud.DiffUseCase{Reader: store}
		_, err := uc.Execute(t.Context(), gcloud.DiffInput{Spec1: spec1, Spec2: spec2})
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("get failure on second spec", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("boom on #2")
		store := &providermock.Store{
			ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
				return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
			},
			GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
				if ref.ID() == "" {
					return nil, sentinel
				}

				return &domain.Entry{Name: name, Value: "v", Version: domain.Version{ID: ref.ID()}}, nil
			},
		}

		uc := &gcloud.DiffUseCase{Reader: store}
		_, err := uc.Execute(t.Context(), gcloud.DiffInput{Spec1: spec1, Spec2: spec2})
		require.ErrorIs(t, err, sentinel)
	})
}

// TestLogUseCase_PerVersionFetchError asserts a resolve/get failure for a single
// version is recorded on that entry's Error field, not fatal to the listing.
func TestLogUseCase_PerVersionFetchError(t *testing.T) {
	t.Parallel()

	now := time.Now()
	versions := []domain.Version{
		{ID: "2", State: "enabled", Created: &now},
		{ID: "1", State: "destroyed", Created: lo.ToPtr(now.Add(-time.Hour))},
	}

	resolveErr := errors.New("cannot resolve destroyed")

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return versions, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			if spec == "#1" {
				return provider.VersionRef{}, resolveErr
			}

			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "val" + ref.ID(), Version: domain.Version{ID: ref.ID()}}, nil
		},
	}

	uc := &gcloud.LogUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.LogInput{Name: "my-secret"})
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)
	assert.True(t, out.InitialIncluded)

	assert.Equal(t, "val2", out.Entries[0].Value)
	require.NoError(t, out.Entries[0].Error)

	assert.Empty(t, out.Entries[1].Value)
	require.ErrorIs(t, out.Entries[1].Error, resolveErr)
}

// TestLogUseCase_GetError asserts a Get failure (as opposed to Resolve) on a
// version is likewise recorded per-entry.
func TestLogUseCase_GetError(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return []domain.Version{{ID: "1", Created: &now}}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, assert.AnError
		},
	}

	uc := &gcloud.LogUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.LogInput{Name: "my-secret"})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	require.ErrorIs(t, out.Entries[0].Error, assert.AnError)
}

// TestLogUseCase_EmptyHistory covers the early return when the secret has no
// versions.
func TestLogUseCase_EmptyHistory(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return nil, nil
		},
	}

	uc := &gcloud.LogUseCase{Reader: store}
	out, err := uc.Execute(t.Context(), gcloud.LogInput{Name: "my-secret"})
	require.NoError(t, err)
	assert.Empty(t, out.Entries)
	assert.False(t, out.InitialIncluded)
}

// TestLogUseCase_HistoryError asserts a History failure is wrapped.
func TestLogUseCase_HistoryError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			return nil, assert.AnError
		},
	}

	uc := &gcloud.LogUseCase{Reader: store}
	_, err := uc.Execute(t.Context(), gcloud.LogInput{Name: "my-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secret versions")
}
