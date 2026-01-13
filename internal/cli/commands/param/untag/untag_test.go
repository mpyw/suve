package untag_test

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
	"github.com/mpyw/suve/internal/cli/commands/param/untag"
	"github.com/mpyw/suve/internal/usecase/param"
)

func TestCommand_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "untag"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
	})

	t.Run("missing key argument", func(t *testing.T) {
		t.Parallel()
		app := appcli.MakeApp()
		err := app.Run(t.Context(), []string{"suve", "param", "untag", "/app/param"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usage:")
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
		opts    untag.Options
		mock    *mockClient
		wantErr string
		check   func(t *testing.T, output string)
	}{
		{
			name: "remove single tag",
			opts: untag.Options{
				Name: "/app/param",
				Keys: []string{"env"},
			},
			mock: &mockClient{
				removeTagsFunc: func(_ context.Context, params *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
					assert.Equal(t, "/app/param", lo.FromPtr(params.ResourceId))
					assert.Equal(t, paramapi.ResourceTypeForTaggingParameter, params.ResourceType)
					assert.Equal(t, []string{"env"}, params.TagKeys)
					return &paramapi.RemoveTagsFromResourceOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "Untagged")
				assert.Contains(t, output, "/app/param")
			},
		},
		{
			name: "remove multiple tags",
			opts: untag.Options{
				Name: "/app/param",
				Keys: []string{"env", "team"},
			},
			mock: &mockClient{
				removeTagsFunc: func(_ context.Context, params *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
					assert.Len(t, params.TagKeys, 2)
					return &paramapi.RemoveTagsFromResourceOutput{}, nil
				},
			},
			check: func(t *testing.T, output string) {
				t.Helper()
				assert.Contains(t, output, "2 key(s)")
			},
		},
		{
			name: "remove tags error",
			opts: untag.Options{
				Name: "/app/param",
				Keys: []string{"env"},
			},
			mock: &mockClient{
				removeTagsFunc: func(_ context.Context, _ *paramapi.RemoveTagsFromResourceInput, _ ...func(*paramapi.Options)) (*paramapi.RemoveTagsFromResourceOutput, error) {
					return nil, fmt.Errorf("AWS error")
				},
			},
			wantErr: "failed to remove tags",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			r := &untag.Runner{
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
