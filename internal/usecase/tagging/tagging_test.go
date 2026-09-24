package tagging_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/tagging"
)

func TestUseCase_Execute_AddTags(t *testing.T) {
	t.Parallel()

	var gotAdd map[string]string

	store := &providermock.Store{
		TagFunc: func(_ context.Context, name string, add map[string]string) error {
			assert.Equal(t, "/app/config", name)

			gotAdd = add

			return nil
		},
	}

	uc := &tagging.UseCase{Tagger: store}

	err := uc.Execute(t.Context(), tagging.Input{
		Name: "/app/config",
		Add:  map[string]string{"env": "prod", "team": "backend"},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod", "team": "backend"}, gotAdd)
}

func TestUseCase_Execute_RemoveTags(t *testing.T) {
	t.Parallel()

	var gotKeys []string

	store := &providermock.Store{
		UntagFunc: func(_ context.Context, name string, keys []string) error {
			assert.Equal(t, "/app/config", name)

			gotKeys = keys

			return nil
		},
	}

	uc := &tagging.UseCase{Tagger: store}

	err := uc.Execute(t.Context(), tagging.Input{
		Name:   "/app/config",
		Remove: []string{"old-tag", "deprecated"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"old-tag", "deprecated"}, gotKeys)
}

func TestUseCase_Execute_AddAndRemoveTags(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		TagFunc:   func(_ context.Context, _ string, _ map[string]string) error { return nil },
		UntagFunc: func(_ context.Context, _ string, _ []string) error { return nil },
	}

	uc := &tagging.UseCase{Tagger: store}

	err := uc.Execute(t.Context(), tagging.Input{
		Name:   "/app/config",
		Add:    map[string]string{"env": "prod"},
		Remove: []string{"old-tag"},
	})
	require.NoError(t, err)
}

func TestUseCase_Execute_NoTags(t *testing.T) {
	t.Parallel()

	// Neither Tag nor Untag should be called; leaving the funcs nil ensures a
	// hit would fail with providermock.ErrNotConfigured.
	store := &providermock.Store{}

	uc := &tagging.UseCase{Tagger: store}

	err := uc.Execute(t.Context(), tagging.Input{Name: "/app/config"})
	require.NoError(t, err)
}

func TestUseCase_Execute_AddTagsError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		TagFunc: func(_ context.Context, _ string, _ map[string]string) error {
			return errAddTagsFailed
		},
	}

	uc := &tagging.UseCase{Tagger: store}

	err := uc.Execute(t.Context(), tagging.Input{
		Name: "/app/config",
		Add:  map[string]string{"env": "prod"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestUseCase_Execute_RemoveTagsError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		UntagFunc: func(_ context.Context, _ string, _ []string) error {
			return errRemoveTagsFailed
		},
	}

	uc := &tagging.UseCase{Tagger: store}

	err := uc.Execute(t.Context(), tagging.Input{
		Name:   "/app/config",
		Remove: []string{"old-tag"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}

var (
	errAddTagsFailed    = errors.New("add tags failed")
	errRemoveTagsFailed = errors.New("remove tags failed")
)
