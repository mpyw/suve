package paramversion_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/version/paramversion"
)

type mockClient struct {
	getParameterFunc        func(ctx context.Context, name string, version string) (*model.Parameter, error)
	getParameterHistoryFunc func(ctx context.Context, name string) (*model.ParameterHistory, error)
}

func (m *mockClient) GetParameter(ctx context.Context, name string, version string) (*model.Parameter, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, name, version)
	}

	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockClient) GetParameterHistory(ctx context.Context, name string) (*model.ParameterHistory, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, name)
	}

	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

func (m *mockClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	return nil, fmt.Errorf("ListParameters not mocked")
}

func TestGetParameterWithVersion_Latest(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getParameterFunc: func(_ context.Context, name string, version string) (*model.Parameter, error) {
			assert.Equal(t, "/my/param", name)
			assert.Empty(t, version)

			return &model.Parameter{
				Name:      "/my/param",
				Value:     "test-value",
				Version:   "3",
				UpdatedAt: &now,
				Metadata:  model.AWSParameterMeta{Type: "String"},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param"}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "/my/param", result.Name)
	assert.Equal(t, "test-value", result.Value)
	assert.Equal(t, "3", result.Version)
}

func TestGetParameterWithVersion_SpecificVersion(t *testing.T) {
	t.Parallel()

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, name string, version string) (*model.Parameter, error) {
			assert.Equal(t, "/my/param", name)
			assert.Equal(t, "2", version)

			return &model.Parameter{
				Name:     "/my/param",
				Value:    "old-value",
				Version:  "2",
				Metadata: model.AWSParameterMeta{Type: "String"},
			}, nil
		},
	}

	v := int64(2)
	spec := &paramversion.Spec{Name: "/my/param", Absolute: paramversion.AbsoluteSpec{Version: &v}}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	assert.Equal(t, "old-value", result.Value)
	assert.Equal(t, "2", result.Version)
}

func TestGetParameterWithVersion_Shift(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, name string) (*model.ParameterHistory, error) {
			assert.Equal(t, "/my/param", name)
			// History is returned oldest first by AWS
			return &model.ParameterHistory{
				Name: "/my/param",
				Parameters: []*model.Parameter{
					{Name: "/my/param", Value: "v1", Version: "1", UpdatedAt: timePtr(now.Add(-2 * time.Hour))},
					{Name: "/my/param", Value: "v2", Version: "2", UpdatedAt: timePtr(now.Add(-time.Hour))},
					{Name: "/my/param", Value: "v3", Version: "3", UpdatedAt: &now},
				},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param", Shift: 1}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	// Shift 1 means one version back from latest (v3), so v2
	assert.Equal(t, "v2", result.Value)
	assert.Equal(t, "2", result.Version)
}

func TestGetParameterWithVersion_ShiftFromSpecificVersion(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ string) (*model.ParameterHistory, error) {
			return &model.ParameterHistory{
				Name: "/my/param",
				Parameters: []*model.Parameter{
					{Name: "/my/param", Value: "v1", Version: "1", UpdatedAt: timePtr(now.Add(-2 * time.Hour))},
					{Name: "/my/param", Value: "v2", Version: "2", UpdatedAt: timePtr(now.Add(-time.Hour))},
					{Name: "/my/param", Value: "v3", Version: "3", UpdatedAt: &now},
				},
			}, nil
		},
	}

	v := int64(3)
	spec := &paramversion.Spec{Name: "/my/param", Absolute: paramversion.AbsoluteSpec{Version: &v}, Shift: 2}
	result, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.NoError(t, err)
	// Version 3, shift 2 means v3 -> v2 -> v1
	assert.Equal(t, "v1", result.Value)
}

func TestGetParameterWithVersion_ShiftOutOfRange(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ string) (*model.ParameterHistory, error) {
			return &model.ParameterHistory{
				Name: "/my/param",
				Parameters: []*model.Parameter{
					{Name: "/my/param", Value: "v1", Version: "1", UpdatedAt: &now},
				},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param", Shift: 5}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "version shift out of range: ~5", err.Error())
}

func TestGetParameterWithVersion_VersionNotFound(t *testing.T) {
	t.Parallel()

	now := time.Now()
	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ string) (*model.ParameterHistory, error) {
			return &model.ParameterHistory{
				Name: "/my/param",
				Parameters: []*model.Parameter{
					{Name: "/my/param", Value: "v1", Version: "1", UpdatedAt: &now},
				},
			}, nil
		},
	}

	v := int64(99)
	spec := &paramversion.Spec{Name: "/my/param", Absolute: paramversion.AbsoluteSpec{Version: &v}, Shift: 1}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "version 99 not found", err.Error())
}

func TestGetParameterWithVersion_EmptyHistory(t *testing.T) {
	t.Parallel()

	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ string) (*model.ParameterHistory, error) {
			return &model.ParameterHistory{
				Name:       "/my/param",
				Parameters: []*model.Parameter{},
			}, nil
		},
	}

	spec := &paramversion.Spec{Name: "/my/param", Shift: 1}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "parameter not found: /my/param", err.Error())
}

func TestGetParameterWithVersion_GetParameterError(t *testing.T) {
	t.Parallel()

	mock := &mockClient{
		getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &paramversion.Spec{Name: "/my/param"}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "failed to get parameter: AWS error", err.Error())
}

func TestGetParameterWithVersion_GetParameterHistoryError(t *testing.T) {
	t.Parallel()

	mock := &mockClient{
		getParameterHistoryFunc: func(_ context.Context, _ string) (*model.ParameterHistory, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	spec := &paramversion.Spec{Name: "/my/param", Shift: 1}
	_, err := paramversion.GetParameterWithVersion(t.Context(), mock, spec)

	require.Error(t, err)
	assert.Equal(t, "failed to get parameter history: AWS error", err.Error())
}

func timePtr(t time.Time) *time.Time {
	return &t
}
