package param_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestTagUseCase_Execute_AddTags(t *testing.T) {
	t.Parallel()

	var gotAdd map[string]string

	store := &providermock.Store{
		TagFunc: func(_ context.Context, name string, add map[string]string) error {
			assert.Equal(t, "/app/config", name)

			gotAdd = add

			return nil
		},
	}

	uc := &param.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), param.TagInput{
		Name: "/app/config",
		Add:  map[string]string{"env": "prod", "team": "backend"},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod", "team": "backend"}, gotAdd)
}

func TestTagUseCase_Execute_RemoveTags(t *testing.T) {
	t.Parallel()

	var gotKeys []string

	store := &providermock.Store{
		UntagFunc: func(_ context.Context, name string, keys []string) error {
			assert.Equal(t, "/app/config", name)

			gotKeys = keys

			return nil
		},
	}

	uc := &param.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), param.TagInput{
		Name:   "/app/config",
		Remove: []string{"old-tag", "deprecated"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"old-tag", "deprecated"}, gotKeys)
}

func TestTagUseCase_Execute_AddAndRemoveTags(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		TagFunc:   func(_ context.Context, _ string, _ map[string]string) error { return nil },
		UntagFunc: func(_ context.Context, _ string, _ []string) error { return nil },
	}

	uc := &param.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), param.TagInput{
		Name:   "/app/config",
		Add:    map[string]string{"env": "prod"},
		Remove: []string{"old-tag"},
	})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_NoTags(t *testing.T) {
	t.Parallel()

	// Neither Tag nor Untag should be called; leaving the funcs nil ensures a
	// hit would fail with providermock.ErrNotConfigured.
	store := &providermock.Store{}

	uc := &param.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), param.TagInput{Name: "/app/config"})
	require.NoError(t, err)
}

func TestTagUseCase_Execute_AddTagsError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		TagFunc: func(_ context.Context, _ string, _ map[string]string) error {
			return errAddTagsFailed
		},
	}

	uc := &param.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), param.TagInput{
		Name: "/app/config",
		Add:  map[string]string{"env": "prod"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestTagUseCase_Execute_RemoveTagsError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		UntagFunc: func(_ context.Context, _ string, _ []string) error {
			return errRemoveTagsFailed
		},
	}

	uc := &param.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), param.TagInput{
		Name:   "/app/config",
		Remove: []string{"old-tag"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}
