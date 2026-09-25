// The all-service command tests share one namespace with their fixtures in
// global_test.go.
//declscope:namespace global

package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	stgcli "github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/store"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

// globalApplyStrategy implements staging.ApplyStrategy for testing.
type globalApplyStrategy struct {
	service              staging.Service
	serviceName          string
	itemName             string
	hasDeleteOptions     bool
	applyFunc            func(ctx context.Context, name string, entry staging.Entry) error
	applyTagsFunc        func(ctx context.Context, name string, tagEntry staging.TagEntry) error
	fetchLastModifiedVal time.Time
	fetchLastModifiedErr error
}

func (m *globalApplyStrategy) Service() staging.Service { return m.service }
func (m *globalApplyStrategy) ServiceName() string      { return m.serviceName }
func (m *globalApplyStrategy) ItemName() string         { return m.itemName }
func (m *globalApplyStrategy) HasDeleteOptions() bool   { return m.hasDeleteOptions }

func (m *globalApplyStrategy) Apply(ctx context.Context, name string, entry staging.Entry) error {
	if m.applyFunc != nil {
		return m.applyFunc(ctx, name, entry)
	}

	return nil
}

func (m *globalApplyStrategy) FetchLastModified(_ context.Context, _ string) (time.Time, error) {
	return m.fetchLastModifiedVal, m.fetchLastModifiedErr
}

func (m *globalApplyStrategy) ApplyTags(ctx context.Context, name string, tagEntry staging.TagEntry) error {
	if m.applyTagsFunc != nil {
		return m.applyTagsFunc(ctx, name, tagEntry)
	}

	return nil
}

func newGlobalApplyParamStrategy() *globalApplyStrategy {
	return &globalApplyStrategy{
		service:          staging.ServiceParam,
		serviceName:      "Parameter Store",
		itemName:         "parameter",
		hasDeleteOptions: false,
	}
}

func newGlobalApplySecretStrategy() *globalApplyStrategy {
	return &globalApplyStrategy{
		service:          staging.ServiceSecret,
		serviceName:      "Secrets Manager",
		itemName:         "secret",
		hasDeleteOptions: true,
	}
}

func TestGlobalApplyCommand_Help(t *testing.T) {
	t.Parallel()

	stdout, _, err := runLeafCmd(t, stgcli.NewGlobalApplyCommand(globalAWSConfig()), nil, "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Apply all staged changes")
}

func TestGlobalApply_NoChanges(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase: globalApplyServices(
			globalApplyParam(newGlobalApplyParamStrategy(), store), globalApplySecret(newGlobalApplySecretStrategy(), store),
		),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	// When called with empty store, Run should return without error
	// and produce no output (action handles the warning)
	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestGlobalApply_ApplyBothServices(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage Parameter Store parameter
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("param-value"),
		StagedAt:  time.Now(),
	})

	// Stage Secrets Manager secret
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("secret-value"),
		StagedAt:  time.Now(),
	})

	paramPutCalled := false
	secretPutCalled := false

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyFunc = func(_ context.Context, name string, _ staging.Entry) error {
		paramPutCalled = true

		assert.Equal(t, "/app/config", name)

		return nil
	}

	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyFunc = func(_ context.Context, name string, _ staging.Entry) error {
		secretPutCalled = true

		assert.Equal(t, "my-secret", name)

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store), globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, paramPutCalled)
	assert.True(t, secretPutCalled)
	assert.Contains(t, buf.String(), "Applying Parameter Store...")
	assert.Contains(t, buf.String(), "Applying Secrets Manager...")
	assert.Contains(t, buf.String(), "Parameter Store: Updated /app/config")
	assert.Contains(t, buf.String(), "Secrets Manager: Updated my-secret")

	// Verify both unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestGlobalApply_ApplyParamOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only Parameter Store parameter
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("param-value"),
		StagedAt:  time.Now(),
	})

	paramPutCalled := false
	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		paramPutCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, paramPutCalled)
	assert.Contains(t, buf.String(), "Applying Parameter Store...")
	assert.NotContains(t, buf.String(), "Applying Secrets Manager...")
}

