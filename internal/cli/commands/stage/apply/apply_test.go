package apply_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/stage/apply"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
)

// mockStrategy implements staging.ApplyStrategy for testing.
type mockStrategy struct {
	service              staging.Service
	serviceName          string
	itemName             string
	hasDeleteOptions     bool
	applyFunc            func(ctx context.Context, name string, entry staging.Entry) error
	applyTagsFunc        func(ctx context.Context, name string, tagEntry staging.TagEntry) error
	fetchLastModifiedVal time.Time
}

func (m *mockStrategy) Service() staging.Service { return m.service }
func (m *mockStrategy) ServiceName() string      { return m.serviceName }
func (m *mockStrategy) ItemName() string         { return m.itemName }
func (m *mockStrategy) HasDeleteOptions() bool   { return m.hasDeleteOptions }

func (m *mockStrategy) Apply(ctx context.Context, name string, entry staging.Entry) error {
	if m.applyFunc != nil {
		return m.applyFunc(ctx, name, entry)
	}

	return nil
}

func (m *mockStrategy) FetchLastModified(_ context.Context, _ string) (time.Time, error) {
	return m.fetchLastModifiedVal, nil
}

func (m *mockStrategy) ApplyTags(ctx context.Context, name string, tagEntry staging.TagEntry) error {
	if m.applyTagsFunc != nil {
		return m.applyTagsFunc(ctx, name, tagEntry)
	}

	return nil
}

func newParamStrategy() *mockStrategy {
	return &mockStrategy{
		service:          staging.ServiceParam,
		serviceName:      "SSM Parameter Store",
		itemName:         "parameter",
		hasDeleteOptions: false,
	}
}

func newSecretStrategy() *mockStrategy {
	return &mockStrategy{
		service:          staging.ServiceSecret,
		serviceName:      "Secrets Manager",
		itemName:         "secret",
		hasDeleteOptions: true,
	}
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()

		var buf bytes.Buffer

		app.Writer = &buf
		err := app.Run(t.Context(), []string{"suve", "stage", "push", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Apply all staged changes")
	})
}

func TestRun_NoChanges(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:  newParamStrategy(),
		SecretStrategy: newSecretStrategy(),
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	// When called with empty store, Run should return without error
	// and produce no output (action handles the warning)
	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestRun_ApplyBothServices(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage SSM Parameter Store parameter
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	// Stage Secrets Manager secret
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	paramPutCalled := false
	secretPutCalled := false

	paramMock := newParamStrategy()
	paramMock.applyFunc = func(_ context.Context, name string, _ staging.Entry) error {
		paramPutCalled = true

		assert.Equal(t, "/app/config", name)

		return nil
	}

	secretMock := newSecretStrategy()
	secretMock.applyFunc = func(_ context.Context, name string, _ staging.Entry) error {
		secretPutCalled = true

		assert.Equal(t, "my-secret", name)

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:  paramMock,
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, paramPutCalled)
	assert.True(t, secretPutCalled)
	assert.Contains(t, buf.String(), "Applying SSM Parameter Store parameters")
	assert.Contains(t, buf.String(), "Applying Secrets Manager secrets")
	assert.Contains(t, buf.String(), "SSM Parameter Store: Updated /app/config")
	assert.Contains(t, buf.String(), "Secrets Manager: Updated my-secret")

	// Verify both unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_ApplyParamOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only SSM Parameter Store parameter
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	paramPutCalled := false
	paramMock := newParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		paramPutCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:  paramMock,
		SecretStrategy: nil, // Should not be needed
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, paramPutCalled)
	assert.Contains(t, buf.String(), "Applying SSM Parameter Store parameters")
	assert.NotContains(t, buf.String(), "Applying Secrets Manager secrets")
}

func TestRun_ApplySecretOnly(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage only Secrets Manager secret
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	secretPutCalled := false
	secretMock := newSecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		secretPutCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:  nil, // Should not be needed
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, secretPutCalled)
	assert.NotContains(t, buf.String(), "Applying SSM Parameter Store parameters")
	assert.Contains(t, buf.String(), "Applying Secrets Manager secrets")
}

func TestRun_ApplyDelete(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage deletes
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/old", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "old-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	paramDeleteCalled := false
	secretDeleteCalled := false

	paramMock := newParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		paramDeleteCalled = true

		return nil
	}

	secretMock := newSecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		secretDeleteCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:  paramMock,
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, paramDeleteCalled)
	assert.True(t, secretDeleteCalled)
	assert.Contains(t, buf.String(), "SSM Parameter Store: Deleted /app/old")
	assert.Contains(t, buf.String(), "Secrets Manager: Deleted old-secret")
}

