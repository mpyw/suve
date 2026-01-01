package reset_test

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
	"github.com/mpyw/suve/internal/cli/ssm/reset"
	"github.com/mpyw/suve/internal/stage"
)

type mockClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing spec without --all", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "reset"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve ssm reset")
	})

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "ssm", "reset", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Unstage parameter or restore")
	})
}

func TestRun_UnstageAll_Empty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), reset.Options{All: true})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No SSM changes staged")
}

func TestRun_UnstageAll(t *testing.T) {
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

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), reset.Options{All: true})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged all SSM parameters (2)")

	// Verify both unstaged
	_, err = store.Get(stage.ServiceSSM, "/app/config1")
	assert.Equal(t, stage.ErrNotStaged, err)
	_, err = store.Get(stage.ServiceSSM, "/app/config2")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_Unstage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a parameter
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value",
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), reset.Options{Spec: "/app/config"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged /app/config")

	// Verify unstaged
	_, err = store.Get(stage.ServiceSSM, "/app/config")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_UnstageNotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), reset.Options{Spec: "/app/config"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "is not staged")
}

func TestRun_RestoreVersion(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			assert.Equal(t, "/app/config:2", lo.FromPtr(params.Name))
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    params.Name,
					Value:   lo.ToPtr("old-value"),
					Type:    types.ParameterTypeString,
					Version: 2,
				},
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &reset.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), reset.Options{Spec: "/app/config#2"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Restored /app/config")
	assert.Contains(t, buf.String(), "version 2")

	// Verify staged
	entry, err := store.Get(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "old-value", entry.Value)
	assert.Equal(t, stage.OperationSet, entry.Operation)
}

func TestRun_RestoreShift(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, params *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: params.Name, Value: lo.ToPtr("v3"), Version: 3},
					{Name: params.Name, Value: lo.ToPtr("v2"), Version: 2},
					{Name: params.Name, Value: lo.ToPtr("v1"), Version: 1},
				},
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &reset.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	// ~1 means one version back from latest
	err := r.Run(context.Background(), reset.Options{Spec: "/app/config~1"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Restored /app/config")

	// Verify staged
	entry, err := store.Get(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "v2", entry.Value)
}

func TestRun_RestoreAWSError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return nil, fmt.Errorf("AWS error: parameter not found")
		},
	}

	var buf bytes.Buffer
	r := &reset.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), reset.Options{Spec: "/app/config#2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS error")
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	require.NoError(t, os.WriteFile(path, []byte("invalid json"), 0o644))

	store := stage.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), reset.Options{All: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestRun_ParseError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &reset.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), reset.Options{Spec: ""})
	require.Error(t, err)
}
