package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/cli"
	"github.com/mpyw/suve/internal/staging/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestTagRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("stages tags successfully", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &cli.TagRunner{
			UseCase: &stagingusecase.TagUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "existing"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), cli.TagOptions{
			Name: "/app/config",
			Tags: []string{"env=prod", "team=platform"},
		})
		require.NoError(t, err)

		assert.Contains(t, stdout.String(), "Staged tags for: /app/config")

		tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod", "team": "platform"}, tagEntry.Add)
	})

	t.Run("error on invalid tag format", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &cli.TagRunner{
			UseCase: &stagingusecase.TagUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "existing"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), cli.TagOptions{
			Name: "/app/config",
			Tags: []string{"invalid-tag-without-equals"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tag format")
	})

	t.Run("error on usecase failure", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &cli.TagRunner{
			UseCase: &stagingusecase.TagUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: assert.AnError},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), cli.TagOptions{
			Name: "/app/config",
			Tags: []string{"env=prod"},
		})
		require.Error(t, err)
	})
}

func TestUntagRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("stages tag removal successfully", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &cli.UntagRunner{
			UseCase: &stagingusecase.TagUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "existing"},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), cli.UntagOptions{
			Name: "/app/config",
			Keys: []string{"deprecated", "old-tag"},
		})
		require.NoError(t, err)

		assert.Contains(t, stdout.String(), "Staged tag removal for: /app/config")

		tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, "/app/config")
		require.NoError(t, err)
		assert.True(t, tagEntry.Remove.Contains("deprecated"))
		assert.True(t, tagEntry.Remove.Contains("old-tag"))
	})

	t.Run("error on usecase failure", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer
		r := &cli.UntagRunner{
			UseCase: &stagingusecase.TagUseCase{
				Strategy: &fullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: assert.AnError},
				Store:    store,
			},
			Stdout: &stdout,
			Stderr: &stderr,
		}

		err := r.Run(t.Context(), cli.UntagOptions{
			Name: "/app/config",
			Keys: []string{"deprecated"},
		})
		require.Error(t, err)
	})
}
