package gcloud_test

import (
	"context"
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
				Name:    name,
				Value:   "hello",
				Version: domain.Version{ID: "3", Label: "enabled"},
				Tags:    []domain.Tag{{Key: "env", Value: "prod"}},
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
	assert.Equal(t, []gcloud.ShowTag{{Key: "env", Value: "prod"}}, out.Tags)
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
