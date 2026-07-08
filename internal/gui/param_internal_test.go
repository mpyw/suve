//go:build production || dev

package gui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/providermock"
)

// fakeNamespaceLister is a minimal appConfigNamespaceLister for testing the
// Azure App Configuration all-namespaces list path without a real client.
type fakeNamespaceLister struct {
	items []appconfig.KeyNamespace
	err   error
}

func (f *fakeNamespaceLister) ListWithNamespaces(_ context.Context) ([]appconfig.KeyNamespace, error) {
	return f.items, f.err
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
