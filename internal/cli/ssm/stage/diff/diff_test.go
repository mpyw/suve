package diff_test

import (
	"bytes"
	"context"
	"fmt"
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
	"github.com/mpyw/suve/internal/stageutil"
)

type mockClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
	putParameterFunc        func(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	deleteParameterFunc     func(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
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

func (m *mockClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("PutParameter not mocked")
}

func (m *mockClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("DeleteParameter not mocked")
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "ssm", "stage", "diff", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Show diff between staged and AWS values")
	})

	t.Run("too many args", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "stage", "diff", "param1", "param2"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("version specifier not allowed", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "stage", "diff", "/param#3"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without version specifier")
	})
}

func TestRun_NothingStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
	mock := &mockClient{}

	var stdout, stderr bytes.Buffer
	r := &stageutil.DiffRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err := r.Run(context.Background(), stageutil.DiffOptions{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "staged")
}

func TestRun_NotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))
	mock := &mockClient{}

	var stdout, stderr bytes.Buffer
	r := &stageutil.DiffRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err := r.Run(context.Background(), stageutil.DiffOptions{Name: "/not-staged"})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "is not staged")
}

func TestRun_ShowDiff(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("old-value"),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stageutil.DiffRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stageutil.DiffOptions{Name: "/app/config"})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-value")
	assert.Contains(t, output, "+new-value")
	assert.Contains(t, output, "(AWS)")
	assert.Contains(t, output, "(staged)")
}

func TestRun_DeleteOperation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("existing-value"),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stageutil.DiffRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stageutil.DiffOptions{Name: "/app/config"})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-existing-value")
	assert.Contains(t, output, "(staged for deletion)")
}

func TestRun_IdenticalValues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "same-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("same-value"),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stageutil.DiffRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stageutil.DiffOptions{Name: "/app/config"})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged /app/config: identical to AWS current")

	// Verify actually unstaged
	_, err = store.Get(stage.ServiceSSM, "/app/config")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_JSONFormat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     `{"key":"new"}`,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr(`{"key":"old"}`),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stageutil.DiffRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stageutil.DiffOptions{
		Name:       "/app/config",
		JSONFormat: true,
	})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestRun_AWSError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return nil, fmt.Errorf("AWS error: parameter not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stageutil.DiffRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stageutil.DiffOptions{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS error")
}

func TestRun_MultipleStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSSM, "/app/config1", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value1",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	err = store.Stage(stage.ServiceSSM, "/app/config2", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value2",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			name := lo.FromPtr(params.Name)
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    params.Name,
					Value:   lo.ToPtr("old-" + name),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stageutil.DiffRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stageutil.DiffOptions{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "/app/config1")
	assert.Contains(t, output, "/app/config2")
}
