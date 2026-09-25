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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// globalNotConfigured mimics an Azure scope resolver whose resource is not
// named (e.g. no --vault-name): the service must be skipped.
func globalNotConfigured(_ context.Context) (staging.ResolvedScope, error) {
	return staging.ResolvedScope{}, fmt.Errorf("%w: no resource", staging.ErrServiceNotConfigured)
}

// globalTarget returns a scope whose Target renders as the given string, so the
// injected store resolver can map it back to a specific mock store.
func globalTarget(target string) staging.ScopeResolver {
	return func(_ context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Target: provider.Target{Segments: []provider.TargetSegment{{Label: "at", Value: target}}}}, nil
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

		return stores[rs.Target.Segments[0].Value], rs, nil
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
		Operation: staging.OperationCreate, Value: new("pv"), StagedAt: time.Now(),
	}))

	secretStore = testutil.NewMockStore()
	require.NoError(t, secretStore.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "kv-secret"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("sv"), StagedAt: time.Now(),
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
	assert.Equal(t, []string{"at store", "at vault"}, targets)
	assert.Same(t, paramStore, useCase.Services[0].Store)
	assert.Same(t, secretStore, useCase.Services[1].Store)
}

// TestGlobalApplyUseCase_SharedScope is the #1004 regression: AWS param and
// secret share one scope resolver, so the all-service apply resolves it once
// (one STS call) and lists the shared target once in the confirmation.
func TestGlobalApplyUseCase_SharedScope(t *testing.T) {
	t.Parallel()

	st := testutil.NewMockStore()
	require.NoError(t, st.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/p"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("pv"), StagedAt: time.Now(),
	}))
	require.NoError(t, st.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "s"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("sv"), StagedAt: time.Now(),
	}))

	calls := 0
	shared := SharedScopeResolver(func(ctx context.Context) (staging.ResolvedScope, error) {
		calls++

		return globalTarget("profile p · account 1 · region r")(ctx)
	})
	specs := []GlobalServiceSpec{
		{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: shared, Factory: globalNilFactory},
		{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory, ScopeResolver: shared, Factory: globalNilFactory},
	}
	resolve := globalResolveFrom(map[string]store.ReadWriteOperator{"profile p · account 1 · region r": st})

	useCase, targets, total, err := globalApplyUseCase(t.Context(), GlobalConfig{Services: specs}, resolve)
	require.NoError(t, err)
	require.Len(t, useCase.Services, 2)
	assert.Equal(t, 2, total)
	assert.Equal(t, []string{"at profile p · account 1 · region r"}, targets)
	assert.Equal(t, 1, calls, "the shared resolver must run once per command")

	// Each command gets a fresh memo, and outside one the resolver calls through.
	_, err = gatherGlobalServices(t.Context(), specs, resolve)
	require.NoError(t, err)
	assert.Equal(t, 2, calls)

	_, err = shared(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

// TestSharedScopeResolver_MemoizesError verifies a failed shared resolution is
// reused too, so the second service does not retry the lookup.
func TestSharedScopeResolver_MemoizesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("sts boom")
	calls := 0
	shared := SharedScopeResolver(func(context.Context) (staging.ResolvedScope, error) {
		calls++

		return staging.ResolvedScope{}, wantErr
	})

	ctx := withSharedScopeMemo(t.Context())

	_, err := shared(ctx)
	require.ErrorIs(t, err, wantErr)

	_, err = shared(ctx)
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, calls)
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
	assert.Contains(t, err.Error(), "failed to initialize Parameter Store client")
}
