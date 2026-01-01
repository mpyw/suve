package edit_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	"github.com/mpyw/suve/internal/cli/sm/stage/edit"
	"github.com/mpyw/suve/internal/stage"
)

type mockClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("ListSecretVersionIds not mocked")
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing secret name", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "edit"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve sm stage edit")
	})

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "edit", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Edit secret value")
	})
}

func TestRun_UseStagedValue(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage an existing value
	_ = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "staged-value",
		StagedAt:  time.Now(),
	})

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			t.Fatal("GetSecretValue should not be called when value is already staged")
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
	// This validates that GetSecretValue is not called
	err := r.Run(context.Background(), edit.Options{Name: "my-secret"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes made")
}

func TestRun_FetchFromAWS(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	fetchCalled := false
	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			fetchCalled = true
			assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
			return &secretsmanager.GetSecretValueOutput{
				Name:          params.SecretId,
				SecretString:  lo.ToPtr("aws-value"),
				VersionId:     lo.ToPtr("v1"),
				VersionStages: []string{"AWSCURRENT"},
			}, nil
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{
						VersionId:     lo.ToPtr("v1"),
						VersionStages: []string{"AWSCURRENT"},
					},
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
	err := r.Run(context.Background(), edit.Options{Name: "my-secret"})
	require.NoError(t, err)
	assert.True(t, fetchCalled, "GetSecretValue should be called when value is not staged")
	assert.Contains(t, buf.String(), "No changes made")
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	require.NoError(t, os.WriteFile(path, []byte("invalid json"), 0o644))

	store := stage.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &edit.Runner{
		Store:  store,
		Stdout: &buf,
		Stderr: &bytes.Buffer{},
	}

	err := r.Run(context.Background(), edit.Options{Name: "my-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestRun_AWSError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("AWS error: secret not found")
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{
						VersionId:     lo.ToPtr("v1"),
						VersionStages: []string{"AWSCURRENT"},
					},
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
	}

	err := r.Run(context.Background(), edit.Options{Name: "my-secret"})
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

	// Invalid secret name (empty)
	err := r.Run(context.Background(), edit.Options{Name: ""})
	require.Error(t, err)
}

func TestRun_NoChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:          params.SecretId,
				SecretString:  lo.ToPtr("original-value"),
				VersionId:     lo.ToPtr("v1"),
				VersionStages: []string{"AWSCURRENT"},
			}, nil
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{
						VersionId:     lo.ToPtr("v1"),
						VersionStages: []string{"AWSCURRENT"},
					},
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

	err := r.Run(context.Background(), edit.Options{Name: "my-secret"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes made")
}

func TestRun_SuccessfulStaging(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:          params.SecretId,
				SecretString:  lo.ToPtr("original-value"),
				VersionId:     lo.ToPtr("v1"),
				VersionStages: []string{"AWSCURRENT"},
			}, nil
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{
						VersionId:     lo.ToPtr("v1"),
						VersionStages: []string{"AWSCURRENT"},
					},
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

	err := r.Run(context.Background(), edit.Options{Name: "my-secret"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Staged")
	assert.Contains(t, buf.String(), "my-secret")

	// Verify staged
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, stage.OperationSet, entry.Operation)
	assert.Equal(t, "new-value", entry.Value)
}

func TestRun_EditorError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:          params.SecretId,
				SecretString:  lo.ToPtr("original-value"),
				VersionId:     lo.ToPtr("v1"),
				VersionStages: []string{"AWSCURRENT"},
			}, nil
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{
						VersionId:     lo.ToPtr("v1"),
						VersionStages: []string{"AWSCURRENT"},
					},
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

	err := r.Run(context.Background(), edit.Options{Name: "my-secret"})
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
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:          params.SecretId,
				SecretString:  lo.ToPtr("original-value"),
				VersionId:     lo.ToPtr("v1"),
				VersionStages: []string{"AWSCURRENT"},
			}, nil
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{
						VersionId:     lo.ToPtr("v1"),
						VersionStages: []string{"AWSCURRENT"},
					},
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

	err := r.Run(context.Background(), edit.Options{Name: "my-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
