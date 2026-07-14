//go:build production || dev

package gui

import (
	"context"
	"testing"
	"time"

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

// recordingFactory is a fakeFactory that also captures the scope it was asked to
// build a store for, so a test can assert how a binding threaded the App
// Configuration namespace into the resolved scope (paramStoreForNamespace).
type recordingFactory struct {
	store    provider.Store
	gotScope provider.Scope
}

func (f *recordingFactory) Store(_ context.Context, sc provider.Scope, _ provider.Kind) (provider.Store, error) {
	f.gotScope = sc

	return f.store, nil
}

// installParamStore swaps the package-global registry for one whose sole factory
// serves store for provider p, restoring the original on cleanup. It centralizes
// the registry override the binding tests share.
func installParamStore(t *testing.T, p provider.Provider, factory provider.Factory) {
	t.Helper()

	orig := registry
	registry = provider.NewRegistry()
	registry.Register(p, factory)

	t.Cleanup(func() { registry = orig })
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

// TestParamShow_MapsEntry covers the ParamShow binding: it resolves + gets via
// the param.ShowUseCase and mirrors every field onto the DTO, including
// Type/Secret from the domain value type (SecureString ⇒ secret) and the
// formatted LastModified/tags (#702).
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamShow_MapsEntry(t *testing.T) {
	modified := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	store := &providermock.Store{
		ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{
				Name:        name,
				Value:       "sekret",
				Version:     domain.Version{ID: "3"},
				Type:        domain.ValueTypeSecret,
				Description: "the description",
				Modified:    &modified,
				Tags:        []domain.Tag{{Key: "env", Value: "prod"}},
			}, nil
		},
	}

	installParamStore(t, provider.ProviderAWS, fakeFactory{store: store})

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	res, err := app.ParamShow("/my/secure", "")
	require.NoError(t, err)

	assert.Equal(t, "/my/secure", res.Name)
	assert.Equal(t, "sekret", res.Value)
	assert.Equal(t, int64(3), res.Version)
	assert.Equal(t, paramtype.SecureString, res.Type)
	assert.True(t, res.Secret, "a SecureString param must be flagged secret")
	assert.Equal(t, "the description", res.Description)
	assert.NotEmpty(t, res.LastModified, "a known modification time must be formatted")
	require.Len(t, res.Tags, 1)
	assert.Equal(t, ParamShowTag{Key: "env", Value: "prod"}, res.Tags[0])
}

// TestParamLog_MapsVersions covers the ParamLog binding: it walks the history
// via param.LogUseCase and mirrors each version's Type/Secret and the IsCurrent
// flag (the highest version) onto the DTO.
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamLog_MapsVersions(t *testing.T) {
	store := &providermock.Store{
		HistoryFunc: func(context.Context, string) ([]domain.Version, error) {
			return []domain.Version{{ID: "2"}, {ID: "1"}}, nil
		},
		ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			// Return a secret value so Type/Secret mapping is exercised.
			return &domain.Entry{Name: name, Value: "v", Type: domain.ValueTypeSecret}, nil
		},
	}

	installParamStore(t, provider.ProviderAWS, fakeFactory{store: store})

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	res, err := app.ParamLog("/my/secure", 0, "")
	require.NoError(t, err)
	assert.Equal(t, "/my/secure", res.Name)
	require.Len(t, res.Entries, 2)

	// History is newest-first, so version 2 leads and is the current one.
	assert.Equal(t, int64(2), res.Entries[0].Version)
	assert.True(t, res.Entries[0].IsCurrent, "the highest version is current")
	assert.Equal(t, paramtype.SecureString, res.Entries[0].Type)
	assert.True(t, res.Entries[0].Secret, "a SecureString version must be flagged secret")

	assert.Equal(t, int64(1), res.Entries[1].Version)
	assert.False(t, res.Entries[1].IsCurrent, "an older version is not current")
}

