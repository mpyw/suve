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
	listSecretsResult    *secretapi.ListSecretsOutput
	listSecretsResults   []*secretapi.ListSecretsOutput // For pagination tests
	listSecretsCallCount int
	listSecretsErr       error
	getSecretValueValue  map[string]string
	getSecretValueErr    map[string]error
}

func (m *mockListClient) ListSecrets(_ context.Context, _ *secretapi.ListSecretsInput, _ ...func(*secretapi.Options)) (*secretapi.ListSecretsOutput, error) {
	if m.listSecretsErr != nil {
		return nil, m.listSecretsErr
	}
	// Support paginated results for testing
	if len(m.listSecretsResults) > 0 {
		idx := m.listSecretsCallCount
		m.listSecretsCallCount++
		if idx < len(m.listSecretsResults) {
			return m.listSecretsResults[idx], nil
		}
		return &secretapi.ListSecretsOutput{}, nil
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

	output, err := uc.Execute(t.Context(), secret.ListInput{})
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

	output, err := uc.Execute(t.Context(), secret.ListInput{})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, "secret-a", output.Entries[0].Name)
	assert.Equal(t, "secret-b", output.Entries[1].Name)
}

func TestListUseCase_Execute_WithPrefix(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResult: &secretapi.ListSecretsOutput{
			SecretList: []secretapi.SecretListEntry{
				{Name: lo.ToPtr("app/config")},
				{Name: lo.ToPtr("app/secret")},
			},
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
		listSecretsResult: &secretapi.ListSecretsOutput{
			SecretList: []secretapi.SecretListEntry{
				{Name: lo.ToPtr("config-a")},
				{Name: lo.ToPtr("secret-b")},
				{Name: lo.ToPtr("config-c")},
			},
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
		listSecretsResult: &secretapi.ListSecretsOutput{},
	}

	uc := &secret.ListUseCase{Client: client}

	_, err := uc.Execute(t.Context(), secret.ListInput{
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

	_, err := uc.Execute(t.Context(), secret.ListInput{})
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

	output, err := uc.Execute(t.Context(), secret.ListInput{
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
		listSecretsResults: []*secretapi.ListSecretsOutput{
			{
				SecretList: []secretapi.SecretListEntry{
					{Name: lo.ToPtr("secret-1")},
					{Name: lo.ToPtr("secret-2")},
				},
				NextToken: lo.ToPtr("token1"),
			},
			{
				SecretList: []secretapi.SecretListEntry{
					{Name: lo.ToPtr("secret-3")},
				},
			},
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
		listSecretsResults: []*secretapi.ListSecretsOutput{
			{
				SecretList: []secretapi.SecretListEntry{
					{Name: lo.ToPtr("secret-3")},
					{Name: lo.ToPtr("secret-4")},
				},
			},
		},
	}

	uc := &secret.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), secret.ListInput{
		MaxResults: 5,
		NextToken:  "token1",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.Empty(t, output.NextToken)
}

func TestListUseCase_Execute_WithPagination_FilterApplied(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listSecretsResults: []*secretapi.ListSecretsOutput{
			{
				SecretList: []secretapi.SecretListEntry{
					{Name: lo.ToPtr("config-1")},
					{Name: lo.ToPtr("secret-1")},
					{Name: lo.ToPtr("config-2")},
				},
				NextToken: lo.ToPtr("token1"),
			},
			{
				SecretList: []secretapi.SecretListEntry{
					{Name: lo.ToPtr("config-3")},
				},
			},
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list secrets")
}

func TestListUseCase_Execute_WithPagination_TrimResults(t *testing.T) {
	t.Parallel()

	// Return more results than requested to trigger trimming
	client := &mockListClient{
		listSecretsResults: []*secretapi.ListSecretsOutput{
			{
				SecretList: []secretapi.SecretListEntry{
					{Name: lo.ToPtr("secret-1")},
					{Name: lo.ToPtr("secret-2")},
					{Name: lo.ToPtr("secret-3")},
					{Name: lo.ToPtr("secret-4")},
				},
				NextToken: lo.ToPtr("token1"),
			},
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
