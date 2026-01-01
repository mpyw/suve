package push_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	"github.com/mpyw/suve/internal/cli/ssm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stage/stagerunner"
)

type mockClient struct {
	putParameterFunc        func(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	deleteParameterFunc     func(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}
	return &ssm.PutParameterOutput{Version: 1}, nil
}

func (m *mockClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, params, optFns...)
	}
	return &ssm.DeleteParameterOutput{}, nil
}

func (m *mockClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	// Return ParameterNotFound error to indicate a new parameter
	return nil, &types.ParameterNotFound{Message: lo.ToPtr("parameter not found")}
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return &ssm.GetParameterHistoryOutput{}, nil
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "ssm", "stage", "push", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Apply staged parameter changes")
	})
}

func TestRun_NoChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(&mockClient{}),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stagerunner.PushOptions{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No SSM changes staged")
}

func TestRun_PushSet(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a parameter
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})

	putCalled := false
	mock := &mockClient{
		putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			putCalled = true
			assert.Equal(t, "/app/config", lo.FromPtr(params.Name))
			assert.Equal(t, "new-value", lo.FromPtr(params.Value))
			return &ssm.PutParameterOutput{Version: 2}, nil
		},
	}

	var buf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stagerunner.PushOptions{})
	require.NoError(t, err)
	assert.True(t, putCalled)
	assert.Contains(t, buf.String(), "Set /app/config")

	// Verify unstaged
	_, err = store.Get(stage.ServiceSSM, "/app/config")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_PushDelete(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a delete
	_ = store.Stage(stage.ServiceSSM, "/app/old-config", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
	})

	deleteCalled := false
	mock := &mockClient{
		deleteParameterFunc: func(_ context.Context, params *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
			deleteCalled = true
			assert.Equal(t, "/app/old-config", lo.FromPtr(params.Name))
			return &ssm.DeleteParameterOutput{}, nil
		},
	}

	var buf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stagerunner.PushOptions{})
	require.NoError(t, err)
	assert.True(t, deleteCalled)
	assert.Contains(t, buf.String(), "Deleted /app/old-config")

	// Verify unstaged
	_, err = store.Get(stage.ServiceSSM, "/app/old-config")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_PushSpecificParameter(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage multiple parameters
	_ = store.Stage(stage.ServiceSSM, "/app/config1", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value1",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(stage.ServiceSSM, "/app/config2", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value2",
		StagedAt:  time.Now(),
	})

	pushedParams := []string{}
	mock := &mockClient{
		putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			pushedParams = append(pushedParams, lo.FromPtr(params.Name))
			return &ssm.PutParameterOutput{Version: 1}, nil
		},
	}

	var buf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	// Push only config1
	err := r.Run(context.Background(), stagerunner.PushOptions{Name: "/app/config1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"/app/config1"}, pushedParams)

	// config2 should still be staged
	entry, err := store.Get(stage.ServiceSSM, "/app/config2")
	require.NoError(t, err)
	assert.Equal(t, "value2", entry.Value)
}

func TestRun_NotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a different parameter
	_ = store.Stage(stage.ServiceSSM, "/app/config1", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value1",
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(&mockClient{}),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	// Try to push non-existent parameter
	err := r.Run(context.Background(), stagerunner.PushOptions{Name: "/app/nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not staged")
}

func TestRun_PushError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a parameter
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})

	mock := &mockClient{
		putParameterFunc: func(_ context.Context, _ *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	var buf, errBuf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &errBuf,
	}

	err := r.Run(context.Background(), stagerunner.PushOptions{})
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
	assert.Contains(t, errBuf.String(), "AWS error")

	// Verify still staged (not cleared on failure)
	entry, err := store.Get(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "new-value", entry.Value)
}

func TestRun_PreserveExistingType(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a parameter
	_ = store.Stage(stage.ServiceSSM, "/app/secure-config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})

	var capturedType types.ParameterType
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Type: types.ParameterTypeSecureString,
				},
			}, nil
		},
		putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			capturedType = params.Type
			return &ssm.PutParameterOutput{Version: 2}, nil
		},
	}

	var buf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stagerunner.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, types.ParameterTypeSecureString, capturedType)
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	require.NoError(t, os.WriteFile(path, []byte("invalid json"), 0o644))

	store := stage.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(&mockClient{}),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stagerunner.PushOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestRun_PushDeleteError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a delete
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
	})

	mock := &mockClient{
		deleteParameterFunc: func(_ context.Context, _ *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
			return nil, fmt.Errorf("delete error")
		},
	}

	var buf, errBuf bytes.Buffer
	r := &stagerunner.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &errBuf,
	}

	err := r.Run(context.Background(), stagerunner.PushOptions{})
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
	assert.Contains(t, errBuf.String(), "delete error")
}
