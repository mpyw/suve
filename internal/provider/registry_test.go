package provider_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
)

// fakeFactory is a tiny in-test Factory. It reports KindSecret as unsupported
// to exercise the ErrUnsupportedKind path without any real (or AWS) store.
type fakeFactory struct{}

func (fakeFactory) Store(_ context.Context, _ provider.Scope, kind provider.Kind) (provider.Store, error) {
	if kind == provider.KindSecret {
		return nil, provider.ErrUnsupportedKind
	}

	return nil, nil //nolint:nilnil // fake: proves Register->Store dispatch without a real store
}

func TestRegistry_StoreSuccess(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	reg.Register(provider.ProviderAWS, fakeFactory{})

	store, err := reg.Store(t.Context(), provider.Scope{Provider: provider.ProviderAWS}, provider.KindParam)

	require.NoError(t, err)
	assert.Nil(t, store)
}

func TestRegistry_StoreNoFactory(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()

	_, err := reg.Store(t.Context(), provider.Scope{Provider: provider.ProviderAWS}, provider.KindParam)

	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrNoFactory)
}

func TestRegistry_StoreUnsupportedKind(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	reg.Register(provider.ProviderAWS, fakeFactory{})

	_, err := reg.Store(t.Context(), provider.Scope{Provider: provider.ProviderAWS}, provider.KindSecret)

	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrUnsupportedKind)
}

func TestRegistry_RegisterOverwrites(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	reg.Register(provider.ProviderAWS, fakeFactory{})
	reg.Register(provider.ProviderAWS, fakeFactory{})

	_, err := reg.Store(t.Context(), provider.Scope{Provider: provider.ProviderAWS}, provider.KindSecret)

	assert.ErrorIs(t, err, provider.ErrUnsupportedKind)
}
