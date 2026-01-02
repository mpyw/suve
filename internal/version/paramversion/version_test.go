package paramversion_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/version/paramversion"
)

type mockClient struct {
	getParameterFunc        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParameterHistoryFunc func(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error)
}

func (m *mockClient) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockClient) GetParameterHistory(ctx context.Context, params *ssm.GetParameterHistoryInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

func TestGetParameterWithVersion_Latest(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			assert.Equal(t, "/my/param", lo.FromPtr(params.Name))
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:             lo.ToPtr("/my/param"),
					Value:            lo.ToPtr("test-value"),
					Version:          3,
					Type:             types.ParameterTypeString,
					LastModifiedDate: &now,
				},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param"}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.NoError(t, err)
	assert.Equal(t, "/my/param", lo.FromPtr(result.Name))
	assert.Equal(t, "test-value", lo.FromPtr(result.Value))
	assert.Equal(t, int64(3), result.Version)
}

func TestGetParameterWithVersion_SpecificVersion(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			assert.Equal(t, "/my/param:2", lo.FromPtr(params.Name))
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    lo.ToPtr("/my/param"),
					Value:   lo.ToPtr("old-value"),
					Version: 2,
					Type:    types.ParameterTypeString,
				},
			}, nil
		},
	}

	v := int64(2)
	spec := &paramversion.Spec{Name: "/my/param", Absolute: paramversion.AbsoluteSpec{Version: &v}}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.NoError(t, err)
	assert.Equal(t, "old-value", lo.FromPtr(result.Value))
	assert.Equal(t, int64(2), result.Version)
}

func TestGetParameterWithVersion_Shift(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, params *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			assert.Equal(t, "/my/param", lo.FromPtr(params.Name))
			// History is returned oldest first by AWS
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
					{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
					{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: &now},
				},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param", Shift: 1}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.NoError(t, err)
	// Shift 1 means one version back from latest (v3), so v2
	assert.Equal(t, "v2", lo.FromPtr(result.Value))
	assert.Equal(t, int64(2), result.Version)
}

func TestGetParameterWithVersion_ShiftFromSpecificVersion(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: lo.ToPtr(now.Add(-2 * time.Hour))},
					{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v2"), Version: 2, LastModifiedDate: lo.ToPtr(now.Add(-time.Hour))},
					{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v3"), Version: 3, LastModifiedDate: &now},
				},
			}, nil
		},
	}

	v := int64(3)
	spec := &paramversion.Spec{Name: "/my/param", Absolute: paramversion.AbsoluteSpec{Version: &v}, Shift: 2}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.NoError(t, err)
	// Version 3, shift 2 means v3 -> v2 -> v1
	assert.Equal(t, "v1", lo.FromPtr(result.Value))
}

func TestGetParameterWithVersion_ShiftOutOfRange(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: &now},
				},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param", Shift: 5}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.Error(t, err)
	assert.Equal(t, "version shift out of range: ~5", err.Error())
}

func TestGetParameterWithVersion_VersionNotFound(t *testing.T) {
	t.Parallel()
	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{
					{Name: lo.ToPtr("/my/param"), Value: lo.ToPtr("v1"), Version: 1, LastModifiedDate: &now},
				},
			}, nil
		},
	}

	v := int64(99)
	spec := &paramversion.Spec{Name: "/my/param", Absolute: paramversion.AbsoluteSpec{Version: &v}, Shift: 1}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.Error(t, err)
	assert.Equal(t, "version 99 not found", err.Error())
}

func TestGetParameterWithVersion_EmptyHistory(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return &ssm.GetParameterHistoryOutput{
				Parameters: []types.ParameterHistory{},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param", Shift: 1}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.Error(t, err)
	assert.Equal(t, "parameter not found: /my/param", err.Error())
}

func TestGetParameterWithVersion_GetParameterError(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &paramversion.Spec{Name: "/my/param"}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.Error(t, err)
	assert.Equal(t, "failed to get parameter: AWS error", err.Error())
}

func TestGetParameterWithVersion_GetParameterHistoryError(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ *ssm.GetParameterHistoryInput, _ ...func(*ssm.Options)) (*ssm.GetParameterHistoryOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &paramversion.Spec{Name: "/my/param", Shift: 1}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.Error(t, err)
	assert.Equal(t, "failed to get parameter history: AWS error", err.Error())
}

func TestGetParameterWithVersion_DecryptFlag(t *testing.T) {
	t.Parallel()
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			assert.True(t, lo.FromPtr(params.WithDecryption))
			return &ssm.GetParameterOutput{
				Parameter: &types.Parameter{
					Name:    lo.ToPtr("/my/param"),
					Value:   lo.ToPtr("decrypted-value"),
					Version: 1,
					Type:    types.ParameterTypeSecureString,
				},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param"}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec, true)

	require.NoError(t, err)
	assert.Equal(t, "decrypted-value", lo.FromPtr(result.Value))
}
