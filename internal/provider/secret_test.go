package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
)

// mockTypedSecretReader implements provider.TypedSecretReader for testing.
type mockTypedSecretReader struct {
	getTypedSecretFunc func(ctx context.Context, name, versionID, versionStage string) (*model.TypedSecret[model.AWSSecretMeta], error)
}

func (m *mockTypedSecretReader) GetTypedSecret(
	ctx context.Context, name, versionID, versionStage string,
) (*model.TypedSecret[model.AWSSecretMeta], error) {
	return m.getTypedSecretFunc(ctx, name, versionID, versionStage)
}

func TestWrapTypedSecretReader_GetSecret(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mock := &mockTypedSecretReader{
			getTypedSecretFunc: func(_ context.Context, name, versionID, _ string) (*model.TypedSecret[model.AWSSecretMeta], error) {
				return &model.TypedSecret[model.AWSSecretMeta]{
					Name:      name,
					Value:     "test-value",
					VersionID: versionID,
					Metadata: model.AWSSecretMeta{
						VersionStages: []string{"AWSCURRENT"},
					},
				}, nil
			},
		}

		reader := provider.WrapTypedSecretReader[model.AWSSecretMeta](mock)
		secret, err := reader.GetSecret(context.Background(), "test-secret", "v1", "AWSCURRENT")

		require.NoError(t, err)
		assert.Equal(t, "test-secret", secret.Name)
		assert.Equal(t, "test-value", secret.Value)
		assert.Equal(t, "v1", secret.VersionID)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		mock := &mockTypedSecretReader{
			getTypedSecretFunc: func(_ context.Context, _, _, _ string) (*model.TypedSecret[model.AWSSecretMeta], error) {
				return nil, errors.New("not found")
			},
		}

		reader := provider.WrapTypedSecretReader[model.AWSSecretMeta](mock)
		_, err := reader.GetSecret(context.Background(), "test-secret", "v1", "AWSCURRENT")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}
