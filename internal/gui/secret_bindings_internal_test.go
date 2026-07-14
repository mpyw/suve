//go:build production || dev

package gui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
)

// injectSecretStore overrides the package-global registry so a binding resolves
// the given store for the given provider, restoring the original on cleanup.
// It is the shared seam for the secret-binding tests (see fakeFactory in
// param_internal_test.go).
func injectSecretStore(t *testing.T, p provider.Provider, store provider.Store) {
	t.Helper()

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(p, fakeFactory{store: store})

	t.Cleanup(func() { registry = orig })
}

// storeWithoutRestore is a provider.Store that does NOT implement
// provider.Restorer: it embeds the three Store interfaces (so it satisfies
// Store) without adding a Restore method, so SecretRestore's type assertion
// fails and falls back to errRestoreUnsupported.
type storeWithoutRestore struct {
	provider.Reader
	provider.Writer
	provider.Tagger
}

var _ provider.Store = storeWithoutRestore{}

// TestSecretList asserts the SecretList binding lists names (alphabetically, per
// the use case) and, with withValue, threads each secret's value through.
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretList(t *testing.T) {
	store := &providermock.Store{
		ListFunc: func(context.Context) ([]string, error) {
			return []string{"beta", "alpha"}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: name, Value: "val-" + name, Type: domain.ValueTypeSecret}, nil
		},
	}

	injectSecretStore(t, provider.ProviderAWS, store)

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	res, err := app.SecretList("", true, "", 0, "")
	require.NoError(t, err)
	require.Len(t, res.Entries, 2)
	assert.Empty(t, res.NextToken, "the secret list use case never paginates")

	// The use case sorts alphabetically (#480).
	assert.Equal(t, "alpha", res.Entries[0].Name)
	require.NotNil(t, res.Entries[0].Value)
	assert.Equal(t, "val-alpha", *res.Entries[0].Value)

	assert.Equal(t, "beta", res.Entries[1].Name)
	require.NotNil(t, res.Entries[1].Value)
	assert.Equal(t, "val-beta", *res.Entries[1].Value)
}

// TestSecretShow_StateVsStagingLabels asserts the SecretShow binding copies a
// version's two independent, provider-specific axes into the RIGHT fields
// (#419): AWS staging labels land in StagingLabels (State empty), while a
// lifecycle state lands in State (StagingLabels empty) — a version never has
// both. It also checks the ARN/description/tags/created mapping.
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretShow_StateVsStagingLabels(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name        string
		version     domain.Version
		wantStaging []string
		wantState   string
	}{
		{
			name:        "aws staging labels populate StagingLabels not State",
			version:     domain.Version{ID: "v1", StagingLabels: []string{"AWSPREVIOUS", "AWSCURRENT"}},
			wantStaging: []string{"AWSCURRENT", "AWSPREVIOUS"}, // stages() sorts.
			wantState:   "",
		},
		{
			name:        "lifecycle state populates State not StagingLabels",
			version:     domain.Version{ID: "v1", State: "enabled"},
			wantStaging: nil,
			wantState:   "enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ver := tt.version
			ver.Created = &created

			store := &providermock.Store{
				ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
					return provider.VersionRef{}, nil
				},
				GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
					return &domain.Entry{
						Name:        name,
						Value:       "secret-value",
						Description: "the description",
						Version:     ver,
						Extra:       []domain.Field{{Label: "ARN", Value: "arn:aws:secretsmanager:x"}},
						Tags:        []domain.Tag{{Key: "env", Value: "prod"}},
					}, nil
				},
			}

			injectSecretStore(t, provider.ProviderAWS, store)

			app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

			res, err := app.SecretShow("my-secret")
			require.NoError(t, err)

			assert.Equal(t, "my-secret", res.Name)
			assert.Equal(t, "arn:aws:secretsmanager:x", res.ARN)
			assert.Equal(t, "v1", res.VersionID)
			assert.Equal(t, "secret-value", res.Value)
			assert.Equal(t, "the description", res.Description)
			assert.Equal(t, tt.wantStaging, res.StagingLabels)
			assert.Equal(t, tt.wantState, res.State)
			assert.NotEmpty(t, res.CreatedDate)

			require.Len(t, res.Tags, 1)
			assert.Equal(t, "env", res.Tags[0].Key)
			assert.Equal(t, "prod", res.Tags[0].Value)
		})
	}
}

