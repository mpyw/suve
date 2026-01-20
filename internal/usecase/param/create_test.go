package param_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockCreateClient struct {
	putParameterResult *model.ParameterWriteResult
	putParameterErr    error
}

func (m *mockCreateClient) PutParameter(_ context.Context, _ *model.Parameter, _ bool) (*model.ParameterWriteResult, error) {
	if m.putParameterErr != nil {
		return nil, m.putParameterErr
	}

	return m.putParameterResult, nil
}

func TestCreateUseCase_Execute(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		putParameterResult: &model.ParameterWriteResult{Name: "/app/new", Version: "1"},
	}

	uc := &param.CreateUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/new",
		Value: "new-value",
		Type:  "String",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
	assert.Equal(t, int64(1), output.Version)
}

func TestCreateUseCase_Execute_WithDescription(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		putParameterResult: &model.ParameterWriteResult{Name: "/app/new", Version: "1"},
	}

	uc := &param.CreateUseCase{Client: client}

	output, err := uc.Execute(t.Context(), param.CreateInput{
		Name:        "/app/new",
		Value:       "new-value",
		Type:        "String",
		Description: "my description",
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
	assert.Equal(t, int64(1), output.Version)
}

func TestCreateUseCase_Execute_AlreadyExists(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		putParameterErr: errors.New("parameter already exists"),
	}

	uc := &param.CreateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/existing",
		Value: "value",
		Type:  "String",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create parameter")
}

func TestCreateUseCase_Execute_PutError(t *testing.T) {
	t.Parallel()

	client := &mockCreateClient{
		putParameterErr: errAWS,
	}

	uc := &param.CreateUseCase{Client: client}

	_, err := uc.Execute(t.Context(), param.CreateInput{
		Name:  "/app/config",
		Value: "value",
		Type:  "String",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create parameter")
}
