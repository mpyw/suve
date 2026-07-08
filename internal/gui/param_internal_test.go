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
