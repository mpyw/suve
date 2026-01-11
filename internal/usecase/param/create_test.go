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

type mockCreateClient struct {
	putParameterResult *paramapi.PutParameterOutput
	putParameterErr    error
}

func (m *mockCreateClient) PutParameter(_ context.Context, _ *paramapi.PutParameterInput, _ ...func(*paramapi.Options)) (*paramapi.PutParameterOutput, error) {
	if m.putParameterErr != nil {
		return nil, m.putParameterErr
	}
	return m.putParameterResult, nil
}

func TestCreateUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		putParameterResult: &paramapi.PutParameterOutput{Version: 1},
	}

	uc := &param.CreateUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/new",
		Value: "new-value",
		Type:  paramapi.ParameterTypeString,
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
	assert.Equal(t, int64(1), output.Version)
}

func TestCreateUseCase_Execute_WithDescription(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		putParameterResult: &paramapi.PutParameterOutput{Version: 1},
	}

	uc := &param.CreateUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.CreateInput{
		Name:        "/app/new",
		Value:       "new-value",
		Type:        paramapi.ParameterTypeString,
		Description: "my description",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
	assert.Equal(t, int64(1), output.Version)
}

func TestCreateUseCase_Execute_AlreadyExists(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		putParameterErr: &paramapi.ParameterAlreadyExists{Message: lo.ToPtr("already exists")},
	}

	uc := &param.CreateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/existing",
		Value: "value",
		Type:  paramapi.ParameterTypeString,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create parameter")
}

func TestCreateUseCase_Execute_PutError(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		putParameterErr: errors.New("aws error"),
	}

	uc := &param.CreateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/config",
		Value: "value",
		Type:  paramapi.ParameterTypeString,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create parameter")
}
