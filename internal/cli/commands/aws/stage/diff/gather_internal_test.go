package diff

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
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// White-box tests for gatherServices — the per-service listing + skip-unconfigured
// + store-error path extracted from runAction so it is unit-reachable. The store
// resolver is injected so these run without disk or a real provider.

func notConfiguredResolver(_ context.Context) (staging.ResolvedScope, error) {
	return staging.ResolvedScope{}, fmt.Errorf("%w: no resource", staging.ErrServiceNotConfigured)
}

func targetResolver(target string) staging.ScopeResolver {
	return func(_ context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Target: target}, nil
	}
}

func resolveFrom(stores map[string]store.ReadWriteOperator) workingStoreResolver {
	return func(ctx context.Context, resolver staging.ScopeResolver) (store.ReadWriteOperator, staging.ResolvedScope, error) {
		rs, err := resolver(ctx)
		if err != nil {
			return nil, staging.ResolvedScope{}, err
		}

		return stores[rs.Target], rs, nil
	}
}

func nilFactory(_ context.Context) (staging.FullStrategy, error) {
	return nil, nil //nolint:nilnil // test stub; gatherServices never invokes the strategy
}

func TestGatherServices_SkipsUnconfigured(t *testing.T) {
	t.Parallel()

	cfg := stgcli.GlobalConfig{
		Services: []stgcli.GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: notConfiguredResolver},
			{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory, ScopeResolver: notConfiguredResolver},
		},
	}

	svcs, err := gatherServices(t.Context(), cfg, resolveFrom(nil))
	require.NoError(t, err)
	assert.Empty(t, svcs, "an unconfigured service holds no staged state and must be skipped")
}

func TestGatherServices_ResolverErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	cfg := stgcli.GlobalConfig{
		Services: []stgcli.GlobalServiceSpec{
			{
				Service:       staging.ServiceParam,
				ParserFactory: staging.AWSParamParserFactory,
				ScopeResolver: func(_ context.Context) (staging.ResolvedScope, error) {
					return staging.ResolvedScope{}, wantErr
				},
			},
		},
	}

	_, err := gatherServices(t.Context(), cfg, resolveFrom(nil))
	require.ErrorIs(t, err, wantErr, "a non-sentinel resolver error must not be swallowed by the skip path")
}

func TestGatherServices_ListEntriesErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("list entries boom")
	st := testutil.NewMockStore()
	st.ListEntriesErr = wantErr

	cfg := stgcli.GlobalConfig{
		Services: []stgcli.GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: targetResolver("t")},
		},
	}

	_, err := gatherServices(t.Context(), cfg, resolveFrom(map[string]store.ReadWriteOperator{"t": st}))
	require.ErrorIs(t, err, wantErr)
}

func TestGatherServices_ListTagsErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("list tags boom")
	st := testutil.NewMockStore()
	st.ListTagsErr = wantErr

	cfg := stgcli.GlobalConfig{
		Services: []stgcli.GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: targetResolver("t")},
		},
	}

	_, err := gatherServices(t.Context(), cfg, resolveFrom(map[string]store.ReadWriteOperator{"t": st}))
	require.ErrorIs(t, err, wantErr)
}

// TestGatherServices_PerServiceStores proves each service is diffed from its OWN
// store (App Configuration and Key Vault live in separate staging buckets).
func TestGatherServices_PerServiceStores(t *testing.T) {
	t.Parallel()

	paramStore := testutil.NewMockStore()
	require.NoError(t, paramStore.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "app/cfg"}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("pv"), StagedAt: time.Now(),
	}))

	secretStore := testutil.NewMockStore()
	require.NoError(t, secretStore.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "kv-secret"}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("sv"), StagedAt: time.Now(),
	}))

	cfg := stgcli.GlobalConfig{
		Services: []stgcli.GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: targetResolver("store"), Factory: nilFactory},
			{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory, ScopeResolver: targetResolver("vault"), Factory: nilFactory},
		},
	}

	svcs, err := gatherServices(t.Context(), cfg, resolveFrom(map[string]store.ReadWriteOperator{
		"store": paramStore,
		"vault": secretStore,
	}))
	require.NoError(t, err)
	require.Len(t, svcs, 2)

	assert.Same(t, paramStore, svcs[0].Store)
	assert.Same(t, secretStore, svcs[1].Store)
	assert.Contains(t, svcs[0].Entries, staging.EntryKey{Name: "app/cfg"})
	assert.NotContains(t, svcs[0].Entries, staging.EntryKey{Name: "kv-secret"})
	assert.Contains(t, svcs[1].Entries, staging.EntryKey{Name: "kv-secret"})
}
