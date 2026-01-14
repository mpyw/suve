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
	"github.com/mpyw/suve/internal/version/secretversion"
)

type mockShowClient struct {
	getSecretValueResult *secretapi.GetSecretValueOutput
	getSecretValueErr    error
	listVersionsResult   *secretapi.ListSecretVersionIDsOutput
	listVersionsErr      error
	describeSecretResult *secretapi.DescribeSecretOutput
	describeSecretErr    error
}

func (m *mockShowClient) GetSecretValue(_ context.Context, _ *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	if m.getSecretValueErr != nil {
		return nil, m.getSecretValueErr
	}

	return m.getSecretValueResult, nil
}

//nolint:revive,stylecheck // Method name must match AWS SDK interface
func (m *mockShowClient) ListSecretVersionIds(_ context.Context, _ *secretapi.ListSecretVersionIDsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretVersionIDsOutput, error) {
	if m.listVersionsErr != nil {
		return nil, m.listVersionsErr
	}

	return m.listVersionsResult, nil
}

func (m *mockShowClient) DescribeSecret(_ context.Context, _ *secretapi.DescribeSecretInput, _ ...func(*secretapi.Options)) (*secretapi.DescribeSecretOutput, error) {
	if m.describeSecretErr != nil {
		return nil, m.describeSecretErr
	}

	if m.describeSecretResult != nil {
		return m.describeSecretResult, nil
	}

	return &secretapi.DescribeSecretOutput{}, nil
}

func TestShowUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockShowClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			Name:          lo.ToPtr("my-secret"),
			SecretString:  lo.ToPtr("secret-value"),
			VersionId:     lo.ToPtr("abc123"),
			VersionStages: []string{"AWSCURRENT"},
			CreatedDate:   &now,
		},
	}

	uc := &secret.ShowUseCase{Client: client}

	spec, err := secretversion.Parse("my-secret")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), secret.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "secret-value", output.Value)
	assert.Equal(t, "abc123", output.VersionID)
	assert.Contains(t, output.VersionStage, "AWSCURRENT")
	assert.NotNil(t, output.CreatedDate)
}

func TestShowUseCase_Execute_WithVersionID(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			Name:          lo.ToPtr("my-secret"),
			SecretString:  lo.ToPtr("old-value"),
			VersionId:     lo.ToPtr("old-version-id"),
			VersionStages: []string{"AWSPREVIOUS"},
		},
	}

	uc := &secret.ShowUseCase{Client: client}

	spec, err := secretversion.Parse("my-secret#old-version-id")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), secret.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "old-value", output.Value)
	assert.Equal(t, "old-version-id", output.VersionID)
}

func TestShowUseCase_Execute_WithLabel(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			Name:          lo.ToPtr("my-secret"),
			SecretString:  lo.ToPtr("current-value"),
			VersionId:     lo.ToPtr("current-id"),
			VersionStages: []string{"AWSCURRENT"},
		},
	}

	uc := &secret.ShowUseCase{Client: client}

	spec, err := secretversion.Parse("my-secret:AWSCURRENT")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), secret.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Equal(t, "current-value", output.Value)
}

func TestShowUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getSecretValueErr: errors.New("aws error"),
	}

	uc := &secret.ShowUseCase{Client: client}

	spec, err := secretversion.Parse("my-secret")
	require.NoError(t, err)

	_, err = uc.Execute(t.Context(), secret.ShowInput{
		Spec: spec,
	})
	assert.Error(t, err)
}

func TestShowUseCase_Execute_NoCreatedDate(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			Name:         lo.ToPtr("my-secret"),
			SecretString: lo.ToPtr("secret-value"),
			VersionId:    lo.ToPtr("abc123"),
		},
	}

	uc := &secret.ShowUseCase{Client: client}

	spec, err := secretversion.Parse("my-secret")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), secret.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Nil(t, output.CreatedDate)
}

func TestShowUseCase_Execute_WithShift(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockShowClient{
		listVersionsResult: &secretapi.ListSecretVersionIDsOutput{
			Versions: []secretapi.SecretVersionsListEntry{
				{VersionId: lo.ToPtr("v1-id"), CreatedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
				{VersionId: lo.ToPtr("v2-id"), CreatedDate: lo.ToPtr(now.Add(-1 * time.Hour))},
				{VersionId: lo.ToPtr("v3-id"), CreatedDate: lo.ToPtr(now)},
			},
		},
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			Name:         lo.ToPtr("my-secret"),
			SecretString: lo.ToPtr("v2-value"),
			VersionId:    lo.ToPtr("v2-id"),
		},
	}

	uc := &secret.ShowUseCase{Client: client}

	spec, err := secretversion.Parse("my-secret~1") // 1 version back from latest
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), secret.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Equal(t, "v2-id", output.VersionID)
	assert.Equal(t, "v2-value", output.Value)
}

func TestShowUseCase_Execute_WithTags(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getSecretValueResult: &secretapi.GetSecretValueOutput{
			Name:         lo.ToPtr("my-secret"),
			SecretString: lo.ToPtr("secret-value"),
			VersionId:    lo.ToPtr("abc123"),
		},
		describeSecretResult: &secretapi.DescribeSecretOutput{
			Tags: []secretapi.Tag{
				{Key: lo.ToPtr("env"), Value: lo.ToPtr("prod")},
				{Key: lo.ToPtr("team"), Value: lo.ToPtr("backend")},
			},
		},
	}

	uc := &secret.ShowUseCase{Client: client}

	spec, err := secretversion.Parse("my-secret")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), secret.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Len(t, output.Tags, 2)
	assert.Equal(t, "env", output.Tags[0].Key)
	assert.Equal(t, "prod", output.Tags[0].Value)
	assert.Equal(t, "team", output.Tags[1].Key)
	assert.Equal(t, "backend", output.Tags[1].Value)
}
