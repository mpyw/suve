package param_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockDeleteClient struct {
	getParameterResult *model.Parameter
	getParameterErr    error
	deleteParameterErr error
}

func (m *mockDeleteClient) GetParameter(_ context.Context, _ string, _ string) (*model.Parameter, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}

	return m.getParameterResult, nil
}

func (m *mockDeleteClient) DeleteParameter(_ context.Context, _ string) error {
	return m.deleteParameterErr
}

func TestDeleteUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getParameterResult: &model.Parameter{
			Name:  "/app/config",
			Value: "current-value",
		},
	}

	uc := &param.DeleteUseCase{Client: client}

	value, err := uc.GetCurrentValue(t.Context(), "/app/config")
	require.NoError(t, err)
	assert.Equal(t, "current-value", value)
}

func TestDeleteUseCase_GetCurrentValue_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getParameterErr: errNotFound,
	}

	uc := &param.DeleteUseCase{Client: client}

	value, err := uc.GetCurrentValue(t.Context(), "/app/not-exists")
	require.NoError(t, err) // GetCurrentValue treats errors as "not found"
	assert.Empty(t, value)
}

func TestDeleteUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{}

	uc := &param.DeleteUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.DeleteInput{
		Name: "/app/to-delete",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/to-delete", output.Name)
}

func TestDeleteUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		deleteParameterErr: errDeleteFailed,
	}

	uc := &param.DeleteUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.DeleteInput{
		Name: "/app/config",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete parameter")
}
