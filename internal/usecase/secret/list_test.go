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

type mockListClient struct {
	listSecretsResult   *secretapi.ListSecretsOutput
	listSecretsErr      error
	getSecretValueValue map[string]string
	getSecretValueErr   map[string]error
}

func (m *mockListClient) ListSecrets(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
	if m.listSecretsErr != nil {
		return nil, m.listSecretsErr
	}
	return m.listSecretsResult, nil
}

func (m *mockListClient) GetSecretValue(_ context.Context, input *secretapi.GetSecretValueInput, _ ...func(*secretapi.Options)) (*secretapi.GetSecretValueOutput, error) {
	name := lo.FromPtr(input.SecretId)
	if m.getSecretValueErr != nil {
		if err, ok := m.getSecretValueErr[name]; ok {
			return nil, err
		}
	}
	if m.getSecretValueValue != nil {
		if value, ok := m.getSecretValueValue[name]; ok {
			return &secretapi.GetSecretValueOutput{SecretString: lo.ToPtr(value)}, nil
		}
	}
	return nil, &secretapi.ResourceNotFoundException{Message: lo.ToPtr("not found")}
}

func TestListUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: &secretapi.ListSecretsOutput{
			SecretList: []secretapi.SecretListEntry{},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.ListInput{})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestListUseCase_Execute_WithSecrets(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: &secretapi.ListSecretsOutput{
			SecretList: []secretapi.SecretListEntry{
				{Name: lo.ToPtr("secret-a")},
				{Name: lo.ToPtr("secret-b")},
			},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.ListInput{})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, "secret-a", output.Entries[0].Name)
	assert.Equal(t, "secret-b", output.Entries[1].Name)
}

func TestListUseCase_Execute_WithFilter(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: &secretapi.ListSecretsOutput{
			SecretList: []secretapi.SecretListEntry{
				{Name: lo.ToPtr("config-a")},
				{Name: lo.ToPtr("secret-b")},
				{Name: lo.ToPtr("config-c")},
			},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.ListInput{
		Filter: "config",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_InvalidFilter(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: &secretapi.ListSecretsOutput{},
	}

	uc := &secret.ListUseCase{Client: client}

	_, err := uc.Execute(context.Background(), secret.ListInput{
		Filter: "[invalid",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter regex")
}

func TestListUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsErr: errors.New("aws error"),
	}

	uc := &secret.ListUseCase{Client: client}

	_, err := uc.Execute(context.Background(), secret.ListInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secrets")
}

func TestListUseCase_Execute_WithValue(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: &secretapi.ListSecretsOutput{
			SecretList: []secretapi.SecretListEntry{
				{Name: lo.ToPtr("secret-a")},
				{Name: lo.ToPtr("secret-b")},
			},
		},
		getSecretValueValue: map[string]string{
			"secret-a": "value-a",
			"secret-b": "value-b",
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	for _, entry := range output.Entries {
		assert.NotNil(t, entry.Value)
		assert.Nil(t, entry.Error)
	}
}

func TestListUseCase_Execute_WithValue_PartialError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: &secretapi.ListSecretsOutput{
			SecretList: []secretapi.SecretListEntry{
				{Name: lo.ToPtr("secret-a")},
				{Name: lo.ToPtr("secret-error")},
			},
		},
		getSecretValueValue: map[string]string{
			"secret-a": "value-a",
		},
		getSecretValueErr: map[string]error{
			"secret-error": errors.New("fetch error"),
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), secret.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	var hasValue, hasError bool
	for _, entry := range output.Entries {
		if entry.Value != nil {
			hasValue = true
		}
		if entry.Error != nil {
			hasError = true
		}
	}
	assert.True(t, hasValue)
	assert.True(t, hasError)
}
