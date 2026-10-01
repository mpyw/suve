// White-box tests of tag.go.
//declscope:namespace tag

package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/staging"
	"github.com/mpyw/suve/internal/staging/store/testutil"
	stagingusecase "github.com/mpyw/suve/internal/usecase/staging"
)

func TestTagRunner_Run(t *testing.T) {
	t.Parallel()

	t.Run("stages tags successfully", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &tagRunner{
			useCase: &stagingusecase.TagUseCase{
				Strategy: &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "existing"},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), tagOptions{
			name: "/app/config",
			tags: []string{"env=prod", "team=platform"},
		})
		require.NoError(t, err)

		assert.Contains(t, stdout.String(), "Staged tags for: /app/config")

		tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod", "team": "platform"}, tagEntry.Add)
	})

	t.Run("error on invalid tag format", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &tagRunner{
			useCase: &stagingusecase.TagUseCase{
				Strategy: &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "existing"},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), tagOptions{
			name: "/app/config",
			tags: []string{"invalid-tag-without-equals"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tag format")
	})

	t.Run("error on usecase failure", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &tagRunner{
			useCase: &stagingusecase.TagUseCase{
				Strategy: &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: assert.AnError},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), tagOptions{
			name: "/app/config",
			tags: []string{"env=prod"},
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

		r := &untagRunner{
			useCase: &stagingusecase.TagUseCase{
				Strategy: &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "existing"},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), untagOptions{
			name: "/app/config",
			keys: []string{"deprecated", "old-tag"},
		})
		require.NoError(t, err)

		assert.Contains(t, stdout.String(), "Staged tag removal for: /app/config")

		tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		assert.True(t, tagEntry.Remove.Contains("deprecated"))
		assert.True(t, tagEntry.Remove.Contains("old-tag"))
	})

	t.Run("error on usecase failure", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &untagRunner{
			useCase: &stagingusecase.TagUseCase{
				Strategy: &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentErr: assert.AnError},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), untagOptions{
			name: "/app/config",
			keys: []string{"deprecated"},
		})
		require.Error(t, err)
	})
}

func TestTagRunner_EmptyTags(t *testing.T) {
	t.Parallel()

	t.Run("empty tags slice returns error", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &tagRunner{
			useCase: &stagingusecase.TagUseCase{
				Strategy: &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "existing"},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), tagOptions{
			name: "/app/config",
			tags: []string{}, // Empty tags
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no tags specified")
	})

	t.Run("tag with equals in value", func(t *testing.T) {
		t.Parallel()

		store := testutil.NewMockStore()

		var stdout, stderr bytes.Buffer

		r := &tagRunner{
			useCase: &stagingusecase.TagUseCase{
				Strategy: &cliFullMockStrategy{service: staging.ServiceParam, fetchCurrentVal: "existing"},
				Store:    store,
			},
			stdout: &stdout,
			stderr: &stderr,
		}

		err := r.run(t.Context(), tagOptions{
			name: "/app/config",
			tags: []string{"url=https://example.com?foo=bar"}, // Value contains equals
		})
		require.NoError(t, err)

		tagEntry, err := store.GetTag(t.Context(), staging.ServiceParam, staging.EntryKey{Name: "/app/config"})
		require.NoError(t, err)
		assert.Equal(t, "https://example.com?foo=bar", tagEntry.Add["url"])
	})
}
