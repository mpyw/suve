package param_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockListClient struct {
	describeResult    *paramapi.DescribeParametersOutput
	describeResults   []*paramapi.DescribeParametersOutput // For pagination tests
	describeCallCount int
	describeErr       error
	getParameterValue map[string]string
	invalidParameters []string // Parameters that should be returned as invalid
	getParametersErr  error    // Error to return from GetParameters
}

func (m *mockListClient) DescribeParameters(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
	if m.describeErr != nil {
		return nil, m.describeErr
	}
	// Support paginated results for testing
	if len(m.describeResults) > 0 {
		idx := m.describeCallCount
		m.describeCallCount++
		if idx < len(m.describeResults) {
			return m.describeResults[idx], nil
		}
		return &paramapi.DescribeParametersOutput{}, nil
	}
	return m.describeResult, nil
}

func (m *mockListClient) GetParameters(_ context.Context, input *paramapi.GetParametersInput, _ ...func(*paramapi.Options)) (*paramapi.GetParametersOutput, error) {
	if m.getParametersErr != nil {
		return nil, m.getParametersErr
	}

	var params []paramapi.Parameter
	var invalidParams []string

	invalidSet := make(map[string]bool)
	for _, name := range m.invalidParameters {
		invalidSet[name] = true
	}

	for _, name := range input.Names {
		if invalidSet[name] {
			invalidParams = append(invalidParams, name)
			continue
		}
		if m.getParameterValue != nil {
			if value, ok := m.getParameterValue[name]; ok {
				params = append(params, paramapi.Parameter{
					Name:  lo.ToPtr(name),
					Value: lo.ToPtr(value),
				})
			}
		}
	}

	return &paramapi.GetParametersOutput{
		Parameters:        params,
		InvalidParameters: invalidParams,
	}, nil
}

func TestListUseCase_Execute_Empty(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}

func TestListUseCase_Execute_WithPrefix(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{
				{Name: lo.ToPtr("/app/config")},
				{Name: lo.ToPtr("/app/secret")},
			},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		Prefix: "/app",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_Recursive(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{
				{Name: lo.ToPtr("/app/config")},
				{Name: lo.ToPtr("/app/sub/nested")},
			},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		Prefix:    "/app",
		Recursive: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_WithFilter(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{
				{Name: lo.ToPtr("/app/config")},
				{Name: lo.ToPtr("/app/secret")},
				{Name: lo.ToPtr("/app/other")},
			},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		Filter: "config|secret",
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
}

func TestListUseCase_Execute_InvalidFilter(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{},
	}

	uc := &param.ListUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.ListInput{
		Filter: "[invalid",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter regex")
}

func TestListUseCase_Execute_DescribeError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeErr: errors.New("aws error"),
	}

	uc := &param.ListUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.ListInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to describe parameters")
}

func TestListUseCase_Execute_WithValue(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{
				{Name: lo.ToPtr("/app/config")},
				{Name: lo.ToPtr("/app/secret")},
			},
		},
		getParameterValue: map[string]string{
			"/app/config": "config-value",
			"/app/secret": "secret-value",
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	// Verify actual values are returned correctly
	valueMap := make(map[string]string)
	for _, entry := range output.Entries {
		require.NotNil(t, entry.Value, "entry %s should have value", entry.Name)
		assert.Nil(t, entry.Error, "entry %s should not have error", entry.Name)
		valueMap[entry.Name] = *entry.Value
	}
	assert.Equal(t, "config-value", valueMap["/app/config"])
	assert.Equal(t, "secret-value", valueMap["/app/secret"])
}

func TestListUseCase_Execute_WithValue_PartialError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{
				{Name: lo.ToPtr("/app/config")},
				{Name: lo.ToPtr("/app/invalid")},
			},
		},
		getParameterValue: map[string]string{
			"/app/config": "config-value",
		},
		invalidParameters: []string{"/app/invalid"},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
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
			assert.Nil(t, entry.Error)
		case "/app/invalid":
			assert.Nil(t, entry.Value)
			require.NotNil(t, entry.Error)
			assert.Contains(t, entry.Error.Error(), "parameter not found")
		default:
			t.Errorf("unexpected entry: %s", entry.Name)
		}
	}
}

