package data_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/tui/data"
)

func capFor(t *testing.T, prov, service string) capability.ServiceCapability {
	t.Helper()

	for _, pc := range capability.All() {
		if pc.Provider != prov {
			continue
		}

		for _, sc := range pc.Services {
			if sc.Service == service {
				return sc
			}
		}
	}

	t.Fatalf("no capability for %s/%s", prov, service)

	return capability.ServiceCapability{}
}

// TestParamSourceShowMasksAndGates pins the param source mapping: a SecureString
// value is flagged secret (masked by the UI), and the version/type/modified meta
// rows are populated for a versioned param service.
func TestParamSourceShowMasksAndGates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	store := &providermock.Store{
		ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name: name, Value: "s3cr3t", Type: domain.ValueTypeSecret,
				Version: domain.Version{ID: "14"}, Modified: &now,
			}, nil
		},
	}

	src := data.NewParamSource(capFor(t, "aws", "param"), func(context.Context, string) (provider.Store, error) {
		return store, nil
	})

	d, err := src.Show(context.Background(), "/app/x", "")
	require.NoError(t, err)
	assert.True(t, d.Secret, "SecureString is masked")
	assert.Equal(t, "s3cr3t", d.Value, "the source carries the raw value; the UI masks it")

	labels := metaLabels(d.Meta)
	assert.Contains(t, labels, "Version")
	assert.Contains(t, labels, "Type")
	assert.NotContains(t, labels, "Namespace", "AWS param has no namespace axis")
}

// TestSecretSourceStateNotInferred pins that a version's State and StagingLabels
// are carried as-is (never one inferred from the other), and the ARN comes from
// Extra.
func TestSecretSourceStateNotInferred(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name: name, Value: "shh", Type: domain.ValueTypeSecret,
				Version: domain.Version{ID: "v1", State: "enabled"},
				Extra:   []domain.Field{{Label: "ARN", Value: "arn:test"}},
			}, nil
		},
	}

	src := data.NewSecretSource(capFor(t, "googlecloud", "secret"), store)

	d, err := src.Show(context.Background(), "api-key", "")
	require.NoError(t, err)
	assert.Equal(t, "enabled", d.State)
	assert.Empty(t, d.StagingLabels, "a State-bearing version has no staging labels")
	assert.Equal(t, "arn:test", d.ARN)
}

// TestParamSourceListFilters pins that the list source applies prefix/filter via
// the param usecase.
func TestParamSourceListFilters(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ListFunc: func(context.Context) ([]string, error) {
			return []string{"/app/a", "/app/b", "/other/c"}, nil
		},
	}

	src := data.NewParamSource(capFor(t, "aws", "param"), func(context.Context, string) (provider.Store, error) {
		return store, nil
	})

	res, err := src.List(context.Background(), data.ListParams{Prefix: "/app", Recursive: true})
	require.NoError(t, err)
	assert.Len(t, res.Items, 2, "prefix filters to the /app subtree")
}

func metaLabels(rows []data.MetaRow) []string {
	labels := make([]string, len(rows))
	for i, r := range rows {
		labels[i] = r.Label
	}

	return labels
}
