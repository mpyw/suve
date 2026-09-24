// These are global.go's in-package tests: gatherGlobalServices and the
// all-service use-case builders take an injected store resolver, so they run
// without touching disk or a real provider.
//declscope:namespace global

package cli

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// globalNotConfigured mimics an Azure scope resolver whose resource is not
// named (e.g. no --vault-name): the service must be skipped.
func globalNotConfigured(_ context.Context) (staging.ResolvedScope, error) {
	return staging.ResolvedScope{}, fmt.Errorf("%w: no resource", staging.ErrServiceNotConfigured)
}

// globalTarget returns a scope whose Target is the given string, so the
// injected store resolver can map it back to a specific mock store.
func globalTarget(target string) staging.ScopeResolver {
	return func(_ context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Target: target}, nil
	}
}

// globalResolveFrom builds a globalStoreResolver that runs the spec's
// ScopeResolver (so skip/error semantics match WorkingStore) and, on success,
// returns the mock store registered under the resolved Target.
func globalResolveFrom(stores map[string]store.ReadWriteOperator) globalStoreResolver {
	return func(ctx context.Context, resolver staging.ScopeResolver) (store.ReadWriteOperator, staging.ResolvedScope, error) {
		rs, err := resolver(ctx)
		if err != nil {
			return nil, staging.ResolvedScope{}, err
		}

		return stores[rs.Target], rs, nil
	}
}

// globalNilFactory returns a nil strategy: the builders store it but never call
// it, so it is enough for the listing/skip paths under test.
func globalNilFactory(_ context.Context) (staging.FullStrategy, error) {
	return nil, nil //nolint:nilnil // test stub; the builders never invoke the strategy
}

// globalSplitStores stages one param in its own store and one secret in
// another, returning the two per-service specs and their resolver (the Azure
// premise: App Configuration and Key Vault live in separate staging buckets).
func globalSplitStores(t *testing.T) (specs []GlobalServiceSpec, resolve globalStoreResolver, paramStore, secretStore *testutil.MockStore) {
	t.Helper()

	paramStore = testutil.NewMockStore()
	require.NoError(t, paramStore.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "app/cfg"}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("pv"), StagedAt: time.Now(),
	}))

	secretStore = testutil.NewMockStore()
	require.NoError(t, secretStore.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "kv-secret"}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("sv"), StagedAt: time.Now(),
	}))

	specs = []GlobalServiceSpec{
		{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: globalTarget("store"), Factory: globalNilFactory},
		{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory, ScopeResolver: globalTarget("vault"), Factory: globalNilFactory},
	}
	resolve = globalResolveFrom(map[string]store.ReadWriteOperator{"store": paramStore, "vault": secretStore})

	return specs, resolve, paramStore, secretStore
}

func TestGatherGlobalServices_SkipsUnconfigured(t *testing.T) {
	t.Parallel()

	specs := []GlobalServiceSpec{
		{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: globalNotConfigured},
		{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory, ScopeResolver: globalNotConfigured},
	}

	svcs, err := gatherGlobalServices(t.Context(), specs, globalResolveFrom(nil))
	require.NoError(t, err)
	assert.Empty(t, svcs, "an unconfigured service holds no staged state and must be skipped")
}

func TestGatherGlobalServices_ResolverErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	specs := []GlobalServiceSpec{
		{
			Service:       staging.ServiceParam,
			ParserFactory: staging.AWSParamParserFactory,
			ScopeResolver: func(_ context.Context) (staging.ResolvedScope, error) {
				return staging.ResolvedScope{}, wantErr
			},
		},
	}

	svcs, err := gatherGlobalServices(t.Context(), specs, globalResolveFrom(nil))
	require.ErrorIs(t, err, wantErr, "a non-sentinel resolver error must not be swallowed by the skip path")
	assert.Empty(t, svcs)
}

func TestGatherGlobalServices_ListErrorsPropagate(t *testing.T) {
	t.Parallel()

	for name, setErr := range map[string]func(*testutil.MockStore, error){
		"entries": func(st *testutil.MockStore, err error) { st.ListEntriesErr = err },
		"tags":    func(st *testutil.MockStore, err error) { st.ListTagsErr = err },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			wantErr := errors.New("list boom")
			st := testutil.NewMockStore()
			setErr(st, wantErr)

			specs := []GlobalServiceSpec{
				{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: globalTarget("t")},
			}

			svcs, err := gatherGlobalServices(t.Context(), specs, globalResolveFrom(map[string]store.ReadWriteOperator{"t": st}))
			require.ErrorIs(t, err, wantErr)
			assert.Empty(t, svcs)
		})
	}
}

// TestGlobalApplyUseCase_PerServiceStores proves each service's apply use case
// is bound to its OWN store, and that the confirmation targets and total come
// from every staged service.
func TestGlobalApplyUseCase_PerServiceStores(t *testing.T) {
	t.Parallel()

	specs, resolve, paramStore, secretStore := globalSplitStores(t)

	useCase, targets, total, err := globalApplyUseCase(t.Context(), GlobalConfig{Services: specs}, resolve)
	require.NoError(t, err)
	require.Len(t, useCase.Services, 2)
	assert.Equal(t, 2, total)
	assert.Equal(t, []string{"store", "vault"}, targets)
	assert.Same(t, paramStore, useCase.Services[0].Store)
	assert.Same(t, secretStore, useCase.Services[1].Store)
}

// TestGlobalDiffUseCases_PerServiceStores proves each service is diffed from
// its OWN store, labelled with the provider.
func TestGlobalDiffUseCases_PerServiceStores(t *testing.T) {
	t.Parallel()

	specs, resolve, paramStore, secretStore := globalSplitStores(t)

	useCases, err := globalDiffUseCases(t.Context(), GlobalConfig{ProviderLabel: "Azure", Services: specs}, resolve)
	require.NoError(t, err)
	require.Len(t, useCases, 2)
	assert.Same(t, paramStore, useCases[0].Store)
	assert.Same(t, secretStore, useCases[1].Store)
	assert.Equal(t, "Azure", useCases[0].RemoteLabel)
}

// TestGlobalDiffUseCases_FactoryError verifies a provider client failure names
// the service it was initializing.
func TestGlobalDiffUseCases_FactoryError(t *testing.T) {
	t.Parallel()

	specs, resolve, _, _ := globalSplitStores(t)
	wantErr := errors.New("no credentials")
	specs[0].Factory = func(context.Context) (staging.FullStrategy, error) { return nil, wantErr }

	_, err := globalDiffUseCases(t.Context(), GlobalConfig{Services: specs}, resolve)
	require.ErrorIs(t, err, wantErr)
	assert.Contains(t, err.Error(), "failed to initialize SSM Parameter Store client")
}
