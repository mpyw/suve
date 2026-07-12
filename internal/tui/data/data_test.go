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

// TestParamSourceVersionContentsSecret pins that a SecureString param's diff
// content is flagged secret, so the diff page masks both sides even though this
// is the param service (#677).
func TestParamSourceVersionContentsSecret(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			// Map "#13"/"#14" suffixes to ids so the two versions differ.
			id := "14"
			if len(spec) >= 3 && spec[:3] == "#13" {
				id = "13"
			}

			return provider.NewVersionRef(id), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name: name, Value: "secret-" + ref.ID(), Type: domain.ValueTypeSecret,
				Version: domain.Version{ID: ref.ID()},
			}, nil
		},
	}

	src := data.NewParamSource(capFor(t, "aws", "param"), func(context.Context, string) (provider.Store, error) {
		return store, nil
	})

	content, err := src.VersionContents(context.Background(), "/app/api/DATABASE_URL", "13", "14", "")
	require.NoError(t, err)
	assert.True(t, content.Secret, "a SecureString param diff must be flagged secret")

	// A plaintext (String) param diff must NOT be flagged secret.
	plainStore := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, _ string) (provider.VersionRef, error) {
			return provider.NewVersionRef("14"), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name: name, Value: "plain", Type: domain.ValueTypePlaintext,
				Version: domain.Version{ID: ref.ID()},
			}, nil
		},
	}
	plainSrc := data.NewParamSource(capFor(t, "aws", "param"), func(context.Context, string) (provider.Store, error) {
		return plainStore, nil
	})

	plainContent, err := plainSrc.VersionContents(context.Background(), "/app/x", "14", "14", "")
	require.NoError(t, err)
	assert.False(t, plainContent.Secret, "a String param diff is not secret")
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

// TestParamSourceHistoryCarriesValues pins #733: each history row carries its
// version's raw value (fetched via the sanctioned Resolve+Get path) and is
// flagged secret for a SecureString so the UI masks it by default.
func TestParamSourceHistoryCarriesValues(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		HistoryFunc: func(context.Context, string) ([]domain.Version, error) {
			return []domain.Version{{ID: "2"}, {ID: "1"}}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			// spec is "#<id>"; carry the id through so Get can vary the value.
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name: name, Value: "secret-v" + ref.ID(), Type: domain.ValueTypeSecret,
				Version: domain.Version{ID: ref.ID()},
			}, nil
		},
	}

	src := data.NewParamSource(capFor(t, "aws", "param"), func(context.Context, string) (provider.Store, error) {
		return store, nil
	})

	rows, err := src.History(context.Background(), "/app/x", "")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "secret-v2", rows[0].Value, "the newest version's value is carried")
	assert.Equal(t, "secret-v1", rows[1].Value)
	assert.True(t, rows[0].Secret, "a SecureString history value is flagged secret")
}

// TestSecretSourceHistoryCarriesValues pins #733 for the secret service: every
// history row carries its value and is flagged secret.
func TestSecretSourceHistoryCarriesValues(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		HistoryFunc: func(context.Context, string) ([]domain.Version, error) {
			return []domain.Version{{ID: "v2"}, {ID: "v1"}}, nil
		},
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(spec[1:]), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name: name, Value: "token-" + ref.ID(), Type: domain.ValueTypeSecret,
				Version: domain.Version{ID: ref.ID()},
			}, nil
		},
	}

	src := data.NewSecretSource(capFor(t, "aws", "secret"), store)

	rows, err := src.History(context.Background(), "prod/key", "")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "token-v2", rows[0].Value)
	assert.True(t, rows[0].Secret, "every secret history value is masked by default")
}

func metaLabels(rows []data.MetaRow) []string {
	labels := make([]string, len(rows))
	for i, r := range rows {
		labels[i] = r.Label
	}

	return labels
}
