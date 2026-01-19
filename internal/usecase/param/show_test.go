package param_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/model"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/paramversion"
)

type mockShowClient struct {
	getParameterResult *model.Parameter
	getParameterErr    error
	getHistoryResult   *model.ParameterHistory
	getHistoryErr      error
	getTagsResult      map[string]string
	getTagsErr         error
}

func (m *mockShowClient) GetParameter(_ context.Context, _ string, _ string) (*model.Parameter, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}

	return m.getParameterResult, nil
}

func (m *mockShowClient) GetParameterHistory(_ context.Context, _ string) (*model.ParameterHistory, error) {
	if m.getHistoryErr != nil {
		return nil, m.getHistoryErr
	}

	if m.getHistoryResult == nil {
		return &model.ParameterHistory{}, nil
	}

	return m.getHistoryResult, nil
}

func (m *mockShowClient) ListParameters(_ context.Context, _ string, _ bool) ([]*model.ParameterListItem, error) {
	return nil, nil
}

func (m *mockShowClient) GetTags(_ context.Context, _ string) (map[string]string, error) {
	if m.getTagsErr != nil {
		return nil, m.getTagsErr
	}

	return m.getTagsResult, nil
}

func (m *mockShowClient) AddTags(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func (m *mockShowClient) RemoveTags(_ context.Context, _ string, _ []string) error {
	return nil
}

func TestShowUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockShowClient{
		getParameterResult: &model.Parameter{
			Name:         "/app/config",
			Value:        "secret-value",
			Version:      "5",
			Type:         "SecureString",
			LastModified: &now,
		},
	}

	uc := &param.ShowUseCase{Client: client}

	spec, err := paramversion.Parse("/app/config")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, "secret-value", output.Value)
	assert.Equal(t, int64(5), output.Version)
	assert.Equal(t, "SecureString", output.Type)
	assert.NotNil(t, output.LastModified)
}

func TestShowUseCase_Execute_WithVersion(t *testing.T) {
	t.Parallel()

	// #VERSION spec without shift uses GetParameter (SSM supports name:version format)
	client := &mockShowClient{
		getParameterResult: &model.Parameter{
			Name:    "/app/config",
			Value:   "old-value",
			Version: "3",
			Type:    "String",
		},
	}

	uc := &param.ShowUseCase{Client: client}

	spec, err := paramversion.Parse("/app/config#3")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, "old-value", output.Value)
	assert.Equal(t, int64(3), output.Version)
}

func TestShowUseCase_Execute_WithShift(t *testing.T) {
	t.Parallel()

	// Spec with shift uses GetParameterHistory
	client := &mockShowClient{
		getHistoryResult: &model.ParameterHistory{
			Name: "/app/config",
			Parameters: []*model.Parameter{
				{Name: "/app/config", Value: "v1", Version: "1", Type: "String"},
				{Name: "/app/config", Value: "v2", Version: "2", Type: "String"},
				{Name: "/app/config", Value: "v3", Version: "3", Type: "String"},
			},
		},
	}

	uc := &param.ShowUseCase{Client: client}

	spec, err := paramversion.Parse("/app/config~1") // 1 version back from latest
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Equal(t, "/app/config", output.Name)
	assert.Equal(t, "v2", output.Value) // v3 - 1 = v2
	assert.Equal(t, int64(2), output.Version)
}

func TestShowUseCase_Execute_Error(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getParameterErr: errAWS,
	}

	uc := &param.ShowUseCase{Client: client}

	spec, err := paramversion.Parse("/app/config")
	require.NoError(t, err)

	_, err = uc.Execute(t.Context(), param.ShowInput{
		Spec: spec,
	})
	require.Error(t, err)
}

func TestShowUseCase_Execute_NoLastModified(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getParameterResult: &model.Parameter{
			Name:    "/app/config",
			Value:   "value",
			Version: "1",
			Type:    "String",
		},
	}

	uc := &param.ShowUseCase{Client: client}

	spec, err := paramversion.Parse("/app/config")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Nil(t, output.LastModified)
}

func TestShowUseCase_Execute_WithTags(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getParameterResult: &model.Parameter{
			Name:    "/app/config",
			Value:   "value",
			Version: "1",
			Type:    "String",
		},
		getTagsResult: map[string]string{
			"env":  "prod",
			"team": "backend",
		},
	}

	uc := &param.ShowUseCase{Client: client}

	spec, err := paramversion.Parse("/app/config")
	require.NoError(t, err)

	output, err := uc.Execute(t.Context(), param.ShowInput{
		Spec: spec,
	})
	require.NoError(t, err)
	assert.Len(t, output.Tags, 2)

	// Convert to map for easier testing (order not guaranteed)
	tagMap := make(map[string]string)
	for _, tag := range output.Tags {
		tagMap[tag.Key] = tag.Value
	}

	assert.Equal(t, "prod", tagMap["env"])
	assert.Equal(t, "backend", tagMap["team"])
}