func TestListUseCase_Execute_WithPagination(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResults: []*paramapi.DescribeParametersOutput{
			{
				Parameters: []paramapi.ParameterMetadata{
					{Name: lo.ToPtr("/app/p1")},
					{Name: lo.ToPtr("/app/p2")},
				},
				NextToken: lo.ToPtr("token1"),
			},
			{
				Parameters: []paramapi.ParameterMetadata{
					{Name: lo.ToPtr("/app/p3")},
				},
			},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		MaxResults: 2,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.NotEmpty(t, output.NextToken)
}

func TestListUseCase_Execute_WithPagination_ContinueToken(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResults: []*paramapi.DescribeParametersOutput{
			{
				Parameters: []paramapi.ParameterMetadata{
					{Name: lo.ToPtr("/app/p3")},
					{Name: lo.ToPtr("/app/p4")},
				},
			},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
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
		describeResults: []*paramapi.DescribeParametersOutput{
			{
				Parameters: []paramapi.ParameterMetadata{
					{Name: lo.ToPtr("/app/config1")},
					{Name: lo.ToPtr("/app/secret1")},
					{Name: lo.ToPtr("/app/config2")},
				},
				NextToken: lo.ToPtr("token1"),
			},
			{
				Parameters: []paramapi.ParameterMetadata{
					{Name: lo.ToPtr("/app/config3")},
				},
			},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
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
		describeErr: errors.New("aws error"),
	}

	uc := &param.ListUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.ListInput{
		MaxResults: 10,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to describe parameters")
}

func TestListUseCase_Execute_WithPagination_TrimResults(t *testing.T) {
	t.Parallel()

	// Return more results than requested to trigger trimming
	client := &mockListClient{
		describeResults: []*paramapi.DescribeParametersOutput{
			{
				Parameters: []paramapi.ParameterMetadata{
					{Name: lo.ToPtr("/app/p1")},
					{Name: lo.ToPtr("/app/p2")},
					{Name: lo.ToPtr("/app/p3")},
					{Name: lo.ToPtr("/app/p4")},
				},
				NextToken: lo.ToPtr("token1"),
			},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		MaxResults: 2,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)
	assert.Equal(t, "/app/p1", output.Entries[0].Name)
	assert.Equal(t, "/app/p2", output.Entries[1].Name)
}

func TestListUseCase_Execute_WithValue_GetParametersError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{
				{Name: lo.ToPtr("/app/param1")},
				{Name: lo.ToPtr("/app/param2")},
			},
		},
		getParametersErr: errors.New("access denied"),
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, 2)

	// All entries should have errors since GetParameters failed
	for _, entry := range output.Entries {
		assert.Nil(t, entry.Value)
		assert.NotNil(t, entry.Error)
		assert.Contains(t, entry.Error.Error(), "access denied")
	}
}

func TestListUseCase_Execute_WithValue_LargeBatch(t *testing.T) {
	t.Parallel()

	// Create 15 parameters to test batching (should split into 10 + 5)
	const numParams = 15
	metadata := make([]paramapi.ParameterMetadata, numParams)
	expectedValues := make(map[string]string, numParams)
	for i := range numParams {
		name := fmt.Sprintf("/app/param%d", i)
		metadata[i] = paramapi.ParameterMetadata{Name: lo.ToPtr(name)}
		expectedValues[name] = fmt.Sprintf("value%d", i)
	}

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: metadata,
		},
		getParameterValue: expectedValues,
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Len(t, output.Entries, numParams)

	// Verify all entries have correct values (ensures batching works correctly)
	for _, entry := range output.Entries {
		require.NotNil(t, entry.Value, "entry %s should have value", entry.Name)
		assert.Nil(t, entry.Error, "entry %s should not have error", entry.Name)
		assert.Equal(t, expectedValues[entry.Name], *entry.Value, "entry %s should have correct value", entry.Name)
	}
}

func TestListUseCase_Execute_WithValue_Empty(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{},
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
		WithValue: true,
	})
	require.NoError(t, err)
	assert.Empty(t, output.Entries)
}