func TestGlobalApply_ApplySecretOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only Secrets Manager secret
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("secret-value"),
		StagedAt:  time.Now(),
	})

	secretPutCalled := false
	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		secretPutCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, secretPutCalled)
	assert.NotContains(t, buf.String(), "Applying Parameter Store...")
	assert.Contains(t, buf.String(), "Applying Secrets Manager...")
}

func TestGlobalApply_ApplyDelete(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage deletes
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/old"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "old-secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	paramDeleteCalled := false
	secretDeleteCalled := false

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		paramDeleteCalled = true

		return nil
	}

	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		secretDeleteCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store), globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, paramDeleteCalled)
	assert.True(t, secretDeleteCalled)
	assert.Contains(t, buf.String(), "Parameter Store: Deleted /app/old")
	assert.Contains(t, buf.String(), "Secrets Manager: Deleted old-secret")
}

func TestGlobalApply_PartialFailure(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage both
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("param-value"),
		StagedAt:  time.Now(),
	})

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("secret-value"),
		StagedAt:  time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("Parameter Store error")
	}

	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return nil
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store), globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied 1, failed 1")

	// Parameter Store should still be staged (failed)
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config", Namespace: ""})
	require.NoError(t, err)
	assert.Equal(t, "param-value", lo.FromPtr(entry.Value))

	// Secrets Manager should be unstaged (succeeded)
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret", Namespace: ""})
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestGlobalApply_SecretDeleteWithForce(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage Secrets Manager delete with force option
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &staging.DeleteOptions{
			Force: true,
		},
	})

	var capturedEntry staging.Entry

	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, entry staging.Entry) error {
		capturedEntry = entry

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	require.NotNil(t, capturedEntry.DeleteOptions)
	assert.True(t, capturedEntry.DeleteOptions.Force)
}

func TestGlobalApply_SecretDeleteWithRecoveryWindow(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage Secrets Manager delete with custom recovery window
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &staging.DeleteOptions{
			RecoveryWindow: 7,
		},
	})

	var capturedEntry staging.Entry

	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, entry staging.Entry) error {
		capturedEntry = entry

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	require.NotNil(t, capturedEntry.DeleteOptions)
	assert.Equal(t, 7, capturedEntry.DeleteOptions.RecoveryWindow)
}

func TestGlobalApply_ParamDeleteError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("delete failed")
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestGlobalApply_SecretSetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("value"),
		StagedAt:  time.Now(),
	})

	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("put secret failed")
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestGlobalApply_SecretDeleteError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("delete secret failed")
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestGlobalApply_ParamSetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("value"),
		StagedAt:  time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("put parameter failed")
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestGlobalApply_ConflictDetection_CreateConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage a create operation
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new-param"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("new-value"),
		StagedAt:  time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()
	// Resource now exists (someone else created it)
	paramMock.fetchLastModifiedVal = time.Now()

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel:   "AWS",
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict(s) detected")
	assert.Contains(t, errBuf.String(), "conflict detected for /app/new-param")
}

func TestGlobalApply_ConflictDetection_UpdateConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          new("updated-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	paramMock := newGlobalApplyParamStrategy()
	// AWS was modified after BaseModifiedAt
	paramMock.fetchLastModifiedVal = time.Now()

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel:   "AWS",
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict(s) detected")
	assert.Contains(t, errBuf.String(), "conflict detected for /app/config")
}

func TestGlobalApply_ConflictDetection_DeleteConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation:      staging.OperationDelete,
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	secretMock := newGlobalApplySecretStrategy()
	// AWS was modified after BaseModifiedAt
	secretMock.fetchLastModifiedVal = time.Now()

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(globalApplySecret(secretMock, store)),
		ProviderLabel:   "AWS",
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict(s) detected")
	assert.Contains(t, errBuf.String(), "conflict detected for my-secret")
}

