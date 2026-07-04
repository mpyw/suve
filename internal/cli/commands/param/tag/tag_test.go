package tag_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/tag"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "tag"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing tag argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "tag", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("invalid tag format", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "tag", "/app/param", "invalid"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value")
	})

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "tag", "/app/param", "=value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    tag.Options
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "add single tag",
			opts: tag.Options{
				Name: "/app/param",
				Tags: map[string]string{"env": "prod"},
			},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, name string, add map[string]string) error {
					assert.Equal(t, "/app/param", name)
					assert.Len(t, add, 1)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Tagged")
				assert.Contains(t, output, "/app/param")
			},
		},
		{
			name: "add multiple tags",
			opts: tag.Options{
				Name: "/app/param",
				Tags: map[string]string{"env": "prod", "team": "backend"},
			},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, _ string, add map[string]string) error {
					assert.Len(t, add, 2)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "2 tag(s)")
			},
		},
		{
			name: "add tags error",
			opts: tag.Options{
				Name: "/app/param",
				Tags: map[string]string{"env": "prod"},
			},
			store: &providermock.Store{
				TagFunc: func(_ context.Context, _ string, _ map[string]string) error {
					return assert.AnError
				},
			},
			wantErr: "failed to add tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			r := &tag.Runner{
				UseCase: &param.TagUseCase{Tagger: tt.store},
				Stdout:  &buf,
			}
			err := r.Run(t.Context(), tt.opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}
