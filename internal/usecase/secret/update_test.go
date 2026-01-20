package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockUpdateClient struct {
	getSecretResult    *model.Secret
	getSecretErr       error
	updateSecretResult *model.SecretWriteResult
	updateSecretErr    error
}

func (m *mockUpdateClient) GetSecret(_ context.Context, _ string, _ string, _ string) (*model.Secret, error) {
	if m.getSecretErr != nil {
		return nil, m.getSecretErr
	}

	return m.getSecretResult, nil
}

func (m *mockUpdateClient) UpdateSecret(_ context.Context, _ string, _ string) (*model.SecretWriteResult, error) {
	if m.updateSecretErr != nil {
		return nil, m.updateSecretErr
	}

	return m.updateSecretResult, nil
}

func TestUpdateUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getSecretResult: &model.Secret{
			Name:  "my-secret",
			Value: "current-value",
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
		getSecretErr: errors.New("aws error"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.GetCurrentValue(t.Context(), "my-secret")
	assert.Error(t, err)
}

func TestUpdateUseCase_Execute_UpdateValue(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		updateSecretResult: &model.SecretWriteResult{
			Name:    "my-secret",
			Version: "new-version-id",
			ARN:     "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
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

func TestUpdateUseCase_Execute_UpdateValueError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		updateSecretErr: errors.New("update failed"),
	}

	uc := &secret.UpdateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.UpdateInput{
		Name:  "my-secret",
		Value: "new-value",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update secret")
}
