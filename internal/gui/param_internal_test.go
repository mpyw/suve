//go:build production || dev

package gui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/aws/param/paramtype"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/providermock"
)

// fakeFactory returns a fixed store for any scope/kind, so a test can inject a
// providermock into the GUI's package-global registry.
type fakeFactory struct{ store provider.Store }

func (f fakeFactory) Store(context.Context, provider.Scope, provider.Kind) (provider.Store, error) {
	return f.store, nil
}

// fakeNamespaceLister is a minimal appConfigNamespaceLister for testing the
// Azure App Configuration all-namespaces list path without a real client.
type fakeNamespaceLister struct {
	items []appconfig.KeyNamespace
	err   error
}

func (f *fakeNamespaceLister) ListWithNamespaces(_ context.Context) ([]appconfig.KeyNamespace, error) {
	return f.items, f.err
}

// TestParamList_PopulatesSecretAndType covers the generic (non-App-Config) list
// path: Secret/Type must mirror the domain value type of each entry so a future
// list-mode masking that trusts entry.secret masks SecureString params (#491).
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamList_PopulatesSecretAndType(t *testing.T) {
	store := &providermock.Store{
		ListFunc: func(context.Context) ([]string, error) {
			return []string{"/my/plain", "/my/secure"}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			if name == "/my/secure" {
				return &domain.Entry{Name: name, Value: "s", Type: domain.ValueTypeSecret}, nil
			}

			return &domain.Entry{Name: name, Value: "p", Type: domain.ValueTypePlaintext}, nil
		},
	}

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(provider.ProviderAWS, fakeFactory{store: store})

	t.Cleanup(func() { registry = orig })

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	// withValue=true so each entry's value (and thus its type) is fetched.
	res, err := app.ParamList("", false, true, "", 0, "")
	require.NoError(t, err)
	require.Len(t, res.Entries, 2)

	byName := make(map[string]ParamListEntry, len(res.Entries))
	for _, e := range res.Entries {
		byName[e.Name] = e
	}

	assert.False(t, byName["/my/plain"].Secret, "plaintext param must not be a secret")
	assert.Equal(t, paramtype.String, byName["/my/plain"].Type)

	assert.True(t, byName["/my/secure"].Secret, "SecureString param must be flagged secret")
	assert.Equal(t, paramtype.SecureString, byName["/my/secure"].Type)
}

// TestParamSet_ThreadsDescription asserts the GUI ParamSet binding forwards the
// description argument into the create use case (and, on the already-exists
// fallback, into the update use case), so a description set in the GUI form
// reaches the provider writer (#767).
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamSet_ThreadsDescription(t *testing.T) {
	var gotCreateDescription, gotPutDescription string

	store := &providermock.Store{
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			// The update path Gets the current entry before Put; return one so the
			// fallback reaches Put rather than a not-found error.
			return &domain.Entry{Name: name, Value: "old", Type: domain.ValueTypePlaintext}, nil
		},
		CreateFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			gotCreateDescription = description

			return domain.Version{ID: "1"}, nil
		},
		PutFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			gotPutDescription = description

			return domain.Version{ID: "2"}, nil
		},
	}

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(provider.ProviderAWS, fakeFactory{store: store})

	t.Cleanup(func() { registry = orig })

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	// Create path: a fresh key routes to Create and carries the description.
	res, err := app.ParamSet("/my/param", "v", "String", "", "my description")
	require.NoError(t, err)
	assert.True(t, res.IsCreated)
	assert.Equal(t, "my description", gotCreateDescription)

	// Update path: when the key already exists, ParamSet falls back to Put and
	// must forward the description there too.
	store.CreateFunc = func(
		context.Context, string, string, domain.ValueType, string, ...provider.WriteOption,
	) (domain.Version, error) {
		return domain.Version{}, provider.ErrAlreadyExists
	}

	res, err = app.ParamSet("/my/param", "v2", "String", "", "updated description")
	require.NoError(t, err)
	assert.False(t, res.IsCreated)
	assert.Equal(t, "updated description", gotPutDescription)
}

// TestParamSet_DropsDescriptionForAzure asserts the server-side guard: Azure App
// Configuration ignores descriptions, so the binding must drop any value even if
// the frontend (which hides the input) is bypassed — defense in depth (#767).
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamSet_DropsDescriptionForAzure(t *testing.T) {
	var gotDescription string

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, _, _ string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			gotDescription = description

			return domain.Version{ID: "1"}, nil
		},
	}

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(provider.ProviderAzure, fakeFactory{store: store})

	t.Cleanup(func() { registry = orig })

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAzure, StoreName: "store"}}

	_, err := app.ParamSet("app/config", "v", "", "", "should-be-dropped")
	require.NoError(t, err)
	assert.Empty(t, gotDescription, "Azure App Configuration must not receive a description")
}

// TestParamListWithNamespaces_PopulatesSecretAndType covers the App
// Configuration list path: its values are always plaintext, so Secret stays
// false and Type is the plaintext display, matching the detail path (#491).
func TestParamListWithNamespaces_PopulatesSecretAndType(t *testing.T) {
	t.Parallel()

	lister := &fakeNamespaceLister{
		items: []appconfig.KeyNamespace{{Key: "app/config", Namespace: "dev", Value: "v"}},
	}
	app := &App{ctx: t.Context()}

	res, err := app.paramListWithNamespaces(lister, "", true, true, "")
	require.NoError(t, err)
	require.Len(t, res.Entries, 1)

	assert.False(t, res.Entries[0].Secret, "App Configuration values are never secrets")
	assert.Equal(t, paramtype.String, res.Entries[0].Type)
}

