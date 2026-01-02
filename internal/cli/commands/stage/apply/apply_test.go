package apply_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/stage/apply"
	"github.com/mpyw/suve/internal/staging"
)

// mockStrategy implements staging.ApplyStrategy for testing.
type mockStrategy struct {
	service              staging.Service
	serviceName          string
	itemName             string
	hasDeleteOptions     bool
	applyFunc            func(ctx context.Context, name string, entry staging.Entry) error
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
		err := app.Run(context.Background(), []string{"suve", "stage", "push", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Apply all staged changes")
	})
}

func TestRun_NoChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

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
	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestRun_ApplyBothServices(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage SSM Parameter Store parameter
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "param-value",
		StagedAt:  time.Now(),
	})

	// Stage Secrets Manager secret
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "secret-value",
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

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, paramPutCalled)
	assert.True(t, secretPutCalled)
	assert.Contains(t, buf.String(), "Applying SSM Parameter Store parameters")
	assert.Contains(t, buf.String(), "Applying Secrets Manager secrets")
	assert.Contains(t, buf.String(), "SSM Parameter Store: Updated /app/config")
	assert.Contains(t, buf.String(), "Secrets Manager: Updated my-secret")

	// Verify both unstaged
	_, err = store.Get(staging.ServiceParam, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.Get(staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_ApplyParamOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only SSM Parameter Store parameter
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "param-value",
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

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, paramPutCalled)
	assert.Contains(t, buf.String(), "Applying SSM Parameter Store parameters")
	assert.NotContains(t, buf.String(), "Applying Secrets Manager secrets")
}

func TestRun_ApplySecretOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only Secrets Manager secret
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "secret-value",
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

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, secretPutCalled)
	assert.NotContains(t, buf.String(), "Applying SSM Parameter Store parameters")
	assert.Contains(t, buf.String(), "Applying Secrets Manager secrets")
}

func TestRun_ApplyDelete(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage deletes
	_ = store.Stage(staging.ServiceParam, "/app/old", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceSecret, "old-secret", staging.Entry{
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

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, paramDeleteCalled)
	assert.True(t, secretDeleteCalled)
	assert.Contains(t, buf.String(), "SSM Parameter Store: Deleted /app/old")
	assert.Contains(t, buf.String(), "Secrets Manager: Deleted old-secret")
}

func TestRun_PartialFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage both
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "param-value",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "secret-value",
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

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied 1, failed 1")

	// SSM Parameter Store should still be staged (failed)
	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "param-value", entry.Value)

	// Secrets Manager should be unstaged (succeeded)
	_, err = store.Get(staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	require.NoError(t, os.WriteFile(path, []byte("invalid json"), 0o644))

	store := staging.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &apply.Runner{
		ParamStrategy:  newParamStrategy(),
		SecretStrategy: newSecretStrategy(),
		Store:          store,
		Stdout:         &buf,
		Stderr:         &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestRun_SecretDeleteWithForce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage Secrets Manager delete with force option
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
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

	err := r.Run(context.Background())
	require.NoError(t, err)
	require.NotNil(t, capturedEntry.DeleteOptions)
	assert.True(t, capturedEntry.DeleteOptions.Force)
}

func TestRun_SecretDeleteWithRecoveryWindow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage Secrets Manager delete with custom recovery window
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
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

	err := r.Run(context.Background())
	require.NoError(t, err)
	require.NotNil(t, capturedEntry.DeleteOptions)
	assert.Equal(t, 7, capturedEntry.DeleteOptions.RecoveryWindow)
}

func TestRun_ParamDeleteError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
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

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_SecretSetError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "value",
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

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_SecretDeleteError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
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

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_ParamSetError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "value",
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

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}
