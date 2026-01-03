package param_test

import (
	"context"
	"errors"
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
	getParameterErr   map[string]error
}

func (m *mockListClient) DescribeParameters(_ context.Context, input *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
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

func (m *mockListClient) GetParameter(_ context.Context, input *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	name := lo.FromPtr(input.Name)
	if m.getParameterErr != nil {
		if err, ok := m.getParameterErr[name]; ok {
			return nil, err
		}
	}
	if m.getParameterValue != nil {
		if value, ok := m.getParameterValue[name]; ok {
			return &paramapi.GetParameterOutput{
				Parameter: &paramapi.Parameter{Value: lo.ToPtr(value)},
			}, nil
		}
	}
	return nil, &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")}
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

	for _, entry := range output.Entries {
		assert.NotNil(t, entry.Value)
		assert.Nil(t, entry.Error)
	}
}

func TestListUseCase_Execute_WithValue_PartialError(t *testing.T) {
	t.Parallel()

	client := &mockListClient{
		describeResult: &paramapi.DescribeParametersOutput{
			Parameters: []paramapi.ParameterMetadata{
				{Name: lo.ToPtr("/app/config")},
				{Name: lo.ToPtr("/app/error")},
			},
		},
		getParameterValue: map[string]string{
			"/app/config": "config-value",
		},
		getParameterErr: map[string]error{
			"/app/error": errors.New("fetch error"),
		},
	}

	uc := &param.ListUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.ListInput{
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
