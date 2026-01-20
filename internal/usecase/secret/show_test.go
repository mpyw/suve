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
	"github.com/mpyw/suve/internal/version/secretversion"
)

type mockShowClient struct {
	getSecretResult *model.Secret
	getSecretErr    error
	versionsResult  []*model.SecretVersion
	versionsErr     error
	describeResult  *model.SecretListItem
	describeErr     error
	tagsResult      map[string]string
	tagsErr         error
}

func (m *mockShowClient) GetSecret(_ context.Context, _, _, _ string) (*model.Secret, error) {
	if m.getSecretErr != nil {
		return nil, m.getSecretErr
	}

	return m.getSecretResult, nil
}

func (m *mockShowClient) GetSecretVersions(_ context.Context, _ string) ([]*model.SecretVersion, error) {
	if m.versionsErr != nil {
		return nil, m.versionsErr
	}

	return m.versionsResult, nil
}

func (m *mockShowClient) ListSecrets(_ context.Context) ([]*model.SecretListItem, error) {
	return nil, errors.New("not implemented")
}

func (m *mockShowClient) DescribeSecret(_ context.Context, _ string) (*model.SecretListItem, error) {
	if m.describeErr != nil {
		return nil, m.describeErr
	}

	return m.describeResult, nil
}

func (m *mockShowClient) GetTags(_ context.Context, _ string) (map[string]string, error) {
	if m.tagsErr != nil {
		return nil, m.tagsErr
	}

	return m.tagsResult, nil
}

func (m *mockShowClient) AddTags(_ context.Context, _ string, _ map[string]string) error {
	return errors.New("not implemented")
}

func (m *mockShowClient) RemoveTags(_ context.Context, _ string, _ []string) error {
	return errors.New("not implemented")
}

func TestShowUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockShowClient{
		getSecretResult: &model.Secret{
			Name:      "my-secret",
			Value:     "secret-value",
			Version:   "abc123",
			CreatedAt: &now,
			Metadata: model.AWSSecretMeta{
				VersionStages: []string{"AWSCURRENT"},
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
	assert.Equal(t, "my-secret", output.Name)
	assert.Equal(t, "secret-value", output.Value)
	assert.Equal(t, "abc123", output.VersionID)
	assert.Contains(t, output.VersionStage, "AWSCURRENT")
	assert.NotNil(t, output.CreatedDate)
}

func TestShowUseCase_Execute_WithVersionID(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getSecretResult: &model.Secret{
			Name:    "my-secret",
			Value:   "old-value",
			Version: "old-version-id",
			Metadata: model.AWSSecretMeta{
				VersionStages: []string{"AWSPREVIOUS"},
			},
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
		getSecretResult: &model.Secret{
			Name:    "my-secret",
			Value:   "current-value",
			Version: "current-id",
			Metadata: model.AWSSecretMeta{
				VersionStages: []string{"AWSCURRENT"},
			},
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
		getSecretErr: errors.New("aws error"),
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
		getSecretResult: &model.Secret{
			Name:    "my-secret",
			Value:   "secret-value",
			Version: "abc123",
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
		versionsResult: []*model.SecretVersion{
			{Version: "v1-id", CreatedAt: ptrTime(now.Add(-2 * time.Hour))},
			{Version: "v2-id", CreatedAt: ptrTime(now.Add(-1 * time.Hour))},
			{Version: "v3-id", CreatedAt: ptrTime(now)},
		},
		getSecretResult: &model.Secret{
			Name:    "my-secret",
			Value:   "v2-value",
			Version: "v2-id",
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
		getSecretResult: &model.Secret{
			Name:    "my-secret",
			Value:   "secret-value",
			Version: "abc123",
		},
		tagsResult: map[string]string{
			"env":  "prod",
			"team": "backend",
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
	// Tags are stored in a map so order may vary
	tagMap := make(map[string]string)
	for _, tag := range output.Tags {
		tagMap[tag.Key] = tag.Value
	}

	assert.Equal(t, "prod", tagMap["env"])
	assert.Equal(t, "backend", tagMap["team"])
}
