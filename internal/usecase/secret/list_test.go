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

type mockListClient struct {
	listSecretsResult []*model.SecretListItem
	listSecretsErr    error
	getSecretValue    map[string]string
	getSecretErr      map[string]error
}

func (m *mockListClient) ListSecrets(_ context.Context) ([]*model.SecretListItem, error) {
	if m.listSecretsErr != nil {
		return nil, m.listSecretsErr
	}

	return m.listSecretsResult, nil
}

func (m *mockListClient) GetSecret(_ context.Context, name, _, _ string) (*model.Secret, error) {
	if m.getSecretErr != nil {
		if err, ok := m.getSecretErr[name]; ok {
			return nil, err
		}
	}

	if m.getSecretValue != nil {
		if value, ok := m.getSecretValue[name]; ok {
			return &model.Secret{Name: name, Value: value}, nil
		}
	}

	return nil, errors.New("not found")
}

func (m *mockListClient) GetSecretVersions(_ context.Context, _ string) ([]*model.SecretVersion, error) {
	return nil, errors.New("not implemented")
}

func TestListUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestListUseCase_Execute_WithSecrets(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "secret-a"},
			{Name: "secret-b"},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, "secret-a", output.Entries[0].Name)
	assert.Equal(t, "secret-b", output.Entries[1].Name)
}

func TestListUseCase_Execute_WithPrefix(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "app/config"},
			{Name: "app/secret"},
			{Name: "other/config"},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		Prefix: "app/",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_WithFilter(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "config-a"},
			{Name: "secret-b"},
			{Name: "config-c"},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		Filter: "config",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_InvalidFilter(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{},
	}

	uc := &secret.ListUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.ListInput{
		Filter: "[invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter regex")
}

func TestListUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsErr: errors.New("aws error"),
	}

	uc := &secret.ListUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.ListInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secrets")
}

func TestListUseCase_Execute_WithValue(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "secret-a"},
			{Name: "secret-b"},
		},
		getSecretValue: map[string]string{
			"secret-a": "value-a",
			"secret-b": "value-b",
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	for _, entry := range output.Entries {
		assert.NotNil(t, entry.Value)
		assert.NoError(t, entry.Error)
	}
}

func TestListUseCase_Execute_WithValue_PartialError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "secret-a"},
			{Name: "secret-error"},
		},
		getSecretValue: map[string]string{
			"secret-a": "value-a",
		},
		getSecretErr: map[string]error{
			"secret-error": errors.New("fetch error"),
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
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

func TestListUseCase_Execute_WithPagination(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "secret-1"},
			{Name: "secret-2"},
			{Name: "secret-3"},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		MaxResults: 2,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.NotEmpty(t, output.NextToken)
}

func TestListUseCase_Execute_WithPagination_ContinueToken(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "secret-1"},
			{Name: "secret-2"},
			{Name: "secret-3"},
			{Name: "secret-4"},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		MaxResults: 5,
		NextToken:  "secret-2",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2) // secret-3, secret-4
	assert.Empty(t, output.NextToken)
}

func TestListUseCase_Execute_WithPagination_FilterApplied(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "config-1"},
			{Name: "secret-1"},
			{Name: "config-2"},
			{Name: "config-3"},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		MaxResults: 2,
		Filter:     "config",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	for _, entry := range output.Entries {
		assert.Contains(t, entry.Name, "config")
	}
}

func TestListUseCase_Execute_WithPagination_Error(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsErr: errors.New("aws error"),
	}

	uc := &secret.ListUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.ListInput{
		MaxResults: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secrets")
}

func TestListUseCase_Execute_WithPagination_TrimResults(t *testing.T) {
	t.Parallel()

	// Return more results than requested to trigger trimming
	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "secret-1"},
			{Name: "secret-2"},
			{Name: "secret-3"},
			{Name: "secret-4"},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		MaxResults: 2,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, "secret-1", output.Entries[0].Name)
	assert.Equal(t, "secret-2", output.Entries[1].Name)
}

func TestListUseCase_Execute_NilSecretsList(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: nil,
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestListUseCase_Execute_WithValue_AllErrors(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: []*model.SecretListItem{
			{Name: "secret-a"},
			{Name: "secret-b"},
		},
		getSecretErr: map[string]error{
			"secret-a": errors.New("error-a"),
			"secret-b": errors.New("error-b"),
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	for _, entry := range output.Entries {
		assert.Nil(t, entry.Value)
		assert.Error(t, entry.Error)
	}
}
