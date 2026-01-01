package diff_test

import (
	"bytes"
	"context"
	"fmt"
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
	stagediff "github.com/mpyw/suve/internal/cli/stage/diff"
	"github.com/mpyw/suve/internal/staging"
)

type mockSSMClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockSSMClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockSSMClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

type mockSMClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error)
}

func (m *mockSMClient) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func (m *mockSMClient) ListSecretVersionIds(ctx context.Context, params *secretsmanager.ListSecretVersionIdsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretVersionIdsOutput, error) {
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
		err := app.Run(context.Background(), []string{"suve", "stage", "diff", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Show diff of all staged changes")
	})

	t.Run("no arguments allowed", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(context.Background(), []string{"suve", "stage", "diff", "extra-arg"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

func TestRun_NothingStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// When called with empty store, Run should return without error
	// and produce no output (action handles the warning)
	err := r.Run(context.Background(), stagediff.Options{})
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
}

func TestRun_SSMOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	ssmMock := &mockSSMClient{
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
	r := &stagediff.Runner{
		SSMClient: ssmMock,
		Store:     store,
		Stdout:    &stdout,
		Stderr:    &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-value")
	assert.Contains(t, output, "+new-value")
	assert.Contains(t, output, "(AWS)")
	assert.Contains(t, output, "(staged)")
}

func TestRun_SMOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSM, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "new-secret",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	smMock := &mockSMClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("old-secret"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SMClient: smMock,
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-secret")
	assert.Contains(t, output, "+new-secret")
}

func TestRun_BothServices(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "ssm-new",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	err = store.Stage(staging.ServiceSM, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "sm-new",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	ssmMock := &mockSSMClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("ssm-old"),
					Version: 1,
				},
			}, nil
		},
	}

	smMock := &mockSMClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("sm-old"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SSMClient: ssmMock,
		SMClient:  smMock,
		Store:     store,
		Stdout:    &stdout,
		Stderr:    &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "-ssm-old")
	assert.Contains(t, output, "+ssm-new")
	assert.Contains(t, output, "-sm-old")
	assert.Contains(t, output, "+sm-new")
}

func TestRun_DeleteOperations(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	err = store.Stage(staging.ServiceSM, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	ssmMock := &mockSSMClient{
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

	smMock := &mockSMClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("existing-secret"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SSMClient: ssmMock,
		SMClient:  smMock,
		Store:     store,
		Stdout:    &stdout,
		Stderr:    &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for deletion)")
	assert.Contains(t, output, "-existing-value")
	assert.Contains(t, output, "-existing-secret")
}

func TestRun_IdenticalValues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "same-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	ssmMock := &mockSSMClient{
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
	r := &stagediff.Runner{
		SSMClient: ssmMock,
		Store:     store,
		Stdout:    &stdout,
		Stderr:    &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged /app/config: identical to AWS current")

	// Verify actually unstaged
	_, err = store.Get(staging.ServiceSSM, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_JSONFormat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     `{"key":"new"}`,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	ssmMock := &mockSSMClient{
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
	r := &stagediff.Runner{
		SSMClient: ssmMock,
		Store:     store,
		Stdout:    &stdout,
		Stderr:    &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{JSONFormat: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestRun_AWSError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSSM, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	ssmMock := &mockSSMClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return nil, fmt.Errorf("AWS error: parameter not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SSMClient: ssmMock,
		Store:     store,
		Stdout:    &stdout,
		Stderr:    &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS error")
}

func TestRun_SMAWSError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSM, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "new-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	smMock := &mockSMClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("AWS error: secret not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SMClient: smMock,
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS error")
}

func TestRun_SMIdenticalValues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSM, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "same-value",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	smMock := &mockSMClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("same-value"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SMClient: smMock,
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged my-secret: identical to AWS current")

	// Verify actually unstaged
	_, err = store.Get(staging.ServiceSM, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_SMJSONFormat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSM, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     `{"key":"new"}`,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	smMock := &mockSMClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr(`{"key":"old"}`),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SMClient: smMock,
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{JSONFormat: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestRun_SMJSONFormatMixed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := staging.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.Stage(staging.ServiceSM, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     "not-json",
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	smMock := &mockSMClient{
		getSecretValueFunc: func(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
			return &secretsmanager.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr(`{"key":"old"}`),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SMClient: smMock,
		Store:    store,
		Stdout:   &stdout,
		Stderr:   &stderr,
	}

	err = r.Run(context.Background(), stagediff.Options{JSONFormat: true})
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "--json has no effect")
}
