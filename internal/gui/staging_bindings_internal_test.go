//go:build production || dev

package gui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// The staging WRITE bindings (StagingAdd/Edit/Delete/AddTag/RemoveTag/Diff) are
// the ones the older staging_integration_internal_test.go never exercises: it
// seeds staged state via stagingStore.StageEntry directly. Covering them needs
// BOTH seams at once — the mock staging store (so the executor has somewhere to
// stage) AND a registry override (so serviceStrategyScoped can resolve a
// providermock.Store-backed strategy through the package-global registry).
//
// setupWriteBindingApp combines them: it builds an App on scope, runs Startup,
// injects the mock staging store, and swaps the package-global registry for one
// whose only factory returns store for every (scope, kind). The registry is a
// package global, so tests that call this must NOT run in parallel (matching
// secret_internal_test.go / param_internal_test.go). The cleanup restores it.
func setupWriteBindingApp(t *testing.T, scope provider.Scope, store provider.Store) *App {
	t.Helper()

	app := NewApp(scope, "")
	app.Startup(t.Context())
	app.stagingStore = testutil.NewMockStore()

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(scope.Provider, fakeFactory{store: store})
	t.Cleanup(func() { registry = orig })

	return app
}

// existingParamStore is a providermock whose Get reports every name as an
// existing plaintext entry EXCEPT the reserved "not-found" names, which report
// provider.ErrNotFound. A create (StagingAdd) targets a not-found name so the
// resource "does not exist"; an edit/delete/tag targets an existing name.
func existingParamStore() *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			if name == "/app/new" || name == "app/flag" {
				return nil, provider.ErrNotFound
			}

			return &domain.Entry{
				Name:    name,
				Value:   "remote-value",
				Type:    domain.ValueTypePlaintext,
				Version: domain.Version{ID: "1"},
			}, nil
		},
	}
}

// existingSecretStore mirrors existingParamStore for the secret service.
func existingSecretStore() *providermock.Store {
	return &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			if name == "new-secret" {
				return nil, provider.ErrNotFound
			}

			return &domain.Entry{
				Name:    name,
				Value:   "remote-secret",
				Type:    domain.ValueTypeSecret,
				Version: domain.Version{ID: "1"},
			}, nil
		},
	}
}

// TestApp_StagingAdd covers StagingAdd for the param and secret services: a
// create against a name the provider store reports as not-found is staged and
// surfaces through StagingStatus as an "create" operation. The secret case also
// exercises the serviceStrategyScoped secret branch + getEditStrategyScoped.
//
//nolint:paralleltest // overrides the package-global registry.
func TestApp_StagingAdd(t *testing.T) {
	t.Run("param create", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		res, err := app.StagingAdd("param", "/app/new", "fresh", "")
		require.NoError(t, err)
		assert.Equal(t, "/app/new", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.Param, 1)
		assert.Equal(t, "/app/new", status.Param[0].Name)
		assert.Equal(t, "create", status.Param[0].Operation)
		require.NotNil(t, status.Param[0].Value)
		assert.Equal(t, "fresh", *status.Param[0].Value)
	})

	t.Run("secret create", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingSecretStore())

		res, err := app.StagingAdd("secret", "new-secret", "s3cr3t", "")
		require.NoError(t, err)
		assert.Equal(t, "new-secret", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.Secret, 1)
		assert.Equal(t, "new-secret", status.Secret[0].Name)
		assert.Equal(t, "create", status.Secret[0].Operation)
	})

	t.Run("invalid service", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		_, err := app.StagingAdd("bogus", "/x", "v", "")
		assert.ErrorIs(t, err, errInvalidService)
	})
}

// TestApp_StagingEdit covers StagingEdit: an update against an existing item is
// staged (value must differ from the provider's current value or the edit would
// auto-skip). Exercises getEditStrategyScoped + serviceStrategyScoped.
//
//nolint:paralleltest // overrides the package-global registry.
func TestApp_StagingEdit(t *testing.T) {
	t.Run("param update", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		res, err := app.StagingEdit("param", "/app/config", "changed", "")
		require.NoError(t, err)
		assert.Equal(t, "/app/config", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.Param, 1)
		assert.Equal(t, "update", status.Param[0].Operation)
		require.NotNil(t, status.Param[0].Value)
		assert.Equal(t, "changed", *status.Param[0].Value)
	})

	t.Run("secret update", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingSecretStore())

		res, err := app.StagingEdit("secret", "my-secret", "rotated", "")
		require.NoError(t, err)
		assert.Equal(t, "my-secret", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.Secret, 1)
		assert.Equal(t, "update", status.Secret[0].Operation)
	})

	t.Run("invalid service", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		_, err := app.StagingEdit("bogus", "/x", "v", "")
		assert.ErrorIs(t, err, errInvalidService)
	})
}

