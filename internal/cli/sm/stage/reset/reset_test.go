package reset_test

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
	"github.com/mpyw/suve/internal/cli/sm/stage/reset"
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

	t.Run("missing spec without --all", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "reset"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage: suve sm stage reset")
	})

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "reset", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Unstage secret or restore")
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
	assert.Contains(t, buf.String(), "No SM changes staged")
}

func TestRun_UnstageAll(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage multiple secrets
	_ = store.Stage(stage.ServiceSM, "secret1", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value1",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(stage.ServiceSM, "secret2", stage.Entry{
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
	assert.Contains(t, buf.String(), "Unstaged all SM secrets (2)")

	// Verify both unstaged
	_, err = store.Get(stage.ServiceSM, "secret1")
	assert.Equal(t, stage.ErrNotStaged, err)
	_, err = store.Get(stage.ServiceSM, "secret2")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_Unstage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a secret
	_ = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
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

	err := r.Run(context.Background(), reset.Options{Spec: "my-secret"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Unstaged my-secret")

	// Verify unstaged
	_, err = store.Get(stage.ServiceSM, "my-secret")
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

	err := r.Run(context.Background(), reset.Options{Spec: "my-secret"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "is not staged")
}

func TestRun_RestoreVersion(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
			assert.Equal(t, "abc123", lo.FromPtr(params.VersionId))
			return &secretsmanager.GetSecretValueOutput{
				Name:          params.SecretId,
				SecretString:  lo.ToPtr("old-value"),
				VersionId:     params.VersionId,
				VersionStages: []string{"AWSPREVIOUS"},
			}, nil
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("def456"), VersionStages: []string{"AWSCURRENT"}},
					{VersionId: lo.ToPtr("abc123"), VersionStages: []string{"AWSPREVIOUS"}},
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

	err := r.Run(context.Background(), reset.Options{Spec: "my-secret#abc123"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Restored my-secret")

	// Verify staged
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "old-value", entry.Value)
	assert.Equal(t, stage.OperationSet, entry.Operation)
}

func TestRun_RestoreLabel(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
			assert.Equal(t, "AWSPREVIOUS", lo.FromPtr(params.VersionStage))
			return &secretsmanager.GetSecretValueOutput{
				Name:          params.SecretId,
				SecretString:  lo.ToPtr("previous-value"),
				VersionId:     lo.ToPtr("abc123"),
				VersionStages: []string{"AWSPREVIOUS"},
			}, nil
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("def456"), VersionStages: []string{"AWSCURRENT"}},
					{VersionId: lo.ToPtr("abc123"), VersionStages: []string{"AWSPREVIOUS"}},
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

	err := r.Run(context.Background(), reset.Options{Spec: "my-secret:AWSPREVIOUS"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Restored my-secret")

	// Verify staged
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "previous-value", entry.Value)
}

func TestRun_RestoreShift(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			// Should get abc123 (one version back from current)
			assert.Equal(t, "abc123", lo.FromPtr(params.VersionId))
			return &secretsmanager.GetSecretValueOutput{
				Name:          params.SecretId,
				SecretString:  lo.ToPtr("v2-value"),
				VersionId:     params.VersionId,
				VersionStages: []string{"AWSPREVIOUS"},
			}, nil
		},
		listSecretVersionIdsFunc: func(_ context.Context, params *secretsmanager.ListSecretVersionIdsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
			return &secretsmanager.ListSecretVersionIdsOutput{
				Name: params.SecretId,
				Versions: []types.SecretVersionsListEntry{
					{VersionId: lo.ToPtr("def456"), VersionStages: []string{"AWSCURRENT"}},
					{VersionId: lo.ToPtr("abc123"), VersionStages: []string{"AWSPREVIOUS"}},
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
	err := r.Run(context.Background(), reset.Options{Spec: "my-secret~1"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Restored my-secret")

	// Verify staged
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "v2-value", entry.Value)
}

func TestRun_RestoreAWSError(t *testing.T) {
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
					{VersionId: lo.ToPtr("abc123"), VersionStages: []string{"AWSCURRENT"}},
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

	err := r.Run(context.Background(), reset.Options{Spec: "my-secret#abc123"})
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
