package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/usecase/secret"
)

type mockTagClient struct {
	addTagsErr    error
	removeTagsErr error
}

func (m *mockTagClient) GetTags(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil //nolint:nilnil // mock implementation
}

func (m *mockTagClient) AddTags(_ context.Context, _ string, _ map[string]string) error {
	return m.addTagsErr
}

func (m *mockTagClient) RemoveTags(_ context.Context, _ string, _ []string) error {
	return m.removeTagsErr
}

func TestTagUseCase_Execute_AddTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name: "my-secret",
		Add:  map[string]string{"env": "prod", "team": "backend"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_RemoveTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name:   "my-secret",
		Remove: []string{"old-tag", "deprecated"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_AddAndRemoveTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name:   "my-secret",
		Add:    map[string]string{"env": "prod"},
		Remove: []string{"old-tag"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_NoTags(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name: "my-secret",
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_AddTagsError(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		addTagsErr: errors.New("add tags failed"),
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name: "my-secret",
		Add:  map[string]string{"env": "prod"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestTagUseCase_Execute_RemoveTagsError(t *testing.T) {
	t.Parallel()

	client := &mockTagClient{
		removeTagsErr: errors.New("remove tags failed"),
	}
	uc := &secret.TagUseCase{Client: client}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name:   "my-secret",
		Remove: []string{"old-tag"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}
