package push_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	"github.com/mpyw/suve/internal/cli/sm/strategy"
	"github.com/mpyw/suve/internal/stage"
	"github.com/mpyw/suve/internal/stageutil"
)

type mockClient struct {
	putSecretValueFunc       func(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	deleteSecretFunc         func(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockClient) PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	if m.putSecretValueFunc != nil {
		return m.putSecretValueFunc(ctx, params, optFns...)
	}
	return &secretsmanager.PutSecretValueOutput{
		Name:      params.SecretId,
		VersionId: lo.ToPtr("v1"),
	}, nil
}

func (m *mockClient) DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	if m.deleteSecretFunc != nil {
		return m.deleteSecretFunc(ctx, params, optFns...)
	}
	return &secretsmanager.DeleteSecretOutput{
		Name: params.SecretId,
	}, nil
}

func (m *mockClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return &secretsmanager.GetSecretValueOutput{
		Name:         params.SecretId,
		SecretString: lo.ToPtr("secret-value"),
		VersionId:    lo.ToPtr("v1"),
	}, nil
}

func (m *mockClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
	if m.listSecretVersionIdsFunc != nil {
		return m.listSecretVersionIdsFunc(ctx, params, optFns...)
	}
	return &secretsmanager.ListSecretVersionIdsOutput{}, nil
}

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		var buf bytes.Buffer
		app.Writer = &buf
		err := app.Run(context.Background(), []string{"suve", "sm", "stage", "push", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Apply staged secret changes")
	})
}

func TestRun_NoChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(&mockClient{}),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stageutil.PushOptions{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No SM changes staged")
}

func TestRun_PushSet(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a secret
	_ = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})

	putCalled := false
	mock := &mockClient{
		putSecretValueFunc: func(_ context.Context, params *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
			putCalled = true
			assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
			assert.Equal(t, "new-value", lo.FromPtr(params.SecretString))
			return &secretsmanager.PutSecretValueOutput{
				Name:      params.SecretId,
				VersionId: lo.ToPtr("v2"),
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stageutil.PushOptions{})
	require.NoError(t, err)
	assert.True(t, putCalled)
	assert.Contains(t, buf.String(), "Set my-secret")

	// Verify unstaged
	_, err = store.Get(stage.ServiceSM, "my-secret")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_PushDeleteForce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a delete with force option
	_ = store.Stage(stage.ServiceSM, "old-secret", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &stage.DeleteOptions{
			Force: true,
		},
	})

	deleteCalled := false
	mock := &mockClient{
		deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
			deleteCalled = true
			assert.Equal(t, "old-secret", lo.FromPtr(params.SecretId))
			assert.True(t, lo.FromPtr(params.ForceDeleteWithoutRecovery))
			return &secretsmanager.DeleteSecretOutput{
				Name: params.SecretId,
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stageutil.PushOptions{})
	require.NoError(t, err)
	assert.True(t, deleteCalled)
	assert.Contains(t, buf.String(), "Deleted old-secret")

	// Verify unstaged
	_, err = store.Get(stage.ServiceSM, "old-secret")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_PushDeleteRecoveryWindow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a delete with recovery window
	_ = store.Stage(stage.ServiceSM, "old-secret", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
		DeleteOptions: &stage.DeleteOptions{
			RecoveryWindow: 14,
		},
	})

	deleteCalled := false
	mock := &mockClient{
		deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
			deleteCalled = true
			assert.Equal(t, "old-secret", lo.FromPtr(params.SecretId))
			assert.Nil(t, params.ForceDeleteWithoutRecovery)
			assert.Equal(t, int64(14), lo.FromPtr(params.RecoveryWindowInDays))
			return &secretsmanager.DeleteSecretOutput{
				Name: params.SecretId,
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stageutil.PushOptions{})
	require.NoError(t, err)
	assert.True(t, deleteCalled)
	assert.Contains(t, buf.String(), "Deleted old-secret")

	// Verify unstaged
	_, err = store.Get(stage.ServiceSM, "old-secret")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_PushDeleteNoOptions(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a delete without options (default AWS behavior)
	_ = store.Stage(stage.ServiceSM, "old-secret", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
	})

	deleteCalled := false
	mock := &mockClient{
		deleteSecretFunc: func(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
			deleteCalled = true
			assert.Equal(t, "old-secret", lo.FromPtr(params.SecretId))
			assert.Nil(t, params.ForceDeleteWithoutRecovery)
			assert.Nil(t, params.RecoveryWindowInDays)
			return &secretsmanager.DeleteSecretOutput{
				Name: params.SecretId,
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stageutil.PushOptions{})
	require.NoError(t, err)
	assert.True(t, deleteCalled)
	assert.Contains(t, buf.String(), "Deleted old-secret")
}

func TestRun_PushSpecificSecret(t *testing.T) {
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

	pushedSecrets := []string{}
	mock := &mockClient{
		putSecretValueFunc: func(_ context.Context, params *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
			pushedSecrets = append(pushedSecrets, lo.FromPtr(params.SecretId))
			return &secretsmanager.PutSecretValueOutput{
				Name:      params.SecretId,
				VersionId: lo.ToPtr("v1"),
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	// Push only secret1
	err := r.Run(context.Background(), stageutil.PushOptions{Name: "secret1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"secret1"}, pushedSecrets)

	// secret2 should still be staged
	entry, err := store.Get(stage.ServiceSM, "secret2")
	require.NoError(t, err)
	assert.Equal(t, "value2", entry.Value)
}

func TestRun_NotStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a different secret
	_ = store.Stage(stage.ServiceSM, "secret1", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "value1",
		StagedAt:  time.Now(),
	})

	var buf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(&mockClient{}),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	// Try to push non-existent secret
	err := r.Run(context.Background(), stageutil.PushOptions{Name: "nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not staged")
}

func TestRun_PushError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a secret
	_ = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})

	mock := &mockClient{
		putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	var buf, errBuf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(mock),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &errBuf,
	}

	err := r.Run(context.Background(), stageutil.PushOptions{})
	require.Error(t, err)
	assert.Contains(t, errBuf.String(), "Failed")
	assert.Contains(t, errBuf.String(), "AWS error")

	// Verify still staged (not cleared on failure)
	entry, err := store.Get(stage.ServiceSM, "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "new-value", entry.Value)
}

func TestRun_StoreError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stage.json")

	// Create invalid JSON
	require.NoError(t, os.WriteFile(path, []byte("invalid json"), 0o644))

	store := stage.NewStoreWithPath(path)

	var buf bytes.Buffer
	r := &stageutil.PushRunner{
		Strategy: strategy.NewStrategy(&mockClient{}),
		Store:    store,
		Stdout:   &buf,
		Stderr:   &bytes.Buffer{},
	}

	err := r.Run(context.Background(), stageutil.PushOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
