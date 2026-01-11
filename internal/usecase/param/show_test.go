package param_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/usecase/param"
	"github.com/mpyw/suve/internal/version/paramversion"
)

type mockShowClient struct {
	getParameterResult *paramapi.GetParameterOutput
	getParameterErr    error
	getHistoryResult   *paramapi.GetParameterHistoryOutput
	getHistoryErr      error
	listTagsResult     *paramapi.ListTagsForResourceOutput
	listTagsErr        error
}

func (m *mockShowClient) GetParameter(_ context.Context, _ *paramapi.GetParameterInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterOutput, error) {
	if m.getParameterErr != nil {
		return nil, m.getParameterErr
	}
	return m.getParameterResult, nil
}

func (m *mockShowClient) GetParameterHistory(_ context.Context, _ *paramapi.GetParameterHistoryInput, _ ...func(*paramapi.Options)) (*paramapi.GetParameterHistoryOutput, error) {
	if m.getHistoryErr != nil {
		return nil, m.getHistoryErr
	}
	if m.getHistoryResult == nil {
		return &paramapi.GetParameterHistoryOutput{}, nil
	}
	return m.getHistoryResult, nil
}

func (m *mockShowClient) ListTagsForResource(_ context.Context, _ *paramapi.ListTagsForResourceInput, _ ...func(*paramapi.Options)) (*paramapi.ListTagsForResourceOutput, error) {
	if m.listTagsErr != nil {
		return nil, m.listTagsErr
	}
	if m.listTagsResult != nil {
		return m.listTagsResult, nil
	}
	return &paramapi.ListTagsForResourceOutput{}, nil
}

func TestShowUseCase_Execute(t *testing.T) {
	t.Parallel()

	now := time.Now()
	client := &mockShowClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{
				Name:             lo.ToPtr("/app/config"),
				Value:            lo.ToPtr("secret-value"),
				Version:          5,
				Type:             paramapi.ParameterTypeSecureString,
				LastModifiedDate: &now,
			},
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
	assert.Equal(t, paramapi.ParameterTypeSecureString, output.Type)
	assert.NotNil(t, output.LastModified)
}

func TestShowUseCase_Execute_WithVersion(t *testing.T) {
	t.Parallel()

	// #VERSION spec without shift uses GetParameter (SSM supports name:version format)
	client := &mockShowClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{
				Name:    lo.ToPtr("/app/config"),
				Value:   lo.ToPtr("old-value"),
				Version: 3,
				Type:    paramapi.ParameterTypeString,
			},
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
		getHistoryResult: &paramapi.GetParameterHistoryOutput{
			Parameters: []paramapi.ParameterHistory{
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v1"), Version: 1, Type: paramapi.ParameterTypeString},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v2"), Version: 2, Type: paramapi.ParameterTypeString},
				{Name: lo.ToPtr("/app/config"), Value: lo.ToPtr("v3"), Version: 3, Type: paramapi.ParameterTypeString},
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
		getParameterErr: errors.New("aws error"),
	}

	uc := &param.ShowUseCase{Client: client}

	spec, err := paramversion.Parse("/app/config")
	require.NoError(t, err)

	_, err = uc.Execute(t.Context(), param.ShowInput{
		Spec: spec,
	})
	assert.Error(t, err)
}

func TestShowUseCase_Execute_NoLastModified(t *testing.T) {
	t.Parallel()

	client := &mockShowClient{
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{
				Name:    lo.ToPtr("/app/config"),
				Value:   lo.ToPtr("value"),
				Version: 1,
				Type:    paramapi.ParameterTypeString,
			},
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
		getParameterResult: &paramapi.GetParameterOutput{
			Parameter: &paramapi.Parameter{
				Name:    lo.ToPtr("/app/config"),
				Value:   lo.ToPtr("value"),
				Version: 1,
				Type:    paramapi.ParameterTypeString,
			},
		},
		listTagsResult: &paramapi.ListTagsForResourceOutput{
			TagList: []paramapi.Tag{
				{Key: lo.ToPtr("env"), Value: lo.ToPtr("prod")},
				{Key: lo.ToPtr("team"), Value: lo.ToPtr("backend")},
			},
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
	assert.Equal(t, "env", output.Tags[0].Key)
	assert.Equal(t, "prod", output.Tags[0].Value)
	assert.Equal(t, "team", output.Tags[1].Key)
	assert.Equal(t, "backend", output.Tags[1].Value)
}