func TestRun_PartialFailure(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage both
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-value"),
		StagedAt:  time.Now(),
	})

	paramMock := newParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("SSM Parameter Store error")
	}

	secretMock := newSecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return nil
	}

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:  paramMock,
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied 1, failed 1")

	// SSM Parameter Store should still be staged (failed)
	entry, err := store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "param-value", lo.FromPtr(entry.Value))

	// Secrets Manager should be unstaged (succeeded)
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListEntriesErr = errors.New("mock store error")

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:  newParamStrategy(),
		SecretStrategy: newSecretStrategy(),
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock store error")
}

func TestRun_SecretDeleteWithForce(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage Secrets Manager delete with force option
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &staging.DeleteOptions{
			Force: true,
		},
	})

	var capturedEntry staging.Entry

	secretMock := newSecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, entry staging.Entry) error {
		capturedEntry = entry

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	require.NotNil(t, capturedEntry.DeleteOptions)
	assert.True(t, capturedEntry.DeleteOptions.Force)
}

func TestRun_SecretDeleteWithRecoveryWindow(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage Secrets Manager delete with custom recovery window
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &staging.DeleteOptions{
			RecoveryWindow: 7,
		},
	})

	var capturedEntry staging.Entry

	secretMock := newSecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, entry staging.Entry) error {
		capturedEntry = entry

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	require.NotNil(t, capturedEntry.DeleteOptions)
	assert.Equal(t, 7, capturedEntry.DeleteOptions.RecoveryWindow)
}

func TestRun_ParamDeleteError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	paramMock := newParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("delete failed")
	}

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_SecretSetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value"),
		StagedAt:  time.Now(),
	})

	secretMock := newSecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("put secret failed")
	}

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_SecretDeleteError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	secretMock := newSecretStrategy()
	secretMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("delete secret failed")
	}

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_ParamSetError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("value"),
		StagedAt:  time.Now(),
	})

	paramMock := newParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("put parameter failed")
	}

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_ConflictDetection_CreateConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage a create operation
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})

	paramMock := newParamStrategy()
	// Resource now exists (someone else created it)
	paramMock.fetchLastModifiedVal = time.Now()

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:   paramMock,
		Store:           store,
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict(s) detected")
	assert.Contains(t, errBuf.String(), "conflict detected for /app/new-param")
}

func TestRun_ConflictDetection_UpdateConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("updated-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	paramMock := newParamStrategy()
	// AWS was modified after BaseModifiedAt
	paramMock.fetchLastModifiedVal = time.Now()

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:   paramMock,
		Store:           store,
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict(s) detected")
	assert.Contains(t, errBuf.String(), "conflict detected for /app/config")
}

func TestRun_ConflictDetection_DeleteConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation:      staging.OperationDelete,
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	secretMock := newSecretStrategy()
	// AWS was modified after BaseModifiedAt
	secretMock.fetchLastModifiedVal = time.Now()

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		SecretStrategy:  secretMock,
		Store:           store,
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict(s) detected")
	assert.Contains(t, errBuf.String(), "conflict detected for my-secret")
}

func TestRun_ConflictDetection_IgnoreConflicts(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("updated-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	applyCalled := false
	paramMock := newParamStrategy()
	// AWS was modified after BaseModifiedAt (conflict)
	paramMock.fetchLastModifiedVal = time.Now()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		applyCalled = true

		return nil
	}

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:   paramMock,
		Store:           store,
		Stdout:          &buf,
		Stderr:          &errBuf,
		IgnoreConflicts: true, // Should bypass conflict check
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, applyCalled, "Apply should be called when IgnoreConflicts is true")
}

func TestRun_ConflictDetection_NoConflict(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now()
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("updated-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	applyCalled := false
	paramMock := newParamStrategy()
	// AWS was modified BEFORE BaseModifiedAt (no conflict)
	paramMock.fetchLastModifiedVal = baseTime.Add(-1 * time.Hour)
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		applyCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:   paramMock,
		Store:           store,
		Stdout:          &buf,
		Stderr:          &bytes.Buffer{},
		IgnoreConflicts: false,
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, applyCalled, "Apply should be called when there's no conflict")
}

func TestRun_ConflictDetection_BothServices(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	baseTime := time.Now().Add(-1 * time.Hour)

	// Stage param with conflict
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("param-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	// Stage secret with conflict
	_ = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation:      staging.OperationUpdate,
		Value:          lo.ToPtr("secret-value"),
		StagedAt:       time.Now(),
		BaseModifiedAt: &baseTime,
	})

	paramMock := newParamStrategy()
	paramMock.fetchLastModifiedVal = time.Now() // conflict

	secretMock := newSecretStrategy()
	secretMock.fetchLastModifiedVal = time.Now() // conflict

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy:   paramMock,
		SecretStrategy:  secretMock,
		Store:           store,
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

