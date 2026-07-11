package diff_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stagediff "github.com/mpyw/suve/internal/cli/commands/aws/stage/diff"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// TestRun_DiffsEntriesUnderTheirNamespace guards the per-namespace diff path: the
// SAME key staged under two namespaces must be diffed against the CURRENT value
// fetched under EACH entry's own namespace. The dev entry's strategy reports a
// different current value ("cur-b"), so if namespace threading were broken the
// dev entry would diff against the null-namespace current ("cur-a") and "cur-b"
// would never appear.
func TestRun_DiffsEntriesUnderTheirNamespace(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := testutil.NewMockStore()
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "k"}, staging.Entry{
		Operation: staging.OperationUpdate, Value: lo.ToPtr("new-a"), StagedAt: time.Now(),
	}))
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "k", Namespace: "dev"}, staging.Entry{
		Operation: staging.OperationUpdate, Value: lo.ToPtr("new-b"), StagedAt: time.Now(),
	}))

	// Current value differs per namespace, so a mis-routed fetch is observable.
	nsCurrent := map[string]string{"": "cur-a", "dev": "cur-b"}

	entries, _ := st.ListEntries(ctx, staging.ServiceParam)
	svc := stagediff.ServiceStrategy{
		Service:  staging.ServiceParam,
		Store:    st,
		Strategy: staging.NewAWSParamStrategy(storeReturning("cur-a", "1")),
		StrategyFor: func(ns string) (staging.DiffStrategy, error) {
			return staging.NewAWSParamStrategy(storeReturning(nsCurrent[ns], "1")), nil
		},
		Entries: entries[staging.ServiceParam],
	}

	var buf bytes.Buffer

	r := &stagediff.Runner{
		Services:      []stagediff.ServiceStrategy{svc},
		ProviderLabel: "Azure",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	require.NoError(t, r.Run(ctx, stagediff.Options{}))

	out := buf.String()
	assert.Contains(t, out, "cur-a")
	assert.Contains(t, out, "new-a")
	assert.Contains(t, out, "cur-b", "the dev entry must be diffed against the value fetched under the dev namespace")
	assert.Contains(t, out, "new-b")
	assert.Contains(t, out, "[dev]", "the dev-namespaced entry must be labelled with its namespace")
}
