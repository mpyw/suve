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

type mockSetClient struct {
	getParameterResult *paramapi.GetParameterOutput
	getParameterErr    error
	putParameterResult *paramapi.PutParameterOutput
	putParameterErr    error
}

func (m *mockSetClient) GetParameter(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}
	return m.getParameterResult, nil
}

func (m *mockSetClient) PutParameter(_ context.Context, _ *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
	if m.putParameterErr != nil {
		return nil, m.putParameterErr
	}
	return m.putParameterResult, nil
}

func TestSetUseCase_Exists(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config")},
		},
	}

	uc := &param.SetUseCase{Client: client}

	exists, err := uc.Exists(context.Background(), "/app/config")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSetUseCase_Exists_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr: &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
	}

	uc := &param.SetUseCase{Client: client}

	exists, err := uc.Exists(context.Background(), "/app/not-exists")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSetUseCase_Exists_Error(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr: errors.New("aws error"),
	}

	uc := &param.SetUseCase{Client: client}

	_, err := uc.Exists(context.Background(), "/app/config")
	assert.Error(t, err)
}

func TestSetUseCase_Execute_Create(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr:    &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
		putParameterResult: &paramapi.PutParameterOutput{Version: 1},
	}

	uc := &param.SetUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.SetInput{
		Name:  "/app/new",
		Value: "new-value",
		Type:  paramapi.ParameterTypeString,
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
	assert.Equal(t, int64(1), output.Version)
	assert.True(t, output.IsCreated)
}

func TestSetUseCase_Execute_Update(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{Name: lo.ToPtr("/app/config")},
		},
		putParameterResult: &paramapi.PutParameterOutput{Version: 5},
	}

	uc := &param.SetUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.SetInput{
		Name:        "/app/config",
		Value:       "updated-value",
		Type:        paramapi.ParameterTypeString,
		Description: "updated description",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, int64(5), output.Version)
	assert.False(t, output.IsCreated)
}

func TestSetUseCase_Execute_ExistsError(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr: errors.New("aws error"),
	}

	uc := &param.SetUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.SetInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
}

func TestSetUseCase_Execute_PutError(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr: &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
		putParameterErr: errors.New("put failed"),
	}

	uc := &param.SetUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.SetInput{
		Name:  "/app/config",
		Value: "value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to put parameter")
}