func TestParamListWithNamespaces_PopulatesNamespace(t *testing.T) {
	t.Parallel()

	lister := &fakeNamespaceLister{
		items: []appconfig.KeyNamespace{
			{Key: "app/config", Namespace: "", Value: "n"},
			{Key: "app/config", Namespace: "dev", Value: "d"},
			{Key: "app/db", Namespace: "prd", Value: "p"},
		},
	}
	app := &App{ctx: t.Context()}

	// withValue=true so values flow through from the list response.
	res, err := app.paramListWithNamespaces(lister, "", true, true, "")
	require.NoError(t, err)
	require.Len(t, res.Entries, 3)

	assert.Equal(t, "app/config", res.Entries[0].Name)
	assert.Empty(t, res.Entries[0].Namespace, "empty namespace is the null/default")
	require.NotNil(t, res.Entries[0].Value)
	assert.Equal(t, "n", *res.Entries[0].Value)

	assert.Equal(t, "app/config", res.Entries[1].Name)
	assert.Equal(t, "dev", res.Entries[1].Namespace)

	assert.Equal(t, "app/db", res.Entries[2].Name)
	assert.Equal(t, "prd", res.Entries[2].Namespace)
}

func TestParamListWithNamespaces_FiltersPrefixAndRegex(t *testing.T) {
	t.Parallel()

	lister := &fakeNamespaceLister{
		items: []appconfig.KeyNamespace{
			{Key: "app/config", Namespace: "dev"},
			{Key: "app/db/url", Namespace: "dev"},
			{Key: "other/thing", Namespace: "prd"},
		},
	}
	app := &App{ctx: t.Context()}

	// Prefix "app" recursive: keeps both app/* entries, drops other/thing.
	res, err := app.paramListWithNamespaces(lister, "app", true, false, "")
	require.NoError(t, err)
	require.Len(t, res.Entries, 2)
	assert.Nil(t, res.Entries[0].Value, "withValue=false leaves Value nil")

	// Regex filter further narrows to the db entry.
	res, err = app.paramListWithNamespaces(lister, "app", true, false, "db")
	require.NoError(t, err)
	require.Len(t, res.Entries, 1)
	assert.Equal(t, "app/db/url", res.Entries[0].Name)
}

// TestAppConfigNamespaceLister_Gate documents the type-assertion gate that
// decides the list path: the concrete App Configuration store implements the
// App-Config-specific lister (so entries carry a namespace), while a neutral
// provider store does not (so ParamList takes the generic path and leaves the
// namespace empty).
func TestAppConfigNamespaceLister_Gate(t *testing.T) {
	t.Parallel()

	var appConfigStore provider.Store = appconfig.New(nil, "")

	_, isLister := appConfigStore.(appConfigNamespaceLister)
	assert.True(t, isLister, "App Configuration store must expose ListWithNamespaces")

	var neutralStore provider.Store = &providermock.Store{}

	_, isLister = neutralStore.(appConfigNamespaceLister)
	assert.False(t, isLister, "neutral provider stores must not expose ListWithNamespaces")
}

// TestParamNamespaceHelpers verifies the create-target namespace helpers that
// back the GUI namespaced-create (#431): validateParamNamespace rejects a filter
// value (`*` / `,`-list) and decodes escapes for App Configuration while leaving
// other providers untouched, and effectiveParamScope overrides the namespace on
// a resolved store scope without mutating the shared read scope.
func TestParamNamespaceHelpers(t *testing.T) {
	t.Parallel()

	appConfigScope := func() provider.Scope {
		return provider.Scope{Provider: provider.ProviderAzure, StoreName: "store"}
	}

	t.Run("validate accepts a concrete namespace", func(t *testing.T) {
		t.Parallel()

		app := &App{ctx: t.Context(), scope: appConfigScope()}
		ns, err := app.validateParamNamespace("dev")
		require.NoError(t, err)
		assert.Equal(t, "dev", ns)
	})

	t.Run("validate decodes an escaped literal", func(t *testing.T) {
		t.Parallel()

		app := &App{ctx: t.Context(), scope: appConfigScope()}
		ns, err := app.validateParamNamespace(`a\*b`)
		require.NoError(t, err)
		assert.Equal(t, "a*b", ns)
	})

	t.Run("validate rejects filter values", func(t *testing.T) {
		t.Parallel()

		for _, ns := range []string{"*", "dev,prd", "dev*"} {
			app := &App{ctx: t.Context(), scope: appConfigScope()}
			_, err := app.validateParamNamespace(ns)
			require.Error(t, err, "namespace %q names all/multiple and must be rejected", ns)
		}
	})

	t.Run("validate is a no-op for non-App-Config scopes", func(t *testing.T) {
		t.Parallel()

		app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}
		// Even a filter-shaped value passes through untouched: AWS has no namespace axis.
		ns, err := app.validateParamNamespace("*")
		require.NoError(t, err)
		assert.Equal(t, "*", ns)
	})

	t.Run("effectiveParamScope overrides the namespace without mutating the read scope", func(t *testing.T) {
		t.Parallel()

		app := &App{ctx: t.Context(), scope: appConfigScope()}
		eff := app.effectiveParamScope("dev")
		assert.Equal(t, "dev", eff.AppConfigNamespace)
		assert.Empty(t, app.currentScope().AppConfigNamespace, "the shared read scope must be untouched")
	})

	t.Run("effectiveParamScope leaves non-App-Config scopes alone", func(t *testing.T) {
		t.Parallel()

		app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}
		eff := app.effectiveParamScope("dev")
		assert.Empty(t, eff.AppConfigNamespace)
	})
}
