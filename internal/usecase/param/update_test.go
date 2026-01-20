package param_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockUpdateClient struct {
	getParameterResult *model.Parameter
	getParameterErr    error
	putParameterResult *model.ParameterWriteResult
	putParameterErr    error
}

func (m *mockUpdateClient) GetParameter(_ context.Context, _ string, _ string) (*model.Parameter, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}

	return m.getParameterResult, nil
}

func (m *mockUpdateClient) PutParameter(_ context.Context, _ *model.Parameter, _ bool) (*model.ParameterWriteResult, error) {
	if m.putParameterErr != nil {
		return nil, m.putParameterErr
	}

	return m.putParameterResult, nil
}

func TestUpdateUseCase_Exists(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterResult: &model.Parameter{Name: "/app/config"},
	}

	uc := &param.UpdateUseCase{Client: client}

	exists, err := uc.Exists(t.Context(), "/app/config")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestUpdateUseCase_Exists_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterErr: errNotFound,
	}

	uc := &param.UpdateUseCase{Client: client}

	exists, err := uc.Exists(t.Context(), "/app/not-exists")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUpdateUseCase_Exists_AnyError(t *testing.T) {
	t.Parallel()

	// The implementation treats any error as "not found" for simplicity
	client := &mockUpdateClient{
		getParameterErr: errAWS,
	}

	uc := &param.UpdateUseCase{Client: client}

	exists, err := uc.Exists(t.Context(), "/app/config")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUpdateUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterResult: &model.Parameter{Name: "/app/config"},
		putParameterResult: &model.ParameterWriteResult{Name: "/app/config", Version: "5"},
	}

	uc := &param.UpdateUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.UpdateInput{
		Name:        "/app/config",
		Value:       "updated-value",
		Type:        "String",
		Description: "updated description",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, int64(5), output.Version)
}

func TestUpdateUseCase_Execute_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterErr: errNotFound,
	}

	uc := &param.UpdateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.UpdateInput{
		Name:  "/app/not-exists",
		Value: "value",
		Type:  "String",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parameter not found")
}

func TestUpdateUseCase_Execute_PutError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterResult: &model.Parameter{Name: "/app/config"},
		putParameterErr:    errPutFailed,
	}

	uc := &param.UpdateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.UpdateInput{
		Name:  "/app/config",
		Value: "value",
		Type:  "String",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update parameter")
}