// TestGlobalApply_ConflictDetection_ProbeError covers #989: a conflict probe
// that fails (here a throttled read) rejects the apply with the probe error
// instead of treating the key as conflict-free, and the delete never runs.
func TestGlobalApply_ConflictDetection_ProbeError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation:      staging.OperationDelete,
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	secretMock := newGlobalApplySecretStrategy()
	secretMock.fetchLastModifiedErr = errors.New("ThrottlingException: rate exceeded")
	secretMock.applyFunc = func(context.Context, string, staging.Entry) error {
		t.Error("apply must not run when the conflict check failed")

		return nil
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.EqualError(t, err, "apply rejected: conflict check failed (ignore conflicts to skip it): "+
		"Secrets Manager: cannot check my-secret for conflicts: ThrottlingException: rate exceeded")
	assert.Empty(t, buf.String())

	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"})
	require.NoError(t, err, "the delete stays staged")
}

func TestGlobalApply_ConflictDetection_IgnoreConflicts(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          new("updated-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	applyCalled := false
	paramMock := newGlobalApplyParamStrategy()
	// AWS was modified after BaseModifiedAt (conflict)
	paramMock.fetchLastModifiedVal = time.Now()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		applyCalled = true

		return nil
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel:   "AWS",
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: true, // Should bypass conflict check
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, applyCalled, "Apply should be called when IgnoreConflicts is true")
}

func TestGlobalApply_ConflictDetection_NoConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          new("updated-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	applyCalled := false
	paramMock := newGlobalApplyParamStrategy()
	// AWS was modified BEFORE BaseModifiedAt (no conflict)
	paramMock.fetchLastModifiedVal = baseTime.Add(-1 * time.Hour)
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		applyCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel:   "AWS",
		Stdout:          &buf,
		Stderr:          &bytes.Buffer{},
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, applyCalled, "Apply should be called when there's no conflict")
}

func TestGlobalApply_ConflictDetection_BothServices(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)

	// Stage param with conflict
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          new("param-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	// Stage secret with conflict
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          new("secret-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	paramMock := newGlobalApplyParamStrategy()
	paramMock.fetchLastModifiedVal = time.Now() // conflict

	secretMock := newGlobalApplySecretStrategy()
	secretMock.fetchLastModifiedVal = time.Now() // conflict

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(globalApplyParam(paramMock, store), globalApplySecret(secretMock, store)),
		ProviderLabel:   "AWS",
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 conflict(s) detected")
	assert.Contains(t, errBuf.String(), "conflict detected for /app/config")
	assert.Contains(t, errBuf.String(), "conflict detected for my-secret")
}

func TestGlobalApply_ConflictDetection_TagConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)

	// Stage a tags-only change (no value entry) with a BaseModifiedAt.
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:            map[string]string{"env": "prod"},
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	applyTagsCalled := false
	paramMock := newGlobalApplyParamStrategy()
	// Remote was modified after BaseModifiedAt (conflict).
	paramMock.fetchLastModifiedVal = time.Now()
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		applyTagsCalled = true

		return nil
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel:   "AWS",
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict(s) detected")
	assert.Contains(t, errBuf.String(), "conflict detected for /app/config")
	assert.False(t, applyTagsCalled, "ApplyTags must not be called when a tag conflict is detected")

	// Tag must remain staged (apply rejected).
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	require.NoError(t, err)
}

func TestGlobalApply_ApplyCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage create operation
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/new-param"}, staging.Entry{
		Operation: staging.OperationCreate,
		Value:     new("new-value"),
		StagedAt:  time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Parameter Store: Created /app/new-param")
}

func TestGlobalApply_ApplyTagsSuccess(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})

	applyTagsCalled := false
	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyTagsFunc = func(_ context.Context, name string, tagEntry staging.TagEntry) error {
		applyTagsCalled = true

		assert.Equal(t, "/app/config", name)
		assert.Equal(t, map[string]string{"env": "prod", "team": "api"}, tagEntry.Add)
		assert.True(t, tagEntry.Remove.Contains("deprecated"))

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, applyTagsCalled)
	assert.Contains(t, buf.String(), "Applying Parameter Store tags")
	assert.Contains(t, buf.String(), "Parameter Store: Tagged /app/config")
	assert.Contains(t, buf.String(), "+2")
	assert.Contains(t, buf.String(), "-1")

	// Verify tag was unstaged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestGlobalApply_ApplyTagsError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		return fmt.Errorf("tag operation failed")
	}

	var buf, errBuf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied 0, failed 1")
	assert.Contains(t, errBuf.String(), "Failed")
	assert.Contains(t, errBuf.String(), "(tags)")

	// Verify tag was NOT unstaged (failed)
	_, err = store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
	require.NoError(t, err)
}

