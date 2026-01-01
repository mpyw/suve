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
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli"
	"github.com/mpyw/suve/internal/cli/push"
	"github.com/mpyw/suve/internal/stage"
)

type mockSSMClient struct {
	putParameterFunc    func(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	deleteParameterFunc func(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
	getParameterFunc    func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

func (m *mockSSMClient) PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	if m.putParameterFunc != nil {
		return m.putParameterFunc(ctx, params, optFns...)
	}
	return &ssm.PutParameterOutput{Version: 1}, nil
}

func (m *mockSSMClient) DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	if m.deleteParameterFunc != nil {
		return m.deleteParameterFunc(ctx, params, optFns...)
	}
	return &ssm.DeleteParameterOutput{}, nil
}

func (m *mockSSMClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("parameter not found")
}

type mockSMClient struct {
	putSecretValueFunc func(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	deleteSecretFunc   func(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
}

func (m *mockSMClient) PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	if m.putSecretValueFunc != nil {
		return m.putSecretValueFunc(ctx, params, optFns...)
	}
	return &secretsmanager.PutSecretValueOutput{
		Name:      params.SecretId,
		VersionId: lo.ToPtr("v1"),
	}, nil
}

func (m *mockSMClient) DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	if m.deleteSecretFunc != nil {
		return m.deleteSecretFunc(ctx, params, optFns...)
	}
	return &secretsmanager.DeleteSecretOutput{
		Name: params.SecretId,
	}, nil
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
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var buf bytes.Buffer
	r := &push.Runner{
		SSMClient: &mockSSMClient{},
		SMClient:  &mockSMClient{},
		Store:     store,
		Stdout:    &buf,
		Stderr:    &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No changes staged")
}

func TestRun_PushBothServices(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage SSM parameter
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "ssm-value",
		StagedAt:  time.Now(),
	})

	// Stage SM secret
	_ = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "sm-value",
		StagedAt:  time.Now(),
	})

	ssmPutCalled := false
	smPutCalled := false

	ssmMock := &mockSSMClient{
		putParameterFunc: func(_ context.Context, params *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			ssmPutCalled = true
			assert.Equal(t, "/app/config", lo.FromPtr(params.Name))
			return &ssm.PutParameterOutput{Version: 2}, nil
		},
	}

	smMock := &mockSMClient{
		putSecretValueFunc: func(_ context.Context, params *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
			smPutCalled = true
			assert.Equal(t, "my-secret", lo.FromPtr(params.SecretId))
			return &secretsmanager.PutSecretValueOutput{
				Name:      params.SecretId,
				VersionId: lo.ToPtr("v2"),
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SSMClient: ssmMock,
		SMClient:  smMock,
		Store:     store,
		Stdout:    &buf,
		Stderr:    &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, ssmPutCalled)
	assert.True(t, smPutCalled)
	assert.Contains(t, buf.String(), "Pushing SSM parameters")
	assert.Contains(t, buf.String(), "Pushing SM secrets")
	assert.Contains(t, buf.String(), "SSM: Set /app/config")
	assert.Contains(t, buf.String(), "SM: Set my-secret")

	// Verify both unstaged
	_, err = store.Get(stage.ServiceSSM, "/app/config")
	assert.Equal(t, stage.ErrNotStaged, err)
	_, err = store.Get(stage.ServiceSM, "my-secret")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_PushSSMOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only SSM parameter
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "ssm-value",
		StagedAt:  time.Now(),
	})

	ssmPutCalled := false
	ssmMock := &mockSSMClient{
		putParameterFunc: func(_ context.Context, _ *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			ssmPutCalled = true
			return &ssm.PutParameterOutput{Version: 2}, nil
		},
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SSMClient: ssmMock,
		SMClient:  nil, // Should not be needed
		Store:     store,
		Stdout:    &buf,
		Stderr:    &bytes.Buffer{},
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
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only SM secret
	_ = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "sm-value",
		StagedAt:  time.Now(),
	})

	smPutCalled := false
	smMock := &mockSMClient{
		putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
			smPutCalled = true
			return &secretsmanager.PutSecretValueOutput{
				VersionId: lo.ToPtr("v2"),
			}, nil
		},
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SSMClient: nil, // Should not be needed
		SMClient:  smMock,
		Store:     store,
		Stdout:    &buf,
		Stderr:    &bytes.Buffer{},
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
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage deletes
	_ = store.Stage(stage.ServiceSSM, "/app/old", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
	})
	_ = store.Stage(stage.ServiceSM, "old-secret", stage.Entry{
		Operation: stage.OperationDelete,
		StagedAt:  time.Now(),
	})

	ssmDeleteCalled := false
	smDeleteCalled := false

	ssmMock := &mockSSMClient{
		deleteParameterFunc: func(_ context.Context, _ *ssm.DeleteParameterInput, _ ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
			ssmDeleteCalled = true
			return &ssm.DeleteParameterOutput{}, nil
		},
	}

	smMock := &mockSMClient{
		deleteSecretFunc: func(_ context.Context, _ *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
			smDeleteCalled = true
			return &secretsmanager.DeleteSecretOutput{}, nil
		},
	}

	var buf bytes.Buffer
	r := &push.Runner{
		SSMClient: ssmMock,
		SMClient:  smMock,
		Store:     store,
		Stdout:    &buf,
		Stderr:    &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.NoError(t, err)
	assert.True(t, ssmDeleteCalled)
	assert.True(t, smDeleteCalled)
	assert.Contains(t, buf.String(), "SSM: Deleted /app/old")
	assert.Contains(t, buf.String(), "SM: Deleted old-secret")
}

func TestRun_PartialFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage both
	_ = store.Stage(stage.ServiceSSM, "/app/config", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "ssm-value",
		StagedAt:  time.Now(),
	})
	_ = store.Stage(stage.ServiceSM, "my-secret", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "sm-value",
		StagedAt:  time.Now(),
	})

	ssmMock := &mockSSMClient{
		putParameterFunc: func(_ context.Context, _ *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
			return nil, fmt.Errorf("SSM error")
		},
	}

	smMock := &mockSMClient{
		putSecretValueFunc: func(_ context.Context, _ *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
			return &secretsmanager.PutSecretValueOutput{
				VersionId: lo.ToPtr("v2"),
			}, nil
		},
	}

	var buf, errBuf bytes.Buffer
	r := &push.Runner{
		SSMClient: ssmMock,
		SMClient:  smMock,
		Store:     store,
		Stdout:    &buf,
		Stderr:    &errBuf,
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pushed 1, failed 1")

	// SSM should still be staged (failed)
	entry, err := store.Get(stage.ServiceSSM, "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "ssm-value", entry.Value)

	// SM should be unstaged (succeeded)
	_, err = store.Get(stage.ServiceSM, "my-secret")
	assert.Equal(t, stage.ErrNotStaged, err)
}

func TestRun_PreserveExistingType(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := stage.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage a parameter
	_ = store.Stage(stage.ServiceSSM, "/app/secure", stage.Entry{
		Operation: stage.OperationSet,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})

	var capturedType types.ParameterType
	ssmMock := &mockSSMClient{
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
	r := &push.Runner{
		SSMClient: ssmMock,
		SMClient:  nil,
		Store:     store,
		Stdout:    &buf,
		Stderr:    &bytes.Buffer{},
	}

	err := r.Run(context.Background())
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
	r := &push.Runner{
		SSMClient: &mockSSMClient{},
		SMClient:  &mockSMClient{},
		Store:     store,
		Stdout:    &buf,
		Stderr:    &bytes.Buffer{},
	}

	err := r.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
