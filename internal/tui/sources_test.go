//nolint:testpackage // white-box: exercises sourceFactory.stagingScope and the memoized AWS identity seam
package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/aws/infra"
)

// TestStagingScope_AWSIdentityMemoized proves the AWS caller identity that keys
// the staging scope is resolved once per launch, not on every staging-store
// access: repeated stagingScope calls across both service kinds trigger exactly
// one GetCallerIdentity (mocked here), and every call returns the same scope key.
func TestStagingScope_AWSIdentityMemoized(t *testing.T) {
	t.Parallel()

	var calls int

	f := newSourceFactory(t.Context(), provider.Scope{Provider: provider.ProviderAWS})
	f.resolveAWSIdentity = func(context.Context) (*infra.AWSIdentity, error) {
		calls++

		return &infra.AWSIdentity{AccountID: "123456789012", Region: "us-east-1"}, nil
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
	f.resolveAWSIdentity = func(context.Context) (*infra.AWSIdentity, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("transient STS failure")
		}

		return &infra.AWSIdentity{AccountID: "123456789012", Region: "us-east-1"}, nil
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
	f.resolveAWSIdentity = func(context.Context) (*infra.AWSIdentity, error) {
		calls++

		return &infra.AWSIdentity{AccountID: "123456789012", Region: "us-east-1"}, nil
	}

	got, err := f.stagingScope(provider.KindParam)
	require.NoError(t, err)
	assert.Equal(t, scope.Key(), got.Key())
	assert.Equal(t, 0, calls, "a pre-hydrated AWS scope must not call STS")
}