func TestGlobalApply_ApplyTagsSecretService(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage secret tag changes
	_ = store.StageTag(t.Context(), staging.ServiceSecret, staging.EntryKey{Name: "my-secret"}, staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		StagedAt: time.Now(),
	})

	applyTagsCalled := false
	secretMock := newGlobalApplySecretStrategy()
	secretMock.applyTagsFunc = func(_ context.Context, name string, _ staging.TagEntry) error {
		applyTagsCalled = true

		assert.Equal(t, "my-secret", name)

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplySecret(secretMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, applyTagsCalled)
	assert.Contains(t, buf.String(), "Applying Secrets Manager tags")
	assert.Contains(t, buf.String(), "Secrets Manager: Tagged my-secret")
}

func TestGlobalApply_ApplyBothEntriesAndTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage entry change
	_ = store.StageEntry(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     new("updated-value"),
		StagedAt:  time.Now(),
	})

	// Stage tag change (different resource)
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/other"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	entryCalled := false
	tagCalled := false

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		entryCalled = true

		return nil
	}
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		tagCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, entryCalled)
	assert.True(t, tagCalled)
	assert.Contains(t, buf.String(), "Applying Parameter Store...")
	assert.Contains(t, buf.String(), "Applying Parameter Store tags")
}

func TestGlobalApply_ApplyTagsOnlyAdditions(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes with only additions
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[+1]")
	assert.NotContains(t, buf.String(), "-")
}

func TestGlobalApply_ApplyTagsOnlyRemovals(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes with only removals
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		Remove:   maputil.NewSet("old-tag", "deprecated"),
		StagedAt: time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		return nil
	}

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[-2]")
	assert.NotContains(t, buf.String(), "+")
}

func TestGlobalApply_FormatTagApplySummaryEmpty(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes with both empty add and remove
	_ = store.StageTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"}, staging.TagEntry{
		StagedAt: time.Now(),
	})

	paramMock := newGlobalApplyParamStrategy()

	var buf bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:       globalApplyServices(globalApplyParam(paramMock, store)),
		ProviderLabel: "AWS",
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	// Should not have [+N] or [-N] suffix when no changes
	assert.Contains(t, buf.String(), "Parameter Store: Tagged /app/config")
	assert.NotContains(t, buf.String(), "[+")
	assert.NotContains(t, buf.String(), "[-")
}

func globalApplyParam(s staging.ApplyStrategy, st store.ReadWriteOperator) *stagingusecase.ApplyUseCase {
	return &stagingusecase.ApplyUseCase{Strategy: s, Store: st}
}

func globalApplySecret(s staging.ApplyStrategy, st store.ReadWriteOperator) *stagingusecase.ApplyUseCase {
	return &stagingusecase.ApplyUseCase{Strategy: s, Store: st}
}

// globalApplyServices bundles per-service apply use cases into the all-service
// apply use case, in apply order.
func globalApplyServices(services ...*stagingusecase.ApplyUseCase) *stagingusecase.GlobalApplyUseCase {
	return &stagingusecase.GlobalApplyUseCase{Services: services}
}

