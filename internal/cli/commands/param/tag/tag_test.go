package tag_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/api/paramapi"
	appcli "github.com/mpyw/suve/internal/cli/commands"
	"github.com/mpyw/suve/internal/cli/commands/param/tag"
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

type mockClient struct {
	addTagsFunc    func(ctx context.Context, params *paramapi.AddTagsToResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error)
	removeTagsFunc func(ctx context.Context, params *paramapi.RemoveTagsFromResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error)
}

func (m *mockClient) AddTagsToResource(ctx context.Context, params *paramapi.AddTagsToResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
	if m.addTagsFunc != nil {
		return m.addTagsFunc(ctx, params, optFns...)
	}

	return &paramapi.AddTagsToResourceOutput{}, nil
}

func (m *mockClient) RemoveTagsFromResource(ctx context.Context, params *paramapi.RemoveTagsFromResourceInput, optFns ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
	if m.removeTagsFunc != nil {
		return m.removeTagsFunc(ctx, params, optFns...)
	}

	return &paramapi.RemoveTagsFromResourceOutput{}, nil
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
				Name: "/app/param",
				Tags: map[string]string{"env": "prod"},
			},
			mock: &mockClient{
				addTagsFunc: func(_ context.Context, params *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
					assert.Equal(t, "/app/param", lo.FromPtr(params.ResourceId))
					assert.Equal(t, paramapi.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Len(t, params.Tags, 1)

					return &paramapi.AddTagsToResourceOutput{}, nil
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
			mock: &mockClient{
				addTagsFunc: func(_ context.Context, params *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
					assert.Len(t, params.Tags, 2)

					return &paramapi.AddTagsToResourceOutput{}, nil
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
			mock: &mockClient{
				addTagsFunc: func(_ context.Context, _ *paramapi.AddTagsToResourceInput, _ ...func(*paramapi.Options)) (*paramapi.AddTagsToResourceOutput, error) {
					return nil, fmt.Errorf("AWS error")
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
				UseCase: &param.TagUseCase{Client: tt.mock},
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
