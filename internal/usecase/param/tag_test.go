package param_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	"github.com/mpyw/suve/internal/usecase/param"
)

type mockTagClient struct {
	addTagsErr    error
	removeTagsErr error
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockTagClient) AddTagsToResource(_ context.Context, _ *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
	if m.addTagsErr != nil {
		return nil, m.addTagsErr
	}

	return &paramapi.AddTagsToResourceOutput{}, nil
}

//nolint:lll // mock function signature must match AWS SDK interface
func (m *mockTagClient) RemoveTagsFromResource(_ context.Context, _ *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
	if m.removeTagsErr != nil {
		return nil, m.removeTagsErr
	}

	return &paramapi.RemoveTagsFromResourceOutput{}, nil
}

func TestTagUseCase_Execute_AddTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{}
	uc := &param.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), param.TagInput{
		Name: "/app/config",
		Add:  map[string]string{"env": "prod", "team": "backend"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_RemoveTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{}
	uc := &param.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), param.TagInput{
		Name:   "/app/config",
		Remove: []string{"old-tag", "deprecated"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_AddAndRemoveTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{}
	uc := &param.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), param.TagInput{
		Name:   "/app/config",
		Add:    map[string]string{"env": "prod"},
		Remove: []string{"old-tag"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_NoTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{}
	uc := &param.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), param.TagInput{
		Name: "/app/config",
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_AddTagsError(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		addTagsErr: errAddTagsFailed,
	}
	uc := &param.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), param.TagInput{
		Name: "/app/config",
		Add:  map[string]string{"env": "prod"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestTagUseCase_Execute_RemoveTagsError(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		removeTagsErr: errRemoveTagsFailed,
	}
	uc := &param.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), param.TagInput{
		Name:   "/app/config",
		Remove: []string{"old-tag"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}