// TestGlobalApply_AppliesEntriesUnderTheirNamespace is the core guard for the
// per-namespace path: the SAME key staged under two namespaces must apply through
// the strategy scoped to EACH entry's own namespace (App Configuration keeps all
// namespaces in one staging store). Without correct threading, both entries would
// go through one namespace's strategy.
func TestGlobalApply_AppliesEntriesUnderTheirNamespace(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := testutil.NewMockStore()
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "k"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("va"), StagedAt: time.Now(),
	}))
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, staging.EntryKey{Name: "k", Namespace: "dev"}, staging.Entry{
		Operation: staging.OperationCreate, Value: new("vb"), StagedAt: time.Now(),
	}))

	var mu sync.Mutex

	appliedByNS := map[string]string{}

	// StrategyFor returns a strategy bound to the requested namespace; its Apply
	// records which value it received, so we can prove each entry was routed to
	// its own namespace's strategy.
	strategyFor := func(ns string) (staging.ApplyStrategy, error) {
		s := newGlobalApplyParamStrategy()
		s.applyFunc = func(_ context.Context, _ string, entry staging.Entry) error {
			mu.Lock()
			defer mu.Unlock()

			appliedByNS[ns] = lo.FromPtr(entry.Value)

			return nil
		}

		return s, nil
	}

	svc := &stagingusecase.ApplyUseCase{
		Store:       st,
		Strategy:    newGlobalApplyParamStrategy(),
		StrategyFor: strategyFor,
	}

	var stdout bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(svc),
		ProviderLabel:   "Azure",
		Stdout:          &stdout,
		Stderr:          &bytes.Buffer{},
		IgnoreConflicts: true, // this test is about namespace routing, not conflicts
	}

	require.NoError(t, r.Run(ctx))

	// Each entry reached the strategy scoped to ITS namespace.
	assert.Equal(t, map[string]string{"": "va", "dev": "vb"}, appliedByNS)

	// Each entry is reported on its own line, labeled with its namespace badge.
	assert.Contains(t, stdout.String(), "Parameter Store: Created k\n")
	assert.Contains(t, stdout.String(), "Parameter Store: Created k [dev]\n")

	// Both entries were unstaged under their own (name, namespace) key.
	remaining, _ := st.ListEntries(ctx, staging.ServiceParam)
	assert.Empty(t, remaining[staging.ServiceParam])
}

// TestGlobalApply_NamespacedFailureAndUnstageWarnings verifies that failures and
// unstage warnings in the all-service apply name the namespace they belong to.
func TestGlobalApply_NamespacedFailureAndUnstageWarnings(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	key := staging.EntryKey{Name: "k", Namespace: "dev"}
	st := testutil.NewMockStore()
	require.NoError(t, st.StageEntry(ctx, staging.ServiceParam, key, staging.Entry{
		Operation: staging.OperationCreate, Value: new("v"), StagedAt: time.Now(),
	}))
	require.NoError(t, st.StageTag(ctx, staging.ServiceParam, key, staging.TagEntry{
		Add: map[string]string{"env": "dev"}, StagedAt: time.Now(),
	}))
	require.NoError(t, st.StageTag(ctx, staging.ServiceParam, staging.EntryKey{Name: "k", Namespace: "prd"}, staging.TagEntry{
		Add: map[string]string{"env": "prd"}, StagedAt: time.Now(),
	}))

	st.UnstageEntryErr = errors.New("disk full")
	st.UnstageTagErr = errors.New("disk full")

	strategy := newGlobalApplyParamStrategy()
	strategy.applyTagsFunc = func(_ context.Context, _ string, tagEntry staging.TagEntry) error {
		if tagEntry.Add["env"] == "prd" {
			return errors.New("tag boom")
		}

		return nil
	}

	var stdout, stderr bytes.Buffer

	r := &stgcli.GlobalApplyRunner{
		UseCase:         globalApplyServices(globalApplyParam(strategy, st)),
		ProviderLabel:   "Azure",
		Stdout:          &stdout,
		Stderr:          &stderr,
		IgnoreConflicts: true,
	}

	require.Error(t, r.Run(ctx))
	assert.Contains(t, stdout.String(), "Parameter Store: Created k [dev]")
	assert.Contains(t, stdout.String(), "Parameter Store: Tagged k [dev] [+1]")
	assert.Contains(t, stderr.String(), "failed to clear staging for k [dev]: disk full")
	assert.Contains(t, stderr.String(), "failed to clear staging for k [dev] tags: disk full")
	assert.Contains(t, stderr.String(), "Parameter Store: k [prd] (tags): tag boom")
}