// TestParamDiff_SetsSecretMaskForSecureString is the risk branch (#702): the
// diff DTO's Secret flag must be true when EITHER side is a SecureString, so the
// diff view masks both sides; it stays false when both sides are plaintext.
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamDiff_SetsSecretMaskForSecureString(t *testing.T) {
	newStore := func(t1, t2 domain.ValueType) *providermock.Store {
		return &providermock.Store{
			ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
				return provider.VersionRef{}, nil
			},
			GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
				if name == "/a" {
					return &domain.Entry{Name: name, Value: "old", Type: t1}, nil
				}

				return &domain.Entry{Name: name, Value: "new", Type: t2}, nil
			},
		}
	}

	t.Run("secure side masks both", func(t *testing.T) {
		installParamStore(t, provider.ProviderAWS,
			fakeFactory{store: newStore(domain.ValueTypePlaintext, domain.ValueTypeSecret)})

		app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

		res, err := app.ParamDiff("/a", "/b", "")
		require.NoError(t, err)
		assert.Equal(t, "/a", res.OldName)
		assert.Equal(t, "/b", res.NewName)
		assert.Equal(t, "old", res.OldValue)
		assert.Equal(t, "new", res.NewValue)
		assert.True(t, res.Secret, "a SecureString on either side must set the mask flag")
	})

	t.Run("plaintext both sides is not masked", func(t *testing.T) {
		installParamStore(t, provider.ProviderAWS,
			fakeFactory{store: newStore(domain.ValueTypePlaintext, domain.ValueTypePlaintext)})

		app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

		res, err := app.ParamDiff("/a", "/b", "")
		require.NoError(t, err)
		assert.False(t, res.Secret, "two plaintext params must not set the mask flag")
	})
}

// TestParamDelete covers the ParamDelete binding: it drives param.DeleteUseCase
// and returns the deleted name.
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamDelete(t *testing.T) {
	var gotName string

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
			gotName = name

			return nil
		},
	}

	installParamStore(t, provider.ProviderAWS, fakeFactory{store: store})

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	res, err := app.ParamDelete("/my/param", "")
	require.NoError(t, err)
	assert.Equal(t, "/my/param", res.Name)
	assert.Equal(t, "/my/param", gotName, "the delete must reach the provider writer")
}

// TestParamAddTag covers the ParamAddTag binding: it forwards a single key/value
// as the Add map of param.TagUseCase.
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamAddTag(t *testing.T) {
	var gotName string

	var gotAdd map[string]string

	store := &providermock.Store{
		TagFunc: func(_ context.Context, name string, add map[string]string) error {
			gotName = name
			gotAdd = add

			return nil
		},
	}

	installParamStore(t, provider.ProviderAWS, fakeFactory{store: store})

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	require.NoError(t, app.ParamAddTag("/my/param", "env", "prod", ""))
	assert.Equal(t, "/my/param", gotName)
	assert.Equal(t, map[string]string{"env": "prod"}, gotAdd)
}

// TestParamRemoveTag covers the ParamRemoveTag binding: it forwards the key as
// the Remove list of param.TagUseCase.
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamRemoveTag(t *testing.T) {
	var gotName string

	var gotKeys []string

	store := &providermock.Store{
		UntagFunc: func(_ context.Context, name string, keys []string) error {
			gotName = name
			gotKeys = keys

			return nil
		},
	}

	installParamStore(t, provider.ProviderAWS, fakeFactory{store: store})

	app := &App{ctx: t.Context(), scope: provider.Scope{Provider: provider.ProviderAWS}}

	require.NoError(t, app.ParamRemoveTag("/my/param", "env", ""))
	assert.Equal(t, "/my/param", gotName)
	assert.Equal(t, []string{"env"}, gotKeys)
}

// TestParamShow_AppConfigNamespaceThreadsThroughStore covers the App
// Configuration namespace axis shared by every param binding: the namespace
// argument must be threaded through paramStoreForNamespace onto the resolved
// store's scope (the label axis), so a namespaced setting is read under its own
// namespace rather than the shared read scope's. It also asserts the App
// Configuration value maps to a non-secret "String" via paramtype.Display.
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamShow_AppConfigNamespaceThreadsThroughStore(t *testing.T) {
	store := &providermock.Store{
		ResolveFunc: func(context.Context, string, string) (provider.VersionRef, error) {
			return provider.VersionRef{}, nil
		},
		GetFunc: func(_ context.Context, name string, _ provider.VersionRef) (*domain.Entry, error) {
			// App Configuration values are always plaintext.
			return &domain.Entry{Name: name, Value: "v", Type: domain.ValueTypePlaintext}, nil
		},
	}

	factory := &recordingFactory{store: store}
	installParamStore(t, provider.ProviderAzure, factory)

	app := &App{
		ctx:   t.Context(),
		scope: provider.Scope{Provider: provider.ProviderAzure, StoreName: "store"},
	}

	res, err := app.ParamShow("app/config", "dev")
	require.NoError(t, err)

	assert.Equal(t, "dev", factory.gotScope.AppConfigNamespace,
		"the entry's namespace must be threaded onto the resolved store scope")
	assert.Equal(t, "store", factory.gotScope.StoreName, "the read scope's store name is preserved")

	assert.False(t, res.Secret, "App Configuration values are never secrets")
	assert.Equal(t, paramtype.Display(domain.ValueTypePlaintext), res.Type)
	assert.Equal(t, paramtype.String, res.Type)
}
