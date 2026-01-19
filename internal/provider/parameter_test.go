package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/provider"
)

// mockTypedParameterReader implements provider.TypedParameterReader for testing.
type mockTypedParameterReader struct {
	getTypedParameterFunc        func(ctx context.Context, name, version string) (*model.TypedParameter[model.AWSParameterMeta], error)
	getTypedParameterHistoryFunc func(ctx context.Context, name string) (*model.TypedParameterHistory[model.AWSParameterMeta], error)
}

func (m *mockTypedParameterReader) GetTypedParameter(
	ctx context.Context, name, version string,
) (*model.TypedParameter[model.AWSParameterMeta], error) {
	return m.getTypedParameterFunc(ctx, name, version)
}

func (m *mockTypedParameterReader) GetTypedParameterHistory(
	ctx context.Context, name string,
) (*model.TypedParameterHistory[model.AWSParameterMeta], error) {
	return m.getTypedParameterHistoryFunc(ctx, name)
}

func TestWrapTypedParameterReader_GetParameter(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mock := &mockTypedParameterReader{
			getTypedParameterFunc: func(_ context.Context, name, version string) (*model.TypedParameter[model.AWSParameterMeta], error) {
				return &model.TypedParameter[model.AWSParameterMeta]{
					Name:    name,
					Value:   "test-value",
					Version: version,
					Metadata: model.AWSParameterMeta{
						ARN: "test-arn",
					},
				}, nil
			},
		}

		reader := provider.WrapTypedParameterReader[model.AWSParameterMeta](mock)
		param, err := reader.GetParameter(context.Background(), "test-param", "1")

		require.NoError(t, err)
		assert.Equal(t, "test-param", param.Name)
		assert.Equal(t, "test-value", param.Value)
		assert.Equal(t, "1", param.Version)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		mock := &mockTypedParameterReader{
			getTypedParameterFunc: func(_ context.Context, _, _ string) (*model.TypedParameter[model.AWSParameterMeta], error) {
				return nil, errors.New("not found")
			},
		}

		reader := provider.WrapTypedParameterReader[model.AWSParameterMeta](mock)
		_, err := reader.GetParameter(context.Background(), "test-param", "1")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestWrapTypedParameterReader_GetParameterHistory(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mock := &mockTypedParameterReader{
			getTypedParameterHistoryFunc: func(_ context.Context, name string) (*model.TypedParameterHistory[model.AWSParameterMeta], error) {
				return &model.TypedParameterHistory[model.AWSParameterMeta]{
					Name: name,
					Parameters: []*model.TypedParameter[model.AWSParameterMeta]{
						{Name: name, Value: "v1", Version: "1"},
						{Name: name, Value: "v2", Version: "2"},
					},
				}, nil
			},
		}

		reader := provider.WrapTypedParameterReader[model.AWSParameterMeta](mock)
		history, err := reader.GetParameterHistory(context.Background(), "test-param")

		require.NoError(t, err)
		assert.Equal(t, "test-param", history.Name)
		assert.Len(t, history.Parameters, 2)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		mock := &mockTypedParameterReader{
			getTypedParameterHistoryFunc: func(_ context.Context, _ string) (*model.TypedParameterHistory[model.AWSParameterMeta], error) {
				return nil, errors.New("history error")
			},
		}

		reader := provider.WrapTypedParameterReader[model.AWSParameterMeta](mock)
		_, err := reader.GetParameterHistory(context.Background(), "test-param")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "history error")
	})
}

func TestWrapTypedParameterReader_ListParameters(t *testing.T) {
	t.Parallel()

	// ListParameters returns nil because TypedParameterReader doesn't include list functionality
	mock := &mockTypedParameterReader{}
	reader := provider.WrapTypedParameterReader[model.AWSParameterMeta](mock)
	items, err := reader.ListParameters(context.Background(), "/", true)

	require.NoError(t, err)
	assert.Nil(t, items)
}
