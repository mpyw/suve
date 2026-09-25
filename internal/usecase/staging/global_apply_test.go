// The all-service apply tests belong with the apply use case.
//declscope:namespace apply

package staging_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	usecasestaging "github.com/mpyw/suve/internal/usecase/staging"
)

// newGlobalApplySecretStrategy is a secret-service apply strategy.
func newGlobalApplySecretStrategy() *mockApplyStrategy {
	s := newMockApplyStrategy()
	s.mockServiceStrategy = newSecretStrategy()

	return s
}

// TestGlobalApplyUseCase_ConflictBlocksEveryService verifies a conflict in one
// service rejects the whole apply: nothing is applied in any service.
func TestGlobalApplyUseCase_ConflictBlocksEveryService(t *testing.T) {
	t.Parallel()

	base := time.Now().Add(-time.Hour)
	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate, Value: new("v"), StagedAt: time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate, Value: new("s"), StagedAt: time.Now(), BaseModifiedAt: &base,
	}))

	uc := &usecasestaging.GlobalApplyUseCase{Services: []*usecasestaging.ApplyUseCase{
		{Strategy: newMockApplyStrategy(), Store: store},
		{Strategy: newGlobalApplySecretStrategy(), Store: store},
	}}

	output, err := uc.Execute(t.Context(), usecasestaging.GlobalApplyInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apply rejected: 1 conflict(s) detected")
	assert.Equal(t, []usecasestaging.GlobalApplyConflict{
		{ServiceName: "Secrets Manager", Key: staging.EntryKey{Name: "my-secret"}},
	}, output.Conflicts)
	assert.Empty(t, output.Services)

	// The conflict-free param stays staged too.
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	require.NoError(t, err)
}

// TestGlobalApplyUseCase_SkipsEmptyServicesAndCounts verifies services with
// nothing staged produce no output, and failures are counted across services.
func TestGlobalApplyUseCase_SkipsEmptyServicesAndCounts(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/ok"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("v"), StagedAt: time.Now(),
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/bad"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("v"), StagedAt: time.Now(),
	}))

	param := newMockApplyStrategy()
	param.lastModified["/ok"] = time.Time{}
	param.lastModified["/bad"] = time.Time{}
	param.applyErrors["/bad"] = errors.New("denied")

	uc := &usecasestaging.GlobalApplyUseCase{Services: []*usecasestaging.ApplyUseCase{
		{Strategy: param, Store: store},
		{Strategy: newGlobalApplySecretStrategy(), Store: store},
	}}

	output, err := uc.Execute(t.Context(), usecasestaging.GlobalApplyInput{})
	require.EqualError(t, err, "applied 1, failed 1")
	require.Len(t, output.Services, 1, "the secret service had nothing staged")
	assert.Equal(t, "Parameter Store", output.Services[0].ServiceName)
	assert.Equal(t, 1, output.Succeeded)
	assert.Equal(t, 1, output.Failed)
}

// TestGlobalApplyUseCase_ProbeErrorBlocksEveryService covers #989: a conflict
// probe that fails (other than not-found) in one service rejects the whole
// apply with the probe error, and nothing is applied in any service.
func TestGlobalApplyUseCase_ProbeErrorBlocksEveryService(t *testing.T) {
	t.Parallel()

	base := time.Now().Add(-time.Hour)
	store := testutil.NewMockStore()
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate, Value: new("v"), StagedAt: time.Now(), BaseModifiedAt: &base,
	}))
	require.NoError(t, store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationDelete, StagedAt: time.Now(), BaseModifiedAt: &base,
	}))

	param := newMockApplyStrategy()
	param.lastModified["/app/config"] = base
	secret := newGlobalApplySecretStrategy()
	secret.fetchModifiedErr = errors.New("ThrottlingException: rate exceeded")

	uc := &usecasestaging.GlobalApplyUseCase{Services: []*usecasestaging.ApplyUseCase{
		{Strategy: param, Store: store},
		{Strategy: secret, Store: store},
	}}

	output, err := uc.Execute(t.Context(), usecasestaging.GlobalApplyInput{})
	require.EqualError(t, err, "apply rejected: conflict check failed (ignore conflicts to skip it): "+
		"Secrets Manager: cannot check my-secret for conflicts: ThrottlingException: rate exceeded")
	assert.Nil(t, output)

	for svc, name := range map[staging.Service]string{staging.ServiceParam: "/app/config", staging.ServiceSecret: "my-secret"} {
		_, err = store.GetEntry(t.Context(), svc, staging.EntryKey{Name: name})
		require.NoError(t, err, "%s must stay staged", name)
	}

	// Ignoring conflicts skips the probe entirely, so the apply goes through.
	output, err = uc.Execute(t.Context(), usecasestaging.GlobalApplyInput{IgnoreConflicts: true})
	require.NoError(t, err)
	assert.Equal(t, 2, output.Succeeded)
}
