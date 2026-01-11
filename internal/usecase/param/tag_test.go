package param_test

import (
	"context"
	"errors"
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

func (m *mockTagClient) AddTagsToResource(_ context.Context, _ *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
	if m.addTagsErr != nil {
		return nil, m.addTagsErr
	}
	return &paramapi.AddTagsToResourceOutput{}, nil
}

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
		addTagsErr: errors.New("add tags failed"),
	}
	uc := &param.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), param.TagInput{
		Name: "/app/config",
		Add:  map[string]string{"env": "prod"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestTagUseCase_Execute_RemoveTagsError(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		removeTagsErr: errors.New("remove tags failed"),
	}
	uc := &param.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), param.TagInput{
		Name:   "/app/config",
		Remove: []string{"old-tag"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}
