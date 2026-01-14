package param_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockDeleteClient struct {
	getParameterResult    *paramapi.GetParameterOutput
	getParameterErr       error
	deleteParameterResult *paramapi.DeleteParameterOutput
	deleteParameterErr    error
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockDeleteClient) GetParameter(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}

	return m.getParameterResult, nil
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockDeleteClient) DeleteParameter(_ context.Context, _ *paramapi.DeleteParameterInput, _ ...func(*paramapi.Options)) (*paramapi.DeleteParameterOutput, error) {
	if m.deleteParameterErr != nil {
		return nil, m.deleteParameterErr
	}

	return m.deleteParameterResult, nil
}

func TestDeleteUseCase_GetCurrentValue(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{
				Value: lo.ToPtr("current-value"),
			},
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
		getParameterErr: &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
	}

	uc := &param.DeleteUseCase{Client: client}

	value, err := uc.GetCurrentValue(t.Context(), "/app/not-exists")
	require.NoError(t, err)
	assert.Empty(t, value)
}

func TestDeleteUseCase_GetCurrentValue_Error(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		getParameterErr: errAWS,
	}

	uc := &param.DeleteUseCase{Client: client}

	_, err := uc.GetCurrentValue(t.Context(), "/app/config")
	require.Error(t, err)
}

func TestDeleteUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockDeleteClient{
		deleteParameterResult: &paramapi.DeleteParameterOutput{},
	}

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
