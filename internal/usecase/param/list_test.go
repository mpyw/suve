package param_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockListClient struct {
	listParametersResult  []*model.ParameterListItem
	listParametersErr     error
	getParameterResult    map[string]*model.Parameter
	getParameterErr       map[string]error // Per-name errors
	getParameterGlobalErr error            // Global error for all GetParameter calls
}

func (m *mockListClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	if m.listParametersErr != nil {
		return nil, m.listParametersErr
	}

	return m.listParametersResult, nil
}

func (m *mockListClient) GetParameter(_ context.Context, name string, _ string) (*model.Parameter, error) {
	if m.getParameterGlobalErr != nil {
		return nil, m.getParameterGlobalErr
	}

	if m.getParameterErr != nil {
		if err, ok := m.getParameterErr[name]; ok {
			return nil, err
		}
	}

	if m.getParameterResult != nil {
		if p, ok := m.getParameterResult[name]; ok {
			return p, nil
		}
	}

	return nil, errors.New("parameter not found")
}

func (m *mockListClient) GetParameterHistory(_ context.Context, _ string) (*model.ParameterHistory, error) {
	return nil, errors.New("not implemented")
}

func TestListUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestListUseCase_Execute_WithPrefix(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/config", Metadata: model.AWSParameterListItemMeta{Type: "String"}},
			{Name: "/app/secret", Metadata: model.AWSParameterListItemMeta{Type: "SecureString"}},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		Prefix: "/app",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_Recursive(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/config"},
			{Name: "/app/sub/nested"},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		Prefix:    "/app",
		Recursive: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_WithFilter(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/config"},
			{Name: "/app/secret"},
			{Name: "/app/other"},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		Filter: "config|secret",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_InvalidFilter(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{},
	}

	uc := &param.ListUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.ListInput{
		Filter: "[invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter regex")
}

func TestListUseCase_Execute_ListError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersErr: errAWS,
	}

	uc := &param.ListUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.ListInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list parameters")
}

func TestListUseCase_Execute_WithValue(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/config", Metadata: model.AWSParameterListItemMeta{Type: "String"}},
			{Name: "/app/secret", Metadata: model.AWSParameterListItemMeta{Type: "SecureString"}},
		},
		getParameterResult: map[string]*model.Parameter{
			"/app/config": {Name: "/app/config", Value: "config-value"},
			"/app/secret": {Name: "/app/secret", Value: "secret-value"},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	// Verify actual values are returned correctly
	valueMap := make(map[string]string)

	for _, entry := range output.Entries {
		require.NotNil(t, entry.Value, "entry %s should have value", entry.Name)
		require.NoError(t, entry.Error, "entry %s should not have error", entry.Name)
		valueMap[entry.Name] = *entry.Value
	}

	assert.Equal(t, "config-value", valueMap["/app/config"])
	assert.Equal(t, "secret-value", valueMap["/app/secret"])
}

func TestListUseCase_Execute_WithValue_PartialError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/config"},
			{Name: "/app/invalid"},
		},
		getParameterResult: map[string]*model.Parameter{
			"/app/config": {Name: "/app/config", Value: "config-value"},
		},
		getParameterErr: map[string]error{
			"/app/invalid": errors.New("parameter not found"),
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	// Verify specific entries
	for _, entry := range output.Entries {
		switch entry.Name {
		case "/app/config":
			require.NotNil(t, entry.Value)
			assert.Equal(t, "config-value", *entry.Value)
			require.NoError(t, entry.Error)
		case "/app/invalid":
			assert.Nil(t, entry.Value)
			require.Error(t, entry.Error)
			assert.Contains(t, entry.Error.Error(), "failed to get parameter")
		default:
			t.Errorf("unexpected entry: %s", entry.Name)
		}
	}
}

func TestListUseCase_Execute_WithPagination(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/p1"},
			{Name: "/app/p2"},
			{Name: "/app/p3"},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		MaxResults: 2,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.NotEmpty(t, output.NextToken)
}

func TestListUseCase_Execute_WithPagination_ContinueToken(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/p1"},
			{Name: "/app/p2"},
			{Name: "/app/p3"},
			{Name: "/app/p4"},
		},
	}

	uc := &param.ListUseCase{Client: client}

	// Simulate continuation from /app/p2 (NextToken is the last item of previous page)
	output, err := uc.Execute(t.Context(), param.ListInput{
		MaxResults: 5,
		NextToken:  "/app/p2",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2) // p3 and p4
	assert.Empty(t, output.NextToken)
	assert.Equal(t, "/app/p3", output.Entries[0].Name)
	assert.Equal(t, "/app/p4", output.Entries[1].Name)
}

func TestListUseCase_Execute_WithPagination_FilterApplied(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/config1"},
			{Name: "/app/secret1"},
			{Name: "/app/config2"},
			{Name: "/app/config3"},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
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
		listParametersErr: errAWS,
	}

	uc := &param.ListUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.ListInput{
		MaxResults: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list parameters")
}

func TestListUseCase_Execute_WithPagination_TrimResults(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/p1"},
			{Name: "/app/p2"},
			{Name: "/app/p3"},
			{Name: "/app/p4"},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		MaxResults: 2,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, "/app/p1", output.Entries[0].Name)
	assert.Equal(t, "/app/p2", output.Entries[1].Name)
}

func TestListUseCase_Execute_WithValue_GetParameterError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{
			{Name: "/app/param1"},
			{Name: "/app/param2"},
		},
		getParameterGlobalErr: errAccessDenied,
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	// All entries should have errors since GetParameter failed
	for _, entry := range output.Entries {
		assert.Nil(t, entry.Value)
		require.Error(t, entry.Error)
		assert.Contains(t, entry.Error.Error(), "access denied")
	}
}

func TestListUseCase_Execute_WithValue_LargeBatch(t *testing.T) {
	t.Parallel()

	// Create 15 parameters to test parallel execution
	const numParams = 15

	items := make([]*model.ParameterListItem, numParams)
	paramResults := make(map[string]*model.Parameter, numParams)

	for i := range numParams {
		name := fmt.Sprintf("/app/param%d", i)
		items[i] = &model.ParameterListItem{Name: name}
		paramResults[name] = &model.Parameter{
			Name:  name,
			Value: fmt.Sprintf("value%d", i),
		}
	}

	client := &mockListClient{
		listParametersResult: items,
		getParameterResult:   paramResults,
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, numParams)

	// Verify all entries have correct values
	for _, entry := range output.Entries {
		require.NotNil(t, entry.Value, "entry %s should have value", entry.Name)
		require.NoError(t, entry.Error, "entry %s should not have error", entry.Name)

		expected := paramResults[entry.Name].Value
		assert.Equal(t, expected, lo.FromPtr(entry.Value), "entry %s should have correct value", entry.Name)
	}
}

func TestListUseCase_Execute_WithValue_Empty(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		listParametersResult: []*model.ParameterListItem{},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}
