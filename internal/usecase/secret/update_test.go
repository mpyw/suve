package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockUpdateClient struct {
	getSecretValueResult *secretapi.GetSecretValueOutput
	getSecretValueErr    error
	updateSecretResult   *secretapi.UpdateSecretOutput
	updateSecretErr      error
	putSecretValueResult *secretapi.PutSecretValueOutput
	putSecretValueErr    error
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockUpdateClient) GetSecretValue(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueErr != nil {
		return nil, m.getSecretValueErr
	}

	return m.getSecretValueResult, nil
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockUpdateClient) UpdateSecret(_ context.Context, _ *secretapi.UpdateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.UpdateSecretOutput, error) {
	if m.updateSecretErr != nil {
		return nil, m.updateSecretErr
	}

	return m.updateSecretResult, nil
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockUpdateClient) PutSecretValue(_ context.Context, _ *secretapi.PutSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.PutSecretValueOutput, error) {
	if m.putSecretValueErr != nil {
		return nil, m.putSecretValueErr
	}

	return m.putSecretValueResult, nil
}

func TestUpdateUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			SecretString: lo.ToPtr("current-value"),
		},
	}

	uc := &secret.UpdateUseCase{Client: client}

	value, err := uc.GetCurrentValue(t.Context(), "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "current-value", value)
}

func TestUpdateUseCase_GetCurrentValue_Error(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretValueErr: errors.New("aws error"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.GetCurrentValue(t.Context(), "my-secret")
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

	output, err := uc.Execute(t.Context(), secret.UpdateInput{
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

	output, err := uc.Execute(t.Context(), secret.UpdateInput{
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

	output, err := uc.Execute(t.Context(), secret.UpdateInput{
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

	_, err := uc.Execute(t.Context(), secret.UpdateInput{
		Name:  "my-secret",
		Value: "new-value",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update secret value")
}

func TestUpdateUseCase_Execute_UpdateDescriptionError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		updateSecretErr: errors.New("update failed"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.UpdateInput{
		Name:        "my-secret",
		Description: "new description",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update secret description")
}
