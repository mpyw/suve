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

type mockUpdateClient struct {
	getParameterResult *paramapi.GetParameterOutput
	getParameterErr    error
	putParameterResult *paramapi.PutParameterOutput
	putParameterErr    error
}

func (m *mockUpdateClient) GetParameter(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}
	return m.getParameterResult, nil
}

func (m *mockUpdateClient) PutParameter(_ context.Context, _ *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
	if m.putParameterErr != nil {
		return nil, m.putParameterErr
	}
	return m.putParameterResult, nil
}

func TestUpdateUseCase_Exists(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config")},
		},
	}

	uc := &param.UpdateUseCase{Client: client}

	exists, err := uc.Exists(context.Background(), "/app/config")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestUpdateUseCase_Exists_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterErr: &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
	}

	uc := &param.UpdateUseCase{Client: client}

	exists, err := uc.Exists(context.Background(), "/app/not-exists")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUpdateUseCase_Exists_Error(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterErr: errors.New("aws error"),
	}

	uc := &param.UpdateUseCase{Client: client}

	_, err := uc.Exists(context.Background(), "/app/config")
	assert.Error(t, err)
}

func TestUpdateUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config")},
		},
		putParameterResult: &paramapi.PutParameterOutput{Version: 5},
	}

	uc := &param.UpdateUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.UpdateInput{
		Name:        "/app/config",
		Value:       "updated-value",
		Type:        paramapi.ParameterTypeString,
		Description: "updated description",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, int64(5), output.Version)
}

func TestUpdateUseCase_Execute_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterErr: &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
	}

	uc := &param.UpdateUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.UpdateInput{
		Name:  "/app/not-exists",
		Value: "value",
		Type:  paramapi.ParameterTypeString,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parameter not found")
}

func TestUpdateUseCase_Execute_ExistsError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterErr: errors.New("aws error"),
	}

	uc := &param.UpdateUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.UpdateInput{
		Name:  "/app/config",
		Value: "value",
		Type:  paramapi.ParameterTypeString,
	})
	assert.Error(t, err)
}

func TestUpdateUseCase_Execute_PutError(t *testing.T) {
	t.Parallel()

	client := &mockUpdateClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config")},
		},
		putParameterErr: errors.New("put failed"),
	}

	uc := &param.UpdateUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.UpdateInput{
		Name:  "/app/config",
		Value: "value",
		Type:  paramapi.ParameterTypeString,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update parameter")
}