func TestRun_ApplyCreate(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage create operation
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})

	paramMock := newParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "SSM Parameter Store: Created /app/new-param")
}

func TestRun_ApplyTagsSuccess(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})

	applyTagsCalled := false
	paramMock := newParamStrategy()
	paramMock.applyTagsFunc = func(_ context.Context, name string, tagEntry staging.TagEntry) error {
		applyTagsCalled = true

		assert.Equal(t, "/app/config", name)
		assert.Equal(t, map[string]string{"env": "prod", "team": "api"}, tagEntry.Add)
		assert.True(t, tagEntry.Remove.Contains("deprecated"))

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, applyTagsCalled)
	assert.Contains(t, buf.String(), "Applying SSM Parameter Store tags")
	assert.Contains(t, buf.String(), "SSM Parameter Store: Tagged /app/config")
	assert.Contains(t, buf.String(), "+2")
	assert.Contains(t, buf.String(), "-1")

	// Verify tag was unstaged
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_ApplyTagsError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	paramMock := newParamStrategy()
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		return fmt.Errorf("tag operation failed")
	}

	var buf, errBuf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &errBuf,
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied 0, failed 1")
	assert.Contains(t, errBuf.String(), "Failed")
	assert.Contains(t, errBuf.String(), "(tags)")

	// Verify tag was NOT unstaged (failed)
	_, err = store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
	require.NoError(t, err)
}

func TestRun_ApplyTagsSecretService(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage secret tag changes
	_ = store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		StagedAt: time.Now(),
	})

	applyTagsCalled := false
	secretMock := newSecretStrategy()
	secretMock.applyTagsFunc = func(_ context.Context, name string, _ staging.TagEntry) error {
		applyTagsCalled = true

		assert.Equal(t, "my-secret", name)

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		SecretStrategy: secretMock,
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, applyTagsCalled)
	assert.Contains(t, buf.String(), "Applying Secrets Manager tags")
	assert.Contains(t, buf.String(), "Secrets Manager: Tagged my-secret")
}

func TestRun_ApplyBothEntriesAndTags(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage entry change
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("updated-value"),
		StagedAt:  time.Now(),
	})

	// Stage tag change (different resource)
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/other", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	entryCalled := false
	tagCalled := false

	paramMock := newParamStrategy()
	paramMock.applyFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		entryCalled = true

		return nil
	}
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		tagCalled = true

		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.True(t, entryCalled)
	assert.True(t, tagCalled)
	assert.Contains(t, buf.String(), "Applying SSM Parameter Store parameters")
	assert.Contains(t, buf.String(), "Applying SSM Parameter Store tags")
}

func TestRun_ApplyTagsOnlyAdditions(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes with only additions
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	paramMock := newParamStrategy()
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[+1]")
	assert.NotContains(t, buf.String(), "-")
}

func TestRun_ApplyTagsOnlyRemovals(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes with only removals
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("old-tag", "deprecated"),
		StagedAt: time.Now(),
	})

	paramMock := newParamStrategy()
	paramMock.applyTagsFunc = func(_ context.Context, _ string, _ staging.TagEntry) error {
		return nil
	}

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "[-2]")
	assert.NotContains(t, buf.String(), "+")
}

func TestRun_WithHintedStore(t *testing.T) {
	t.Parallel()

	store := testutil.NewHintedMockStore()

	// Stage SSM Parameter Store parameter
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	paramMock := newParamStrategy()

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "SSM Parameter Store: Updated /app/config")
	assert.Equal(t, "apply", store.LastHint)
}

func TestRun_WithHintedStoreTagApply(t *testing.T) {
	t.Parallel()

	store := testutil.NewHintedMockStore()

	// Stage tag changes
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})

	paramMock := newParamStrategy()

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "SSM Parameter Store: Tagged /app/config")
	assert.Equal(t, "apply", store.LastHint)
}

func TestRun_FormatTagApplySummaryEmpty(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()

	// Stage tag changes with both empty add and remove
	_ = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		StagedAt: time.Now(),
	})

	paramMock := newParamStrategy()

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: paramMock,
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.NoError(t, err)
	// Should not have [+N] or [-N] suffix when no changes
	assert.Contains(t, buf.String(), "SSM Parameter Store: Tagged /app/config")
	assert.NotContains(t, buf.String(), "[+")
	assert.NotContains(t, buf.String(), "[-")
}

func TestRun_ListTagsError(t *testing.T) {
	t.Parallel()

	store := testutil.NewMockStore()
	store.ListTagsErr = errors.New("mock list tags error")

	// Stage some entries so we get past the first ListEntries call
	_ = store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-value"),
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer

	r := &apply.Runner{
		ParamStrategy: newParamStrategy(),
		Store:         store,
		Stdout:        &buf,
		Stderr:        &bytes.Buffer{},
	}

	err := r.Run(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock list tags error")
}
