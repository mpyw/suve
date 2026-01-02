package push_test

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
	"github.com/mpyw/suve/internal/cli/commands/stage/push"
	"github.com/mpyw/suve/internal/staging"
)

// mockStrategy implements staging.PushStrategy for testing.
type mockStrategy struct {
	service              staging.Service
	serviceName          string
	itemName             string
	hasDeleteOptions     bool
	pushFunc             func(ctx context.Context, name string, entry staging.Entry) error
	fetchLastModifiedVal time.Time
}

func (m *mockStrategy) Service() staging.Service { return m.service }
func (m *mockStrategy) ServiceName() string      { return m.serviceName }
func (m *mockStrategy) ItemName() string         { return m.itemName }
func (m *mockStrategy) HasDeleteOptions() bool   { return m.hasDeleteOptions }

func (m *mockStrategy) Push(ctx context.Context, name string, entry staging.Entry) error {
	if m.pushFunc != nil {
		return m.pushFunc(ctx, name, entry)
	}
	return nil
}

func (m *mockStrategy) FetchLastModified(_ context.Context, _ string) (time.Time, error) {
	return m.fetchLastModifiedVal, nil
}

func newSSMStrategy() *mockStrategy {
	return &mockStrategy{
		service:          staging.ServiceParam,
		serviceName:      "SSM Parameter Store",
		itemName:         "parameter",
		hasDeleteOptions: false,
	}
}

func newSMStrategy() *mockStrategy {
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
	r := &push.Runner{
		SSMStrategy: newSSMStrategy(),
		SMStrategy:  newSMStrategy(),
		Store:       store,
		Stdout:      &buf,
		Stderr:      &bytes.Buffer{},
	}

	// When called with empty store, Run should return without error
	// and produce no output (action handles the warning)
	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestRun_PushBothServices(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage SSM parameter
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "ssm-value",
		StagedAt:  time.Now(),
	})

	// Stage SM secret
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "sm-value",
		StagedAt:  time.Now(),
	})

	ssmPutCalled := false
	smPutCalled := false

	ssmMock := newSSMStrategy()
	ssmMock.pushFunc = func(_ context.Context, name string, _ staging.Entry) error {
		ssmPutCalled = true
		assert.Equal(t, "/app/config", name)
		return nil
	}

	smMock := newSMStrategy()
	smMock.pushFunc = func(_ context.Context, name string, _ staging.Entry) error {
		smPutCalled = true
		assert.Equal(t, "my-secret", name)
		return nil
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SSMStrategy: ssmMock,
		SMStrategy:  smMock,
		Store:       store,
		Stdout:      &buf,
		Stderr:      &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, ssmPutCalled)
	assert.True(t, smPutCalled)
	assert.Contains(t, buf.String(), "Pushing SSM parameters")
	assert.Contains(t, buf.String(), "Pushing SM secrets")
	assert.Contains(t, buf.String(), "SSM Parameter Store: Updated /app/config")
	assert.Contains(t, buf.String(), "Secrets Manager: Updated my-secret")

	// Verify both unstaged
	_, err = store.Get(staging.ServiceParam, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
	_, err = store.Get(staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_PushSSMOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only SSM parameter
	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "ssm-value",
		StagedAt:  time.Now(),
	})

	ssmPutCalled := false
	ssmMock := newSSMStrategy()
	ssmMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		ssmPutCalled = true
		return nil
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SSMStrategy: ssmMock,
		SMStrategy:  nil, // Should not be needed
		Store:       store,
		Stdout:      &buf,
		Stderr:      &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, ssmPutCalled)
	assert.Contains(t, buf.String(), "Pushing SSM parameters")
	assert.NotContains(t, buf.String(), "Pushing SM secrets")
}

func TestRun_PushSMOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only SM secret
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "sm-value",
		StagedAt:  time.Now(),
	})

	smPutCalled := false
	smMock := newSMStrategy()
	smMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		smPutCalled = true
		return nil
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SSMStrategy: nil, // Should not be needed
		SMStrategy:  smMock,
		Store:       store,
		Stdout:      &buf,
		Stderr:      &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, smPutCalled)
	assert.NotContains(t, buf.String(), "Pushing SSM parameters")
	assert.Contains(t, buf.String(), "Pushing SM secrets")
}

