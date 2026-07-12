//nolint:testpackage // white-box: builds the app with providermock-backed sources and shares the vt harness
package tui

import (
	"context"
	"time"

	"github.com/mpyw/suve/internal/capability"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/tui/data"
)

// Fixed timestamps so date columns render deterministically (paired with TZ=UTC
// in the golden setup).
//
//nolint:gochecknoglobals // immutable test fixtures
var (
	fxT1 = time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	fxT2 = time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC)
	fxT3 = time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
)

// capFor looks up a service capability for the fixtures.
func capFor(prov, service string) capability.ServiceCapability {
	sc, _ := capabilityFor(provider.Provider(prov), service)

	return sc
}

// staticProbe is a test staging probe returning a fixed staged-key set, so a
// golden can exercise the [staged] badge and the staged-changes banner without a
// keychain.
type staticProbe struct{ keys map[data.StagedKey]struct{} }

func (p staticProbe) StagedKeys(context.Context) (map[data.StagedKey]struct{}, error) {
	return p.keys, nil
}

// ---------------------------------------------------------------------------
// AWS param
// ---------------------------------------------------------------------------

func awsParamStore() *providermock.Store {
	names := []string{"/app/api/DATABASE_URL", "/app/api/REDIS_URL", "/app/web/BASE_URL"}

	return &providermock.Store{
		ListFunc: func(context.Context) ([]string, error) { return names, nil },
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(specID(spec)), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			id := ref.ID()
			if id == "" {
				id = "14"
			}

			return &domain.Entry{
				Name:     name,
				Value:    "postgres://db.internal:5432/app",
				Type:     domain.ValueTypeSecret,
				Version:  domain.Version{ID: id, Created: &fxT1},
				Modified: &fxT1,
				Tags:     []domain.Tag{{Key: "env", Value: "prod"}, {Key: "team", Value: "api"}},
			}, nil
		},
		HistoryFunc: func(context.Context, string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "14", Created: &fxT1},
				{ID: "13", Created: &fxT2},
				{ID: "12", Created: &fxT3},
			}, nil
		},
	}
}

func awsParamSource() data.Source {
	store := awsParamStore()

	return data.NewParamSource(capFor("aws", "param"), func(context.Context, string) (provider.Store, error) {
		return store, nil
	})
}

// awsParamDiffSource is a param source whose value varies by version, so the
// diff golden shows real +/- lines. The content is non-secret fixture JSON (a
// diff necessarily shows content; no real secret is ever rendered).
func awsParamDiffSource() data.Source {
	values := map[string]string{
		"13": "{\n  \"host\": \"db-old.internal\",\n  \"port\": 5432\n}",
		"14": "{\n  \"host\": \"db-new.internal\",\n  \"port\": 5432\n}",
	}

	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(specID(spec)), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   values[ref.ID()],
				Type:    domain.ValueTypePlaintext,
				Version: domain.Version{ID: ref.ID()},
			}, nil
		},
	}

	return data.NewParamSource(capFor("aws", "param"), func(context.Context, string) (provider.Store, error) {
		return store, nil
	})
}

// ---------------------------------------------------------------------------
// AWS secret (staging labels + ARN)
// ---------------------------------------------------------------------------

func awsSecretStore() *providermock.Store {
	names := []string{"prod/api/key", "prod/web/session"}

	return &providermock.Store{
		ListFunc: func(context.Context) ([]string, error) { return names, nil },
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(specID(spec)), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			id := ref.ID()
			if id == "" {
				id = "a1b2c3d4-1111-2222-3333-444455556666"
			}

			return &domain.Entry{
				Name:  name,
				Value: "token-abcdef",
				Type:  domain.ValueTypeSecret,
				Version: domain.Version{
					ID: id, StagingLabels: []string{"AWSCURRENT"}, Created: &fxT1,
				},
				Extra: []domain.Field{{Label: "ARN", Value: "arn:aws:secretsmanager:ap-northeast-1:123456789012:secret:prod/api/key"}},
			}, nil
		},
		HistoryFunc: func(context.Context, string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "a1b2c3d4-1111-2222-3333-444455556666", StagingLabels: []string{"AWSCURRENT"}, Created: &fxT1},
				{ID: "e5f6a7b8-9999-8888-7777-666655554444", StagingLabels: []string{"AWSPREVIOUS"}, Created: &fxT2},
			}, nil
		},
	}
}

func awsSecretSource() data.Source {
	return data.NewSecretSource(capFor("aws", "secret"), awsSecretStore())
}

// ---------------------------------------------------------------------------
// Google Cloud secret (per-version State)
// ---------------------------------------------------------------------------

