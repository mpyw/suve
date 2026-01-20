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

type mockRestoreClient struct {
	restoreResult *model.SecretRestoreResult
	restoreErr    error
}

func (m *mockRestoreClient) RestoreSecret(_ context.Context, _ string) (*model.SecretRestoreResult, error) {
	if m.restoreErr != nil {
		return nil, m.restoreErr
	}

	return m.restoreResult, nil
}

func TestRestoreUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockRestoreClient{
		restoreResult: &model.SecretRestoreResult{
			Name: "my-secret",
			ARN:  "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
		},
	}

	uc := &secret.RestoreUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.RestoreInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret", output.ARN)
}

func TestRestoreUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockRestoreClient{
		restoreErr: errors.New("restore failed"),
	}

	uc := &secret.RestoreUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.RestoreInput{
		Name: "my-secret",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to restore secret")
}