func TestRun_PushDelete(t *testing.T) {
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

	ssmDeleteCalled := false
	smDeleteCalled := false

	ssmMock := newSSMStrategy()
	ssmMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		ssmDeleteCalled = true
		return nil
	}

	smMock := newSMStrategy()
	smMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		smDeleteCalled = true
		return nil
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SSMStrategy: ssmMock,
		SMStrategy:  smMock,
		Store:       store,
		Stdout:      &buf,
		Stderr:      &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, ssmDeleteCalled)
	assert.True(t, smDeleteCalled)
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
		Value:     "ssm-value",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "sm-value",
		StagedAt:  time.Now(),
	})

	ssmMock := newSSMStrategy()
	ssmMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("SSM error")
	}

	smMock := newSMStrategy()
	smMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return nil
	}

	var buf, errBuf bytes.Buffer
	r := &push.Runner{
		SSMStrategy: ssmMock,
		SMStrategy:  smMock,
		Store:       store,
		Stdout:      &buf,
		Stderr:      &errBuf,
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pushed 1, failed 1")

	// SSM should still be staged (failed)
	entry, err := store.Get(staging.ServiceParam, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "ssm-value", entry.Value)

	// SM should be unstaged (succeeded)
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
	r := &push.Runner{
		SSMStrategy: newSSMStrategy(),
		SMStrategy:  newSMStrategy(),
		Store:       store,
		Stdout:      &buf,
		Stderr:      &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestRun_SMDeleteWithForce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage SM delete with force option
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &staging.DeleteOptions{
			Force: true,
		},
	})

	var capturedEntry staging.Entry
	smMock := newSMStrategy()
	smMock.pushFunc = func(_ context.Context, _ string, entry staging.Entry) error {
		capturedEntry = entry
		return nil
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SMStrategy: smMock,
		Store:      store,
		Stdout:     &buf,
		Stderr:     &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	require.NotNil(t, capturedEntry.DeleteOptions)
	assert.True(t, capturedEntry.DeleteOptions.Force)
}

func TestRun_SMDeleteWithRecoveryWindow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage SM delete with custom recovery window
	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &staging.DeleteOptions{
			RecoveryWindow: 7,
		},
	})

	var capturedEntry staging.Entry
	smMock := newSMStrategy()
	smMock.pushFunc = func(_ context.Context, _ string, entry staging.Entry) error {
		capturedEntry = entry
		return nil
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SMStrategy: smMock,
		Store:      store,
		Stdout:     &buf,
		Stderr:     &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	require.NotNil(t, capturedEntry.DeleteOptions)
	assert.Equal(t, 7, capturedEntry.DeleteOptions.RecoveryWindow)
}

func TestRun_SSMDeleteError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	ssmMock := newSSMStrategy()
	ssmMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("delete failed")
	}

	var buf, errBuf bytes.Buffer
	r := &push.Runner{
		SSMStrategy: ssmMock,
		Store:       store,
		Stdout:      &buf,
		Stderr:      &errBuf,
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_SMSetError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "value",
		StagedAt:  time.Now(),
	})

	smMock := newSMStrategy()
	smMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("put secret failed")
	}

	var buf, errBuf bytes.Buffer
	r := &push.Runner{
		SMStrategy: smMock,
		Store:      store,
		Stdout:     &buf,
		Stderr:     &errBuf,
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_SMDeleteError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	_ = store.Stage(staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})

	smMock := newSMStrategy()
	smMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("delete secret failed")
	}

	var buf, errBuf bytes.Buffer
	r := &push.Runner{
		SMStrategy: smMock,
		Store:      store,
		Stdout:     &buf,
		Stderr:     &errBuf,
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}

func TestRun_SSMSetError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	_ = store.Stage(staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "value",
		StagedAt:  time.Now(),
	})

	ssmMock := newSSMStrategy()
	ssmMock.pushFunc = func(_ context.Context, _ string, _ staging.Entry) error {
		return fmt.Errorf("put parameter failed")
	}

	var buf, errBuf bytes.Buffer
	r := &push.Runner{
		SSMStrategy: ssmMock,
		Store:       store,
		Stdout:      &buf,
		Stderr:      &errBuf,
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
}
