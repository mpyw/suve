// White-box tests of baseline.go.
//declscope:namespace baseline

package staging

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

func TestEditUseCase_Baseline_NotStaged_FetchesFromAWS(t *testing.T) {
	t.Parallel()

	// When nothing is staged, should fetch from AWS
	store := testutil.NewMockStore()

	strategy := doublesNewMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Now(),
	}

	uc := &EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Baseline(t.Context(), BaselineInput{Key: staging.EntryKey{Name: "/app/config"}})
	require.NoError(t, err)
	assert.Equal(t, "aws-value", output.Value)
	assert.False(t, output.isStagedEdit)
}

func TestEditUseCase_Baseline_Staged(t *testing.T) {
	t.Parallel()

	// When an entry is staged, should return the staged value
	store := testutil.NewMockStore()

	// Pre-stage an UPDATE operation
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("staged-value"),
		StagedAt:  time.Now(),
	}))

	strategy := doublesNewMockEditStrategy()
	strategy.fetchResult = &staging.EditFetchResult{
		Value:        "aws-value",
		LastModified: time.Now(),
	}

	uc := &EditUseCase{
		Strategy: strategy,
		Store:    store,
	}

	output, err := uc.Baseline(t.Context(), BaselineInput{Key: staging.EntryKey{Name: "/app/config"}})
	require.NoError(t, err)
	assert.Equal(t, "staged-value", output.Value)
	assert.True(t, output.isStagedEdit)
}
