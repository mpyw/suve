package secret_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockDeleteClient struct {
	getSecretResult    *model.Secret
	getSecretErr       error
	deleteSecretResult *model.SecretDeleteResult
	deleteSecretErr    error
}

func (m *mockDeleteClient) GetSecret(_ context.Context, _ string, _ string, _ string) (*model.Secret, error) {
	if m.getSecretErr != nil {
		return nil, m.getSecretErr
	}

	return m.getSecretResult, nil
}

func (m *mockDeleteClient) DeleteSecret(_ context.Context, _ string, _ bool) (*model.SecretDeleteResult, error) {
	if m.deleteSecretErr != nil {
		return nil, m.deleteSecretErr
	}

	return m.deleteSecretResult, nil
}

func TestDeleteUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getSecretResult: &model.Secret{
			Name:  "my-secret",
			Value: "current-value",
		},
	}

	uc := &secret.DeleteUseCase{Client: client}

	value, err := uc.GetCurrentValue(t.Context(), "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "current-value", value)
}

func TestDeleteUseCase_GetCurrentValue_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getSecretErr: errors.New("not found"),
	}

	uc := &secret.DeleteUseCase{Client: client}

	value, err := uc.GetCurrentValue(t.Context(), "not-exists")
	require.NoError(t, err) // GetCurrentValue treats errors as "not found"
	assert.Empty(t, value)
}

func TestDeleteUseCase_Execute(t *testing.T) {
	t.Parallel()

	deletionDate := time.Now().Add(7 * 24 * time.Hour)
	client := &mockDeleteClient{
		deleteSecretResult: &model.SecretDeleteResult{
			Name:         "my-secret",
			DeletionDate: &deletionDate,
			ARN:          "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
		},
	}

	uc := &secret.DeleteUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.DeleteInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.NotNil(t, output.DeletionDate)
}

func TestDeleteUseCase_Execute_Force(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		deleteSecretResult: &model.SecretDeleteResult{
			Name: "my-secret",
			ARN:  "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
		},
	}

	uc := &secret.DeleteUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.DeleteInput{
		Name:  "my-secret",
		Force: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
}

func TestDeleteUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		deleteSecretErr: errors.New("delete failed"),
	}

	uc := &secret.DeleteUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.DeleteInput{
		Name: "my-secret",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete secret")
}
