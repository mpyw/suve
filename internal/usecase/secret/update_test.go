package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/tagging"
	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockUpdateClient struct {
	getSecretValueResult *secretapi.GetSecretValueOutput
	getSecretValueErr    error
	updateSecretResult   *secretapi.UpdateSecretOutput
	updateSecretErr      error
	putSecretValueResult *secretapi.PutSecretValueOutput
	putSecretValueErr    error
	tagResourceErr       error
	untagResourceErr     error
}

func (m *mockUpdateClient) GetSecretValue(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueErr != nil {
		return nil, m.getSecretValueErr
	}
	return m.getSecretValueResult, nil
}

func (m *mockUpdateClient) UpdateSecret(_ context.Context, _ *secretapi.UpdateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error) {
	if m.updateSecretErr != nil {
		return nil, m.updateSecretErr
	}
	return m.updateSecretResult, nil
}

func (m *mockUpdateClient) PutSecretValue(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
	if m.putSecretValueErr != nil {
		return nil, m.putSecretValueErr
	}
	return m.putSecretValueResult, nil
}

func (m *mockUpdateClient) TagResource(_ context.Context, _ *secretapi.TagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.TagResourceOutput, error) {
	if m.tagResourceErr != nil {
		return nil, m.tagResourceErr
	}
	return &secretapi.TagResourceOutput{}, nil
}

func (m *mockUpdateClient) UntagResource(_ context.Context, _ *secretapi.UntagResourceInput, _ ...func(*secretapi.Options)) (*secretapi.UntagResourceOutput, error) {
	if m.untagResourceErr != nil {
		return nil, m.untagResourceErr
	}
	return &secretapi.UntagResourceOutput{}, nil
}

func TestUpdateUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			SecretString: lo.ToPtr("current-value"),
		},
	}

	uc := &secret.UpdateUseCase{Client: client}

	value, err := uc.GetCurrentValue(context.Background(), "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "current-value", value)
}

func TestUpdateUseCase_GetCurrentValue_Error(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretValueErr: errors.New("aws error"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.GetCurrentValue(context.Background(), "my-secret")
	assert.Error(t, err)
}

func TestUpdateUseCase_Execute_UpdateValue(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		putSecretValueResult: &secretapi.PutSecretValueOutput{
			VersionId: lo.ToPtr("new-version-id"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.UpdateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name:  "my-secret",
		Value: "new-value",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "new-version-id", output.VersionID)
}

func TestUpdateUseCase_Execute_UpdateDescription(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		updateSecretResult: &secretapi.UpdateSecretOutput{
			VersionId: lo.ToPtr("version-id"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.UpdateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name:        "my-secret",
		Description: "new description",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "version-id", output.VersionID)
}

func TestUpdateUseCase_Execute_UpdateValueAndDescription(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		putSecretValueResult: &secretapi.PutSecretValueOutput{
			VersionId: lo.ToPtr("new-version-id"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
		updateSecretResult: &secretapi.UpdateSecretOutput{
			VersionId: lo.ToPtr("desc-version-id"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.UpdateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name:        "my-secret",
		Value:       "new-value",
		Description: "new description",
	})
	require.NoError(t, err)
	// VersionID from PutSecretValue takes precedence
	assert.Equal(t, "new-version-id", output.VersionID)
}

func TestUpdateUseCase_Execute_PutValueError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		putSecretValueErr: errors.New("put value failed"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name:  "my-secret",
		Value: "new-value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update secret value")
}

func TestUpdateUseCase_Execute_UpdateDescriptionError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		updateSecretErr: errors.New("update failed"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name:        "my-secret",
		Description: "new description",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update secret description")
}

func TestUpdateUseCase_Execute_WithTags(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.UpdateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name: "my-secret",
		TagChange: &tagging.Change{
			Add:    map[string]string{"env": "prod"},
			Remove: []string{"old-tag"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
}

func TestUpdateUseCase_Execute_TagsWithARNFromPutValue(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		putSecretValueResult: &secretapi.PutSecretValueOutput{
			VersionId: lo.ToPtr("new-version-id"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.UpdateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name:  "my-secret",
		Value: "new-value",
		TagChange: &tagging.Change{
			Add: map[string]string{"env": "prod"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "new-version-id", output.VersionID)
}

func TestUpdateUseCase_Execute_GetARNError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretValueErr: errors.New("get failed"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name: "my-secret",
		TagChange: &tagging.Change{
			Add: map[string]string{"env": "prod"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get secret ARN")
}

func TestUpdateUseCase_Execute_TagResourceError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
		tagResourceErr: errors.New("tag failed"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name: "my-secret",
		TagChange: &tagging.Change{
			Add: map[string]string{"env": "prod"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestUpdateUseCase_Execute_UntagResourceError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			ARN: lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
		untagResourceErr: errors.New("untag failed"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name: "my-secret",
		TagChange: &tagging.Change{
			Remove: []string{"old-tag"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}

func TestUpdateUseCase_Execute_EmptyTagChange(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		putSecretValueResult: &secretapi.PutSecretValueOutput{
			VersionId: lo.ToPtr("new-version-id"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.UpdateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.UpdateInput{
		Name:      "my-secret",
		Value:     "new-value",
		TagChange: &tagging.Change{},
	})
	require.NoError(t, err)
	assert.Equal(t, "new-version-id", output.VersionID)
}
