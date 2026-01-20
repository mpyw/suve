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

type mockCreateClient struct {
	createResult *model.SecretWriteResult
	createErr    error
}

func (m *mockCreateClient) CreateSecret(_ context.Context, _ *model.Secret) (*model.SecretWriteResult, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}

	return m.createResult, nil
}

func TestCreateUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		createResult: &model.SecretWriteResult{
			Name:    "my-secret",
			Version: "abc123",
			ARN:     "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
		},
	}

	uc := &secret.CreateUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.CreateInput{
		Name:  "my-secret",
		Value: "secret-value",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "abc123", output.VersionID)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret", output.ARN)
}

func TestCreateUseCase_Execute_WithDescription(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		createResult: &model.SecretWriteResult{
			Name:    "my-secret",
			Version: "abc123",
			ARN:     "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
		},
	}

	uc := &secret.CreateUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.CreateInput{
		Name:        "my-secret",
		Value:       "secret-value",
		Description: "my description",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
}

func TestCreateUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		createErr: errors.New("aws error"),
	}

	uc := &secret.CreateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.CreateInput{
		Name:  "my-secret",
		Value: "secret-value",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create secret")
}
