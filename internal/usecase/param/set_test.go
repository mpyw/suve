package param_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/tagging"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockSetClient struct {
	getParameterResult *paramapi.GetParameterOutput
	getParameterErr    error
	putParameterResult *paramapi.PutParameterOutput
	putParameterErr    error
	addTagsErr         error
	removeTagsErr      error
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

func (m *mockSetClient) AddTagsToResource(_ context.Context, _ *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
	if m.addTagsErr != nil {
		return nil, m.addTagsErr
	}
	return &paramapi.AddTagsToResourceOutput{}, nil
}

func (m *mockSetClient) RemoveTagsFromResource(_ context.Context, _ *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
	if m.removeTagsErr != nil {
		return nil, m.removeTagsErr
	}
	return &paramapi.RemoveTagsFromResourceOutput{}, nil
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

func TestSetUseCase_Execute_WithTags(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr:    &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
		putParameterResult: &paramapi.PutParameterOutput{Version: 1},
	}

	uc := &param.SetUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.SetInput{
		Name:  "/app/new",
		Value: "value",
		Type:  paramapi.ParameterTypeString,
		TagChange: &tagging.Change{
			Add:    map[string]string{"env": "prod"},
			Remove: []string{"old-tag"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
}

func TestSetUseCase_Execute_AddTagsError(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr:    &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
		putParameterResult: &paramapi.PutParameterOutput{Version: 1},
		addTagsErr:         errors.New("add tags failed"),
	}

	uc := &param.SetUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.SetInput{
		Name:  "/app/new",
		Value: "value",
		TagChange: &tagging.Change{
			Add: map[string]string{"env": "prod"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestSetUseCase_Execute_RemoveTagsError(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr:    &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
		putParameterResult: &paramapi.PutParameterOutput{Version: 1},
		removeTagsErr:      errors.New("remove tags failed"),
	}

	uc := &param.SetUseCase{Client: client}

	_, err := uc.Execute(context.Background(), param.SetInput{
		Name:  "/app/new",
		Value: "value",
		TagChange: &tagging.Change{
			Remove: []string{"old-tag"},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}

func TestSetUseCase_Execute_EmptyTagChange(t *testing.T) {
	t.Parallel()

	client := &mockSetClient{
		getParameterErr:    &paramapi.ParameterNotFound{Message: lo.ToPtr("not found")},
		putParameterResult: &paramapi.PutParameterOutput{Version: 1},
	}

	uc := &param.SetUseCase{Client: client}

	output, err := uc.Execute(context.Background(), param.SetInput{
		Name:      "/app/new",
		Value:     "value",
		TagChange: &tagging.Change{},
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/new", output.Name)
}