// TestApp_StagingDelete covers StagingDelete: a delete against an existing item
// is staged. Exercises getDeleteStrategyScoped + serviceStrategyScoped.
//
//nolint:paralleltest // overrides the package-global registry.
func TestApp_StagingDelete(t *testing.T) {
	t.Run("param delete", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		res, err := app.StagingDelete("param", "/app/config", false, 0, "")
		require.NoError(t, err)
		assert.Equal(t, "/app/config", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.Param, 1)
		assert.Equal(t, "delete", status.Param[0].Operation)
		assert.Nil(t, status.Param[0].Value)
	})

	t.Run("secret delete", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingSecretStore())

		// Secret delete has recovery-window options; force=true skips validation.
		res, err := app.StagingDelete("secret", "my-secret", true, 0, "")
		require.NoError(t, err)
		assert.Equal(t, "my-secret", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.Secret, 1)
		assert.Equal(t, "delete", status.Secret[0].Operation)
	})

	t.Run("invalid service", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		_, err := app.StagingDelete("bogus", "/x", false, 0, "")
		assert.ErrorIs(t, err, errInvalidService)
	})
}

// TestApp_StagingAddRemoveTag covers StagingAddTag and StagingRemoveTag: staged
// tag add/remove against an existing item surface through StagingStatus.
//
//nolint:paralleltest // overrides the package-global registry.
func TestApp_StagingAddRemoveTag(t *testing.T) {
	t.Run("add tag", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		res, err := app.StagingAddTag("param", "/app/config", "env", "prod", "")
		require.NoError(t, err)
		assert.Equal(t, "/app/config", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.ParamTags, 1)
		assert.Equal(t, "/app/config", status.ParamTags[0].Name)
		assert.Equal(t, "prod", status.ParamTags[0].AddTags["env"])
	})

	t.Run("remove tag", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		res, err := app.StagingRemoveTag("param", "/app/config", "deprecated", "")
		require.NoError(t, err)
		assert.Equal(t, "/app/config", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.ParamTags, 1)
		assert.Contains(t, status.ParamTags[0].RemoveTags, "deprecated")
	})

	t.Run("add tag invalid service", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		_, err := app.StagingAddTag("bogus", "/x", "k", "v", "")
		assert.ErrorIs(t, err, errInvalidService)
	})

	t.Run("remove tag invalid service", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		_, err := app.StagingRemoveTag("bogus", "/x", "k", "")
		assert.ErrorIs(t, err, errInvalidService)
	})
}

// TestApp_StagingDiff covers StagingDiff: after staging an update via the write
// binding, the diff resolves the provider's current value through the
// registry-backed strategy (getDiffStrategyScoped) and reports a normal entry.
//
//nolint:paralleltest // overrides the package-global registry.
func TestApp_StagingDiff(t *testing.T) {
	t.Run("param diff of a staged update", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		_, err := app.StagingEdit("param", "/app/config", "changed", "")
		require.NoError(t, err)

		diff, err := app.StagingDiff("param", "/app/config")
		require.NoError(t, err)
		require.Len(t, diff.Entries, 1)
		assert.Equal(t, "/app/config", diff.Entries[0].Name)
		assert.Equal(t, "normal", diff.Entries[0].Type)
		assert.Equal(t, "remote-value", diff.Entries[0].RemoteValue)
		assert.Equal(t, "changed", diff.Entries[0].StagedValue)
	})

	t.Run("secret diff of a staged update", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingSecretStore())

		_, err := app.StagingEdit("secret", "my-secret", "rotated", "")
		require.NoError(t, err)

		diff, err := app.StagingDiff("secret", "my-secret")
		require.NoError(t, err)
		require.Len(t, diff.Entries, 1)
		assert.Equal(t, "normal", diff.Entries[0].Type)
		assert.True(t, diff.Entries[0].Secret, "a secret-service entry masks its value")
	})

	t.Run("diff of an unstaged item warns", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		diff, err := app.StagingDiff("param", "/app/config")
		require.NoError(t, err)
		require.Len(t, diff.Entries, 1)
		assert.Equal(t, "warning", diff.Entries[0].Type)
	})

	t.Run("invalid service", func(t *testing.T) {
		app := setupWriteBindingApp(t, provider.Scope{Provider: provider.ProviderAWS}, existingParamStore())

		_, err := app.StagingDiff("bogus", "/x")
		assert.ErrorIs(t, err, errInvalidService)
	})
}

