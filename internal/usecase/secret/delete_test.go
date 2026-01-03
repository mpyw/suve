package secret_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/secretapi"
	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockDeleteClient struct {
	getSecretValueResult *secretapi.GetSecretValueOutput
	getSecretValueErr    error
	deleteSecretResult   *secretapi.DeleteSecretOutput
	deleteSecretErr      error
}

func (m *mockDeleteClient) GetSecretValue(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueErr != nil {
		return nil, m.getSecretValueErr
	}
	return m.getSecretValueResult, nil
}

func (m *mockDeleteClient) DeleteSecret(_ context.Context, _ *secretapi.DeleteSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DeleteSecretOutput, error) {
	if m.deleteSecretErr != nil {
		return nil, m.deleteSecretErr
	}
	return m.deleteSecretResult, nil
}

func TestDeleteUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			SecretString: lo.ToPtr("current-value"),
		},
	}

	uc := &secret.DeleteUseCase{Client: client}

	value, err := uc.GetCurrentValue(context.Background(), "my-secret")
	require.NoError(t, err)
	assert.Equal(t, "current-value", value)
}

func TestDeleteUseCase_GetCurrentValue_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getSecretValueErr: &secretapi.ResourceNotFoundException{Message: lo.ToPtr("not found")},
	}

	uc := &secret.DeleteUseCase{Client: client}

	value, err := uc.GetCurrentValue(context.Background(), "not-exists")
	require.NoError(t, err)
	assert.Empty(t, value)
}

func TestDeleteUseCase_GetCurrentValue_Error(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getSecretValueErr: errors.New("aws error"),
	}

	uc := &secret.DeleteUseCase{Client: client}

	_, err := uc.GetCurrentValue(context.Background(), "my-secret")
	assert.Error(t, err)
}

func TestDeleteUseCase_Execute(t *testing.T) {
	t.Parallel()

	deletionDate := time.Now().Add(7 * 24 * time.Hour)
	client := &mockDeleteClient{
		deleteSecretResult: &secretapi.DeleteSecretOutput{
			Name:         lo.ToPtr("my-secret"),
			DeletionDate: &deletionDate,
			ARN:          lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.DeleteUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.DeleteInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.NotNil(t, output.DeletionDate)
}

func TestDeleteUseCase_Execute_Force(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		deleteSecretResult: &secretapi.DeleteSecretOutput{
			Name: lo.ToPtr("my-secret"),
			ARN:  lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.DeleteUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.DeleteInput{
		Name:  "my-secret",
		Force: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
}

func TestDeleteUseCase_Execute_RecoveryWindow(t *testing.T) {
	t.Parallel()

	deletionDate := time.Now().Add(30 * 24 * time.Hour)
	client := &mockDeleteClient{
		deleteSecretResult: &secretapi.DeleteSecretOutput{
			Name:         lo.ToPtr("my-secret"),
			DeletionDate: &deletionDate,
			ARN:          lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.DeleteUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.DeleteInput{
		Name:           "my-secret",
		RecoveryWindow: 30,
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.NotNil(t, output.DeletionDate)
}

func TestDeleteUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		deleteSecretErr: errors.New("delete failed"),
	}

	uc := &secret.DeleteUseCase{Client: client}

	_, err := uc.Execute(context.Background(), secret.DeleteInput{
		Name: "my-secret",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete secret")
}
