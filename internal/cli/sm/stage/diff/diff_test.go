package diff_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	smstageddiff "github.com/mpyw/suve/internal/cli/sm/stage/diff"
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

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "diff", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Show diff between staged and AWS values")
	})

	t.Run("too many args", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "diff", "secret1", "secret2"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("version specifier not allowed", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "diff", "my-secret#abc123"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without version specifier")
	})

	t.Run("label specifier not allowed", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "diff", "my-secret:AWSCURRENT"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without version specifier")
	})
}

func TestRun_NothingStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var stdout, stderr bytes.Buffer
	r := &smstageddiff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err := r.Run(context.Background(), smstageddiff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "nothing staged")
}

func TestRun_NotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var stdout, stderr bytes.Buffer
	r := &smstageddiff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err := r.Run(context.Background(), smstageddiff.Options{Name: "not-staged"})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "is not staged")
}

func TestRun_ShowDiff(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("old-value"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &smstageddiff.Runner{
		Client: mock,
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(context.Background(), smstageddiff.Options{Name: "my-secret"})
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

	err := store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("existing-value"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &smstageddiff.Runner{
		Client: mock,
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(context.Background(), smstageddiff.Options{Name: "my-secret"})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-existing-value")
	assert.Contains(t, output, "(staged for deletion)")
}

func TestRun_IdenticalValues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "same-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("same-value"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &smstageddiff.Runner{
		Client: mock,
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(context.Background(), smstageddiff.Options{Name: "my-secret"})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "staged value is identical to AWS current")
}

func TestRun_JSONFormat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     `{"key":"new"}`,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr(`{"key":"old"}`),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &smstageddiff.Runner{
		Client: mock,
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(context.Background(), smstageddiff.Options{
		Name:       "my-secret",
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

	err := store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("AWS error: secret not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &smstageddiff.Runner{
		Client: mock,
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(context.Background(), smstageddiff.Options{Name: "my-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS error")
}

func TestRun_MultipleStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(stage.ServiceSM, "secret1", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value1",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	err = store.Stage(stage.ServiceSM, "secret2", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value2",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	mock := &mockClient{
		getSecretValueFunc: func(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			name := lo.FromPtr(params.SecretId)
			return &secretsmanager.GetSecretValueOutput{
				Name:         params.SecretId,
				SecretString: lo.ToPtr("old-" + name),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &smstageddiff.Runner{
		Client: mock,
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(context.Background(), smstageddiff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "secret1")
	assert.Contains(t, output, "secret2")
}