// TestSecretLog_StateVsStagingLabels asserts the SecretLog binding maps each
// version's independent axes into the right fields (#419), computes IsCurrent,
// and threads each version's value. As in SecretShow, AWS carries staging
// labels (no State) while Google Cloud / Key Vault carry a lifecycle state (no
// staging labels).
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretLog_StateVsStagingLabels(t *testing.T) {
	tests := []struct {
		name     string
		versions []domain.Version
		// asserted against the first (current) entry.
		wantStaging []string
		wantState   string
	}{
		{
			name: "aws staging labels populate StagingLabels not State",
			versions: []domain.Version{
				{ID: "v2", StagingLabels: []string{"AWSCURRENT"}},
				{ID: "v1", StagingLabels: []string{"AWSPREVIOUS"}},
			},
			wantStaging: []string{"AWSCURRENT"},
			wantState:   "",
		},
		{
			name: "lifecycle state populates State not StagingLabels",
			versions: []domain.Version{
				{ID: "v2", State: "enabled"},
				{ID: "v1", State: "disabled"},
			},
			wantStaging: nil,
			wantState:   "enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &providermock.Store{
				HistoryFunc: func(context.Context, string) ([]domain.Version, error) {
					return tt.versions, nil
				},
				ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
					// SecretLog resolves each version by "#"+id to fetch its value.
					return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
				},
				GetFunc: func(_ context.Context, _ string, ref provider.VersionRef) (*domain.Entry, error) {
					return &domain.Entry{Value: "value-" + ref.ID()}, nil
				},
			}

			injectSecretStore(t, provider.ProviderAWS, store)

			app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

			res, err := app.SecretLog("my-secret", 0)
			require.NoError(t, err)
			require.Len(t, res.Entries, 2)

			// History is newest first; v2 is current.
			assert.Equal(t, "v2", res.Entries[0].VersionID)
			assert.Equal(t, tt.wantStaging, res.Entries[0].StagingLabels)
			assert.Equal(t, tt.wantState, res.Entries[0].State)
			assert.True(t, res.Entries[0].IsCurrent, "the newest version is current")
			assert.Equal(t, "value-v2", res.Entries[0].Value)

			assert.Equal(t, "v1", res.Entries[1].VersionID)
			assert.False(t, res.Entries[1].IsCurrent)
			assert.Equal(t, "value-v1", res.Entries[1].Value)
		})
	}
}

// TestSecretDelete_RecoveryWindowGatedOnAWS asserts the SecretDelete binding
// surfaces a synthetic recovery-window date ONLY for a soft delete on AWS
// Secrets Manager. A force delete (AWS) and any delete on Google Cloud / Azure
// Key Vault must NOT get a false "recoverable until" date (secret.go:327). It
// also checks force threads a ForceDelete option to the provider.
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretDelete_RecoveryWindowGatedOnAWS(t *testing.T) {
	tests := []struct {
		name      string
		scope     provider.Scope
		force     bool
		wantDate  bool
		wantForce bool
	}{
		{
			name:      "aws soft delete computes a recovery date",
			scope:     provider.Scope{Provider: provider.ProviderAWS},
			force:     false,
			wantDate:  true,
			wantForce: false,
		},
		{
			name:      "aws force delete skips the recovery date",
			scope:     provider.Scope{Provider: provider.ProviderAWS},
			force:     true,
			wantDate:  false,
			wantForce: true,
		},
		{
			name:      "google cloud delete gets no false recovery date",
			scope:     provider.Scope{Provider: provider.ProviderGoogleCloud, ProjectID: "p"},
			force:     false,
			wantDate:  false,
			wantForce: false,
		},
		{
			name:      "azure key vault delete gets no false recovery date",
			scope:     provider.Scope{Provider: provider.ProviderAzure, VaultName: "v"},
			force:     false,
			wantDate:  false,
			wantForce: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotForce bool

			store := &providermock.Store{
				DeleteFunc: func(_ context.Context, _ string, opts ...provider.DeleteOption) error {
					gotForce = len(opts) > 0

					return nil
				},
			}

			injectSecretStore(t, tt.scope.Provider, store)

			app := &App{ctx: t.Context(), scope: tt.scope}

			res, err := app.SecretDelete("my-secret", tt.force)
			require.NoError(t, err)
			assert.Equal(t, "my-secret", res.Name)
			assert.Equal(t, tt.wantForce, gotForce, "force flag must control the ForceDelete option")

			if tt.wantDate {
				assert.NotEmpty(t, res.DeletionDate, "AWS soft delete must surface a recovery-window date")
			} else {
				assert.Empty(t, res.DeletionDate, "no false recovery date for %q", tt.name)
			}
		})
	}
}

