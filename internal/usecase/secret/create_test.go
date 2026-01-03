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

type mockCreateClient struct {
	createResult *secretapi.CreateSecretOutput
	createErr    error
}

func (m *mockCreateClient) CreateSecret(_ context.Context, _ *secretapi.CreateSecretInput, _ ...func(*secretapi.Options)) (*secretapi.CreateSecretOutput, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.createResult, nil
}

func TestCreateUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		createResult: &secretapi.CreateSecretOutput{
			Name:      lo.ToPtr("my-secret"),
			VersionId: lo.ToPtr("abc123"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.CreateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.CreateInput{
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
		createResult: &secretapi.CreateSecretOutput{
			Name:      lo.ToPtr("my-secret"),
			VersionId: lo.ToPtr("abc123"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.CreateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.CreateInput{
		Name:        "my-secret",
		Value:       "secret-value",
		Description: "my description",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
}

func TestCreateUseCase_Execute_WithTags(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		createResult: &secretapi.CreateSecretOutput{
			Name:      lo.ToPtr("my-secret"),
			VersionId: lo.ToPtr("abc123"),
			ARN:       lo.ToPtr("arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret"),
		},
	}

	uc := &secret.CreateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.CreateInput{
		Name:  "my-secret",
		Value: "secret-value",
		Tags: map[string]string{
			"env":     "prod",
			"project": "test",
		},
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

	_, err := uc.Execute(context.Background(), secret.CreateInput{
		Name:  "my-secret",
		Value: "secret-value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create secret")
}
