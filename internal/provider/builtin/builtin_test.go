package builtin_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/builtin"
)

func TestNewRegistry_RegistersEveryProvider(t *testing.T) {
	t.Parallel()

	reg := builtin.NewRegistry()

	for _, p := range []provider.Provider{provider.ProviderAWS, provider.ProviderGoogleCloud, provider.ProviderAzure} {
		_, err := reg.Store(t.Context(), provider.Scope{Provider: p}, provider.Kind("bogus"))
		require.ErrorIs(t, err, provider.ErrUnsupportedKind, "provider %q", p)
		require.NotErrorIs(t, err, provider.ErrNoFactory, "provider %q", p)
	}
}

func TestNewRegistry_UnknownProvider(t *testing.T) {
	t.Parallel()

	_, err := builtin.NewRegistry().Store(t.Context(), provider.Scope{Provider: provider.Provider("bogus")}, provider.KindParam)
	require.ErrorIs(t, err, provider.ErrNoFactory)
}
