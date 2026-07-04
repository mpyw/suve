package untag_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/untag"
	"github.com/mpyw/suve/internal/provider/providermock"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "untag"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing key argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "untag", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    untag.Options
		store   *providermock.Store
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "remove single tag",
			opts: untag.Options{Name: "my-secret", Keys: []string{"env"}},
			store: &providermock.Store{
				UntagFunc: func(_ context.Context, name string, keys []string) error {
					assert.Equal(t, "my-secret", name)
					assert.Equal(t, []string{"env"}, keys)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Untagged")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "remove multiple tags",
			opts: untag.Options{Name: "my-secret", Keys: []string{"env", "team"}},
			store: &providermock.Store{
				UntagFunc: func(_ context.Context, _ string, keys []string) error {
					assert.Len(t, keys, 2)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "2 key(s)")
			},
		},
		{
			name:    "untag resource error",
			opts:    untag.Options{Name: "my-secret", Keys: []string{"env"}},
			wantErr: "failed to remove tags",
			store: &providermock.Store{
				UntagFunc: func(_ context.Context, _ string, _ []string) error {
					return errors.New("AWS error")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			r := &untag.Runner{
				UseCase: &secret.TagUseCase{Tagger: tt.store},
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
