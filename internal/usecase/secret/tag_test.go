package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestTagUseCase_Execute_AddTags(t *testing.T) {
	t.Parallel()

	var gotAdd map[string]string

	store := &providermock.Store{
		TagFunc: func(_ context.Context, _ string, add map[string]string) error {
			gotAdd = add

			return nil
		},
	}

	uc := &secret.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name: "my-secret",
		Add:  map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod"}, gotAdd)
}

func TestTagUseCase_Execute_RemoveTags(t *testing.T) {
	t.Parallel()

	var gotKeys []string

	store := &providermock.Store{
		UntagFunc: func(_ context.Context, _ string, keys []string) error {
			gotKeys = keys

			return nil
		},
	}

	uc := &secret.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), secret.TagInput{
		Name:   "my-secret",
		Remove: []string{"env", "team"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"env", "team"}, gotKeys)
}

func TestTagUseCase_Execute_AddError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		TagFunc: func(_ context.Context, _ string, _ map[string]string) error {
			return errors.New("tag failed")
		},
	}

	uc := &secret.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), secret.TagInput{Name: "my-secret", Add: map[string]string{"env": "prod"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add tags")
}

func TestTagUseCase_Execute_RemoveError(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		UntagFunc: func(_ context.Context, _ string, _ []string) error {
			return errors.New("untag failed")
		},
	}

	uc := &secret.TagUseCase{Tagger: store}

	err := uc.Execute(t.Context(), secret.TagInput{Name: "my-secret", Remove: []string{"env"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove tags")
}

func TestTagUseCase_Execute_NoOp(t *testing.T) {
	t.Parallel()

	uc := &secret.TagUseCase{Tagger: &providermock.Store{}}

	err := uc.Execute(t.Context(), secret.TagInput{Name: "my-secret"})
	require.NoError(t, err)
}
