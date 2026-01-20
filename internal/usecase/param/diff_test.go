package param_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/paramversion"
)

type mockDiffClient struct {
	getParameterFunc        func(ctx context.Context, name string, version string) (*model.Parameter, error)
	getParameterHistoryFunc func(ctx context.Context, name string) (*model.ParameterHistory, error)
}

func (m *mockDiffClient) GetParameter(ctx context.Context, name string, version string) (*model.Parameter, error) {
	if m.getParameterFunc != nil {
		return m.getParameterFunc(ctx, name, version)
	}

	return nil, fmt.Errorf("GetParameter not mocked")
}

func (m *mockDiffClient) GetParameterHistory(ctx context.Context, name string) (*model.ParameterHistory, error) {
	if m.getParameterHistoryFunc != nil {
		return m.getParameterHistoryFunc(ctx, name)
	}

	return nil, fmt.Errorf("GetParameterHistory not mocked")
}

func (m *mockDiffClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	return nil, fmt.Errorf("ListParameters not mocked")
}

func TestDiffUseCase_Execute(t *testing.T) {
	t.Parallel()

	// #VERSION specs without shift use GetParameter (with name:version format)
	callCount := 0
	client := &mockDiffClient{
		getParameterFunc: func(_ context.Context, name string, version string) (*model.Parameter, error) {
			callCount++

			assert.Equal(t, "/app/config", name)

			if callCount == 1 {
				assert.Equal(t, "1", version)

				return &model.Parameter{Name: "/app/config", Value: "old-value", Version: "1"}, nil
			}

			assert.Equal(t, "2", version)

			return &model.Parameter{Name: "/app/config", Value: "new-value", Version: "2"}, nil
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config#1")
	spec2, _ := paramversion.Parse("/app/config#2")

	output, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.OldName)
	assert.Equal(t, int64(1), output.OldVersion)
	assert.Equal(t, "old-value", output.OldValue)
	assert.Equal(t, "/app/config", output.NewName)
	assert.Equal(t, int64(2), output.NewVersion)
	assert.Equal(t, "new-value", output.NewValue)
}

func TestDiffUseCase_Execute_Spec1Error(t *testing.T) {
	t.Parallel()

	client := &mockDiffClient{
		getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
			return nil, errGetParameter
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config#1")
	spec2, _ := paramversion.Parse("/app/config#2")

	_, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_Spec2Error(t *testing.T) {
	t.Parallel()

	callCount := 0
	client := &mockDiffClient{
		getParameterFunc: func(_ context.Context, _ string, _ string) (*model.Parameter, error) {
			callCount++
			if callCount == 1 {
				return &model.Parameter{Name: "/app/config", Value: "old-value", Version: "1"}, nil
			}

			return nil, errGetParameter
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config#1")
	spec2, _ := paramversion.Parse("/app/config#2")

	_, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	assert.Error(t, err)
}

func TestDiffUseCase_Execute_WithLatest(t *testing.T) {
	t.Parallel()

	// Both specs without shift use GetParameter
	callCount := 0
	client := &mockDiffClient{
		getParameterFunc: func(_ context.Context, _ string, version string) (*model.Parameter, error) {
			callCount++
			if callCount == 1 {
				assert.Equal(t, "3", version)

				return &model.Parameter{Name: "/app/config", Value: "old-value", Version: "3"}, nil
			}

			assert.Empty(t, version) // latest

			return &model.Parameter{Name: "/app/config", Value: "latest-value", Version: "5"}, nil
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config#3")
	spec2, _ := paramversion.Parse("/app/config")

	output, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), output.OldVersion)
	assert.Equal(t, int64(5), output.NewVersion)
}

func TestDiffUseCase_Execute_WithShift(t *testing.T) {
	t.Parallel()

	// Specs with shift use GetParameterHistory
	client := &mockDiffClient{
		getParameterHistoryFunc: func(_ context.Context, name string) (*model.ParameterHistory, error) {
			assert.Equal(t, "/app/config", name)

			return &model.ParameterHistory{
				Name: "/app/config",
				Parameters: []*model.Parameter{
					{Name: "/app/config", Value: "v1", Version: "1"},
					{Name: "/app/config", Value: "v2", Version: "2"},
					{Name: "/app/config", Value: "v3", Version: "3"},
				},
			}, nil
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config~2") // 2 versions back from latest (v3 -> v1)
	spec2, _ := paramversion.Parse("/app/config~1") // 1 version back from latest (v3 -> v2)

	output, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), output.OldVersion)
	assert.Equal(t, "v1", output.OldValue)
	assert.Equal(t, int64(2), output.NewVersion)
	assert.Equal(t, "v2", output.NewValue)
}

func TestDiffUseCase_Execute_WithShift_Error(t *testing.T) {
	t.Parallel()

	client := &mockDiffClient{
		getParameterHistoryFunc: func(_ context.Context, _ string) (*model.ParameterHistory, error) {
			return nil, errHistoryFailed
		},
	}

	uc := &param.DiffUseCase{Client: client}

	spec1, _ := paramversion.Parse("/app/config~1")
	spec2, _ := paramversion.Parse("/app/config")

	_, err := uc.Execute(t.Context(), param.DiffInput{
		Spec1: spec1,
		Spec2: spec2,
	})
	assert.Error(t, err)
}
