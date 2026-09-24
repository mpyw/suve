//declscope:namespace app
//
// White-box tests of sourceFactory, the App's source seam.
//
// The top level of internal/tui holds one device — the app shell — and
// every separable unit lives in a subpackage, so these files share
// app.go's namespace rather than each claiming their own.

//nolint:testpackage // white-box: exercises sourceFactory.stagingScope and the memoized AWS identity seam
package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/staging"
)

// TestStagingScope_AWSIdentityMemoized proves the AWS caller identity that keys
// the staging scope is resolved once per launch, not on every staging-store
// access: repeated stagingScope calls across both service kinds trigger exactly
// one GetCallerIdentity (mocked here), and every call returns the same scope key.
func TestStagingScope_AWSIdentityMemoized(t *testing.T) {
	t.Parallel()

	var calls int

	f := newSourceFactory(t.Context(), provider.Scope{Provider: provider.ProviderAWS})
	f.resolveIdentity = func(context.Context) (staging.ResolvedScope, error) {
		calls++

		return staging.ResolvedScope{Scope: provider.AWSScope("123456789012", "us-east-1")}, nil
	}

	want := provider.AWSScope("123456789012", "us-east-1").Key()

	// Every staging-store access resolves the scope; simulate several probes/writes
	// across both param and secret services.
	for _, kind := range []provider.Kind{provider.KindParam, provider.KindSecret, provider.KindParam} {
		for range 3 {
			scope, err := f.stagingScope(kind)
			require.NoError(t, err)
			assert.Equal(t, want, scope.Key())
		}
	}

	assert.Equal(t, 1, calls, "STS GetCallerIdentity should resolve once per launch, not per staging-store access")
}

// TestStagingScope_AWSIdentityErrorNotCached proves a transient STS failure is
// not memoized: the next staging-store access retries and can succeed.
func TestStagingScope_AWSIdentityErrorNotCached(t *testing.T) {
	t.Parallel()

	var calls int

	f := newSourceFactory(t.Context(), provider.Scope{Provider: provider.ProviderAWS})
	f.resolveIdentity = func(context.Context) (staging.ResolvedScope, error) {
		calls++
		if calls == 1 {
			return staging.ResolvedScope{}, errors.New("transient STS failure")
		}

		return staging.ResolvedScope{Scope: provider.AWSScope("123456789012", "us-east-1")}, nil
	}

	_, err := f.stagingScope(provider.KindParam)
	require.Error(t, err)

	scope, err := f.stagingScope(provider.KindParam)
	require.NoError(t, err)
	assert.Equal(t, provider.AWSScope("123456789012", "us-east-1").Key(), scope.Key())

	// Once resolved, it stays memoized: no third resolution.
	_, err = f.stagingScope(provider.KindParam)
	require.NoError(t, err)
	assert.Equal(t, 2, calls, "identity should resolve on retry after a transient failure, then stay memoized")
}

// TestStagingScope_AWSPrehydratedSkipsResolution proves a launch scope that
// already carries account+region never issues an STS call.
func TestStagingScope_AWSPrehydratedSkipsResolution(t *testing.T) {
	t.Parallel()

	var calls int

	scope := provider.AWSScope("999999999999", "eu-west-1")
	f := newSourceFactory(t.Context(), scope)
	f.resolveIdentity = func(context.Context) (staging.ResolvedScope, error) {
		calls++

		return staging.ResolvedScope{Scope: provider.AWSScope("123456789012", "us-east-1")}, nil
	}

	got, err := f.stagingScope(provider.KindParam)
	require.NoError(t, err)
	assert.Equal(t, scope.Key(), got.Key())
	assert.Equal(t, 0, calls, "a pre-hydrated AWS scope must not call STS")
}

// TestStrategyBuilders_PerProvider pins the staging strategy each provider gets,
// and that an unknown provider (or one without the service) is an error rather
// than a silent AWS strategy.
func TestStrategyBuilders_PerProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   provider.Provider
		wantParam  staging.FullStrategy
		wantSecret staging.FullStrategy
	}{
		{"aws", provider.ProviderAWS, &staging.AWSParamStrategy{}, &staging.AWSSecretStrategy{}},
		{"google cloud", provider.ProviderGoogleCloud, nil, &staging.GoogleCloudSecretStrategy{}},
		{"azure", provider.ProviderAzure, &staging.AzureAppConfigParamStrategy{}, &staging.AzureKeyVaultSecretStrategy{}},
		{"unknown", provider.Provider("oracle"), nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newSourceFactory(t.Context(), provider.Scope{Provider: tt.provider})

			param, err := f.strategyBuilder(provider.KindParam)(nil)
			if tt.wantParam == nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tt.wantParam, param)
			}

			secret, err := f.strategyBuilder(provider.KindSecret)(nil)
			if tt.wantSecret == nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tt.wantSecret, secret)
			}
		})
	}
}

// TestParserFor_PerProvider pins the store-less parser each provider gets, and
// that an unknown provider (or one without the service) is an error rather than
// a silent AWS parser.
func TestParserFor_PerProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider provider.Provider
		service  string
		want     staging.Parser
	}{
		{"aws param", provider.ProviderAWS, "param", &staging.AWSParamStrategy{}},
		{"aws secret", provider.ProviderAWS, "secret", &staging.AWSSecretStrategy{}},
		{"google cloud secret", provider.ProviderGoogleCloud, "secret", &staging.GoogleCloudSecretStrategy{}},
		{"azure param", provider.ProviderAzure, "param", &staging.AzureAppConfigParamStrategy{}},
		{"azure secret", provider.ProviderAzure, "secret", &staging.AzureKeyVaultSecretStrategy{}},
		{"google cloud param", provider.ProviderGoogleCloud, "param", nil},
		{"unknown secret", provider.Provider("oracle"), "secret", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parserFor(tt.provider, tt.service)
			if tt.want == nil {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.IsType(t, tt.want, got)
		})
	}
}

// scopeRecordingFactory is a provider.Factory that records the scope it was
// asked to build a store for.
type scopeRecordingFactory struct{ got *provider.Scope }

func (f scopeRecordingFactory) Store(_ context.Context, sc provider.Scope, _ provider.Kind) (provider.Store, error) {
	*f.got = sc

	return &providermock.Store{}, nil
}

// TestParamResolver_NamespaceOnlyForAppConfig pins that the param store
// resolver applies the namespace only to a service with a namespace axis (Azure
// App Configuration) and leaves every other scope alone.
//
//nolint:paralleltest // overrides the package-global registry.
func TestParamResolver_NamespaceOnlyForAppConfig(t *testing.T) {
	orig := registry
	t.Cleanup(func() { registry = orig })

	var got provider.Scope

	registry = provider.NewRegistry()
	registry.Register(provider.ProviderAzure, scopeRecordingFactory{got: &got})
	registry.Register(provider.ProviderAWS, scopeRecordingFactory{got: &got})

	azure := provider.Scope{Provider: provider.ProviderAzure, VaultName: "v", StoreName: "s", AppConfigNamespace: "base"}
	_, err := newSourceFactory(t.Context(), azure).paramResolver()(t.Context(), "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev", got.AppConfigNamespace)

	aws := provider.Scope{Provider: provider.ProviderAWS}
	_, err = newSourceFactory(t.Context(), aws).paramResolver()(t.Context(), "dev")
	require.NoError(t, err)
	assert.Equal(t, aws, got)
}