// TestSecretAddTag asserts the SecretAddTag binding forwards a single key/value
// as an Add map to the provider's Tagger.
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretAddTag(t *testing.T) {
	var (
		gotName string
		gotAdd  map[string]string
	)

	store := &providermock.Store{
		TagFunc: func(_ context.Context, name string, add map[string]string) error {
			gotName = name
			gotAdd = add

			return nil
		},
	}

	injectSecretStore(t, provider.ProviderAWS, store)

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	require.NoError(t, app.SecretAddTag("my-secret", "env", "prod"))
	assert.Equal(t, "my-secret", gotName)
	assert.Equal(t, map[string]string{"env": "prod"}, gotAdd)
}

// TestSecretRemoveTag asserts the SecretRemoveTag binding forwards a single key
// as a Remove list to the provider's Tagger.
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretRemoveTag(t *testing.T) {
	var (
		gotName string
		gotKeys []string
	)

	store := &providermock.Store{
		UntagFunc: func(_ context.Context, name string, keys []string) error {
			gotName = name
			gotKeys = keys

			return nil
		},
	}

	injectSecretStore(t, provider.ProviderAWS, store)

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	require.NoError(t, app.SecretRemoveTag("my-secret", "env"))
	assert.Equal(t, "my-secret", gotName)
	assert.Equal(t, []string{"env"}, gotKeys)
}

// TestSecretDiff asserts the SecretDiff binding resolves both specs and maps the
// old/new name, version, and value into the result.
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretDiff(t *testing.T) {
	store := &providermock.Store{
		ResolveFunc: func(_ context.Context, _, spec string) (provider.VersionRef, error) {
			return provider.NewVersionRef(strings.TrimPrefix(spec, "#")), nil
		},
		GetFunc: func(_ context.Context, name string, ref provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:    name,
				Value:   "value-" + ref.ID(),
				Version: domain.Version{ID: ref.ID()},
			}, nil
		},
	}

	injectSecretStore(t, provider.ProviderAWS, store)

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	res, err := app.SecretDiff("my-secret#v1", "my-secret#v2")
	require.NoError(t, err)

	assert.Equal(t, "my-secret", res.OldName)
	assert.Equal(t, "v1", res.OldVersionID)
	assert.Equal(t, "value-v1", res.OldValue)

	assert.Equal(t, "my-secret", res.NewName)
	assert.Equal(t, "v2", res.NewVersionID)
	assert.Equal(t, "value-v2", res.NewValue)
}

// TestSecretRestore_RestorerGate asserts the SecretRestore binding's capability
// gate: a store implementing provider.Restorer restores (and its Restore is
// called with the name), while a store that does NOT implement Restorer yields
// errRestoreUnsupported without touching the store (secret.go:410).
//
//nolint:paralleltest // overrides the package-global registry.
func TestSecretRestore_RestorerGate(t *testing.T) {
	t.Run("restorer store restores the secret", func(t *testing.T) {
		var gotName string

		store := &providermock.Store{
			RestoreFunc: func(_ context.Context, name string) error {
				gotName = name

				return nil
			},
		}

		injectSecretStore(t, provider.ProviderAWS, store)

		app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

		res, err := app.SecretRestore("my-secret")
		require.NoError(t, err)
		assert.Equal(t, "my-secret", res.Name)
		assert.Equal(t, "my-secret", gotName)
	})

	t.Run("non-restorer store falls back to errRestoreUnsupported", func(t *testing.T) {
		injectSecretStore(t, provider.ProviderAzure, storeWithoutRestore{})

		app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAzure, VaultName: "v"}}

		res, err := app.SecretRestore("my-secret")
		require.ErrorIs(t, err, errRestoreUnsupported)
		assert.Nil(t, res)
	})
}
