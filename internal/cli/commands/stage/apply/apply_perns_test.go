package apply_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/stage/apply"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// TestRun_AppliesEntriesUnderTheirNamespace is the core guard for the
// per-namespace path: the SAME key staged under two namespaces must apply through
// the strategy scoped to EACH entry's own namespace (App Configuration keeps all
// namespaces in one staging store). Without correct threading, both entries would
// go through one namespace's strategy.
func TestRun_AppliesEntriesUnderTheirNamespace(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := testutil.NewMockStore()
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "k"}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("va"), StagedAt: time.Now(),
	}))
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "k", Namespace: "dev"}, staging.Entry{
		Operation: staging.OperationCreate, Value: lo.ToPtr("vb"), StagedAt: time.Now(),
	}))

	var mu sync.Mutex

	appliedByNS := map[string]string{}

	// StrategyFor returns a strategy bound to the requested namespace; its Apply
	// records which value it received, so we can prove each entry was routed to
	// its own namespace's strategy.
	strategyFor := func(ns string) (staging.ApplyStrategy, error) {
		s := newParamStrategy()
		s.applyFunc = func(_ context.Context, _ string, entry staging.Entry) error {
			mu.Lock()
			defer mu.Unlock()

			appliedByNS[ns] = lo.FromPtr(entry.Value)

			return nil
		}

		return s, nil
	}

	entries, _ := st.ListEntries(ctx, staging.ServiceParam)
	svc := apply.ServiceApply{
		Service:     staging.ServiceParam,
		Store:       st,
		Strategy:    newParamStrategy(),
		StrategyFor: strategyFor,
		Entries:     entries[staging.ServiceParam],
	}

	r := &apply.Runner{
		Services:        []apply.ServiceApply{svc},
		ProviderLabel:   "Azure",
		Stdout:          &bytes.Buffer{},
		Stderr:          &bytes.Buffer{},
		IgnoreConflicts: true, // this test is about namespace routing, not conflicts
	}

	require.NoError(t, r.Run(ctx))

	// Each entry reached the strategy scoped to ITS namespace.
	assert.Equal(t, map[string]string{"": "va", "dev": "vb"}, appliedByNS)

	// Both entries were unstaged under their own (name, namespace) key.
	remaining, _ := st.ListEntries(ctx, staging.ServiceParam)
	assert.Empty(t, remaining[staging.ServiceParam])
}