// TestApp_StagingWriteBindings_AzureAppConfig covers the Azure App Configuration
// namespace-guard branches: editStrategyForNamespace validates+scopes the target
// namespace for the param service (staging.go:491-503), StagingDelete's own App
// Config branch (staging.go:558-568), and StagingDiff's serviceStrategyScoped
// Azure-param branch. A filter-shaped namespace (`*` / `,`-list) is rejected so
// a write can never target more than one concrete (key, namespace).
//
//nolint:paralleltest // overrides the package-global registry.
func TestApp_StagingWriteBindings_AzureAppConfig(t *testing.T) {
	azureScope := provider.Scope{Provider: provider.ProviderAzure, StoreName: "my-store"}

	t.Run("add stages under the concrete namespace", func(t *testing.T) {
		app := setupWriteBindingApp(t, azureScope, existingParamStore())

		res, err := app.StagingAdd("param", "app/flag", "on", "dev")
		require.NoError(t, err)
		assert.Equal(t, "app/flag", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.Param, 1)
		assert.Equal(t, "app/flag", status.Param[0].Name)
		assert.Equal(t, "dev", status.Param[0].Namespace, "the create is keyed to its namespace")
		assert.Equal(t, "create", status.Param[0].Operation)
	})

	t.Run("delete stages under the concrete namespace", func(t *testing.T) {
		app := setupWriteBindingApp(t, azureScope, existingParamStore())

		res, err := app.StagingDelete("param", "app/existing", false, 0, "dev")
		require.NoError(t, err)
		assert.Equal(t, "app/existing", res.Name)

		status, err := app.StagingStatus()
		require.NoError(t, err)
		require.Len(t, status.Param, 1)
		assert.Equal(t, "dev", status.Param[0].Namespace)
		assert.Equal(t, "delete", status.Param[0].Operation)
	})

	t.Run("add rejects a filter-shaped namespace", func(t *testing.T) {
		app := setupWriteBindingApp(t, azureScope, existingParamStore())

		_, err := app.StagingAdd("param", "app/flag", "on", "*")
		require.Error(t, err, "a `*` namespace names all namespaces and must be rejected")
	})

	t.Run("delete rejects a filter-shaped namespace", func(t *testing.T) {
		app := setupWriteBindingApp(t, azureScope, existingParamStore())

		_, err := app.StagingDelete("param", "app/existing", false, 0, "dev,prd")
		require.Error(t, err, "a `,`-list namespace names multiple namespaces and must be rejected")
	})

	t.Run("diff resolves the App Config param strategy", func(t *testing.T) {
		app := setupWriteBindingApp(t, azureScope, existingParamStore())

		_, err := app.StagingEdit("param", "app/existing", "changed", "dev")
		require.NoError(t, err)

		diff, err := app.StagingDiff("param", "app/existing")
		require.NoError(t, err)
		require.Len(t, diff.Entries, 1)
		assert.Equal(t, "app/existing", diff.Entries[0].Name)
		assert.Equal(t, "dev", diff.Entries[0].Namespace)
	})
}

// TestApp_StagingWriteBindings_AzureKeyVault covers the serviceStrategyScoped
// Azure secret branch: a Key Vault secret create resolves the Key Vault strategy
// through the registry override and stages a create.
//
//nolint:paralleltest // overrides the package-global registry.
func TestApp_StagingWriteBindings_AzureKeyVault(t *testing.T) {
	azureScope := provider.Scope{Provider: provider.ProviderAzure, VaultName: "my-vault"}
	app := setupWriteBindingApp(t, azureScope, existingSecretStore())

	res, err := app.StagingAdd("secret", "new-secret", "s3cr3t", "")
	require.NoError(t, err)
	assert.Equal(t, "new-secret", res.Name)

	status, err := app.StagingStatus()
	require.NoError(t, err)
	require.Len(t, status.Secret, 1)
	assert.Equal(t, "create", status.Secret[0].Operation)
}

// TestApp_StagingWriteBindings_GoogleCloud covers the serviceStrategyScoped
// Google Cloud secret branch (Google Cloud has no param service): a secret
// create resolves the Google Cloud strategy through the registry override.
//
//nolint:paralleltest // overrides the package-global registry.
func TestApp_StagingWriteBindings_GoogleCloud(t *testing.T) {
	gcloudScope := provider.Scope{Provider: provider.ProviderGoogleCloud}
	app := setupWriteBindingApp(t, gcloudScope, existingSecretStore())

	res, err := app.StagingAdd("secret", "new-secret", "s3cr3t", "")
	require.NoError(t, err)
	assert.Equal(t, "new-secret", res.Name)

	status, err := app.StagingStatus()
	require.NoError(t, err)
	require.Len(t, status.Secret, 1)
	assert.Equal(t, "create", status.Secret[0].Operation)
	assert.Empty(t, status.Param, "Google Cloud has no param service")
}
