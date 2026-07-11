package apply

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
// + store-error path that used to live inline in runAction and was therefore
// unreachable from the (external) Runner tests. The store resolver is injected so
// these run without touching disk or a real provider.

// notConfiguredResolver mimics an Azure scope resolver whose resource is not
// named (e.g. no --vault-name): the service must be skipped.
func notConfiguredResolver(_ context.Context) (staging.ResolvedScope, error) {
	return staging.ResolvedScope{}, fmt.Errorf("%w: no resource", staging.ErrServiceNotConfigured)
}

// targetResolver returns a scope whose Target is the given string, so the
// injected working-store resolver can map it back to a specific mock store.
func targetResolver(target string) staging.ScopeResolver {
	return func(_ context.Context) (staging.ResolvedScope, error) {
		return staging.ResolvedScope{Target: target}, nil
	}
}

// resolveFrom builds a workingStoreResolver that runs the spec's ScopeResolver
// (so skip/error semantics match WorkingStore) and, on success, returns the mock
// store registered under the resolved Target.
func resolveFrom(stores map[string]store.ReadWriteOperator) workingStoreResolver {
	return func(ctx context.Context, resolver staging.ScopeResolver) (store.ReadWriteOperator, staging.ResolvedScope, error) {
		rs, err := resolver(ctx)
		if err != nil {
			return nil, staging.ResolvedScope{}, err
		}

		return stores[rs.Target], rs, nil
	}
}

// nilFactory returns a nil strategy: gatherServices stores it but never calls it,
// so it is enough for the listing/skip paths under test.
func nilFactory(_ context.Context) (staging.FullStrategy, error) {
	return nil, nil //nolint:nilnil // test stub; gatherServices never invokes the strategy
}

func TestGatherServices_SkipsUnconfigured(t *testing.T) {
	t.Parallel()

	cfg := stgcli.GlobalConfig{
		ProviderLabel: "Azure",
		Services: []stgcli.GlobalServiceSpec{
			{Service: staging.ServiceParam, ParserFactory: staging.AWSParamParserFactory, ScopeResolver: notConfiguredResolver},
			{Service: staging.ServiceSecret, ParserFactory: staging.AWSSecretParserFactory, ScopeResolver: notConfiguredResolver},
		},
	}

	svcs, targets, total, err := gatherServices(t.Context(), cfg, resolveFrom(nil))
	require.NoError(t, err)
	assert.Empty(t, svcs, "an unconfigured service holds no staged state and must be skipped")
	assert.Empty(t, targets)
	assert.Zero(t, total)
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

	svcs, _, _, err := gatherServices(t.Context(), cfg, resolveFrom(nil))
	require.ErrorIs(t, err, wantErr, "a non-sentinel resolver error must not be swallowed by the skip path")
	assert.Empty(t, svcs)
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

	svcs, _, _, err := gatherServices(t.Context(), cfg, resolveFrom(map[string]store.ReadWriteOperator{"t": st}))
	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, svcs)
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

	svcs, _, _, err := gatherServices(t.Context(), cfg, resolveFrom(map[string]store.ReadWriteOperator{"t": st}))
	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, svcs)
}

// TestGatherServices_PerServiceStores proves each service is listed from and
// bound to its OWN store — the whole Azure premise (App Configuration and Key
// Vault live in separate staging buckets), which the AWS Runner tests (shared
// store) never exercise.
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

	svcs, targets, total, err := gatherServices(t.Context(), cfg, resolveFrom(map[string]store.ReadWriteOperator{
		"store": paramStore,
		"vault": secretStore,
	}))
	require.NoError(t, err)
	require.Len(t, svcs, 2)
	assert.Equal(t, 2, total)
	assert.Equal(t, []string{"store", "vault"}, targets)

	// Each ServiceApply is bound to ITS service's store and entries — no bleed.
	assert.Same(t, paramStore, svcs[0].Store)
	assert.Same(t, secretStore, svcs[1].Store)
	assert.Contains(t, svcs[0].Entries, staging.EntryKey{Name: "app/cfg"})
	assert.NotContains(t, svcs[0].Entries, staging.EntryKey{Name: "kv-secret"})
	assert.Contains(t, svcs[1].Entries, staging.EntryKey{Name: "kv-secret"})
}
