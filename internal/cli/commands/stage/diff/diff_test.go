package diff_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/api/secretapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	stagediff "github.com/mpyw/suve/internal/cli/commands/stage/diff"
	"github.com/mpyw/suve/internal/maputil"
	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/file"
)

type mockParamClient struct {
	getParameterFunc        func(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error)
}

func (m *mockParamClient) GetParameter(ctx context.Context, params *paramapi.GetParameterInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockParamClient) GetParameterHistory(ctx context.Context, params *paramapi.GetParameterHistoryInput, optFns ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

type mockSecretClient struct {
	getSecretValueFunc       func(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error)
	listSecretVersionIdsFunc func(ctx context.Context, params *secretapi.ListSecretVersionIdsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIdsOutput, error)
}

func (m *mockSecretClient) GetSecretValue(ctx context.Context, params *secretapi.GetSecretValueInput, optFns ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueFunc != nil {
		return m.getSecretValueFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetSecretValue not mocked")
}

func (m *mockSecretClient) ListSecretVersionIds(ctx context.Context, params *secretapi.ListSecretVersionIdsInput, optFns ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIdsOutput, error) {
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
		err := app.Run(t.Context(), []string{"suve", "stage", "diff", "--help"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Show diff of all staged changes")
	})

	t.Run("no arguments allowed", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "stage", "diff", "extra-arg"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

func TestRun_NothingStaged(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// When called with empty store, Run should return without error
	// and produce no output (action handles the warning)
	err := r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
}

func TestRun_ParamOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("old-value"),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-value")
	assert.Contains(t, output, "+new-value")
	assert.Contains(t, output, "(AWS)")
	assert.Contains(t, output, "(staged)")
}

func TestRun_SecretOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-secret"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("old-secret"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-old-secret")
	assert.Contains(t, output, "+new-secret")
}

func TestRun_BothServices(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("param-new"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	err = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("secret-new"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("param-old"),
					Version: 1,
				},
			}, nil
		},
	}

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("secret-old"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient:  paramMock,
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "-param-old")
	assert.Contains(t, output, "+param-new")
	assert.Contains(t, output, "-secret-old")
	assert.Contains(t, output, "+secret-new")
}

func TestRun_DeleteOperations(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	err = store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("existing-value"),
					Version: 1,
				},
			}, nil
		},
	}

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("existing-secret"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient:  paramMock,
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for deletion)")
	assert.Contains(t, output, "-existing-value")
	assert.Contains(t, output, "-existing-secret")
}

func TestRun_IdenticalValues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("same-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("same-value"),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged /app/config: identical to AWS current")

	// Verify actually unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_ParseJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(`{"key":"new"}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr(`{"key":"old"}`),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestRun_ParamUpdateAutoUnstageWhenDeleted(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return nil, fmt.Errorf("parameter not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "no longer exists")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestRun_SecretUpdateAutoUnstageWhenDeleted(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("secret not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "no longer exists")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestRun_SecretIdenticalValues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("same-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr("same-value"),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unstaged my-secret: identical to AWS current")

	// Verify actually unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.Equal(t, staging.ErrNotStaged, err)
}

func TestRun_SecretParseJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr(`{"key":"new"}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr(`{"key":"old"}`),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "+")
}

func TestRun_SecretParseJSONMixed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("not-json"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return &secretapi.GetSecretValueOutput{
				Name:         lo.ToPtr("my-secret"),
				SecretString: lo.ToPtr(`{"key":"old"}`),
				VersionId:    lo.ToPtr("abc123def456"),
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "--parse-json has no effect")
}

func TestRun_ParamCreateOperation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/new-param", staging.Entry{
		Operation:   staging.OperationCreate,
		Value:       lo.ToPtr("new-value"),
		Description: lo.ToPtr("New parameter"),
		StagedAt:    time.Now(),
	})
	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceParam, "/app/new-param", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "platform"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return nil, fmt.Errorf("parameter not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(not in AWS)")
	assert.Contains(t, output, "(staged for creation)")
	assert.Contains(t, output, "+new-value")
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "New parameter")
	// Tags are now staged separately and displayed in tag diff section
}

func TestRun_SecretCreateOperation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "new-secret", staging.Entry{
		Operation:   staging.OperationCreate,
		Value:       lo.ToPtr("secret-value"),
		Description: lo.ToPtr("New secret"),
		StagedAt:    time.Now(),
	})
	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceSecret, "new-secret", staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("secret not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(not in AWS)")
	assert.Contains(t, output, "(staged for creation)")
	assert.Contains(t, output, "+secret-value")
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "New secret")
	// Tags are now staged separately and displayed in tag diff section
}

func TestRun_CreateWithParseJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(`{"key":"value","nested":{"a":1}}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return nil, fmt.Errorf("parameter not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for creation)")
	// JSON should be formatted (has newlines)
	assert.Contains(t, output, "\"key\":")
}

