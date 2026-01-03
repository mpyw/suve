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
	describeErr       error
	getParameterValue map[string]string
	getParameterErr   map[string]error
}

func (m *mockListClient) DescribeParameters(_ context.Context, _ *paramapi.DescribeParametersInput, _ ...func(*paramapi.Options)) (*paramapi.DescribeParametersOutput, error) {
	if m.describeErr != nil {
		return nil, m.describeErr
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
