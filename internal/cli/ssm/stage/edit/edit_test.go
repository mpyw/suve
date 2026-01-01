package edit_test

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
	"github.com/mpyw/suve/internal/cli/ssm/stage/edit"
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

	t.Run("missing parameter name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "ssm", "stage", "edit"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve ssm stage edit")
	})

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "ssm", "stage", "edit", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Edit parameter value")
	})
}

func TestRun_UseStagedValue(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage an existing value
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "staged-value",
		StagedAt:  time.Now(),
	})

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			t.Fatal("GetParameter should not be called when value is already staged")
			return nil, nil
		},
	}

	var buf bytes.Buffer
	r := &edit.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
		OpenEditor: func(content string) (string, error) {
			// Verify staged value is passed to editor
			assert.Equal(t, "staged-value", content)
			return content, nil // No changes
		},
	}

	// Test that it uses staged value (will hit the "no changes" path)
	// This validates that GetParameter is not called
	err := r.Run(context.Background(), edit.Options{Name: "/app/config"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes made")
}

func TestRun_FetchFromAWS(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	fetchCalled := false
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			fetchCalled = true
			assert.Equal(t, "/app/config", lo.FromPtr(params.Name))
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:  params.Name,
					Value: lo.ToPtr("aws-value"),
					Type:  types.ParameterTypeString,
				},
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &edit.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
		OpenEditor: func(content string) (string, error) {
			// Verify AWS value is passed to editor
			assert.Equal(t, "aws-value", content)
			return content, nil // No changes
		},
	}

	// Test that it fetches from AWS when not staged
	err := r.Run(context.Background(), edit.Options{Name: "/app/config"})
	require.NoError(t, err)
	assert.True(t, fetchCalled, "GetParameter should be called when value is not staged")
	assert.Contains(t, buf.String(), "No changes made")
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	require.NoError(t, writeFile(path, "invalid json"))

	store := stage.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &edit.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), edit.Options{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestRun_AWSError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return nil, fmt.Errorf("AWS error: parameter not found")
		},
	}

	var buf bytes.Buffer
	r := &edit.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), edit.Options{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS error")
}

func TestRun_ParseError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &edit.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	// Invalid parameter name
	err := r.Run(context.Background(), edit.Options{Name: ""})
	require.Error(t, err)
}

func TestRun_NoChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:  params.Name,
					Value: lo.ToPtr("original-value"),
					Type:  types.ParameterTypeString,
				},
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &edit.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
		OpenEditor: func(content string) (string, error) {
			// Return the same value (no changes)
			return content, nil
		},
	}

	err := r.Run(context.Background(), edit.Options{Name: "/app/config"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes made")
}

func TestRun_SuccessfulStaging(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:  params.Name,
					Value: lo.ToPtr("original-value"),
					Type:  types.ParameterTypeString,
				},
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &edit.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
		OpenEditor: func(_ string) (string, error) {
			return "new-value", nil
		},
	}

	err := r.Run(context.Background(), edit.Options{Name: "/app/config"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Staged")
	assert.Contains(t, buf.String(), "/app/config")

	// Verify staged
	entry, err := store.Get(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, stage.OperationSet, entry.Operation)
	assert.Equal(t, "new-value", entry.Value)
}

func TestRun_EditorError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:  params.Name,
					Value: lo.ToPtr("original-value"),
					Type:  types.ParameterTypeString,
				},
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &edit.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
		OpenEditor: func(_ string) (string, error) {
			return "", fmt.Errorf("editor failed")
		},
	}

	err := r.Run(context.Background(), edit.Options{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to edit")
}

func TestRun_StagingStoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create a valid stage file first
	store := stage.NewStoreWithPath(path)

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:  params.Name,
					Value: lo.ToPtr("original-value"),
					Type:  types.ParameterTypeString,
				},
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &edit.Runner{
		Client: mock,
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
		OpenEditor: func(_ string) (string, error) {
			// Make the store file invalid before returning
			_ = os.WriteFile(path, []byte("invalid json"), 0o644)
			return "new-value", nil
		},
	}

	err := r.Run(context.Background(), edit.Options{Name: "/app/config"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
