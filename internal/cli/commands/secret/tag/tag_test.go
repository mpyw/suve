package tag_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/secret/tag"
	"github.com/mpyw/suve/internal/usecase/secret"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "tag"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing tag argument", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "tag", "my-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("invalid tag format", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "tag", "my-secret", "invalid"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected key=value")
	})

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()

		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "secret", "tag", "my-secret", "=value"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")
	})
}

// mockClient implements provider.SecretTagger for testing.
type mockClient struct {
	addTagsFunc    func(ctx context.Context, name string, tags map[string]string) error
	removeTagsFunc func(ctx context.Context, name string, keys []string) error
}

func (m *mockClient) GetTags(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil //nolint:nilnil // mock implementation
}

func (m *mockClient) AddTags(ctx context.Context, name string, tags map[string]string) error {
	if m.addTagsFunc != nil {
		return m.addTagsFunc(ctx, name, tags)
	}

	return nil
}

func (m *mockClient) RemoveTags(ctx context.Context, name string, keys []string) error {
	if m.removeTagsFunc != nil {
		return m.removeTagsFunc(ctx, name, keys)
	}

	return nil
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    tag.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "add single tag",
			opts: tag.Options{
				Name: "my-secret",
				Tags: map[string]string{"env": "prod"},
			},
			mock: &mockClient{
				addTagsFunc: func(_ context.Context, name string, tags map[string]string) error {
					assert.Equal(t, "my-secret", name)
					assert.Len(t, tags, 1)

					return nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Tagged")
				assert.Contains(t, output, "my-secret")
			},
		},
		{
			name: "add multiple tags",
			opts: tag.Options{
				Name: "my-secret",
				Tags: map[string]string{"env": "prod", "team": "backend"},
			},
			mock: &mockClient{
				addTagsFunc: func(_ context.Context, _ string, tags map[string]string) error {
					assert.Len(t, tags, 2)

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
				Name: "my-secret",
				Tags: map[string]string{"env": "prod"},
			},
			mock: &mockClient{
				addTagsFunc: func(_ context.Context, _ string, _ map[string]string) error {
					return fmt.Errorf("AWS error")
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
				UseCase: &secret.TagUseCase{Client: tt.mock},
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