func TestRun_DeleteAutoUnstageWhenAlreadyDeleted(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return nil, fmt.Errorf("parameter not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "already deleted")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceParam, "/app/config")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestRun_SecretDeleteAutoUnstageWhenAlreadyDeleted(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "my-secret", staging.Entry{
		Operation: staging.OperationDelete,
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("secret not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "unstaged")
	assert.Contains(t, stderr.String(), "already deleted")

	// Verify unstaged
	_, err = store.GetEntry(t.Context(), staging.ServiceSecret, "my-secret")
	assert.ErrorIs(t, err, staging.ErrNotStaged)
}

func TestRun_MetadataWithDescription(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation:   staging.OperationUpdate,
		Value:       lo.ToPtr("new-value"),
		Description: lo.ToPtr("Updated config"),
		StagedAt:    time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("old-value"),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Description:")
	assert.Contains(t, output, "Updated config")
}

func TestRun_MetadataWithTags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)
	err = store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "platform"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("old-value"),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	// Entry diff should be displayed (value change)
	assert.Contains(t, output, "--- /app/config")
	assert.Contains(t, output, "+++ /app/config")
	// Tags are now staged separately and would be displayed in tag diff section
}

func TestRun_TagOnlyDiff(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only tag changes (no entry change)
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Add:      map[string]string{"env": "prod", "team": "api"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "(staged tag changes)")
	assert.Contains(t, output, "+")
	assert.Contains(t, output, "env=prod")
	assert.Contains(t, output, "team=api")
}

func TestRun_TagOnlyRemovalsDiff(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage only tag removals (no additions)
	err := store.StageTag(t.Context(), staging.ServiceParam, "/app/config", staging.TagEntry{
		Remove:   maputil.NewSet("deprecated", "old-tag"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "deprecated")
	assert.Contains(t, output, "old-tag")
}

func TestRun_SecretTagDiff(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage secret tag changes
	err := store.StageTag(t.Context(), staging.ServiceSecret, "my-secret", staging.TagEntry{
		Add:      map[string]string{"env": "staging"},
		Remove:   maputil.NewSet("deprecated"),
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		Store:  store,
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "my-secret")
	assert.Contains(t, output, "+")
	assert.Contains(t, output, "env=staging")
	assert.Contains(t, output, "-")
	assert.Contains(t, output, "deprecated")
}

func TestRun_SecretCreateWithParseJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	err := store.StageEntry(t.Context(), staging.ServiceSecret, "new-secret", staging.Entry{
		Operation: staging.OperationCreate,
		Value:     lo.ToPtr(`{"key":"value","nested":{"a":1}}`),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	secretMock := &mockSecretClient{
		getSecretValueFunc: func(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
			return nil, fmt.Errorf("secret not found")
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		SecretClient: secretMock,
		Store:        store,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{ParseJSON: true})
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "(staged for creation)")
	// JSON should be formatted (has newlines)
	assert.Contains(t, output, "\"key\":")
}

func TestRun_BothEntriesAndTags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	store := file.NewStoreWithPath(filepath.Join(tmpDir, "stage.json"))

	// Stage entry change
	err := store.StageEntry(t.Context(), staging.ServiceParam, "/app/config", staging.Entry{
		Operation: staging.OperationUpdate,
		Value:     lo.ToPtr("new-value"),
		StagedAt:  time.Now(),
	})
	require.NoError(t, err)

	// Stage tag change (different resource)
	err = store.StageTag(t.Context(), staging.ServiceParam, "/app/other", staging.TagEntry{
		Add:      map[string]string{"env": "prod"},
		StagedAt: time.Now(),
	})
	require.NoError(t, err)

	paramMock := &mockParamClient{
		getParameterFunc: func(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{
					Name:    lo.ToPtr("/app/config"),
					Value:   lo.ToPtr("old-value"),
					Version: 1,
				},
			}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	r := &stagediff.Runner{
		ParamClient: paramMock,
		Store:       store,
		Stdout:      &stdout,
		Stderr:      &stderr,
	}

	err = r.Run(t.Context(), stagediff.Options{})
	require.NoError(t, err)

	output := stdout.String()
	// Entry diff
	assert.Contains(t, output, "/app/config")
	assert.Contains(t, output, "-old-value")
	assert.Contains(t, output, "+new-value")
	// Tag diff
	assert.Contains(t, output, "Tags:")
	assert.Contains(t, output, "/app/other")
}