func gcloudSecretStore() *providermock.Store {
	names := []string{"api-key", "db-password"}

	return &providermock.Store{
		ListFunc: func(context.Context) ([]string, error) { return names, nil },
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(specID(spec)), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			id := ref.ID()
			if id == "" {
				id = "3"
			}

			// Values vary per version (and differ in length) so a secret diff shows
			// real +/- lines — which the diff page masks. Distinct lengths make the
			// masked bullet runs differ, proving a change WITHOUT revealing content.
			values := map[string]string{
				"3": "googlecloud-secret-value-three",
				"2": "googlecloud-secret-2",
				"1": "googlecloud-old-1",
			}

			return &domain.Entry{
				Name:    name,
				Value:   values[id],
				Type:    domain.ValueTypeSecret,
				Version: domain.Version{ID: id, State: "enabled", Created: &fxT1},
			}, nil
		},
		HistoryFunc: func(context.Context, string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "3", State: "enabled", Created: &fxT1},
				{ID: "2", State: "disabled", Created: &fxT2},
				{ID: "1", State: "destroyed", Created: &fxT3},
			}, nil
		},
	}
}

func gcloudSecretSource() data.Source {
	return data.NewSecretSource(capFor("googlecloud", "secret"), gcloudSecretStore())
}

// ---------------------------------------------------------------------------
// Azure Key Vault (per-version tags + State)
// ---------------------------------------------------------------------------

func azureKVStore() *providermock.Store {
	names := []string{"vault-secret", "api-token"}

	return &providermock.Store{
		ListFunc: func(context.Context) ([]string, error) { return names, nil },
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(specID(spec)), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			id := ref.ID()
			if id == "" {
				id = "9f8e7d6c5b4a"
			}

			return &domain.Entry{
				Name:    name,
				Value:   "kv-secret-value",
				Type:    domain.ValueTypeSecret,
				Version: domain.Version{ID: id, State: "enabled", Created: &fxT1, Tags: []domain.Tag{{Key: "rotation", Value: "2026Q2"}}},
				Tags:    []domain.Tag{{Key: "rotation", Value: "2026Q2"}},
			}, nil
		},
		HistoryFunc: func(context.Context, string) ([]domain.Version, error) {
			return []domain.Version{
				{ID: "9f8e7d6c5b4a", State: "enabled", Created: &fxT1, Tags: []domain.Tag{{Key: "rotation", Value: "2026Q2"}}},
				{ID: "4c3b2a1908f7", State: "disabled", Created: &fxT2, Tags: []domain.Tag{{Key: "rotation", Value: "2026Q1"}}},
			}, nil
		},
	}
}

func azureKVSource() data.Source {
	return data.NewSecretSource(capFor("azure", "secret"), azureKVStore())
}

// ---------------------------------------------------------------------------
// Azure App Configuration (namespaces, no history)
// ---------------------------------------------------------------------------

// appConfigStore embeds a providermock and adds the App-Config-specific
// cross-namespace lister so the param source discovers namespaces (the GUI's
// ListWithNamespaces contract).
type appConfigStore struct {
	*providermock.Store

	rows []appconfig.KeyNamespace
}

func (s *appConfigStore) ListWithNamespaces(context.Context) ([]appconfig.KeyNamespace, error) {
	return s.rows, nil
}

func azureAppConfigStore() *appConfigStore {
	rows := []appconfig.KeyNamespace{
		{Key: "app/FeatureX", Namespace: "staging", Value: "enabled"},
		{Key: "app/FeatureY", Namespace: "", Value: "disabled"},
		{Key: "app/Timeout", Namespace: "", Value: "30s"},
	}

	base := &providermock.Store{
		ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:     name,
				Value:    "enabled",
				Type:     domain.ValueTypePlaintext,
				Version:  domain.Version{ID: "1", Created: &fxT1},
				Modified: &fxT1,
				Tags:     []domain.Tag{{Key: "team", Value: "web"}},
			}, nil
		},
	}

	return &appConfigStore{Store: base, rows: rows}
}

func azureAppConfigSource() data.Source {
	store := azureAppConfigStore()

	return data.NewParamSource(capFor("azure", "param"), func(context.Context, string) (provider.Store, error) {
		return store, nil
	})
}

// specID decodes the leading "#<id>" of a resolve spec suffix into a version id
// ("" for the latest), so the mock's Get can vary its value per version in diff
// fixtures.
func specID(spec string) string {
	if len(spec) < 2 || spec[0] != '#' {
		return ""
	}

	id := spec[1:]
	for i := range len(id) {
		if id[i] == '~' || id[i] == ':' {
			return id[:i]
		}
	}

	return id
}

// sourceForShape returns a sourceFor closure serving one shape's source for its
// service tab (and a nil probe for other services).
func sourceForShape(service string, src data.Source, probe data.StagingProbe) func(string) (data.Source, data.StagingProbe) {
	return func(requested string) (data.Source, data.StagingProbe) {
		if requested == service {
			return src, probe
		}

		return nil, nil
	}
}
